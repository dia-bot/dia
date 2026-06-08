-- +goose Up
-- Per-guild uploaded assets (Card Studio images + premium custom fonts). The
-- file lives in object storage; this row holds the pointer + size, which backs
-- the storage-usage overview and quota enforcement. Fonts also carry a family
-- name (the renderer resolves family → url from here).
CREATE TABLE guild_uploads (
    id         BIGSERIAL   PRIMARY KEY,
    guild_id   BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    kind       TEXT        NOT NULL,            -- 'image' | 'font'
    family     TEXT        NOT NULL DEFAULT '', -- font family (kind = 'font')
    object_key TEXT        NOT NULL DEFAULT '', -- storage key, for deletion
    url        TEXT        NOT NULL,
    bytes      BIGINT      NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_guild_uploads_guild ON guild_uploads(guild_id);

-- +goose Down
DROP TABLE IF EXISTS guild_uploads;
