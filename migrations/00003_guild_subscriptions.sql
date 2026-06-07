-- +goose Up
-- Stripe subscription state per guild (the $3.99/mo premium plan). Premium
-- entitlement is derived from status + current_period_end. Updated by the
-- signature-verified Stripe webhook.
CREATE TABLE guild_subscriptions (
    guild_id               BIGINT      PRIMARY KEY REFERENCES guilds(id) ON DELETE CASCADE,
    stripe_customer_id     TEXT        NOT NULL DEFAULT '',
    stripe_subscription_id TEXT        NOT NULL DEFAULT '',
    status                 TEXT        NOT NULL DEFAULT '', -- active|trialing|past_due|canceled|...
    current_period_end     TIMESTAMPTZ,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_guild_subs_customer ON guild_subscriptions(stripe_customer_id);
CREATE INDEX idx_guild_subs_subscription ON guild_subscriptions(stripe_subscription_id);

-- +goose Down
DROP TABLE IF EXISTS guild_subscriptions;
