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

mkdir -p "$EMQX_CERTS_DIR" "$LISTENER_CERTS_DIR" "$DEVICE_CERTS_DIR"

step_exec() { $COMPOSE exec -T step-ca "$@"; }
psql_exec() { $COMPOSE exec -T postgres psql -U torque -d torque -c "$1"; }

issue_cert() {
  CN="$1"
  OUT_CRT="$2"
  OUT_KEY="$3"
  step_exec sh -c "
    step ca certificate '$CN' /tmp/out.crt /tmp/out.key \
      --provisioner=torque \
      --provisioner-password-file=/run/secrets/ca-password \
      --not-after=8760h -f > /dev/null 2>&1
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
print_step "1/7  Starting step-ca, postgres and rabbitmq"
# ──────────────────────────────────────────────────────────────
$COMPOSE up -d step-ca postgres rabbitmq
echo "  ✓ services started"

# ──────────────────────────────────────────────────────────────
print_step "2/7  Waiting for step-ca to become healthy"
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
$COMPOSE cp step-ca:/home/step/certs/root_ca.crt "$EMQX_CERTS_DIR/ca.crt"
issue_cert "emqx" "$EMQX_CERTS_DIR/server.crt" "$EMQX_CERTS_DIR/server.key"
echo "  ✓ certificates written to $EMQX_CERTS_DIR"

# ──────────────────────────────────────────────────────────────
print_step "5/8  Issuing mqtt-listener service certificate"
# ──────────────────────────────────────────────────────────────
$COMPOSE cp step-ca:/home/step/certs/root_ca.crt "$LISTENER_CERTS_DIR/ca.crt"
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
DEVICE_ID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen | tr '[:upper:]' '[:lower:]')
$COMPOSE cp step-ca:/home/step/certs/root_ca.crt "$DEVICE_CERTS_DIR/ca.crt"
issue_cert "$DEVICE_ID" "$DEVICE_CERTS_DIR/device.crt" "$DEVICE_CERTS_DIR/device.key"
echo "  ✓ certificate issued for CN: $DEVICE_ID"

# ──────────────────────────────────────────────────────────────
print_step "8/8  Seeding test device in Postgres"
# ──────────────────────────────────────────────────────────────
VEHICLE_ID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen | tr '[:upper:]' '[:lower:]')
VEHICLE_VIN="TEST00000000000001"

psql_exec "
  INSERT INTO device.devices (id, name, certificate_cn, vehicle_id, vehicle_vin)
  VALUES ('$DEVICE_ID', 'test-device', '$DEVICE_ID', '$VEHICLE_ID', '$VEHICLE_VIN')
  ON CONFLICT (certificate_cn) DO NOTHING;
"

printf '{\n  "device_id": "%s",\n  "vehicle_id": "%s",\n  "vin": "%s"\n}\n' \
  "$DEVICE_ID" "$VEHICLE_ID" "$VEHICLE_VIN" > "$DEVICE_CERTS_DIR/meta.json"

echo "  ✓ test device seeded"

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
echo "    step-ca  → https://localhost:9000"
echo "    emqx     → ssl://localhost:8884"
echo "    rabbitmq → amqp://localhost:5673  (UI: :15673)"
echo "    postgres → localhost:5434"
echo ""
