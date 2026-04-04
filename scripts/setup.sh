#!/bin/sh
# Setup script for torque-iot local development environment.
# Run once after `docker compose up -d` to configure Vault PKI roles and seed a test device.
# Idempotent — safe to run multiple times.
set -e

VAULT_ADDR="http://localhost:8201"
VAULT_TOKEN="torque"
COMPOSE_DIR="$(cd "$(dirname "$0")/../infra" && pwd)"

vault_post() {
  curl -sf -X POST \
    -H "X-Vault-Token: $VAULT_TOKEN" \
    -H "Content-Type: application/json" \
    -d "$2" \
    "$VAULT_ADDR/v1/$1"
}

psql_exec() {
  docker compose -f "$COMPOSE_DIR/docker-compose.yml" exec -T postgres \
    psql -U torque -d torque -c "$1"
}

# --- PKI roles ---
echo "==> Creating Vault 'device' role..."
vault_post "pki/roles/device" '{
  "allow_any_name": true,
  "enforce_hostnames": false,
  "key_type": "ec",
  "key_bits": 256,
  "ttl": "8760h",
  "max_ttl": "87600h",
  "client_flag": true,
  "server_flag": false,
  "no_store": false
}' > /dev/null
echo "    done"

echo "==> Creating Vault 'service' role..."
vault_post "pki/roles/service" '{
  "allow_any_name": false,
  "allowed_domains": ["torque-listener"],
  "allow_bare_domains": true,
  "allow_subdomains": false,
  "key_type": "ec",
  "key_bits": 256,
  "ttl": "8760h",
  "max_ttl": "87600h",
  "client_flag": true,
  "server_flag": false,
  "enforce_hostnames": false,
  "cn_validations": []
}' > /dev/null
echo "    done"

# --- Test device ---
DEVICE_ID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen | tr '[:upper:]' '[:lower:]')
VEHICLE_ID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || uuidgen | tr '[:upper:]' '[:lower:]')
VEHICLE_VIN="TEST00000000000001"

echo "==> Issuing certificate for test device (CN: $DEVICE_ID)..."
CERT_JSON=$(vault_post "pki/issue/device" "{\"common_name\": \"$DEVICE_ID\", \"ttl\": \"8760h\"}")
echo "    done"

echo "==> Seeding test device in Postgres..."
psql_exec "
  INSERT INTO device.devices (id, name, certificate_cn, vehicle_id, vehicle_vin)
  VALUES ('$DEVICE_ID', 'test-device', '$DEVICE_ID', '$VEHICLE_ID', '$VEHICLE_VIN')
  ON CONFLICT (certificate_cn) DO NOTHING;
"
echo "    done"

# --- Save cert ---
CERT_FILE="$(cd "$(dirname "$0")/.." && pwd)/.test-device.json"
printf '%s' "$CERT_JSON" > "$CERT_FILE"

echo ""
echo "Setup complete!"
echo ""
echo "  Device ID  : $DEVICE_ID"
echo "  Vehicle VIN: $VEHICLE_VIN"
echo "  Cert saved : $CERT_FILE  (add to .gitignore if needed)"
