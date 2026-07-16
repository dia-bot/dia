-- Scheduled messages: composed messages posted on a schedule (once, every N
-- minutes, daily, weekly). next_run_at is the durable timer: the worker sweeps
-- due rows, so a restart resumes cleanly.

-- +goose Up
CREATE TABLE scheduled_messages (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT NOT NULL,
    name        TEXT NOT NULL DEFAULT '',
    channel_id  BIGINT NOT NULL,
    spec        JSONB NOT NULL DEFAULT '{}'::jsonb,
    schedule    JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    next_run_at TIMESTAMPTZ,
    last_run_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX scheduled_messages_guild_idx ON scheduled_messages (guild_id);
CREATE INDEX scheduled_messages_due_idx ON scheduled_messages (next_run_at) WHERE enabled;

-- +goose Down
DROP TABLE scheduled_messages;
