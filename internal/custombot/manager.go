// Package custombot drives the customer-bot ("bring your own token") control
// plane from the Go side: it turns dashboard actions and stored state into the
// ensure/remove/presence commands the Elixir gateway executes, and reconciles
// the gateway's reported state back into the database.
package custombot

import (
	"context"
	"strconv"

	"github.com/dia-bot/dia/internal/botreg"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/eventbus"
	"github.com/dia-bot/dia/internal/secret"
	"github.com/dia-bot/dia/internal/store"

	"encoding/json"
	"log/slog"
)

// Manager publishes control commands to the gateway and keeps the client
// registry in sync. It is safe to construct in both the API (to react to
// dashboard actions) and the worker (to reconcile). It holds no long-running
// state of its own.
type Manager struct {
	store *store.Store
	box   *secret.Box
	bus   eventbus.Bus
	bots  *botreg.Registry
	log   *slog.Logger
}

// NewManager builds a Manager. When the secret.Box is disabled (no key) callers
// should gate on Enabled() first; the token-touching methods otherwise surface
// secret.ErrNoKey.
func NewManager(st *store.Store, box *secret.Box, bus eventbus.Bus, bots *botreg.Registry, log *slog.Logger) *Manager {
	return &Manager{store: st, box: box, bus: bus, bots: bots, log: log}
}

// Enabled reports whether custom bots are configured (an encryption key is set).
func (m *Manager) Enabled() bool { return m.box.Enabled() }

// EnsureGuild reconciles the gateway with a guild's stored custom bot: it starts
// (or restyles) the bot when the row is enabled, or tears it down otherwise.
// Call it after any change to the guild's custom-bot row.
func (m *Manager) EnsureGuild(ctx context.Context, guildID int64) error {
	row, ok, err := m.store.CustomBots.Get(ctx, guildID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if !row.Enabled {
		return m.removeApp(row.ApplicationID)
	}
	token, err := m.box.DecryptString(row.TokenEnc)
	if err != nil {
		return err
	}
	return m.publish(event.BotCommand{
		Action:   event.BotActionEnsure,
		AppID:    strconv.FormatInt(row.ApplicationID, 10),
		Token:    token,
		Intents:  int(row.Intents),
		Presence: presenceOf(row),
	})
}

// Presence pushes just the presence/activity for a guild's running bot.
func (m *Manager) Presence(ctx context.Context, guildID int64) error {
	row, ok, err := m.store.CustomBots.Get(ctx, guildID)
	if err != nil || !ok || !row.Enabled {
		return err
	}
	return m.publish(event.BotCommand{
		Action:   event.BotActionPresence,
		AppID:    strconv.FormatInt(row.ApplicationID, 10),
		Presence: presenceOf(row),
	})
}

// RemoveApp tears down a running bot by application id (used when a bot is
// deleted or its guild disables it).
func (m *Manager) RemoveApp(appID int64) error { return m.removeApp(appID) }

func (m *Manager) removeApp(appID int64) error {
	m.bots.Invalidate(appID)
	return m.publish(event.BotCommand{Action: event.BotActionRemove, AppID: strconv.FormatInt(appID, 10)})
}

// ReplayAll re-issues an ensure for every enabled custom bot. Called when the
// gateway announces it (re)started and periodically as a reconcile, so the
// gateway's running set converges on the database's desired set.
func (m *Manager) ReplayAll(ctx context.Context) error {
	rows, err := m.store.CustomBots.ListEnabled(ctx)
	if err != nil {
		return err
	}
	seen := map[int64]bool{}
	for _, row := range rows {
		if seen[row.ApplicationID] {
			continue // one ensure per application, even if it backs several guilds
		}
		seen[row.ApplicationID] = true
		token, derr := m.box.DecryptString(row.TokenEnc)
		if derr != nil {
			m.log.Warn("custombot: decrypt on replay failed", "app_id", row.ApplicationID, "err", derr)
			continue
		}
		if perr := m.publish(event.BotCommand{
			Action:   event.BotActionEnsure,
			AppID:    strconv.FormatInt(row.ApplicationID, 10),
			Token:    token,
			Intents:  int(row.Intents),
			Presence: presenceOf(row),
		}); perr != nil {
			m.log.Warn("custombot: replay publish failed", "app_id", row.ApplicationID, "err", perr)
		}
	}
	return nil
}

func (m *Manager) publish(cmd event.BotCommand) error {
	body, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	return m.bus.PublishCore(event.SubjectBotCommand, body)
}

func presenceOf(b store.CustomBot) *event.Presence {
	return &event.Presence{
		Status:       b.PresenceStatus,
		ActivityType: b.ActivityType,
		ActivityText: b.ActivityText,
		ActivityURL:  b.ActivityURL,
	}
}
