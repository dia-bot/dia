package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/giveaway"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/gin-gonic/gin"
)

// giveawayManager builds a giveaway.Manager over the server's shared deps so the
// dashboard's create/edit/start/end/reroll/cancel actions reuse the worker's
// exact post + draw + announce path.
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
// by an optional ?status= (all | active | draft | running | ended | scheduled |
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

// handleGetGiveaway returns one giveaway with its full composed spec, for the
// editor.
func (s *Server) handleGetGiveaway(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	g, err := s.giveawayManager().Get(c.Request.Context(), gidInt, c.Param("gwid"))
	if err != nil {
		giveawayActionError(c, err)
		return
	}
	count, _ := s.store.Giveaways.EntryCount(c.Request.Context(), g.ID)
	c.JSON(http.StatusOK, giveawayDetail(g, count))
}

// giveawayCreateBody is the editor's create payload. starts_at / ends_at are the
// (client-computed) window; a draft may omit them.
type giveawayCreateBody struct {
	Name         string          `json:"name"`
	Prize        string          `json:"prize"`
	Description  string          `json:"description"`
	ChannelID    string          `json:"channel_id"`
	WinnerCount  int             `json:"winner_count"`
	ImageURL     string          `json:"image_url"`
	Color        string          `json:"color"`
	Spec         json.RawMessage `json:"spec"`
	Requirements json.RawMessage `json:"requirements"`
	Status       string          `json:"status"` // draft | scheduled | running
	StartsAt     *time.Time      `json:"starts_at"`
	EndsAt       *time.Time      `json:"ends_at"`
}

func (s *Server) handleCreateGiveaway(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	var body giveawayCreateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid giveaway")
		return
	}
	actor := s.actorID(c)
	chID, _ := event.ParseID(body.ChannelID)
	in := giveaway.CreateInput{
		Name:         body.Name,
		Prize:        body.Prize,
		Description:  body.Description,
		ChannelID:    chID,
		WinnerCount:  body.WinnerCount,
		HostID:       actor,
		CreatedBy:    actor,
		ImageURL:     body.ImageURL,
		Color:        body.Color,
		Spec:         decodeGiveawaySpec(body.Spec),
		Requirements: decodeGiveawayReq(body.Requirements),
		Status:       body.Status,
	}
	if body.StartsAt != nil {
		in.StartsAt = *body.StartsAt
	}
	if body.EndsAt != nil {
		in.EndsAt = *body.EndsAt
	}
	g, err := s.giveawayManager().Create(c.Request.Context(), gidInt, in)
	if err != nil {
		giveawayActionError(c, err)
		return
	}
	s.audit(c, gidInt, "giveaway.create", gin.H{"id": g.ID, "prize": g.Prize, "status": g.Status})
	c.JSON(http.StatusOK, giveawayDetail(g, 0))
}

// giveawayUpdateBody is a partial edit; only present fields are written.
type giveawayUpdateBody struct {
	Name         *string         `json:"name"`
	Prize        *string         `json:"prize"`
	Description  *string         `json:"description"`
	WinnerCount  *int            `json:"winner_count"`
	ChannelID    *string         `json:"channel_id"`
	ImageURL     *string         `json:"image_url"`
	Color        *string         `json:"color"`
	Spec         json.RawMessage `json:"spec"`
	Requirements json.RawMessage `json:"requirements"`
	StartsAt     *time.Time      `json:"starts_at"`
	EndsAt       *time.Time      `json:"ends_at"`
}

func (s *Server) handleUpdateGiveaway(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	var body giveawayUpdateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		fail(c, http.StatusBadRequest, "invalid giveaway")
		return
	}
	patch := store.GiveawayPatch{
		Name:        body.Name,
		Prize:       body.Prize,
		Description: body.Description,
		WinnerCount: body.WinnerCount,
		ImageURL:    body.ImageURL,
		Color:       body.Color,
		Spec:        body.Spec,
		StartsAt:    body.StartsAt,
		EndsAt:      body.EndsAt,
	}
	if len(body.Requirements) > 0 {
		patch.Requirements = body.Requirements
	}
	if body.ChannelID != nil {
		if id, ok := event.ParseID(*body.ChannelID); ok {
			patch.ChannelID = &id
		}
	}
	g, err := s.giveawayManager().Update(c.Request.Context(), gidInt, c.Param("gwid"), patch)
	if err != nil {
		giveawayActionError(c, err)
		return
	}
	count, _ := s.store.Giveaways.EntryCount(c.Request.Context(), g.ID)
	s.audit(c, gidInt, "giveaway.update", gin.H{"id": g.ID, "prize": g.Prize})
	c.JSON(http.StatusOK, giveawayDetail(g, count))
}

