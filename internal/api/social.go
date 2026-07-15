package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	sn "github.com/dia-bot/dia/internal/features/socialnotifications"
	"github.com/dia-bot/dia/internal/social"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/gin-gonic/gin"
)

var errUnknownProvider = errors.New("unknown provider")

// socialSubJSON shapes one subscription for the dashboard (snowflakes as
// strings, per the event contract).
func socialSubJSON(s store.SocialSubscription) gin.H {
	ping := ""
	if s.PingRoleID != 0 {
		ping = event.FormatID(s.PingRoleID)
	}
	return gin.H{
		"id":           strconv.FormatInt(s.ID, 10),
		"provider":     s.Provider,
		"account_id":   s.AccountID,
		"account_name": s.AccountName,
		"account_url":  s.AccountURL,
		"channel_id":   event.FormatID(s.ChannelID),
		"ping_role_id": ping,
		"template":     s.Template,
		"embed":        s.Embed,
		"enabled":      s.Enabled,
		"live":         s.Live,
		"hook_status":  s.HookStatus,
		"last_error":   s.LastError,
		"created_at":   s.CreatedAt.UnixMilli(),
	}
}

// socialLimit returns the guild's subscription allowance.
func (s *Server) socialLimit(ctx context.Context, gid string) int {
	if s.isPremium(ctx, gid) {
		return sn.PremiumSubscriptionLimit
	}
	return sn.FreeSubscriptionLimit
}

// handleListSocial returns the provider capability catalogue (env-detected;
// locked providers surface as coming_soon) plus the guild's subscriptions.
func (s *Server) handleListSocial(c *gin.Context) {
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)
	subs, err := s.store.Social.ListByGuild(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load subscriptions")
		return
	}
	out := make([]gin.H, 0, len(subs))
	for _, sub := range subs {
		out = append(out, socialSubJSON(sub))
	}
	c.JSON(http.StatusOK, gin.H{
		"capabilities":  social.Capabilities(s.cfg),
		"subscriptions": out,
		"limit":         s.socialLimit(c.Request.Context(), gid),
	})
}

type socialSubReq struct {
	Provider   string `json:"provider"`
	Account    string `json:"account"`
	ChannelID  string `json:"channel_id"`
	PingRoleID string `json:"ping_role_id"`
	Template   string `json:"template"`
	Embed      *bool  `json:"embed"`
	Enabled    *bool  `json:"enabled"`
}

// handleCreateSocial validates and resolves the account with the provider's
// API, stores the subscription, primes the seen ledger (so history is never
// announced) and kicks off the upstream webhook subscription for push
// providers. The sync worker retries anything that fails here.
func (s *Server) handleCreateSocial(c *gin.Context) {
	var req socialSubReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if !social.Available(s.cfg, req.Provider) {
		fail(c, http.StatusBadRequest, "this provider isn't available yet")
		return
	}
	req.Account = strings.TrimSpace(req.Account)
	if req.Account == "" {
		fail(c, http.StatusBadRequest, "account is required")
		return
	}
	chID, ok := event.ParseID(req.ChannelID)
	if !ok || chID == 0 {
		fail(c, http.StatusBadRequest, "pick a channel to announce in")
		return
	}
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)
	ctx := c.Request.Context()

	if n, err := s.store.Social.CountByGuild(ctx, gidInt); err != nil {
		fail(c, http.StatusInternalServerError, "could not check limits")
		return
	} else if limit := s.socialLimit(ctx, gid); n >= limit {
		fail(c, http.StatusForbidden, "subscription limit reached ("+strconv.Itoa(limit)+"); upgrade to follow more accounts")
		return
	}

	sub := store.SocialSubscription{
		GuildID:   gidInt,
		Provider:  req.Provider,
		ChannelID: chID,
		Template:  strings.TrimSpace(req.Template),
		Embed:     req.Embed == nil || *req.Embed,
		Enabled:   req.Enabled == nil || *req.Enabled,
	}
	if req.PingRoleID != "" {
		if rid, ok := event.ParseID(req.PingRoleID); ok {
			sub.PingRoleID = rid
		}
	}

	if err := s.resolveSocialAccount(ctx, &sub, req.Account); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}

	created, err := s.store.Social.Create(ctx, sub)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			fail(c, http.StatusConflict, "this account is already followed in this server")
			return
		}
		fail(c, http.StatusInternalServerError, "could not save subscription")
		return
	}

	// First subscription auto-enables the feature so announcements actually
	// send; an explicit off stays off.
	if _, ferr := s.store.Features.Get(ctx, gidInt, sn.FeatureKey); ferr != nil {
		_ = s.store.Features.Upsert(ctx, gidInt, sn.FeatureKey, true, []byte("{}"))
	}

	s.primeAndHook(created)
	s.audit(c, gidInt, "social.create", gin.H{"provider": created.Provider, "account": created.AccountName})
	c.JSON(http.StatusOK, gin.H{"subscription": socialSubJSON(created)})
}

