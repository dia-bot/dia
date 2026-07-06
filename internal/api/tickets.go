package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/tickets"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
	"github.com/gin-gonic/gin"
)

// ── Panels ───────────────────────────────────────────────────

func (s *Server) handleListPanels(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	rows, err := s.store.Tickets.ListPanels(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load panels")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, p := range rows {
		out = append(out, gin.H{
			"id":         p.ID,
			"name":       p.Name,
			"style":      p.Style,
			"enabled":    p.Enabled,
			"position":   p.Position,
			"channel_id": event.FormatID(p.ChannelID),
			"message_id": event.FormatID(p.MessageID),
			"config":     json.RawMessage(p.Config),
		})
	}
	c.JSON(http.StatusOK, gin.H{"panels": out})
}

type upsertPanelReq struct {
	ID      string          `json:"id"`
	Name    string          `json:"name"`
	Style   string          `json:"style"`
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config"`
}

func (s *Server) handleUpsertPanel(c *gin.Context) {
	var req upsertPanelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	panel := store.TicketPanel{
		ID:      req.ID,
		GuildID: gidInt,
		Name:    req.Name,
		Style:   req.Style,
		Enabled: req.Enabled,
		Config:  req.Config,
	}
	saved, err := s.store.Tickets.UpsertPanel(c.Request.Context(), panel)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "panel not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not save panel")
		return
	}
	action := "ticket.panel.update"
	if req.ID == "" {
		action = "ticket.panel.create"
	}
	s.audit(c, gidInt, action, gin.H{"id": saved.ID})
	c.JSON(http.StatusOK, gin.H{"id": saved.ID})
}

func (s *Server) handleDeletePanel(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	id := c.Param("pid")
	if err := s.store.Tickets.DeletePanel(c.Request.Context(), gidInt, id); err != nil {
		fail(c, http.StatusInternalServerError, "could not delete panel")
		return
	}
	s.audit(c, gidInt, "ticket.panel.delete", gin.H{"id": id})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type postPanelReq struct {
	ChannelID string `json:"channel_id"`
}

// handlePostPanel publishes a saved panel to a channel, reusing the same build +
// send + record path as the /tickets post command.
func (s *Server) handlePostPanel(c *gin.Context) {
	var req postPanelReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ChannelID == "" {
		fail(c, http.StatusBadRequest, "channel_id is required")
		return
	}
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)
	panelID := c.Param("pid")
	msgID, err := tickets.PostPanel(c.Request.Context(), s.discord, s.store, gid, req.ChannelID, panelID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		fail(c, http.StatusNotFound, "panel not found")
		return
	case errors.Is(err, tickets.ErrPanelNoCategories):
		fail(c, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		failDiscord(c, err, "could not post the panel")
		return
	}
	s.audit(c, gidInt, "ticket.panel.post", gin.H{"panel": panelID, "channel": req.ChannelID})
	c.JSON(http.StatusOK, gin.H{"ok": true, "message_id": msgID})
}

// ── Tickets (queue + detail + force-close) ───────────────────

func ticketJSON(t store.Ticket) gin.H {
	claimedBy := ""
	if t.ClaimedBy != 0 {
		claimedBy = event.FormatID(t.ClaimedBy)
	}
	return gin.H{
		"id":             t.ID,
		"number":         t.Number,
		"panel_id":       t.PanelID,
		"category_id":    t.CategoryID,
		"category_label": t.CategoryLabel,
		"channel_id":     event.FormatID(t.ChannelID),
		"is_thread":      t.IsThread,
		"opener_id":      event.FormatID(t.OpenerID),
		"subject":        t.Subject,
		"status":         t.Status,
		"claimed_by":     claimedBy,
		"opened_at":      t.OpenedAt,
		"closed_at":      t.ClosedAt,
		"rating":         t.Rating,
		"transcript_url": t.TranscriptURL,
	}
}

func (s *Server) handleListTickets(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	status := c.Query("status")
	rows, err := s.store.Tickets.ListTickets(c.Request.Context(), gidInt, status, 100)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load tickets")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, t := range rows {
		out = append(out, ticketJSON(t))
	}
	c.JSON(http.StatusOK, gin.H{"tickets": out})
}

func (s *Server) handleGetTicket(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	tid := c.Param("tid")
	t, err := s.store.Tickets.GetTicket(c.Request.Context(), gidInt, tid)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "ticket not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load ticket")
		return
	}
	events, _ := s.store.Tickets.ListEvents(c.Request.Context(), tid, 100)
	notes, _ := s.store.Tickets.ListNotes(c.Request.Context(), tid)
	evOut := make([]gin.H, 0, len(events))
	for _, e := range events {
		evOut = append(evOut, gin.H{
			"kind": e.Kind, "actor_id": event.FormatID(e.ActorID),
			"data": json.RawMessage(e.Data), "created_at": e.CreatedAt,
		})
	}
	noteOut := make([]gin.H, 0, len(notes))
	for _, n := range notes {
		noteOut = append(noteOut, gin.H{
			"id": n.ID, "author_id": event.FormatID(n.AuthorID), "body": n.Body, "created_at": n.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"ticket": ticketJSON(t), "events": evOut, "notes": noteOut})
}

// handleCloseTicket force-closes a ticket from the dashboard: it marks the row
// closed and, best-effort, locks the channel. The full close flow (transcripts,
// automations, ratings) runs when a ticket is closed inside Discord.
func (s *Server) handleCloseTicket(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	tid := c.Param("tid")
	t, err := s.store.Tickets.GetTicket(c.Request.Context(), gidInt, tid)
	if errors.Is(err, store.ErrNotFound) {
		fail(c, http.StatusNotFound, "ticket not found")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load ticket")
		return
	}
	if t.Status != "open" {
		fail(c, http.StatusBadRequest, "ticket is not open")
		return
	}
	ok, err := s.store.Tickets.CloseTicket(c.Request.Context(), gidInt, tid, 0, "Closed from the dashboard")
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not close ticket")
		return
	}
	if ok && t.ChannelID != 0 && !t.IsThread {
		chID := event.FormatID(t.ChannelID)
		_ = s.discord.SetMemberPermission(chID, event.FormatID(t.OpenerID),
			discordgo.PermissionViewChannel|discordgo.PermissionReadMessageHistory,
			discordgo.PermissionSendMessages, "ticket: closed from dashboard")
		_, _ = s.discord.SendMessage(chID, &discordgo.MessageSend{
			Embeds:          []*discordgo.MessageEmbed{{Title: "Ticket closed", Description: "Closed from the dashboard.", Color: 0xED4245}},
			AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
		})
	}
	s.audit(c, gidInt, "ticket.close", gin.H{"id": tid})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleTicketStats(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	st, err := s.store.Tickets.Stats(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load stats")
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": gin.H{
		"open":                       st.Open,
		"closed":                     st.Closed,
		"total":                      st.Total,
		"opened_7d":                  st.Opened7d,
		"closed_7d":                  st.Closed7d,
		"rated":                      st.Rated,
		"avg_rating":                 st.AvgRating,
		"avg_first_response_seconds": st.AvgFirstResponseS,
		"avg_resolution_seconds":     st.AvgResolutionS,
	}})
}
