-- Composed announcement messages for social subscriptions: per-event-kind
-- config (enable, composed message, attached automation) as JSONB. Empty means
-- the legacy template + brand-embed path.

-- +goose Up
ALTER TABLE social_subscriptions ADD COLUMN IF NOT EXISTS spec JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE social_subscriptions DROP COLUMN IF EXISTS spec;
