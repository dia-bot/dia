-- +goose Up

-- Giveaways: a hosted prize draw with a live entry button, a scheduled end, and
-- a weighted random winner draw. Each giveaway is one row; entries are one row
-- per member (weighted by bonus tickets). A background sweeper ends running
-- giveaways whose ends_at has passed, so timers survive restarts (the same
-- "row + ticker + atomic claim" pattern as automation waits and mod expiry).
-- Each giveaway carries its OWN presentation (the `spec` JSONB: the composed
-- message/embeds, the Enter button, the winner announcement, and behaviour
-- toggles), so every giveaway is independently customizable from the dashboard.
-- The per-guild feature config (guild_feature_configs keyed "giveaway") holds
-- only the manager roles and the library of reusable presets that seed new
-- giveaways; this table is the live state.
CREATE TABLE giveaways (
    id            TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    guild_id      BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id    BIGINT      NOT NULL,
    message_id    BIGINT      NOT NULL DEFAULT 0,           -- posted giveaway message (0 until posted)
    name          TEXT        NOT NULL DEFAULT '',          -- dashboard label ('' = fall back to prize)
    prize         TEXT        NOT NULL,
    description   TEXT        NOT NULL DEFAULT '',
    winner_count  INTEGER     NOT NULL DEFAULT 1,
    host_id       BIGINT      NOT NULL DEFAULT 0,           -- shown as "Hosted by"
    status        TEXT        NOT NULL DEFAULT 'running',   -- draft | scheduled | running | ended | cancelled
    spec          JSONB       NOT NULL DEFAULT '{}'::jsonb, -- this giveaway's composed message + button + announce + behaviour
    requirements  JSONB       NOT NULL DEFAULT '{}'::jsonb, -- resolved entry requirements for this giveaway
    image_url     TEXT        NOT NULL DEFAULT '',
    color         TEXT        NOT NULL DEFAULT '',          -- hex override ('' = use embed default)
    winner_ids    BIGINT[]    NOT NULL DEFAULT '{}',        -- most recent draw (after ending / reroll)
    starts_at     TIMESTAMPTZ NOT NULL DEFAULT now(),       -- when a scheduled giveaway should post
    ends_at       TIMESTAMPTZ NOT NULL,
    ended_at      TIMESTAMPTZ,
    created_by    BIGINT      NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_giveaways_guild ON giveaways (guild_id, created_at DESC);
-- The sweeper claims due running giveaways: a cheap range scan over just the
-- rows that can still end. Drafts are never swept (posted only on an explicit
-- Start), and scheduled rows post when their start time arrives.
CREATE INDEX idx_giveaways_due ON giveaways (ends_at) WHERE status = 'running';
-- Scheduled giveaways waiting to post.
CREATE INDEX idx_giveaways_scheduled ON giveaways (starts_at) WHERE status = 'scheduled';
-- A posted giveaway is found by its message id when its Enter button is clicked.
CREATE UNIQUE INDEX idx_giveaways_message ON giveaways (message_id) WHERE message_id <> 0;

-- One entry per member per giveaway. `entries` is the member's weighted ticket
-- count (1 base + any role bonuses), used to bias the random draw. Leaving the
-- giveaway deletes the row.
CREATE TABLE giveaway_entries (
    giveaway_id  TEXT        NOT NULL REFERENCES giveaways(id) ON DELETE CASCADE,
    user_id      BIGINT      NOT NULL,
    entries      INTEGER     NOT NULL DEFAULT 1,
    entered_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (giveaway_id, user_id)
);
CREATE INDEX idx_giveaway_entries_gw ON giveaway_entries (giveaway_id);

-- +goose Down
DROP TABLE IF EXISTS giveaway_entries;
DROP TABLE IF EXISTS giveaways;
