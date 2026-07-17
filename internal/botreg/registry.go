// Package botreg resolves the Discord REST client that should act for a given
// guild or application. For most guilds that is the shared platform bot; for a
// guild running a customer's custom bot it is a client built from that bot's
// decrypted token, so sends, role grants and moderation act as (and with the
// permissions of) the customer's bot.
package botreg

import (
	"context"
	"log/slog"
	"strconv"
	"sync"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/secret"
	"github.com/dia-bot/dia/internal/store"
)

// Registry hands out per-bot REST clients, caching one client per application.
type Registry struct {
	platform      *discord.Client
	platformAppID string
	store         *store.Store
	box           *secret.Box
	log           *slog.Logger

	mu    sync.Mutex
	byApp map[int64]*discord.Client
}

// New builds a registry over the platform client. box may be a disabled
// secret.Box (no key configured): custom bots are then simply never resolved
// and everything falls back to the platform client.
func New(platform *discord.Client, st *store.Store, box *secret.Box, log *slog.Logger) *Registry {
	return &Registry{
		platform:      platform,
		platformAppID: platform.AppID(),
		store:         st,
		box:           box,
		log:           log,
		byApp:         map[int64]*discord.Client{},
	}
}

// Platform returns the shared bot's client.
func (r *Registry) Platform() *discord.Client { return r.platform }

// ForApp returns the client for an application id. The platform id (or an empty
// id) yields the platform client. Any error resolving a custom bot falls back
// to the platform client so a lookup failure never drops the work entirely.
func (r *Registry) ForApp(ctx context.Context, appID string) *discord.Client {
	if appID == "" || appID == r.platformAppID || !r.box.Enabled() {
		return r.platform
	}
	id, err := strconv.ParseInt(appID, 10, 64)
	if err != nil {
		return r.platform
	}

	r.mu.Lock()
	if c, ok := r.byApp[id]; ok {
		r.mu.Unlock()
		return c
	}
	r.mu.Unlock()

	row, ok, err := r.store.CustomBots.GetByApp(ctx, id)
	if err != nil || !ok {
		return r.platform
	}
	token, err := r.box.DecryptString(row.TokenEnc)
	if err != nil {
		r.log.Warn("botreg: decrypt token failed", "app_id", appID, "err", err)
		return r.platform
	}
	c, err := discord.New(token, appID, r.log)
	if err != nil {
		r.log.Warn("botreg: build client failed", "app_id", appID, "err", err)
		return r.platform
	}

	r.mu.Lock()
	// Re-check in case another goroutine built it while we were loading.
	if existing, ok := r.byApp[id]; ok {
		r.mu.Unlock()
		return existing
	}
	r.byApp[id] = c
	r.mu.Unlock()
	return c
}

// ForGuild returns the client that serves a guild: its custom bot when one is
// enabled, else the platform client.
func (r *Registry) ForGuild(ctx context.Context, guildID int64) *discord.Client {
	if guildID == 0 || !r.box.Enabled() {
		return r.platform
	}
	row, ok, err := r.store.CustomBots.Get(ctx, guildID)
	if err != nil || !ok || !row.Enabled {
		return r.platform
	}
	return r.ForApp(ctx, strconv.FormatInt(row.ApplicationID, 10))
}

// Invalidate drops the cached client for an application (call after its token
// changes or the bot is removed).
func (r *Registry) Invalidate(appID int64) {
	r.mu.Lock()
	delete(r.byApp, appID)
	r.mu.Unlock()
}
