-- +goose Up

-- UUID primary keys for the custom-commands domain. Pre-prod, so a clean
-- rebuild of the command tables is the right call (matching 00004's approach)
-- rather than an in-place type swap across live FKs. Discord snowflakes
-- (guild_id, invoker_id, owner_id, channel_id) stay BIGINT; run ids stay ULID.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

DROP TABLE IF EXISTS command_run_logs;
DROP TABLE IF EXISTS command_runs;
DROP TABLE IF EXISTS custom_command_versions;
DROP TABLE IF EXISTS feature_kv;
DROP TABLE IF EXISTS custom_commands;
DROP TABLE IF EXISTS command_groups;

CREATE TABLE command_groups (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    guild_id   BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name       TEXT        NOT NULL,
    position   INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_command_groups_guild ON command_groups (guild_id, position, id);

CREATE TABLE custom_commands (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    guild_id           BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name               TEXT        NOT NULL,
    description        TEXT        NOT NULL DEFAULT '',
    enabled            BOOLEAN     NOT NULL DEFAULT true,
    status             TEXT        NOT NULL DEFAULT 'draft',
    version            INTEGER     NOT NULL DEFAULT 1,
    requires_defer     BOOLEAN     NOT NULL DEFAULT false,
    definition         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    group_id           UUID        REFERENCES command_groups(id) ON DELETE SET NULL,
    created_by         BIGINT      NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (guild_id, name)
);
CREATE INDEX idx_custom_commands_guild ON custom_commands (guild_id);
CREATE INDEX idx_custom_commands_group ON custom_commands (group_id);

CREATE TABLE custom_command_versions (
    command_id   UUID        NOT NULL REFERENCES custom_commands(id) ON DELETE CASCADE,
    version      INTEGER     NOT NULL,
    definition   JSONB       NOT NULL,
    published_by BIGINT      NOT NULL DEFAULT 0,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (command_id, version)
);

CREATE TABLE command_runs (
    id                   TEXT        PRIMARY KEY,           -- ULID
    command_id           UUID        NOT NULL REFERENCES custom_commands(id) ON DELETE CASCADE,
    command_version      INTEGER     NOT NULL,
    guild_id             BIGINT      NOT NULL,
    invoker_id           BIGINT      NOT NULL DEFAULT 0,
    channel_id           BIGINT      NOT NULL DEFAULT 0,
    trigger_kind         TEXT        NOT NULL,
    interaction_id       TEXT        NOT NULL DEFAULT '',
    interaction_token    TEXT        NOT NULL DEFAULT '',
    interaction_expires  TIMESTAMPTZ,
    scope                JSONB       NOT NULL DEFAULT '{}'::jsonb,
    cursor               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    status               TEXT        NOT NULL DEFAULT 'running',
    resume_at            TIMESTAMPTZ,
    awaiting_custom_id   TEXT        NOT NULL DEFAULT '',
    awaiting_user_id     BIGINT      NOT NULL DEFAULT 0,
    awaiting_kind        TEXT        NOT NULL DEFAULT '',
    definition_snapshot  JSONB       NOT NULL,
    started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at         TIMESTAMPTZ,
    error                TEXT        NOT NULL DEFAULT ''
);
CREATE INDEX idx_command_runs_guild ON command_runs (guild_id, started_at DESC);
CREATE INDEX idx_command_runs_command ON command_runs (command_id, started_at DESC);
CREATE INDEX idx_command_runs_resume ON command_runs (resume_at) WHERE status = 'waiting' AND resume_at IS NOT NULL;
CREATE INDEX idx_command_runs_awaiting ON command_runs (awaiting_custom_id) WHERE status = 'waiting' AND awaiting_custom_id <> '';

CREATE TABLE command_run_logs (
    id          BIGSERIAL PRIMARY KEY,
    run_id      TEXT        NOT NULL REFERENCES command_runs(id) ON DELETE CASCADE,
    step_id     TEXT        NOT NULL,
    step_kind   TEXT        NOT NULL,
    cursor_path TEXT        NOT NULL DEFAULT '',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_ms INTEGER     NOT NULL DEFAULT 0,
    status      TEXT        NOT NULL DEFAULT 'ok',
    input       JSONB,
    output      JSONB,
    error       TEXT        NOT NULL DEFAULT ''
);
CREATE INDEX idx_command_run_logs_run ON command_run_logs (run_id, started_at);

-- command_id is the owning command's UUID; the all-zero UUID is the
-- guild-shared sentinel (was 0 under the integer scheme).
CREATE TABLE feature_kv (
    guild_id   BIGINT      NOT NULL,
    command_id UUID        NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    scope      TEXT        NOT NULL,
    owner_id   BIGINT      NOT NULL DEFAULT 0,
    key        TEXT        NOT NULL,
    value      JSONB       NOT NULL,
    expires_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, command_id, scope, owner_id, key)
);
CREATE INDEX idx_feature_kv_expiry ON feature_kv (expires_at) WHERE expires_at IS NOT NULL;

-- +goose Down
-- No downgrade: this is a destructive rebuild of pre-prod tables.
