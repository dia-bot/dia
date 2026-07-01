-- +goose Up

-- Automod infractions: the "heat" ledger behind escalation. Each row is one
-- automod rule hit that awarded points to a user. Active points (those not yet
-- past expires_at) are summed per user to decide which escalation tier applies,
-- so repeat offenders climb the ladder (timeout -> kick -> ban) automatically.
-- Snowflakes stay BIGINT; rule_id is the automod rule's stable text id.
CREATE TABLE automod_infractions (
    id           BIGSERIAL   PRIMARY KEY,
    guild_id     BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    user_id      BIGINT      NOT NULL,
    rule_id      TEXT        NOT NULL DEFAULT '',
    rule_name    TEXT        NOT NULL DEFAULT '',
    trigger_type TEXT        NOT NULL DEFAULT '',
    points       INTEGER     NOT NULL DEFAULT 0,
    reason       TEXT        NOT NULL DEFAULT '',
    channel_id   BIGINT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ                       -- NULL = never decays
);

-- Sum of active points for a user (escalation lookup) and per-user history.
CREATE INDEX idx_automod_infractions_user ON automod_infractions (guild_id, user_id, created_at DESC);
-- Active-points window scan (expires_at IS NULL OR expires_at > now()).
CREATE INDEX idx_automod_infractions_active ON automod_infractions (guild_id, user_id, expires_at);
-- Recent activity + leaderboard for the dashboard.
CREATE INDEX idx_automod_infractions_recent ON automod_infractions (guild_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS automod_infractions;
