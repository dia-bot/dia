-- +goose Up

-- Catch-up for dev databases that applied an early draft of 00012 before it
-- was amended in place (the giveaway redesign added the dashboard `name` label
-- and the per-giveaway composed `spec`, plus the scheduled-post index). Goose
-- tracks by version number, so an already-applied 00012 never re-runs; these
-- guarded statements bring such a database up to the current shape and are
-- no-ops on any database created from the final 00012.
ALTER TABLE giveaways ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE giveaways ADD COLUMN IF NOT EXISTS spec JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE giveaways ADD COLUMN IF NOT EXISTS starts_at TIMESTAMPTZ NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS idx_giveaways_scheduled ON giveaways (starts_at) WHERE status = 'scheduled';

-- +goose Down
-- Nothing to undo: 00012's Down drops the giveaways table entirely, and on a
-- database built from the final 00012 this migration changed nothing.
SELECT 1;
