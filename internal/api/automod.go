package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/moderation"
	"github.com/dia-bot/dia/internal/store"
	"github.com/gin-gonic/gin"
)

// handleListInfractions returns the automod heat ledger, optionally filtered to
// one user (?user=<id>). Newest first.
func (s *Server) handleListInfractions(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))

	rows, err := func() ([]store.AutomodInfraction, error) {
		if u := c.Query("user"); u != "" {
			if uid, ok := event.ParseID(u); ok {
				return s.store.Infractions.ListByUser(c.Request.Context(), gidInt, uid, 50)
			}
			return nil, nil
		}
		return s.store.Infractions.ListRecent(c.Request.Context(), gidInt, 50)
	}()
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load infractions")
		return
	}

	out := make([]gin.H, 0, len(rows))
	for _, in := range rows {
		h := gin.H{
			"id":           event.FormatID(in.ID),
			"user_id":      event.FormatID(in.UserID),
			"rule_id":      in.RuleID,
			"rule_name":    in.RuleName,
			"trigger_type": in.TriggerType,
			"points":       in.Points,
			"reason":       in.Reason,
			"channel_id":   nil,
			"created_at":   in.CreatedAt,
			"expires_at":   in.ExpiresAt,
		}
		if in.ChannelID != nil {
			h["channel_id"] = event.FormatID(*in.ChannelID)
		}
		out = append(out, h)
	}
	c.JSON(http.StatusOK, gin.H{"infractions": out})
}

// handleAutomodStats returns the automod overview: hit counts over the last day
// and week, the number of enabled rules, and the recent top offenders.
func (s *Server) handleAutomodStats(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	ctx := c.Request.Context()
	now := time.Now()

	hits24h, err := s.store.Infractions.CountSince(ctx, gidInt, now.Add(-24*time.Hour))
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load stats")
		return
	}
	hits7d, err := s.store.Infractions.CountSince(ctx, gidInt, now.Add(-7*24*time.Hour))
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load stats")
		return
	}

	// Count enabled rules from the stored automod config (best-effort: a missing
	// or unparsable config simply means zero rules).
	rules := 0
	if fc, err := s.store.Features.Get(ctx, gidInt, moderation.AutomodKey); err == nil && len(fc.Config) > 0 {
		var cfg moderation.AutomodConfig
		if json.Unmarshal(fc.Config, &cfg) == nil {
			for _, r := range cfg.Rules {
				if r.Enabled {
					rules++
				}
			}
		}
	}

	offenders, err := s.store.Infractions.TopOffenders(ctx, gidInt, now.Add(-7*24*time.Hour), now, 10)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load stats")
		return
	}
	offOut := make([]gin.H, 0, len(offenders))
	for _, o := range offenders {
		offOut = append(offOut, gin.H{
			"user_id":      event.FormatID(o.UserID),
			"total_points": o.TotalPoints,
			"hits":         o.Hits,
			"last_at":      o.LastAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"hits_24h":  hits24h,
		"hits_7d":   hits7d,
		"rules":     rules,
		"offenders": offOut,
	})
}
