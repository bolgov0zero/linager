-- +goose Up
-- Devices: every installation of an application (the desktop application, the iPhone application, later Android) that signs in
-- has a device with a secret token of its own (only its hash is kept). When the administrator turns on the confirmation of devices
-- (setting `device_approval`, the time it was turned on), a new device waits as `pending` until an administrator approves it in the
-- application; a `denied` one is refused. A device belongs to one user: the same phone used by two people is two devices.
CREATE TABLE devices (
    id          bigserial   PRIMARY KEY,
    user_id     bigint      NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  bytea       NOT NULL,
    platform    text        NOT NULL DEFAULT '',
    app_version text        NOT NULL DEFAULT '',
    os_version  text        NOT NULL DEFAULT '',
    model       text        NOT NULL DEFAULT '',
    label       text        NOT NULL DEFAULT '',
    status      text        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','denied')),
    last_ip     text        NOT NULL DEFAULT '',
    last_seen   timestamptz NOT NULL DEFAULT now(),
    created_at  timestamptz NOT NULL DEFAULT now(),
    decided_at  timestamptz,
    decided_by  bigint      REFERENCES users(id) ON DELETE SET NULL
);
CREATE UNIQUE INDEX devices_token_user_key ON devices (token_hash, user_id);

-- The session of a device ends with it: removing a device signs it out.
ALTER TABLE sessions ADD COLUMN device_id bigint REFERENCES devices(id) ON DELETE CASCADE;
CREATE INDEX sessions_device_idx ON sessions (device_id);

