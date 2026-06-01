-- +goose Up

-- Guilds the bot is a member of. Channels/roles are NOT stored here — they live
-- in the Redis guild snapshot (rebuilt from the gateway) for realtime dashboard
-- reads. Postgres holds durable configuration only.
CREATE TABLE guilds (
    id           BIGINT PRIMARY KEY,            -- Discord guild snowflake
    name         TEXT        NOT NULL DEFAULT '',
    icon         TEXT        NOT NULL DEFAULT '',
    owner_id     BIGINT      NOT NULL DEFAULT 0,
    member_count INTEGER     NOT NULL DEFAULT 0,
    joined_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    left_at      TIMESTAMPTZ,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Generic, extensible per-feature configuration. Each feature serializes its
-- typed config struct into `config` (JSONB) and validates on write. New features
-- need no schema migration — just a new feature_key.
CREATE TABLE guild_feature_configs (
    guild_id    BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    feature_key TEXT        NOT NULL,           -- 'welcome' | 'leveling' | 'autorole' | 'moderation' | 'automod' | 'customcommands' | 'reactionroles'
    enabled     BOOLEAN     NOT NULL DEFAULT false,
    config      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, feature_key)
);

-- Leveling: per-member XP/level state.
CREATE TABLE level_users (
    guild_id        BIGINT      NOT NULL,
    user_id         BIGINT      NOT NULL,
    xp              BIGINT      NOT NULL DEFAULT 0,
    level           INTEGER     NOT NULL DEFAULT 0,
    messages        BIGINT      NOT NULL DEFAULT 0,
    last_message_at TIMESTAMPTZ,
    PRIMARY KEY (guild_id, user_id)
);
CREATE INDEX idx_level_users_leaderboard ON level_users (guild_id, xp DESC);

-- Leveling: role rewards granted at a given level.
CREATE TABLE level_rewards (
    guild_id   BIGINT  NOT NULL,
    level      INTEGER NOT NULL,
    role_id    BIGINT  NOT NULL,
    remove_previous BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (guild_id, level)
);

-- Moderation: case log (ban/kick/timeout/warn).
CREATE TABLE mod_cases (
    id               BIGSERIAL PRIMARY KEY,
    guild_id         BIGINT      NOT NULL,
    case_number      INTEGER     NOT NULL,
    user_id          BIGINT      NOT NULL,
    moderator_id     BIGINT      NOT NULL,
    action           TEXT        NOT NULL,       -- 'ban' | 'kick' | 'timeout' | 'warn' | 'unban' | 'untimeout'
    reason           TEXT        NOT NULL DEFAULT '',
    duration_seconds INTEGER     NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at       TIMESTAMPTZ,
    active           BOOLEAN     NOT NULL DEFAULT true,
    UNIQUE (guild_id, case_number)
);
CREATE INDEX idx_mod_cases_user ON mod_cases (guild_id, user_id);
CREATE INDEX idx_mod_cases_expiry ON mod_cases (expires_at) WHERE active AND expires_at IS NOT NULL;

-- Reaction/self-assign role menus (buttons + select menus, slash-native).
CREATE TABLE reaction_role_menus (
    id         BIGSERIAL PRIMARY KEY,
    guild_id   BIGINT      NOT NULL,
    channel_id BIGINT      NOT NULL DEFAULT 0,
    message_id BIGINT      NOT NULL DEFAULT 0,
    title      TEXT        NOT NULL DEFAULT '',
    mode       TEXT        NOT NULL DEFAULT 'toggle',  -- 'toggle' | 'unique' | 'verify'
    options    JSONB       NOT NULL DEFAULT '[]'::jsonb, -- [{role_id,label,emoji,description}]
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_reaction_role_menus_guild ON reaction_role_menus (guild_id);

-- Admin-defined custom slash commands.
CREATE TABLE custom_commands (
    id          BIGSERIAL PRIMARY KEY,
    guild_id    BIGINT      NOT NULL,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    response    JSONB       NOT NULL DEFAULT '{}'::jsonb,  -- {content, embeds, ephemeral, ...}
    enabled     BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (guild_id, name)
);

-- Dashboard audit trail (who changed what).
CREATE TABLE dashboard_audit_log (
    id         BIGSERIAL PRIMARY KEY,
    guild_id   BIGINT      NOT NULL,
    user_id    BIGINT      NOT NULL,
    action     TEXT        NOT NULL,
    detail     JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_dashboard_audit_guild ON dashboard_audit_log (guild_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS dashboard_audit_log;
DROP TABLE IF EXISTS custom_commands;
DROP TABLE IF EXISTS reaction_role_menus;
DROP TABLE IF EXISTS mod_cases;
DROP TABLE IF EXISTS level_rewards;
DROP TABLE IF EXISTS level_users;
DROP TABLE IF EXISTS guild_feature_configs;
DROP TABLE IF EXISTS guilds;
