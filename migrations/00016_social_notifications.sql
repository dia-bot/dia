-- +goose Up
-- Social notification subscriptions: one row per followed account per guild.
-- account_id is the provider's canonical id (Twitch user id, YouTube UC…
-- channel id, Kick broadcaster user id, Bluesky DID, or the feed URL for RSS);
-- account_name is the human handle shown in announcements and the dashboard.
CREATE TABLE social_subscriptions (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    guild_id      BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    provider      TEXT        NOT NULL, -- twitch|youtube|kick|bluesky|rss
    account_id    TEXT        NOT NULL,
    account_name  TEXT        NOT NULL,
    account_url   TEXT        NOT NULL DEFAULT '',
    channel_id    BIGINT      NOT NULL,           -- Discord channel announcements post to
    ping_role_id  BIGINT      NOT NULL DEFAULT 0, -- role mentioned before the message (0 = none)
    template      TEXT        NOT NULL DEFAULT '', -- Go template message line ('' = provider default)
    embed         BOOLEAN     NOT NULL DEFAULT TRUE,
    enabled       BOOLEAN     NOT NULL DEFAULT TRUE,
    live          BOOLEAN     NOT NULL DEFAULT FALSE, -- twitch/kick: currently live (suppresses duplicate announces)
    hook_status   TEXT        NOT NULL DEFAULT '',    -- push providers: pending|active|error ('' for polled ones)
    last_error    TEXT        NOT NULL DEFAULT '',
    etag          TEXT        NOT NULL DEFAULT '', -- rss conditional GET state
    last_modified TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (guild_id, provider, account_id)
);
CREATE INDEX idx_social_subs_guild ON social_subscriptions(guild_id);
CREATE INDEX idx_social_subs_account ON social_subscriptions(provider, account_id);

-- Upstream items already announced per subscription, so pollers and webhook
-- replays never announce the same video/post twice across restarts.
CREATE TABLE social_seen_items (
    subscription_id BIGINT      NOT NULL REFERENCES social_subscriptions(id) ON DELETE CASCADE,
    item_id         TEXT        NOT NULL,
    seen_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (subscription_id, item_id)
);

-- +goose Down
DROP TABLE IF EXISTS social_seen_items;
DROP TABLE IF EXISTS social_subscriptions;