// resolveSocialAccount turns user input into the canonical account for each
// provider, filling AccountID / AccountName / AccountURL (and the initial live
// state for streams).
func (s *Server) resolveSocialAccount(ctx context.Context, sub *store.SocialSubscription, input string) error {
	switch sub.Provider {
	case social.ProviderTwitch:
		u, err := s.social.Twitch.ResolveUser(ctx, input)
		if err != nil {
			return err
		}
		sub.AccountID, sub.AccountName = u.ID, u.DisplayName
		sub.AccountURL = "https://twitch.tv/" + u.Login
		sub.HookStatus = "pending"
	case social.ProviderKick:
		ch, err := s.social.Kick.ResolveChannel(ctx, input)
		if err != nil {
			return err
		}
		sub.AccountID = social.FormatKickID(ch.BroadcasterUserID)
		sub.AccountName = ch.Slug
		sub.AccountURL = "https://kick.com/" + ch.Slug
		sub.HookStatus = "pending"
	case social.ProviderYouTube:
		ch, err := s.social.YouTube.ResolveChannel(ctx, input)
		if err != nil {
			return err
		}
		sub.AccountID, sub.AccountName = ch.ID, ch.Title
		sub.AccountURL = "https://www.youtube.com/channel/" + ch.ID
		sub.HookStatus = "pending"
	case social.ProviderBluesky:
		handle := strings.ToLower(strings.TrimPrefix(input, "@"))
		did, err := s.social.Bluesky.ResolveHandle(ctx, handle)
		if err != nil {
			return err
		}
		sub.AccountID, sub.AccountName = did, handle
		sub.AccountURL = social.ProfileURL(handle)
	case social.ProviderRSS:
		title, err := s.social.RSS.Validate(ctx, input)
		if err != nil {
			return err
		}
		sub.AccountID, sub.AccountName, sub.AccountURL = input, title, input
	default:
		return errUnknownProvider
	}
	return nil
}

// primeAndHook runs the slow post-create work off the request: mark the
// account's current items as seen (history must not be announced) and register
// the upstream webhook for push providers. Failures are recorded on the row
// and retried by the worker's sync loop.
func (s *Server) primeAndHook(sub store.SocialSubscription) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		switch sub.Provider {
		case social.ProviderTwitch:
			if stream, err := s.social.Twitch.GetStream(ctx, sub.AccountID); err == nil && stream.Live {
				_, _ = s.store.Social.ClaimLive(ctx, sub.ID, true)
			}
			for _, typ := range []string{"stream.online", "stream.offline"} {
				if err := s.social.Twitch.Subscribe(ctx, typ, sub.AccountID); err != nil && !social.IsConflict(err) {
					_ = s.store.Social.SetHookStatus(ctx, sub.Provider, sub.AccountID, "error", err.Error())
					return
				}
			}
			_ = s.store.Social.SetHookStatus(ctx, sub.Provider, sub.AccountID, "active", "")
		case social.ProviderKick:
			if ch, err := s.social.Kick.ResolveChannel(ctx, sub.AccountName); err == nil && ch.Live {
				_, _ = s.store.Social.ClaimLive(ctx, sub.ID, true)
			}
			bid, ok := event.ParseID(sub.AccountID)
			if !ok {
				return
			}
			if err := s.social.Kick.Subscribe(ctx, bid); err != nil {
				_ = s.store.Social.SetHookStatus(ctx, sub.Provider, sub.AccountID, "error", err.Error())
				return
			}
			_ = s.store.Social.SetHookStatus(ctx, sub.Provider, sub.AccountID, "active", "")
		case social.ProviderYouTube:
			if entries, err := s.social.YouTube.RecentEntries(ctx, sub.AccountID); err == nil {
				for _, e := range entries {
					_, _ = s.store.Social.MarkSeen(ctx, sub.ID, e.VideoID)
				}
			}
			if err := s.social.YouTube.Subscribe(ctx, sub.AccountID); err != nil {
				_ = s.store.Social.SetHookStatus(ctx, sub.Provider, sub.AccountID, "error", err.Error())
				return
			}
			_ = s.store.Social.SetHookStatus(ctx, sub.Provider, sub.AccountID, "active", "")
		case social.ProviderBluesky:
			if posts, err := s.social.Bluesky.AuthorFeed(ctx, sub.AccountID, 25); err == nil {
				for _, p := range posts {
					_, _ = s.store.Social.MarkSeen(ctx, sub.ID, p.URI)
				}
			}
		case social.ProviderRSS:
			if feed, etag, lastMod, _, err := s.social.RSS.Fetch(ctx, sub.AccountID, "", ""); err == nil {
				for _, item := range feed.Items {
					_, _ = s.store.Social.MarkSeen(ctx, sub.ID, item.ID)
				}
				_ = s.store.Social.SetPollState(ctx, sub.ID, etag, lastMod)
			}
		}
	}()
}

