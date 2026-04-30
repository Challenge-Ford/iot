#!/bin/sh
# torque-iot local development setup
#
# Usage:
#   ./scripts/setup.sh
#
# Bootstraps the full local environment in order:
#   1. Start step-ca, postgres and rabbitmq
#   2. Wait for step-ca to become healthy
#   3. Configure step-ca certificate duration limits
#   4. Generate EMQX TLS certificates
#   5. Issue mqtt-listener service certificate
#   6. Start EMQX
#   7. Issue a test device certificate
#   8. Seed the test device in Postgres
#
# Idempotent — safe to run multiple times.
# Requires: docker, docker compose
set -e

COMPOSE="docker compose -f $(cd "$(dirname "$0")/../infra" && pwd)/docker-compose.yml"
REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
EMQX_CERTS_DIR="$REPO_DIR/certs/emqx"
LISTENER_CERTS_DIR="$REPO_DIR/certs/listener"
DEVICE_CERTS_DIR="$REPO_DIR/certs/device"
EMQX_SERVER_SANS="${EMQX_SERVER_SANS:-}"

mkdir -p "$EMQX_CERTS_DIR" "$LISTENER_CERTS_DIR" "$DEVICE_CERTS_DIR"

step_exec() { $COMPOSE exec -T step-ca "$@"; }
psql_exec() { $COMPOSE exec -T postgres psql -U torque -d torque -c "$1"; }

# Fetch a CA bundle from step-ca: prefer intermediate + root if intermediate exists
fetch_ca_bundle() {
  # args: $1 -> dest bundle path, $2 -> dest intermediate path (optional)
  TMP_BUNDLE_REMOTE="/tmp/ca_bundle.crt"
  TMP_INTER_REMOTE="/tmp/intermediate_ca.crt"
  step_exec sh -c "if [ -f /home/step/certs/intermediate_ca.crt ]; then cat /home/step/certs/intermediate_ca.crt /home/step/certs/root_ca.crt > $TMP_BUNDLE_REMOTE; cp /home/step/certs/intermediate_ca.crt $TMP_INTER_REMOTE; else cp /home/step/certs/root_ca.crt $TMP_BUNDLE_REMOTE; fi"
  $COMPOSE cp step-ca:$TMP_BUNDLE_REMOTE "$1"
  if [ -n "$2" ]; then
    # try to copy the intermediate; ignore failure if it doesn't exist
    set +e
    $COMPOSE cp step-ca:$TMP_INTER_REMOTE "$2" 2>/dev/null || true
    set -e
  fi
  step_exec sh -c "rm -f $TMP_BUNDLE_REMOTE $TMP_INTER_REMOTE"
}

issue_cert() {
  CN="$1"
  OUT_CRT="$2"
  OUT_KEY="$3"
  SANS_CSV="$4"

  SAN_FLAGS=""
  if [ -n "$SANS_CSV" ]; then
    OLD_IFS="$IFS"
    IFS=','
    for SAN in $SANS_CSV; do
      SAN_TRIMMED=$(printf "%s" "$SAN" | sed 's/^ *//;s/ *$//')
      if [ -n "$SAN_TRIMMED" ]; then
        SAN_FLAGS="$SAN_FLAGS --san '$SAN_TRIMMED'"
      fi
    done
    IFS="$OLD_IFS"
  fi

  step_exec sh -c "
    step ca certificate '$CN' /tmp/out.crt /tmp/out.key \
      --provisioner=torque \
      --provisioner-password-file=/run/secrets/ca-password \
      --not-after=8760h $SAN_FLAGS -f > /dev/null 2>&1
  "
  $COMPOSE cp step-ca:/tmp/out.crt "$OUT_CRT"
  $COMPOSE cp step-ca:/tmp/out.key "$OUT_KEY"
}

print_step() {
  echo ""
  echo "──────────────────────────────────────────────────"
  printf " %s\n" "$1"
  echo "──────────────────────────────────────────────────"
}

# ──────────────────────────────────────────────────────────────
print_step "1/8  Starting step-ca, postgres and rabbitmq"
# ──────────────────────────────────────────────────────────────
$COMPOSE up -d step-ca postgres rabbitmq
echo "  ✓ services started"

# ──────────────────────────────────────────────────────────────
print_step "2/8  Waiting for step-ca to become healthy"
# ──────────────────────────────────────────────────────────────
until step_exec step ca health \
  --ca-url=https://localhost:9000 \
  --root=/home/step/certs/root_ca.crt > /dev/null 2>&1; do
  printf "  waiting...\r"
  sleep 2
done
echo "  ✓ step-ca is healthy"

