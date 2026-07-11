-- +goose Up

-- Ticketing: support tickets opened from a panel. A panel is a posted message
-- (embed + buttons or a select) whose categories each open a private channel (or
-- thread) for the member and staff. Panels + their categories live in
-- ticket_panels (categories are a JSONB array on config); live tickets are rows
-- in tickets. The feature toggle + shared settings live in guild_feature_configs
-- under feature_key 'tickets'. UUIDs are TEXT (gen_random_uuid()::text) to match
-- automations; Discord snowflakes stay BIGINT.
CREATE TABLE ticket_panels (
    id         TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    guild_id   BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    channel_id BIGINT      NOT NULL DEFAULT 0,  -- where the panel is posted
    message_id BIGINT      NOT NULL DEFAULT 0,  -- the posted panel message
    name       TEXT        NOT NULL DEFAULT '',
    style      TEXT        NOT NULL DEFAULT 'buttons', -- buttons | select
    config     JSONB       NOT NULL DEFAULT '{}'::jsonb, -- embed + categories[]
    enabled    BOOLEAN     NOT NULL DEFAULT true,
    position   INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ticket_panels_guild ON ticket_panels (guild_id, position);

-- One live (or historical) ticket. number is a per-guild monotonic counter used
-- for the channel name. category_id is the stable category key inside the
-- panel's config JSONB (panels may be edited/deleted, so it is not an FK).
CREATE TABLE tickets (
    id                  TEXT        PRIMARY KEY DEFAULT gen_random_uuid()::text,
    guild_id            BIGINT      NOT NULL REFERENCES guilds(id) ON DELETE CASCADE,
    number              INTEGER     NOT NULL,
    panel_id            TEXT        REFERENCES ticket_panels(id) ON DELETE SET NULL,
    category_id         TEXT        NOT NULL DEFAULT '',
    category_label      TEXT        NOT NULL DEFAULT '',
    channel_id          BIGINT      NOT NULL DEFAULT 0,   -- the ticket channel or thread
    is_thread           BOOLEAN     NOT NULL DEFAULT false,
    opener_id           BIGINT      NOT NULL,
    opener_username     TEXT        NOT NULL DEFAULT '', -- captured at open so rebuilds/transcripts show the name
    opener_global_name  TEXT        NOT NULL DEFAULT '',
    subject             TEXT        NOT NULL DEFAULT '',
    status              TEXT        NOT NULL DEFAULT 'open', -- open | closed | deleted
    claimed_by          BIGINT      NOT NULL DEFAULT 0,
    form_answers        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    auto_close_minutes  INTEGER     NOT NULL DEFAULT 0,  -- 0 = no inactivity close
    auto_warn_minutes   INTEGER     NOT NULL DEFAULT 0,  -- grace after the warning
    opened_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    claimed_at          TIMESTAMPTZ,
    first_response_at   TIMESTAMPTZ,
    last_activity_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    close_warned_at     TIMESTAMPTZ,
    closed_at           TIMESTAMPTZ,
    closed_by           BIGINT      NOT NULL DEFAULT 0,
    close_reason        TEXT        NOT NULL DEFAULT '',
    rating              SMALLINT    NOT NULL DEFAULT 0,  -- 0 = unrated, else 1..5
    feedback            TEXT        NOT NULL DEFAULT '',
    transcript_url      TEXT        NOT NULL DEFAULT '',
    transcript_messages INTEGER     NOT NULL DEFAULT 0,
    UNIQUE (guild_id, number)
);
-- Dashboard list / queue.
CREATE INDEX idx_tickets_guild ON tickets (guild_id, status, opened_at DESC);
-- O(1) channel -> ticket lookup for activity tracking; a channel maps to at most
-- one non-deleted ticket. channel_id = 0 (a ticket mid-creation, before its
-- channel exists) is excluded so concurrent opens don't collide on the sentinel.
CREATE UNIQUE INDEX idx_tickets_channel ON tickets (channel_id) WHERE status <> 'deleted' AND channel_id <> 0;
-- Inactivity sweep.
CREATE INDEX idx_tickets_autoclose ON tickets (last_activity_at) WHERE status = 'open' AND auto_close_minutes > 0;

-- Append-only lifecycle log (opened, claimed, closed, user added, note, ...).
-- Powers the ticket log channel and the dashboard timeline + analytics.
CREATE TABLE ticket_events (
    id         BIGSERIAL   PRIMARY KEY,
    ticket_id  TEXT        NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    guild_id   BIGINT      NOT NULL,
    kind       TEXT        NOT NULL,
    actor_id   BIGINT      NOT NULL DEFAULT 0,
    data       JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ticket_events_ticket ON ticket_events (ticket_id, created_at);
CREATE INDEX idx_ticket_events_guild ON ticket_events (guild_id, created_at DESC);

-- Extra members granted access to a ticket (opener always has access via the
-- channel overwrite; this tracks staff/others added with /ticket add).
CREATE TABLE ticket_participants (
    ticket_id TEXT        NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    user_id   BIGINT      NOT NULL,
    role      TEXT        NOT NULL DEFAULT 'added', -- opener | added
    added_by  BIGINT      NOT NULL DEFAULT 0,
    added_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (ticket_id, user_id)
);

-- Staff-only notes on a ticket (never shown to the opener).
CREATE TABLE ticket_notes (
    id         BIGSERIAL   PRIMARY KEY,
    ticket_id  TEXT        NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    guild_id   BIGINT      NOT NULL,
    author_id  BIGINT      NOT NULL,
    body       TEXT        NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_ticket_notes_ticket ON ticket_notes (ticket_id, created_at);

-- +goose Down
DROP TABLE IF EXISTS ticket_notes;
DROP TABLE IF EXISTS ticket_participants;
DROP TABLE IF EXISTS ticket_events;
DROP TABLE IF EXISTS tickets;
DROP TABLE IF EXISTS ticket_panels;
