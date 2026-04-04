CREATE SCHEMA IF NOT EXISTS vehicle;

CREATE TABLE IF NOT EXISTS vehicle.vehicles (
    id         UUID        PRIMARY KEY,
    vin        TEXT        NOT NULL UNIQUE,
    deleted_at TIMESTAMPTZ
);

CREATE SCHEMA IF NOT EXISTS device;

CREATE TABLE IF NOT EXISTS device.devices (
    id             UUID        PRIMARY KEY,
    name           TEXT        NOT NULL UNIQUE,
    certificate_cn TEXT        NOT NULL UNIQUE,
    vehicle_id     UUID        REFERENCES vehicle.vehicles(id),
    deleted_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_devices_cn ON device.devices (certificate_cn) WHERE deleted_at IS NULL;
