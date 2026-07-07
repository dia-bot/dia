package api

import (
	"errors"
	"net/http"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/giveaway"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/gin-gonic/gin"
)

// giveawayManager builds a giveaway.Manager over the server's shared deps so the
// dashboard's end/reroll/cancel actions reuse the worker's exact draw + announce
// path.
func (s *Server) giveawayManager() *giveaway.Manager {
	return giveaway.NewManager(plugin.Deps{
		Config:     s.cfg,
		Log:        s.log,
		Store:      s.store,
		Cache:      s.cache,
		Discord:    s.discord,
		Imaging:    s.imaging,
		Bus:        s.bus,
		GuildState: s.gstate,
	})
}

// handleListGiveaways returns the guild's giveaways for the dashboard, filtered
// by an optional ?status= (all | active | running | ended | scheduled |
// cancelled), each with its live entry count.
func (s *Server) handleListGiveaways(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	rows, err := s.store.Giveaways.ListByGuild(c.Request.Context(), gidInt, c.Query("status"), 100)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load giveaways")
		return
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	counts, err := s.store.Giveaways.EntryCounts(c.Request.Context(), ids)
	if err != nil {
		s.log.Warn("giveaway entry counts", "guild", gidInt, "err", err)
		counts = map[string]int{}
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, giveawaySummary(r, counts[r.ID]))
	}
	c.JSON(http.StatusOK, gin.H{"giveaways": out})
}

func (s *Server) handleEndGiveaway(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	g, err := s.giveawayManager().End(c.Request.Context(), gidInt, c.Param("gwid"))
	if err != nil {
		giveawayActionError(c, err)
		return
	}
	s.audit(c, gidInt, "giveaway.end", gin.H{"id": g.ID, "prize": g.Prize})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleRerollGiveaway(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	var body struct {
		Winners int `json:"winners"`
	}
	_ = c.ShouldBindJSON(&body)
	winners, err := s.giveawayManager().Reroll(c.Request.Context(), gidInt, c.Param("gwid"), body.Winners)
	if err != nil {
		giveawayActionError(c, err)
		return
	}
	ids := make([]string, len(winners))
	for i, w := range winners {
		ids[i] = event.FormatID(w)
	}
	s.audit(c, gidInt, "giveaway.reroll", gin.H{"id": c.Param("gwid"), "winners": ids})
	c.JSON(http.StatusOK, gin.H{"ok": true, "winners": ids})
}

func (s *Server) handleCancelGiveaway(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	g, err := s.giveawayManager().Cancel(c.Request.Context(), gidInt, c.Param("gwid"))
	if err != nil {
		giveawayActionError(c, err)
		return
	}
	s.audit(c, gidInt, "giveaway.cancel", gin.H{"id": g.ID, "prize": g.Prize})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// giveawaySummary is the JSON shape the dashboard renders for one giveaway.
func giveawaySummary(g store.Giveaway, entryCount int) gin.H {
	winners := make([]string, len(g.WinnerIDs))
	for i, w := range g.WinnerIDs {
		winners[i] = event.FormatID(w)
	}
	return gin.H{
		"id":           g.ID,
		"channel_id":   event.FormatID(g.ChannelID),
		"message_id":   snowflakeOrEmpty(g.MessageID),
		"prize":        g.Prize,
		"description":  g.Description,
		"winner_count": g.WinnerCount,
		"host_id":      snowflakeOrEmpty(g.HostID),
		"status":       g.Status,
		"image_url":    g.ImageURL,
		"color":        g.Color,
		"winners":      winners,
		"entry_count":  entryCount,
		"requirements": jsonRaw(g.Requirements),
		"starts_at":    g.StartsAt,
		"ends_at":      g.EndsAt,
		"ended_at":     g.EndedAt,
		"created_at":   g.CreatedAt,
	}
}

// snowflakeOrEmpty renders a 0 id as "" (not posted / no host).
func snowflakeOrEmpty(id int64) string {
	if id == 0 {
		return ""
	}
	return event.FormatID(id)
}

// giveawayActionError maps a Manager/store error to the right HTTP status.
func giveawayActionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrGiveawayNotFound):
		fail(c, http.StatusNotFound, "giveaway not found")
	case errors.Is(err, giveaway.ErrNotRunning),
		errors.Is(err, giveaway.ErrNotEnded),
		errors.Is(err, giveaway.ErrNotCancellable):
		fail(c, http.StatusBadRequest, err.Error())
	default:
		fail(c, http.StatusInternalServerError, "giveaway action failed")
	}
}