// giveawayStartBody starts a draft now (starts_in_seconds == 0) or schedules it.
type giveawayStartBody struct {
	DurationSeconds int64 `json:"duration_seconds"`
	StartsInSeconds int64 `json:"starts_in_seconds"`
}

func (s *Server) handleStartGiveaway(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	var body giveawayStartBody
	_ = c.ShouldBindJSON(&body)
	g, err := s.giveawayManager().Start(c.Request.Context(), gidInt, c.Param("gwid"),
		time.Duration(body.StartsInSeconds)*time.Second, time.Duration(body.DurationSeconds)*time.Second)
	if err != nil {
		giveawayActionError(c, err)
		return
	}
	s.audit(c, gidInt, "giveaway.start", gin.H{"id": g.ID, "prize": g.Prize, "status": g.Status})
	c.JSON(http.StatusOK, giveawayDetail(g, 0))
}

func (s *Server) handleDeleteGiveaway(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	if err := s.giveawayManager().Delete(c.Request.Context(), gidInt, c.Param("gwid")); err != nil {
		giveawayActionError(c, err)
		return
	}
	s.audit(c, gidInt, "giveaway.delete", gin.H{"id": c.Param("gwid")})
	c.JSON(http.StatusOK, gin.H{"ok": true})
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

// giveawaySummary is the JSON shape the dashboard list renders for one giveaway.
func giveawaySummary(g store.Giveaway, entryCount int) gin.H {
	winners := make([]string, len(g.WinnerIDs))
	for i, w := range g.WinnerIDs {
		winners[i] = event.FormatID(w)
	}
	return gin.H{
		"id":           g.ID,
		"name":         g.Name,
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

// giveawayDetail is the summary plus the full composed spec, for the editor.
func giveawayDetail(g store.Giveaway, entryCount int) gin.H {
	out := giveawaySummary(g, entryCount)
	out["spec"] = jsonRaw(g.Spec)
	return out
}

// snowflakeOrEmpty renders a 0 id as "" (not posted / no host).
func snowflakeOrEmpty(id int64) string {
	if id == 0 {
		return ""
	}
	return event.FormatID(id)
}

// actorID is the acting dashboard user's id (0 when unauthenticated), used as the
// giveaway host / creator.
func (s *Server) actorID(c *gin.Context) int64 {
	if sess := currentSession(c); sess != nil {
		id, _ := event.ParseID(sess.UserID)
		return id
	}
	return 0
}

func decodeGiveawaySpec(raw json.RawMessage) giveaway.Spec {
	var s giveaway.Spec
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &s)
	}
	return s
}

func decodeGiveawayReq(raw json.RawMessage) giveaway.RequirementConfig {
	var r giveaway.RequirementConfig
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &r)
	}
	return r
}

// giveawayActionError maps a Manager/store error to the right HTTP status.
func giveawayActionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrGiveawayNotFound):
		fail(c, http.StatusNotFound, "giveaway not found")
	case errors.Is(err, giveaway.ErrNoPrize),
		errors.Is(err, giveaway.ErrNoChannel),
		errors.Is(err, giveaway.ErrBadDuration),
		errors.Is(err, giveaway.ErrNotRunning),
		errors.Is(err, giveaway.ErrNotEnded),
		errors.Is(err, giveaway.ErrNotCancellable),
		errors.Is(err, giveaway.ErrNotEditable),
		errors.Is(err, giveaway.ErrNotStartable),
		errors.Is(err, giveaway.ErrNotDeletable):
		fail(c, http.StatusBadRequest, err.Error())
	default:
		fail(c, http.StatusInternalServerError, "giveaway action failed")
	}
}
