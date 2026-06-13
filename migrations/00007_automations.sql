-- +goose Up

-- Server-event automations: "when X happens, run these steps". An automation
-- pairs a trigger (a gateway event + filters) with the same JSONB Step[] program
-- custom commands use. It reuses the command runtime (tree-walking interpreter)
-- and resumes durable steps (wait / wait_for) via automation_runs. UUIDs are
-- TEXT (gen_random_uuid()::text), matching the custom-commands convention;
-- Discord snowflakes stay BIGINT; run ids stay ULID-ish TEXT.
CREATE TABLE automations (
    id             TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    guild_id       BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    name           TEXT        NOT NULL,
    description    TEXT        NOT NULL DEFAULT '',
    enabled        BOOLEAN     NOT NULL DEFAULT true,
    status         TEXT        NOT NULL DEFAULT 'draft',  -- draft | published | archived
    version        INTEGER     NOT NULL DEFAULT 1,
    trigger_type   TEXT        NOT NULL,                  -- catalogue key (member_join, message_create, ...)
    event_type     TEXT        NOT NULL DEFAULT '',       -- resolved gateway event (indexed for dispatch)
    trigger_config JSONB       NOT NULL DEFAULT '{}'::jsonb,
    definition     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_by     BIGINT      NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_automations_guild ON automations (guild_id);
-- Partial index over enabled rows by event so per-event dispatch is a cheap
-- range scan even on busy MESSAGE_CREATE traffic.
CREATE INDEX idx_automations_event ON automations (guild_id, event_type) WHERE enabled;

-- Immutable per-publish snapshot of the definition + trigger so in-flight
-- durable runs survive re-edits (same Temporal pattern as command versions).
CREATE TABLE automation_versions (
    automation_id  TEXT        NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
    version        INTEGER     NOT NULL,
    definition     JSONB       NOT NULL,
    trigger_type   TEXT        NOT NULL DEFAULT '',
    trigger_config JSONB       NOT NULL DEFAULT '{}'::jsonb,
    published_by   BIGINT      NOT NULL DEFAULT 0,
    published_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (automation_id, version)
);

-- One execution of an automation. Persisted only for durable steps (wait,
-- wait_for, parallel); pure synchronous runs leave only their log rows. Mirrors
-- command_runs so the same exec engine + scheduler patterns apply.
CREATE TABLE automation_runs (
    id                   TEXT        PRIMARY KEY,           -- ULID-ish
    automation_id        TEXT        NOT NULL REFERENCES automations(id) ON DELETE CASCADE,
    automation_version   INTEGER     NOT NULL,
    guild_id             BIGINT      NOT NULL,
    invoker_id           BIGINT      NOT NULL DEFAULT 0,    -- the event actor
    channel_id           BIGINT      NOT NULL DEFAULT 0,
    trigger_kind         TEXT        NOT NULL,              -- trigger catalogue key
    interaction_id       TEXT        NOT NULL DEFAULT '',   -- set only on component-resumed runs
    interaction_token    TEXT        NOT NULL DEFAULT '',
    interaction_expires  TIMESTAMPTZ,
    scope                JSONB       NOT NULL DEFAULT '{}'::jsonb,
    cursor               JSONB       NOT NULL DEFAULT '[]'::jsonb,
    status               TEXT        NOT NULL DEFAULT 'running',  -- running | waiting | done | failed | exited
    resume_at            TIMESTAMPTZ,
    awaiting_custom_id   TEXT        NOT NULL DEFAULT '',
    awaiting_user_id     BIGINT      NOT NULL DEFAULT 0,
    awaiting_kind        TEXT        NOT NULL DEFAULT '',
    definition_snapshot  JSONB       NOT NULL,
    started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at         TIMESTAMPTZ,
    error                TEXT        NOT NULL DEFAULT ''
);
CREATE INDEX idx_automation_runs_guild ON automation_runs (guild_id, started_at DESC);
CREATE INDEX idx_automation_runs_auto ON automation_runs (automation_id, started_at DESC);
CREATE INDEX idx_automation_runs_resume ON automation_runs (resume_at) WHERE status = 'waiting' AND resume_at IS NOT NULL;
CREATE INDEX idx_automation_runs_awaiting ON automation_runs (awaiting_custom_id) WHERE status = 'waiting' AND awaiting_custom_id <> '';

-- One row per executed step — powers the automation Runs-tab timeline view.
CREATE TABLE automation_run_logs (
    id          BIGSERIAL PRIMARY KEY,
    run_id      TEXT        NOT NULL REFERENCES automation_runs(id) ON DELETE CASCADE,
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
CREATE INDEX idx_automation_run_logs_run ON automation_run_logs (run_id, started_at);

-- +goose Down
DROP TABLE IF EXISTS automation_run_logs;
DROP TABLE IF EXISTS automation_runs;
DROP TABLE IF EXISTS automation_versions;
DROP TABLE IF EXISTS automations;
