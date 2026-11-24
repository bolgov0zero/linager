-- +goose Up
-- Tags of a server: short labels the administrator puts on it. What they are used for comes later; for now they are only
-- kept and shown. The order is the order the administrator gave.
CREATE TABLE server_tags (
    server_id uuid    NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    tag       text    NOT NULL,
    position  integer NOT NULL DEFAULT 0,
    PRIMARY KEY (server_id, tag)
);
