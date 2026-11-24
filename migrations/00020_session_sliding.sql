-- +goose Up
-- A session of the desktop application lives longer than a browser one and is renewed by every request: while the person
-- keeps using the application it stays signed in, after that long without a request the password and 2FA are asked again.
-- 0 = a fixed lifetime (browser sessions, as before).
ALTER TABLE sessions ADD COLUMN sliding_secs integer NOT NULL DEFAULT 0;