// handleUpdateSocial saves the editable fields of one subscription.
func (s *Server) handleUpdateSocial(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	sid, err := strconv.ParseInt(c.Param("sid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid subscription id")
		return
	}
	sub, ok, err := s.store.Social.Get(c.Request.Context(), gidInt, sid)
	if err != nil || !ok {
		fail(c, http.StatusNotFound, "subscription not found")
		return
	}
	var req socialSubReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.ChannelID != "" {
		if chID, ok := event.ParseID(req.ChannelID); ok && chID != 0 {
			sub.ChannelID = chID
		}
	}
	sub.PingRoleID = 0
	if req.PingRoleID != "" {
		if rid, ok := event.ParseID(req.PingRoleID); ok {
			sub.PingRoleID = rid
		}
	}
	sub.Template = strings.TrimSpace(req.Template)
	if req.Embed != nil {
		sub.Embed = *req.Embed
	}
	if req.Enabled != nil {
		sub.Enabled = *req.Enabled
	}
	if err := s.store.Social.Update(c.Request.Context(), sub); err != nil {
		fail(c, http.StatusInternalServerError, "could not save subscription")
		return
	}
	s.audit(c, gidInt, "social.update", gin.H{"provider": sub.Provider, "account": sub.AccountName})
	c.JSON(http.StatusOK, gin.H{"subscription": socialSubJSON(sub)})
}

// handleDeleteSocial removes a subscription. Upstream webhooks are torn down
// lazily: YouTube unsubscribes here when no guild follows the channel anymore;
// Twitch/Kick orphans are swept by the worker's reconciler.
func (s *Server) handleDeleteSocial(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	sid, err := strconv.ParseInt(c.Param("sid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid subscription id")
		return
	}
	ctx := c.Request.Context()
	sub, ok, err := s.store.Social.Get(ctx, gidInt, sid)
	if err != nil || !ok {
		fail(c, http.StatusNotFound, "subscription not found")
		return
	}
	if err := s.store.Social.Delete(ctx, gidInt, sid); err != nil {
		fail(c, http.StatusInternalServerError, "could not delete subscription")
		return
	}
	if sub.Provider == social.ProviderYouTube && s.social.YouTube != nil {
		if n, err := s.store.Social.CountByAccount(ctx, sub.Provider, sub.AccountID); err == nil && n == 0 {
			go func() {
				uctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				_ = s.social.YouTube.Unsubscribe(uctx, sub.AccountID)
			}()
		}
	}
	s.audit(c, gidInt, "social.delete", gin.H{"provider": sub.Provider, "account": sub.AccountName})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleTestSocial posts a sample announcement for one subscription to its
// configured channel, using the exact runtime composition.
func (s *Server) handleTestSocial(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	sid, err := strconv.ParseInt(c.Param("sid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid subscription id")
		return
	}
	ctx := c.Request.Context()
	sub, ok, err := s.store.Social.Get(ctx, gidInt, sid)
	if err != nil || !ok {
		fail(c, http.StatusNotFound, "subscription not found")
		return
	}
	upd := sampleUpdate(sub)
	send := sn.BuildAnnouncement(ctx, templating.New(), sub, upd)
	if _, err := s.discord.SendMessage(event.FormatID(sub.ChannelID), send); err != nil {
		fail(c, http.StatusBadGateway, "could not send: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// sampleUpdate fabricates a representative update for the Test button.
func sampleUpdate(sub store.SocialSubscription) event.SocialUpdate {
	upd := event.SocialUpdate{
		GuildID:        event.FormatID(sub.GuildID),
		SubscriptionID: sub.ID,
		Provider:       sub.Provider,
		AccountID:      sub.AccountID,
		AccountName:    sub.AccountName,
		AccountURL:     sub.AccountURL,
		URL:            sub.AccountURL,
	}
	switch sub.Provider {
	case social.ProviderTwitch, social.ProviderKick:
		upd.Kind = social.KindLiveStart
		upd.Title = "Test alert: this is what a live announcement looks like"
		upd.Category = "Just Chatting"
	case social.ProviderYouTube:
		upd.Kind = social.KindNewVideo
		upd.Title = "Test alert: this is what an upload announcement looks like"
	default:
		upd.Kind = social.KindNewPost
		upd.Title = "Test alert: this is what a post announcement looks like"
	}
	return upd
}
