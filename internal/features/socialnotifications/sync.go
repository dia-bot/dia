package socialnotifications

import (
	"context"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/social"
)

const (
	syncEvery       = 5 * time.Minute
	websubRenewEach = 6 * time.Hour // leases last ~10 days; renew comfortably inside that
	seenRetention   = 90 * 24 * time.Hour
)

// syncLoop reconciles upstream push subscriptions with the database: Twitch
// EventSub and Kick webhooks are created for every followed account and torn
// down for orphans, YouTube WebSub leases are renewed, and the seen-item
// ledger is pruned. The API makes a best-effort subscribe at CRUD time; this
// loop is the retrying safety net that also heals revocations.
func (p *Plugin) syncLoop(ctx context.Context, d plugin.Deps) {
	var lastRenew time.Time
	run := func() {
		sctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if p.clients.Twitch != nil {
			p.syncTwitch(sctx, d)
		}
		if p.clients.Kick != nil {
			p.syncKick(sctx, d)
		}
		if p.clients.YouTube != nil && time.Since(lastRenew) >= websubRenewEach {
			lastRenew = time.Now()
			p.renewYouTube(sctx, d)
			_ = d.Store.Social.PruneSeen(sctx, seenRetention)
		}
	}
	run()
	t := time.NewTicker(syncEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

// twitchEventTypes are the EventSub subscriptions kept per broadcaster.
var twitchEventTypes = []string{"stream.online", "stream.offline"}

func (p *Plugin) syncTwitch(ctx context.Context, d plugin.Deps) {
	tw := p.clients.Twitch
	subs, err := d.Store.Social.ListEnabledByProvider(ctx, social.ProviderTwitch)
	if err != nil {
		d.Log.Warn("social: list twitch subscriptions", "err", err)
		return
	}
	needed := map[string]bool{}
	for _, s := range subs {
		needed[s.AccountID] = true
	}

	existing, err := tw.ListSubscriptions(ctx)
	if err != nil {
		d.Log.Warn("social: list eventsub subscriptions", "err", err)
		return
	}
	have := map[string]bool{} // "type:broadcaster" with a healthy subscription
	for _, e := range existing {
		if e.Transport.Callback != tw.Callback() {
			continue // another deployment's subscription on the same app
		}
		mine := false
		for _, typ := range twitchEventTypes {
			if e.Type == typ {
				mine = true
			}
		}
		if !mine {
			continue
		}
		healthy := e.Status == "enabled" || e.Status == "webhook_callback_verification_pending"
		if healthy && needed[e.Condition.BroadcasterUserID] {
			have[e.Type+":"+e.Condition.BroadcasterUserID] = true
			continue
		}
		// Orphaned (nobody follows the account anymore) or dead (revoked,
		// failed verification): remove so it can be recreated cleanly.
		if err := tw.Unsubscribe(ctx, e.ID); err != nil {
			d.Log.Warn("social: eventsub unsubscribe", "id", e.ID, "err", err)
		}
	}

	for account := range needed {
		failed := ""
		for _, typ := range twitchEventTypes {
			if have[typ+":"+account] {
				continue
			}
			if err := tw.Subscribe(ctx, typ, account); err != nil && !social.IsConflict(err) {
				failed = err.Error()
				d.Log.Warn("social: eventsub subscribe", "type", typ, "broadcaster", account, "err", err)
			}
		}
		if failed != "" {
			_ = d.Store.Social.SetHookStatus(ctx, social.ProviderTwitch, account, "error", truncate(failed, 300))
		} else {
			_ = d.Store.Social.SetHookStatus(ctx, social.ProviderTwitch, account, "active", "")
		}
	}
}

func (p *Plugin) syncKick(ctx context.Context, d plugin.Deps) {
	kick := p.clients.Kick
	subs, err := d.Store.Social.ListEnabledByProvider(ctx, social.ProviderKick)
	if err != nil {
		d.Log.Warn("social: list kick subscriptions", "err", err)
		return
	}
	needed := map[string]bool{}
	for _, s := range subs {
		needed[s.AccountID] = true
	}

	existing, err := kick.ListSubscriptions(ctx)
	if err != nil {
		d.Log.Warn("social: list kick event subscriptions", "err", err)
		return
	}
	have := map[string]bool{}
	var orphans []string
	for _, e := range existing {
		if !strings.HasPrefix(e.Event, "livestream.status") {
			continue
		}
		id := social.FormatKickID(e.BroadcasterUserID)
		if needed[id] {
			have[id] = true
		} else {
			orphans = append(orphans, e.ID)
		}
	}
	if err := kick.Unsubscribe(ctx, orphans); err != nil {
		d.Log.Warn("social: kick unsubscribe orphans", "err", err)
	}

	for _, s := range subs {
		if have[s.AccountID] {
			_ = d.Store.Social.SetHookStatus(ctx, social.ProviderKick, s.AccountID, "active", "")
			continue
		}
		bid, ok := event.ParseID(s.AccountID)
		if !ok {
			continue
		}
		if err := kick.Subscribe(ctx, bid); err != nil {
			d.Log.Warn("social: kick subscribe", "broadcaster", s.AccountID, "err", err)
			_ = d.Store.Social.SetHookStatus(ctx, social.ProviderKick, s.AccountID, "error", truncate(err.Error(), 300))
		} else {
			_ = d.Store.Social.SetHookStatus(ctx, social.ProviderKick, s.AccountID, "active", "")
		}
		have[s.AccountID] = true
	}
}

func (p *Plugin) renewYouTube(ctx context.Context, d plugin.Deps) {
	yt := p.clients.YouTube
	subs, err := d.Store.Social.ListEnabledByProvider(ctx, social.ProviderYouTube)
	if err != nil {
		d.Log.Warn("social: list youtube subscriptions", "err", err)
		return
	}
	renewed := map[string]bool{}
	for _, s := range subs {
		if renewed[s.AccountID] {
			continue
		}
		renewed[s.AccountID] = true
		if err := yt.Subscribe(ctx, s.AccountID); err != nil {
			d.Log.Warn("social: websub renew", "channel", s.AccountID, "err", err)
			_ = d.Store.Social.SetHookStatus(ctx, social.ProviderYouTube, s.AccountID, "error", truncate(err.Error(), 300))
		} else {
			_ = d.Store.Social.SetHookStatus(ctx, social.ProviderYouTube, s.AccountID, "active", "")
		}
	}
}
