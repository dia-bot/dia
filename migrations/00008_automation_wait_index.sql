-- +goose Up

-- Message / reaction waits resume when a matching event arrives: on each
-- MESSAGE_CREATE / MESSAGE_REACTION_ADD the worker looks up runs parked on that
-- kind for the guild. This partial index keeps that lookup to the handful of
-- currently-waiting runs instead of scanning the guild's whole run history.
CREATE INDEX idx_automation_runs_wait_kind ON automation_runs (guild_id, awaiting_kind)
    WHERE status = 'waiting';

-- +goose Down
DROP INDEX IF EXISTS idx_automation_runs_wait_kind;