# ──────────────────────────────────────────────────────────────
print_step "3/8  Configuring step-ca certificate duration limits"
# ──────────────────────────────────────────────────────────────
step_exec sh -c "
  jq '.authority.claims = {
    \"minTLSCertDuration\": \"5m\",
    \"maxTLSCertDuration\": \"8760h\",
    \"defaultTLSCertDuration\": \"8760h\"
  }' /home/step/config/ca.json > /tmp/ca.json.tmp &&
  mv /tmp/ca.json.tmp /home/step/config/ca.json
"
$COMPOSE restart step-ca
until step_exec step ca health \
  --ca-url=https://localhost:9000 \
  --root=/home/step/certs/root_ca.crt > /dev/null 2>&1; do
  printf "  waiting...\r"
  sleep 2
done
echo "  ✓ certificate duration configured (max: 8760h)"

# ──────────────────────────────────────────────────────────────
print_step "4/8  Generating EMQX TLS certificates"
# ──────────────────────────────────────────────────────────────
fetch_ca_bundle "$EMQX_CERTS_DIR/ca.crt" "$EMQX_CERTS_DIR/intermediate.crt"
issue_cert "emqx" "$EMQX_CERTS_DIR/server.crt" "$EMQX_CERTS_DIR/server.key" "$EMQX_SERVER_SANS"
echo "  ✓ certificates written to $EMQX_CERTS_DIR"

# ──────────────────────────────────────────────────────────────
print_step "5/8  Issuing mqtt-listener service certificate"
# ──────────────────────────────────────────────────────────────
fetch_ca_bundle "$LISTENER_CERTS_DIR/ca.crt" "$LISTENER_CERTS_DIR/intermediate.crt"
issue_cert "torque-listener" "$LISTENER_CERTS_DIR/listener.crt" "$LISTENER_CERTS_DIR/listener.key"
echo "  ✓ certificates written to $LISTENER_CERTS_DIR"

# ──────────────────────────────────────────────────────────────
print_step "6/8  Starting EMQX"
# ──────────────────────────────────────────────────────────────
$COMPOSE up -d emqx
echo "  ✓ EMQX started"

# ──────────────────────────────────────────────────────────────
print_step "7/8  Issuing test device certificate"
# ──────────────────────────────────────────────────────────────
META="$DEVICE_CERTS_DIR/meta.json"
TMP_DEVICE_CA="$(mktemp -t step_device_ca.crt.XXXX)"
fetch_ca_bundle "$TMP_DEVICE_CA"

if [ -f "$META" ]; then
  DEVICE_ID=$(grep -o '"device_id": *"[^"]*"' "$META" | cut -d'"' -f4)
  VEHICLE_VIN=$(grep -o '"vin": *"[^"]*"' "$META" | cut -d'"' -f4)

  if [ -f "$DEVICE_CERTS_DIR/ca.crt" ] && cmp -s "$TMP_DEVICE_CA" "$DEVICE_CERTS_DIR/ca.crt"; then
    echo "  ✓ device already exists (CN: $DEVICE_ID), CA up-to-date; skipping cert issuance"
  else
    echo "  ✓ device exists (CN: $DEVICE_ID), CA changed — updating CA and reissuing device cert"
    fetch_ca_bundle "$DEVICE_CERTS_DIR/ca.crt" "$DEVICE_CERTS_DIR/intermediate.crt"
    issue_cert "$DEVICE_ID" "$DEVICE_CERTS_DIR/device.crt" "$DEVICE_CERTS_DIR/device.key"
    echo "  ✓ device certificate reissued for CN: $DEVICE_ID"
  fi
else
  DEVICE_ID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen | tr '[:upper:]' '[:lower:]')
  fetch_ca_bundle "$DEVICE_CERTS_DIR/ca.crt" "$DEVICE_CERTS_DIR/intermediate.crt"
  issue_cert "$DEVICE_ID" "$DEVICE_CERTS_DIR/device.crt" "$DEVICE_CERTS_DIR/device.key"
  echo "  ✓ certificate issued for CN: $DEVICE_ID"
fi

rm -f "$TMP_DEVICE_CA"

