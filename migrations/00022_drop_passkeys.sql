-- +goose Up
-- Sign-in with a passkey (WebAuthn) is gone together with the web panel: the application has no way to use it, and the
-- credentials were only ever read by that code. The password (and TOTP when it is on) stays the way in.
DROP TABLE IF EXISTS webauthn_credentials;
ALTER TABLE users DROP COLUMN IF EXISTS webauthn_handle;

