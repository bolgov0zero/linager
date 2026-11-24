-- +goose Up
-- Nothing stands in front of the application port any more, so the journal no longer records whether a request came through
-- a proxy and whether that proxy was trusted (both were always false after the web panel was removed).
ALTER TABLE access_log DROP COLUMN IF EXISTS via_proxy;
ALTER TABLE access_log DROP COLUMN IF EXISTS proxy_trusted;
