package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/dia-bot/dia/internal/event"
	sn "github.com/dia-bot/dia/internal/features/socialnotifications"
	"github.com/dia-bot/dia/internal/social"
	"github.com/gin-gonic/gin"
)

// maxHookBody caps webhook payload reads.
const maxHookBody = 1 << 20 // 1 MiB

// readHookBody drains a webhook request body (bounded).
func readHookBody(c *gin.Context) ([]byte, bool) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxHookBody))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return nil, false
	}
	return body, true
}

// ── Twitch EventSub ─────────────────────────────────────────────────────────

// handleTwitchWebhook receives EventSub deliveries: the callback-verification
// challenge, revocations, and stream.online / stream.offline notifications.
// Notifications fan out to one SOCIAL_UPDATE per following guild; the live
// flag transition dedupes Twitch's at-least-once redeliveries.
func (s *Server) handleTwitchWebhook(c *gin.Context) {
	if s.social.Twitch == nil {
		c.Status(http.StatusNotFound)
		return
	}
	body, ok := readHookBody(c)
	if !ok {
		return
	}
	if !s.social.Twitch.VerifySignature(c.Request.Header, body) {
		c.Status(http.StatusForbidden)
		return
	}

	switch c.GetHeader("Twitch-Eventsub-Message-Type") {
	case "webhook_callback_verification":
		var v struct {
			Challenge string `json:"challenge"`
		}
		if err := json.Unmarshal(body, &v); err != nil || v.Challenge == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		c.String(http.StatusOK, v.Challenge)
		return

	case "revocation":
		var r struct {
			Subscription struct {
				Status    string `json:"status"`
				Condition struct {
					BroadcasterUserID string `json:"broadcaster_user_id"`
				} `json:"condition"`
			} `json:"subscription"`
		}
		if err := json.Unmarshal(body, &r); err == nil && r.Subscription.Condition.BroadcasterUserID != "" {
			_ = s.store.Social.SetHookStatus(c.Request.Context(), social.ProviderTwitch,
				r.Subscription.Condition.BroadcasterUserID, "error", "revoked: "+r.Subscription.Status)
		}
		c.Status(http.StatusNoContent)
		return

	case "notification":
		var n struct {
			Subscription struct {
				Type string `json:"type"`
			} `json:"subscription"`
			Event struct {
				ID                   string `json:"id"` // stream id on stream.online
				BroadcasterUserID    string `json:"broadcaster_user_id"`
				BroadcasterUserLogin string `json:"broadcaster_user_login"`
				BroadcasterUserName  string `json:"broadcaster_user_name"`
				StartedAt            string `json:"started_at"`
			} `json:"event"`
		}
		if err := json.Unmarshal(body, &n); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		// Ack immediately (Twitch requires a fast 2xx); the fan-out runs off
		// the request.
		c.Status(http.StatusNoContent)
		msgID := c.GetHeader("Twitch-Eventsub-Message-Id")
		go s.twitchNotify(n.Subscription.Type, n.Event.ID, n.Event.BroadcasterUserID,
			n.Event.BroadcasterUserLogin, n.Event.BroadcasterUserName, n.Event.StartedAt, msgID)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) twitchNotify(typ, streamID, buid, login, name, startedAt, msgID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	subs, err := s.store.Social.ListEnabledByAccount(ctx, social.ProviderTwitch, buid)
	if err != nil || len(subs) == 0 {
		return
	}
	goingLive := typ == "stream.online"

	var stream social.TwitchStream
	if goingLive {
		stream, _ = s.social.Twitch.GetStream(ctx, buid) // best-effort enrichment
	}
	for _, sub := range subs {
		claimed, err := s.store.Social.ClaimLive(ctx, sub.ID, goingLive)
		if err != nil || !claimed {
			continue // duplicate delivery, or state already matched
		}
		upd := event.SocialUpdate{
			GuildID:        event.FormatID(sub.GuildID),
			SubscriptionID: sub.ID,
			Provider:       social.ProviderTwitch,
			AccountID:      buid,
			AccountName:    name,
			AccountURL:     "https://twitch.tv/" + login,
			URL:            "https://twitch.tv/" + login,
		}
		if goingLive {
			upd.Kind = social.KindLiveStart
			upd.ItemID = streamID
			upd.Title = stream.Title
			upd.Category = stream.Game
			upd.Thumbnail = stream.Thumbnail
			upd.StartedAt = startedAt
		} else {
			upd.Kind = social.KindLiveEnd
			upd.ItemID = msgID
		}
		sn.Publish(ctx, s.bus, s.log, upd)
	}
}

// ── Kick webhooks ───────────────────────────────────────────────────────────

// handleKickWebhook receives Kick event deliveries (livestream.status.updated),
// verified against Kick's published RSA key.
func (s *Server) handleKickWebhook(c *gin.Context) {
	if s.social.Kick == nil {
		c.Status(http.StatusNotFound)
		return
	}
	body, ok := readHookBody(c)
	if !ok {
		return
	}
	if !s.social.Kick.VerifySignature(c.Request.Context(), c.Request.Header, body) {
		c.Status(http.StatusForbidden)
		return
	}
	if c.GetHeader("Kick-Event-Type") != "livestream.status.updated" {
		c.Status(http.StatusOK)
		return
	}
	var n struct {
		Broadcaster struct {
			UserID      int64  `json:"user_id"`
			ChannelSlug string `json:"channel_slug"`
		} `json:"broadcaster"`
		IsLive    bool   `json:"is_live"`
		Title     string `json:"title"`
		StartedAt string `json:"started_at"`
	}
	if err := json.Unmarshal(body, &n); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	c.Status(http.StatusOK)
	msgID := c.GetHeader("Kick-Event-Message-Id")
	go s.kickNotify(n.Broadcaster.UserID, n.Broadcaster.ChannelSlug, n.IsLive, n.Title, n.StartedAt, msgID)
}

func (s *Server) kickNotify(userID int64, slug string, isLive bool, title, startedAt, msgID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	accountID := social.FormatKickID(userID)
	subs, err := s.store.Social.ListEnabledByAccount(ctx, social.ProviderKick, accountID)
	if err != nil {
		return
	}
	for _, sub := range subs {
		claimed, err := s.store.Social.ClaimLive(ctx, sub.ID, isLive)
		if err != nil || !claimed {
			continue
		}
		kind := social.KindLiveEnd
		if isLive {
			kind = social.KindLiveStart
		}
		itemID := startedAt
		if itemID == "" {
			itemID = msgID
		}
		sn.Publish(ctx, s.bus, s.log, event.SocialUpdate{
			GuildID:        event.FormatID(sub.GuildID),
			SubscriptionID: sub.ID,
			Provider:       social.ProviderKick,
			Kind:           kind,
			AccountID:      accountID,
			AccountName:    sub.AccountName,
			AccountURL:     "https://kick.com/" + slug,
			URL:            "https://kick.com/" + slug,
			ItemID:         itemID,
			Title:          title,
			StartedAt:      startedAt,
		})
	}
}

// ── YouTube WebSub ──────────────────────────────────────────────────────────

// handleYouTubeVerify answers the hub's subscribe/unsubscribe verification
// (GET with hub.challenge).
func (s *Server) handleYouTubeVerify(c *gin.Context) {
	if s.social.YouTube == nil {
		c.Status(http.StatusNotFound)
		return
	}
	challenge := c.Query("hub.challenge")
	if challenge == "" {
		c.Status(http.StatusBadRequest)
		return
	}
	c.String(http.StatusOK, challenge)
}

// handleYouTubeNotify receives Atom pushes for a channel's upload feed. Each
// video announces once per following guild (the seen ledger dedupes edits and
// replays); the Data API — when a key is configured — tells uploads apart from
// live broadcasts.
func (s *Server) handleYouTubeNotify(c *gin.Context) {
	if s.social.YouTube == nil {
		c.Status(http.StatusNotFound)
		return
	}
	channelID := c.Query("channel_id")
	body, ok := readHookBody(c)
	if !ok {
		return
	}
	if channelID == "" || !s.social.YouTube.VerifySignature(c.Request.Header, channelID, body) {
		c.Status(http.StatusForbidden)
		return
	}
	entries, err := social.ParseFeed(body)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	c.Status(http.StatusNoContent)
	if len(entries) == 0 {
		return
	}
	go s.youtubeNotify(channelID, entries)
}

func (s *Server) youtubeNotify(channelID string, entries []social.FeedEntry) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	subs, err := s.store.Social.ListEnabledByAccount(ctx, social.ProviderYouTube, channelID)
	if err != nil || len(subs) == 0 {
		return
	}
	for _, entry := range entries {
		kind := social.KindNewVideo
		title := entry.Title
		thumb := ""
		if info, ok, err := s.social.YouTube.VideoInfo(ctx, entry.VideoID); err == nil && ok {
			if info.Live {
				kind = social.KindLiveStart
			}
			if info.Title != "" {
				title = info.Title
			}
			thumb = info.Thumbnail
		}
		for _, sub := range subs {
			newlySeen, err := s.store.Social.MarkSeen(ctx, sub.ID, entry.VideoID)
			if err != nil || !newlySeen {
				continue
			}
			name := sub.AccountName
			if entry.Author != "" {
				name = entry.Author
			}
			sn.Publish(ctx, s.bus, s.log, event.SocialUpdate{
				GuildID:        event.FormatID(sub.GuildID),
				SubscriptionID: sub.ID,
				Provider:       social.ProviderYouTube,
				Kind:           kind,
				AccountID:      channelID,
				AccountName:    name,
				AccountURL:     "https://www.youtube.com/channel/" + channelID,
				ItemID:         entry.VideoID,
				Title:          title,
				URL:            entry.Link,
				Thumbnail:      thumb,
			})
		}
	}
}
