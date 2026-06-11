-- +goose Up

-- Drop the legacy v1 custom_commands; prod isn't deployed yet so a clean rebuild
-- is the right call rather than threading a fallback interpreter through the new
-- runtime.
DROP TABLE IF EXISTS custom_commands;

-- A custom command is a JSONB program (slash surface + declared variables +
-- nested Step[] tree). The runtime is a tree-walking interpreter that resumes
-- via command_runs for wait/wait_for/scheduled steps.
CREATE TABLE custom_commands (
    id                 BIGSERIAL PRIMARY KEY,
    guild_id           BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name               TEXT        NOT NULL,
    description        TEXT        NOT NULL DEFAULT '',
    enabled            BOOLEAN     NOT NULL DEFAULT true,
    status             TEXT        NOT NULL DEFAULT 'draft',  -- draft | published | archived
    version            INTEGER     NOT NULL DEFAULT 1,
    requires_defer     BOOLEAN     NOT NULL DEFAULT false,    -- validator output: must Defer to keep within 3s
    definition         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_by         BIGINT      NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (guild_id, name)
);
CREATE INDEX idx_custom_commands_guild ON custom_commands (guild_id);

-- Immutable per-publish snapshot of the Definition. command_runs.command_version
-- pins to a row here so in-flight runs survive re-publishes (Temporal pattern).
CREATE TABLE custom_command_versions (
    command_id   BIGINT      NOT NULL REFERENCES custom_commands(id) ON DELETE CASCADE,
    version      INTEGER     NOT NULL,
    definition   JSONB       NOT NULL,
    published_by BIGINT      NOT NULL DEFAULT 0,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (command_id, version)
);

-- A single execution of a command. Persisted only for durable steps (wait,
-- wait_for, scheduled triggers, parallel). Pure synchronous runs are tracked
-- transiently in memory and only their logs land in command_run_logs.
CREATE TABLE command_runs (
    id                   TEXT        PRIMARY KEY,           -- ULID
    command_id           BIGINT      NOT NULL REFERENCES custom_commands(id) ON DELETE CASCADE,
    command_version      INTEGER     NOT NULL,              -- pinned to custom_command_versions
    guild_id             BIGINT      NOT NULL,
    invoker_id           BIGINT      NOT NULL DEFAULT 0,
    channel_id           BIGINT      NOT NULL DEFAULT 0,
    trigger_kind         TEXT        NOT NULL,              -- slash | component | modal | event | schedule
    interaction_id       TEXT        NOT NULL DEFAULT '',
    interaction_token    TEXT        NOT NULL DEFAULT '',
    interaction_expires  TIMESTAMPTZ,                       -- 15 min after first Defer
    scope                JSONB       NOT NULL DEFAULT '{}'::jsonb,
    cursor               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    status               TEXT        NOT NULL DEFAULT 'running',  -- running | waiting | done | failed | cancelled
    resume_at            TIMESTAMPTZ,                       -- wait deadline
    awaiting_custom_id   TEXT        NOT NULL DEFAULT '',   -- wait_for component/modal id (prefix match)
    awaiting_user_id     BIGINT      NOT NULL DEFAULT 0,    -- 0 = anyone
    awaiting_kind        TEXT        NOT NULL DEFAULT '',   -- component | modal | message | reaction
    definition_snapshot  JSONB       NOT NULL,              -- frozen Definition at run start
    started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at         TIMESTAMPTZ,
    error                TEXT        NOT NULL DEFAULT ''
);
CREATE INDEX idx_command_runs_guild ON command_runs (guild_id, started_at DESC);
CREATE INDEX idx_command_runs_command ON command_runs (command_id, started_at DESC);
CREATE INDEX idx_command_runs_resume ON command_runs (resume_at) WHERE status = 'waiting' AND resume_at IS NOT NULL;
CREATE INDEX idx_command_runs_awaiting ON command_runs (awaiting_custom_id) WHERE status = 'waiting' AND awaiting_custom_id <> '';

-- One row per executed step — powers the Runs-tab timeline view.
CREATE TABLE command_run_logs (
    id          BIGSERIAL PRIMARY KEY,
    run_id      TEXT        NOT NULL REFERENCES command_runs(id) ON DELETE CASCADE,
    step_id     TEXT        NOT NULL,
    step_kind   TEXT        NOT NULL,
    cursor_path TEXT        NOT NULL DEFAULT '',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    duration_ms INTEGER     NOT NULL DEFAULT 0,
    status      TEXT        NOT NULL DEFAULT 'ok',  -- ok | error | skipped
    input       JSONB,
    output      JSONB,
    error       TEXT        NOT NULL DEFAULT ''
);
CREATE INDEX idx_command_run_logs_run ON command_run_logs (run_id, started_at);

-- Durable per-guild / per-member key/value store for kv_get & kv_set steps.
CREATE TABLE feature_kv (
    guild_id   BIGINT      NOT NULL,
    command_id BIGINT      NOT NULL DEFAULT 0,   -- 0 = shared across all commands in this guild
    scope      TEXT        NOT NULL,             -- guild | member
    owner_id   BIGINT      NOT NULL DEFAULT 0,   -- member snowflake when scope=member, 0 when scope=guild
    key        TEXT        NOT NULL,
    value      JSONB       NOT NULL,
    expires_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, command_id, scope, owner_id, key)
);
CREATE INDEX idx_feature_kv_expiry ON feature_kv (expires_at) WHERE expires_at IS NOT NULL;

-- A library of reusable Card Studio layouts. image_render steps reference these
-- by template_id so an admin designs once and uses across many commands.
CREATE TABLE command_image_templates (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    layout      JSONB       NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (guild_id, name)
);
CREATE INDEX idx_command_image_templates_guild ON command_image_templates (guild_id);

-- +goose Down
DROP TABLE IF EXISTS command_image_templates;
DROP TABLE IF EXISTS feature_kv;
DROP TABLE IF EXISTS command_run_logs;
DROP TABLE IF EXISTS command_runs;
DROP TABLE IF EXISTS custom_command_versions;
DROP TABLE IF EXISTS custom_commands;

CREATE TABLE custom_commands (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT      NOT NULL,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    response    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    enabled     BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (guild_id, name)
);
