package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/store"
	"github.com/gin-gonic/gin"
)

// ── Command runs (history + per-step timeline) ──────────────────────────────

func (s *Server) handleListCommandRuns(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	var cmdID int64
	if v := c.Query("command_id"); v != "" {
		cmdID, _ = strconv.ParseInt(v, 10, 64)
	}
	limit := 25
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	rows, err := s.store.CommandRuns.ListByGuild(c.Request.Context(), gidInt, cmdID, limit)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load runs")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{
			"id":              r.ID,
			"command_id":      r.CommandID,
			"command_version": r.CommandVersion,
			"invoker_id":      event.FormatID(r.InvokerID),
			"channel_id":      event.FormatID(r.ChannelID),
			"trigger_kind":    r.TriggerKind,
			"status":          r.Status,
			"started_at":      r.StartedAt,
			"completed_at":    r.CompletedAt,
			"error":           r.Error,
		})
	}
	c.JSON(http.StatusOK, gin.H{"runs": out})
}

func (s *Server) handleGetCommandRun(c *gin.Context) {
	id := c.Param("rid")
	if id == "" {
		fail(c, http.StatusBadRequest, "run id required")
		return
	}
	run, err := s.store.CommandRuns.Get(c.Request.Context(), id)
	if err != nil {
		fail(c, http.StatusNotFound, "not found")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	if run.GuildID != gidInt {
		fail(c, http.StatusNotFound, "not found")
		return
	}
	logs, _ := s.store.CommandRuns.ListLogs(c.Request.Context(), run.ID)
	logOut := make([]gin.H, 0, len(logs))
	for _, l := range logs {
		logOut = append(logOut, gin.H{
			"id":          l.ID,
			"step_id":     l.StepID,
			"step_kind":   l.StepKind,
			"cursor_path": l.CursorPath,
			"started_at":  l.StartedAt,
			"duration_ms": l.DurationMs,
			"status":      l.Status,
			"input":       jsonRaw(l.Input),
			"output":      jsonRaw(l.Output),
			"error":       l.Error,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"run": gin.H{
			"id":              run.ID,
			"command_id":      run.CommandID,
			"command_version": run.CommandVersion,
			"invoker_id":      event.FormatID(run.InvokerID),
			"channel_id":      event.FormatID(run.ChannelID),
			"trigger_kind":    run.TriggerKind,
			"status":          run.Status,
			"started_at":      run.StartedAt,
			"completed_at":    run.CompletedAt,
			"resume_at":       run.ResumeAt,
			"error":           run.Error,
		},
		"logs": logOut,
	})
}

// ── Image templates (Card Studio layouts referenced by image_render) ────────

func (s *Server) handleListImageTemplates(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	rows, err := s.store.ImageTemplates.List(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load templates")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, t := range rows {
		out = append(out, gin.H{
			"id":          t.ID,
			"name":        t.Name,
			"description": t.Description,
			"layout":      jsonRaw(t.Layout),
			"updated_at":  t.UpdatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"templates": out})
}

type upsertImageTemplateReq struct {
	ID          int64           `json:"id,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Layout      json.RawMessage `json:"layout"`
}

func (s *Server) handleUpsertImageTemplate(c *gin.Context) {
	var req upsertImageTemplateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name == "" {
		fail(c, http.StatusBadRequest, "name required")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	saved, err := s.store.ImageTemplates.Upsert(c.Request.Context(), store.CommandImageTemplate{
		ID: req.ID, GuildID: gidInt,
		Name: req.Name, Description: req.Description,
		Layout: req.Layout,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not save template")
		return
	}
	s.audit(c, gidInt, "ccmd_template.upsert", gin.H{"id": saved.ID, "name": saved.Name})
	c.JSON(http.StatusOK, gin.H{"id": saved.ID})
}

func (s *Server) handleDeleteImageTemplate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("tid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	if err := s.store.ImageTemplates.Delete(c.Request.Context(), gidInt, id); err != nil {
		fail(c, http.StatusInternalServerError, "could not delete template")
		return
	}
	s.audit(c, gidInt, "ccmd_template.delete", gin.H{"id": id})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func jsonRaw(r json.RawMessage) any {
	if len(r) == 0 {
		return nil
	}
	return r
}
