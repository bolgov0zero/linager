-- +goose Up
-- Tags become a registry: a tag has an id, a name and a colour, and the servers and the users refer to the id. A tag can then be
-- renamed or recoloured in one place, and a tag given to a user (a way to give access next to a single server and a group)
-- keeps working after the rename. Deleting a tag takes it off every server and out of every grant (the foreign keys cascade).
CREATE TABLE tags (
    id         bigserial   PRIMARY KEY,
    name       text        NOT NULL,
    color      text        NOT NULL DEFAULT 'gray',
    created_at timestamptz NOT NULL DEFAULT now()
);
-- a name is unique without regard to case: "Prod" and "prod" are one tag
CREATE UNIQUE INDEX tags_name_lower_key ON tags (lower(name));

-- the tags that servers have now go into the registry (the first spelling in alphabetical order stays)
INSERT INTO tags (name)
SELECT DISTINCT ON (lower(tag)) tag FROM server_tags ORDER BY lower(tag), tag;

ALTER TABLE server_tags ADD COLUMN tag_id bigint REFERENCES tags(id) ON DELETE CASCADE;
UPDATE server_tags st SET tag_id = t.id FROM tags t WHERE lower(t.name) = lower(st.tag);
ALTER TABLE server_tags ALTER COLUMN tag_id SET NOT NULL;
ALTER TABLE server_tags DROP CONSTRAINT server_tags_pkey;
ALTER TABLE server_tags DROP COLUMN tag;
ALTER TABLE server_tags ADD PRIMARY KEY (server_id, tag_id);
CREATE INDEX server_tags_tag_idx ON server_tags (tag_id);

-- Tags a user may reach: every server that has the tag (a server tagged later is included).
CREATE TABLE user_tags (
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tag_id  bigint NOT NULL REFERENCES tags(id)  ON DELETE CASCADE,
    PRIMARY KEY (user_id, tag_id)
);
CREATE INDEX user_tags_tag_idx ON user_tags (tag_id);