# ──────────────────────────────────────────────────────────────
print_step "8/8  Seeding test device in Postgres"
# ──────────────────────────────────────────────────────────────
if [ -f "$META" ]; then
  DEVICE_ID=$(grep -o '"device_id": *"[^\"]*"' "$META" | cut -d'"' -f4)
  VEHICLE_ID=$(grep -o '"vehicle_id": *"[^\"]*"' "$META" | cut -d'"' -f4)
  VEHICLE_VIN=$(grep -o '"vin": *"[^\"]*"' "$META" | cut -d'"' -f4)

  echo "  ✓ meta.json present — ensuring DB matches meta.json"
  # if a vehicle with the same VIN already exists but with a different id,
  # migrate any device rows to the meta vehicle_id and remove the old vehicle.
  EXISTING_VID=$($COMPOSE exec -T postgres psql -U torque -d torque -t -A -c "SELECT id FROM vehicle.vehicles WHERE vin = '$VEHICLE_VIN' LIMIT 1;" 2>/dev/null | tr -d '[:space:]' || true)
  if [ -n "$EXISTING_VID" ] && [ "$EXISTING_VID" != "$VEHICLE_ID" ]; then
    echo "  › found existing vehicle id $EXISTING_VID for VIN $VEHICLE_VIN — migrating to meta vehicle_id $VEHICLE_ID"
    # check if meta vehicle row already exists
    META_EXISTS=$($COMPOSE exec -T postgres psql -U torque -d torque -t -A -c "SELECT 1 FROM vehicle.vehicles WHERE id = '$VEHICLE_ID' LIMIT 1;" 2>/dev/null | tr -d '[:space:]' || true)
    if [ -n "$META_EXISTS" ]; then
      psql_exec "BEGIN;
        UPDATE device.devices SET vehicle_id = '$VEHICLE_ID' WHERE vehicle_id = '$EXISTING_VID' OR name = '$VEHICLE_VIN';
        DELETE FROM vehicle.vehicles WHERE id = '$EXISTING_VID';
      COMMIT;"
    else
      TEMP_VIN="${VEHICLE_VIN}-migrating-$(date +%s)"
      psql_exec "BEGIN;
        -- create meta vehicle with a temp VIN to avoid unique vin conflict
        INSERT INTO vehicle.vehicles (id, vin) VALUES ('$VEHICLE_ID', '$TEMP_VIN');
        -- move devices to the new meta vehicle id
        UPDATE device.devices SET vehicle_id = '$VEHICLE_ID' WHERE vehicle_id = '$EXISTING_VID' OR name = '$VEHICLE_VIN';
        -- remove old vehicle
        DELETE FROM vehicle.vehicles WHERE id = '$EXISTING_VID';
        -- set the correct VIN on the meta vehicle now that old row is removed
        UPDATE vehicle.vehicles SET vin = '$VEHICLE_VIN' WHERE id = '$VEHICLE_ID';
      COMMIT;"
    fi
    echo "  › migration complete"
  fi

  psql_exec "BEGIN;
    -- ensure vehicle exists (insert by id or by vin)
    INSERT INTO vehicle.vehicles (id, vin)
    VALUES ('$VEHICLE_ID', '$VEHICLE_VIN')
    ON CONFLICT (id) DO UPDATE SET vin = EXCLUDED.vin, deleted_at = NULL;

    -- upsert device by certificate_cn
    INSERT INTO device.devices (id, name, certificate_cn, vehicle_id)
    VALUES ('$DEVICE_ID', '$VEHICLE_VIN', '$DEVICE_ID', '$VEHICLE_ID')
    ON CONFLICT (certificate_cn) DO UPDATE SET name = EXCLUDED.name, vehicle_id = EXCLUDED.vehicle_id, deleted_at = NULL;

    -- remove any OTHER devices that reference the same VIN or vehicle but have a different certificate_cn
    DELETE FROM device.devices
    WHERE certificate_cn != '$DEVICE_ID'
      AND (name = '$VEHICLE_VIN' OR vehicle_id = '$VEHICLE_ID');
  COMMIT;"

  echo "  ✓ database normalized to single device for VIN: $VEHICLE_VIN"
else
  VEHICLE_ID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen | tr '[:upper:]' '[:lower:]')
  VEHICLE_VIN="TEST00000000000001"

  psql_exec "
    INSERT INTO vehicle.vehicles (id, vin)
    VALUES ('$VEHICLE_ID', '$VEHICLE_VIN')
    ON CONFLICT (vin) DO NOTHING;

    INSERT INTO device.devices (id, name, certificate_cn, vehicle_id)
    VALUES ('$DEVICE_ID', 'test-device', '$DEVICE_ID', '$VEHICLE_ID')
    ON CONFLICT DO NOTHING;
  "

  printf '{\n  "device_id": "%s",\n  "vehicle_id": "%s",\n  "vin": "%s"\n}\n' \
    "$DEVICE_ID" "$VEHICLE_ID" "$VEHICLE_VIN" > "$META"

  echo "  ✓ test device seeded"
fi

echo ""
echo "══════════════════════════════════════════════════"
echo " Setup complete!"
echo "══════════════════════════════════════════════════"
echo ""
echo "  Device ID  : $DEVICE_ID"
echo "  Vehicle VIN: $VEHICLE_VIN"
echo ""
echo "  Certs:"
echo "    EMQX     → $EMQX_CERTS_DIR"
echo "    Listener → $LISTENER_CERTS_DIR"
echo "    Device   → $DEVICE_CERTS_DIR"
echo ""
echo "  Services:"
echo "    step-ca  → https://localhost:9001"
echo "    emqx     → ssl://localhost:8884"
echo "    rabbitmq → amqp://localhost:5673  (UI: :15673)"
echo "    postgres → localhost:5434"
echo ""
