-- +goose Up

-- Command groups are the dashboard's organizational folders: a command belongs
-- to at most one group (group_id NULL = ungrouped). Deleting a group ungroups
-- its commands rather than removing them.
CREATE TABLE command_groups (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    position    INTEGER     NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_command_groups_guild ON command_groups (guild_id, position, id);

ALTER TABLE custom_commands
    ADD COLUMN group_id BIGINT REFERENCES command_groups(id) ON DELETE SET NULL;
CREATE INDEX idx_custom_commands_group ON custom_commands (group_id);

-- +goose Down
ALTER TABLE custom_commands DROP COLUMN IF EXISTS group_id;
DROP TABLE IF EXISTS command_groups;
