-- +goose Up
-- The port of the agent listener an agent connected to last time: it shows, while the agent port is being changed,
-- which agents already moved to the new port. The ports themselves live in the settings table (key "connection").
ALTER TABLE servers ADD COLUMN last_port integer NOT NULL DEFAULT 0;
