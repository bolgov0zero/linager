-- +goose Up
-- Groups become folders: a folder can sit in another folder, to any depth (the application limits it to 8 levels). The server
-- still lies in exactly one folder (servers.group_id). The name is unique among the folders of the same parent, not in the whole
-- system, so "Web" can exist under "Prod" and under "Test". An access given on a folder covers its whole subtree.
-- The existing groups become folders of the top level. A folder is never deleted with children under it: the application moves
-- the children up first, so the key has no ON DELETE action on purpose.
ALTER TABLE server_groups ADD COLUMN parent_id bigint REFERENCES server_groups(id);
DROP INDEX server_groups_name_idx;
CREATE UNIQUE INDEX server_groups_sibling_name_idx ON server_groups (COALESCE(parent_id, 0), lower(name));
CREATE INDEX server_groups_parent_idx ON server_groups (parent_id);
