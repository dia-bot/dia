package socialnotifications

import (
	"context"
	"math/rand"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/social"
	"github.com/dia-bot/dia/internal/store"
)

const (
	pollTick        = 30 * time.Second
	freePollEvery   = 5 * time.Minute
	premiumPollTick = time.Minute
	// maxAnnouncePerPoll caps how many new items one poll may announce, so a
	// feed that dumps its whole history at once can't flood a channel.
	maxAnnouncePerPoll = 3
)

// pollLoop drives the keyless providers (RSS, Bluesky): each enabled
// subscription is fetched on its cadence (premium guilds poll faster), new
// items are deduped against the seen ledger and published as SOCIAL_UPDATE.
func (p *Plugin) pollLoop(ctx context.Context, d plugin.Deps) {
	next := map[int64]time.Time{} // subscription id → next poll
	t := time.NewTicker(pollTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.pollDue(ctx, d, next)
		}
	}
}

func (p *Plugin) pollDue(ctx context.Context, d plugin.Deps, next map[int64]time.Time) {
	now := time.Now()
	premiumByGuild := map[int64]bool{} // cached per sweep
	live := map[int64]bool{}           // ids seen this sweep (prunes deleted subs from next)

	for _, provider := range []string{social.ProviderRSS, social.ProviderBluesky} {
		subs, err := d.Store.Social.ListEnabledByProvider(ctx, provider)
		if err != nil {
			d.Log.Warn("social: list poll subscriptions", "provider", provider, "err", err)
			continue
		}
		for _, sub := range subs {
			live[sub.ID] = true
			if due, ok := next[sub.ID]; ok && now.Before(due) {
				continue
			}
			interval := freePollEvery
			prem, ok := premiumByGuild[sub.GuildID]
			if !ok {
				prem = p.isPremium(ctx, d, sub.GuildID)
				premiumByGuild[sub.GuildID] = prem
			}
			if prem {
				interval = premiumPollTick
			}
			// ±10% jitter spreads polls so they never bunch on one tick.
			jitter := time.Duration(rand.Int63n(int64(interval) / 5))
			next[sub.ID] = now.Add(interval - interval/10 + jitter)

			pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
			p.pollOne(pctx, d, sub)
			cancel()
		}
	}
	for id := range next {
		if !live[id] {
			delete(next, id)
		}
	}
}

func (p *Plugin) pollOne(ctx context.Context, d plugin.Deps, sub store.SocialSubscription) {
	switch sub.Provider {
	case social.ProviderRSS:
		p.pollRSS(ctx, d, sub)
	case social.ProviderBluesky:
		p.pollBluesky(ctx, d, sub)
	}
}

func (p *Plugin) pollRSS(ctx context.Context, d plugin.Deps, sub store.SocialSubscription) {
	feed, etag, lastMod, notModified, err := p.clients.RSS.Fetch(ctx, sub.AccountID, sub.ETag, sub.LastModified)
	if err != nil {
		d.Log.Warn("social: rss poll failed", "feed", sub.AccountID, "err", err)
		return
	}
	if notModified {
		return
	}
	if etag != sub.ETag || lastMod != sub.LastModified {
		_ = d.Store.Social.SetPollState(ctx, sub.ID, etag, lastMod)
	}
	// Feeds list newest first; collect the unseen ones and announce oldest
	// first so a burst reads chronologically.
	var fresh []social.FeedItem
	for _, item := range feed.Items {
		newlySeen, err := d.Store.Social.MarkSeen(ctx, sub.ID, item.ID)
		if err != nil || !newlySeen {
			continue
		}
		if len(fresh) < maxAnnouncePerPoll {
			fresh = append(fresh, item)
		}
	}
	for i := len(fresh) - 1; i >= 0; i-- {
		item := fresh[i]
		Publish(ctx, d.Bus, d.Log, event.SocialUpdate{
			GuildID:        event.FormatID(sub.GuildID),
			SubscriptionID: sub.ID,
			Provider:       sub.Provider,
			Kind:           social.KindNewPost,
			AccountID:      sub.AccountID,
			AccountName:    sub.AccountName,
			AccountURL:     sub.AccountURL,
			ItemID:         item.ID,
			Title:          item.Title,
			URL:            item.Link,
		})
	}
}

func (p *Plugin) pollBluesky(ctx context.Context, d plugin.Deps, sub store.SocialSubscription) {
	posts, err := p.clients.Bluesky.AuthorFeed(ctx, sub.AccountID, 10)
	if err != nil {
		d.Log.Warn("social: bluesky poll failed", "actor", sub.AccountID, "err", err)
		return
	}
	var fresh []social.BlueskyPost
	for _, post := range posts { // newest first
		newlySeen, err := d.Store.Social.MarkSeen(ctx, sub.ID, post.URI)
		if err != nil || !newlySeen {
			continue
		}
		if len(fresh) < maxAnnouncePerPoll {
			fresh = append(fresh, post)
		}
	}
	for i := len(fresh) - 1; i >= 0; i-- {
		post := fresh[i]
		handle := post.Handle
		if handle == "" {
			handle = sub.AccountName
		}
		Publish(ctx, d.Bus, d.Log, event.SocialUpdate{
			GuildID:        event.FormatID(sub.GuildID),
			SubscriptionID: sub.ID,
			Provider:       sub.Provider,
			Kind:           social.KindNewPost,
			AccountID:      sub.AccountID,
			AccountName:    sub.AccountName,
			AccountURL:     sub.AccountURL,
			ItemID:         post.URI,
			Title:          truncate(post.Text, 180),
			Description:    post.Text,
			URL:            social.PostURL(handle, post.URI),
		})
	}
}

// isPremium mirrors the API's entitlement check: the env allowlist stub OR an
// active Stripe subscription.
func (p *Plugin) isPremium(ctx context.Context, d plugin.Deps, guildID int64) bool {
	if d.Config.IsPremiumGuild(event.FormatID(guildID)) {
		return true
	}
	sub, found, err := d.Store.Subscriptions.Get(ctx, guildID)
	return err == nil && found && sub.Active(time.Now())
}
