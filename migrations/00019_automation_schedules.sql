-- Time-triggered automations: the "schedule" trigger runs a flow on a cadence
-- instead of a gateway event. next_run_at is the durable timer the runtime's
-- scheduler sweeps.

-- +goose Up
ALTER TABLE automations ADD COLUMN IF NOT EXISTS next_run_at TIMESTAMPTZ;
CREATE INDEX automations_due_idx ON automations (next_run_at)
    WHERE enabled AND trigger_type = 'schedule';

-- +goose Down
DROP INDEX IF EXISTS automations_due_idx;
ALTER TABLE automations DROP COLUMN IF EXISTS next_run_at;
