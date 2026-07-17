-- +goose Up
-- Custom bots: a customer's own Discord application ("bring your own token"),
-- run on our infrastructure so their server's bot wears their name, avatar and
-- presence while running every Dia feature. One row per guild; the same
-- application (token) may back more than one guild, so the gateway keys a live
-- connection by application_id and refcounts the guilds that enabled it.
--
-- The bot token and OAuth client secret are god-mode credentials, so they are
-- stored ENCRYPTED (AES-256-GCM via internal/secret); the *_enc columns hold
-- nonce||ciphertext, never plaintext.
CREATE TABLE custom_bots (
    guild_id        BIGINT      PRIMARY KEY REFERENCES guilds(id) ON DELETE CASCADE,
    application_id  BIGINT      NOT NULL,           -- the customer's application / client id
    bot_user_id     BIGINT      NOT NULL,           -- the bot user's id (== application_id for bots)
    username        TEXT        NOT NULL DEFAULT '', -- last-seen bot username (for the dashboard)
    avatar          TEXT        NOT NULL DEFAULT '', -- last-seen avatar hash
    token_enc       BYTEA       NOT NULL,            -- AES-GCM(bot token)
    secret_enc      BYTEA,                           -- AES-GCM(oauth client secret); NULL until provided
    intents         BIGINT      NOT NULL DEFAULT 0,  -- gateway intents to IDENTIFY with

    -- Presence the gateway sets for this bot (its own connection, so unlike the
    -- shared bot this actually works per customer).
    presence_status TEXT        NOT NULL DEFAULT 'online',   -- online|idle|dnd|invisible
    activity_type   INTEGER     NOT NULL DEFAULT -1,         -- -1 none, 0 playing, 1 streaming, 2 listening, 3 watching, 5 competing
    activity_text   TEXT        NOT NULL DEFAULT '',
    activity_url    TEXT        NOT NULL DEFAULT '',         -- streaming url (activity_type = 1)

    enabled         BOOLEAN     NOT NULL DEFAULT FALSE,      -- customer flipped it on (desired running state)
    state           TEXT        NOT NULL DEFAULT 'disconnected', -- connecting|ready|error|disconnected (reported by the gateway)
    last_error      TEXT        NOT NULL DEFAULT '',         -- last IDENTIFY / connection error, surfaced on the dashboard
    commands_synced BOOLEAN     NOT NULL DEFAULT FALSE,      -- commands registered under this application id

    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- The gateway ensures one connection per application; look up rows by app fast.
CREATE INDEX idx_custom_bots_application ON custom_bots(application_id);
-- The control loop scans enabled rows to compute the desired running set.
CREATE INDEX idx_custom_bots_enabled ON custom_bots(application_id) WHERE enabled;

-- +goose Down
DROP TABLE IF EXISTS custom_bots;
