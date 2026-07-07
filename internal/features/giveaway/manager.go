package giveaway

import (
	"context"
	"errors"

	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
)

// Manager exposes the giveaway lifecycle actions (end, reroll, cancel) to
// callers outside the worker, chiefly the dashboard API. It reuses the exact
// same draw / announce / publish path the slash commands and the sweeper use, so
// a dashboard action behaves identically to `/giveaway end` in Discord.
type Manager struct{ p *Plugin }

// NewManager builds a Manager over the given shared deps.
func NewManager(d plugin.Deps) *Manager { return &Manager{p: &Plugin{deps: d}} }

// Errors surfaced to API callers for a clean 4xx.
var (
	ErrNotRunning     = errors.New("giveaway is not running")
	ErrNotEnded       = errors.New("giveaway has not ended")
	ErrNotCancellable = errors.New("giveaway can't be cancelled")
)

// End ends a running giveaway now, drawing + announcing winners. The context is
// detached (WithoutCancel) so a dashboard client aborting the HTTP request mid
// flight can't strand a claimed giveaway with its draw/announce half-done — once
// started, the whole draw→claim→announce→persist runs to completion.
func (m *Manager) End(ctx context.Context, guildID int64, id string) (store.Giveaway, error) {
	ctx = context.WithoutCancel(ctx)
	g, err := m.p.deps.Store.Giveaways.Get(ctx, guildID, id)
	if err != nil {
		return store.Giveaway{}, err
	}
	if g.Status != "running" {
		return store.Giveaway{}, ErrNotRunning
	}
	if !m.p.finishGiveaway(ctx, m.loadCfg(ctx, guildID), g) {
		return store.Giveaway{}, ErrNotRunning
	}
	return g, nil
}

// Reroll draws replacement winners for an already-ended giveaway (no-op if the
// eligible pool is exhausted). Detached context, as with End.
func (m *Manager) Reroll(ctx context.Context, guildID int64, id string, count int) ([]int64, error) {
	ctx = context.WithoutCancel(ctx)
	g, err := m.p.deps.Store.Giveaways.Get(ctx, guildID, id)
	if err != nil {
		return nil, err
	}
	if g.Status != "ended" {
		return nil, ErrNotEnded
	}
	return m.p.rerollGiveaway(ctx, m.loadCfg(ctx, guildID), g, count), nil
}

// Cancel cancels a running/scheduled giveaway with no draw. Detached context.
func (m *Manager) Cancel(ctx context.Context, guildID int64, id string) (store.Giveaway, error) {
	ctx = context.WithoutCancel(ctx)
	cancelled, ok, err := m.p.deps.Store.Giveaways.Cancel(ctx, guildID, id)
	if err != nil {
		return store.Giveaway{}, err
	}
	if !ok {
		return store.Giveaway{}, ErrNotCancellable
	}
	m.p.markCancelled(ctx, m.loadCfg(ctx, guildID), cancelled)
	return cancelled, nil
}

func (m *Manager) loadCfg(ctx context.Context, guildID int64) Config {
	cfg, _, _ := plugin.LoadConfig[Config](ctx, m.p.deps, guildID, FeatureKey)
	return cfg
}
