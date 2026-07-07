package giveaway

import (
	"context"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
)

// runScheduler is the durable-timer worker. Every tick it posts scheduled
// giveaways whose start time arrived and ends running giveaways whose deadline
// passed. Because the deadline lives in Postgres (not an in-memory timer), a
// restart resumes cleanly: giveaways that expired while the worker was down are
// ended on the first sweep.
func (p *Plugin) runScheduler(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	// One immediate sweep so a giveaway that expired during downtime ends
	// promptly rather than after a full interval.
	p.sweep(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.sweep(ctx)
		}
	}
}

func (p *Plugin) sweep(ctx context.Context) {
	now := time.Now()

	// Post scheduled giveaways whose start time has arrived.
	if scheduled, err := p.deps.Store.Giveaways.ListScheduledDue(ctx, now, 25); err == nil {
		for _, g := range scheduled {
			if ctx.Err() != nil {
				return
			}
			p.postScheduled(ctx, g)
		}
	} else {
		p.deps.Log.Warn("giveaway: list scheduled failed", "err", err)
	}

	// End running giveaways past their deadline. finishGiveaway draws first, then
	// atomically claims running→ended, so a concurrent sweep or a restart can't
	// end (and announce) the same giveaway twice.
	due, err := p.deps.Store.Giveaways.ListDue(ctx, now, 25)
	if err != nil {
		p.deps.Log.Warn("giveaway: list due failed", "err", err)
		return
	}
	for _, g := range due {
		if ctx.Err() != nil {
			return
		}
		cfg, _, _ := plugin.LoadConfig[Config](ctx, p.deps, g.GuildID, FeatureKey)
		p.finishGiveaway(ctx, cfg, g)
	}
}

// postScheduled activates a scheduled giveaway. It claims the row atomically
// (scheduled→running) BEFORE posting, so a slow or failed post can never let the
// next sweep re-post the same giveaway; it then posts the message and records the
// message id. If the feature was disabled since scheduling, the giveaway is
// cancelled so its row doesn't linger.
func (p *Plugin) postScheduled(ctx context.Context, g store.Giveaway) {
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, p.deps, g.GuildID, FeatureKey)
	if err != nil {
		return
	}
	if !enabled {
		_, _, _ = p.deps.Store.Giveaways.Cancel(ctx, g.GuildID, g.ID)
		return
	}
	claimed, ok, err := p.deps.Store.Giveaways.ClaimScheduled(ctx, g.ID)
	if err != nil {
		p.deps.Log.Warn("giveaway: claim scheduled failed", "giveaway", g.ID, "err", err)
		return
	}
	if !ok {
		return // another sweep already activated it
	}
	msg, err := p.postGiveaway(ctx, cfg, claimed, 0)
	if err != nil {
		// The giveaway is now running but unposted; the end sweep will still draw
		// from any (unlikely) entries and announce to the channel. Not double-posted.
		p.deps.Log.Warn("giveaway: post scheduled failed", "giveaway", g.ID, "err", err)
		return
	}
	mid, _ := event.ParseID(msg.ID)
	if err := p.deps.Store.Giveaways.SetMessageID(ctx, g.ID, mid); err != nil {
		p.deps.Log.Warn("giveaway: set scheduled message id failed", "giveaway", g.ID, "err", err)
	}
}
