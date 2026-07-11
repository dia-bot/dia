package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/automations"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/giveaway"
	"github.com/dia-bot/dia/internal/features/leveling"
	"github.com/dia-bot/dia/internal/features/moderation"
	"github.com/dia-bot/dia/internal/features/roles"
	"github.com/dia-bot/dia/internal/features/welcome"
	"github.com/dia-bot/dia/internal/store"
	"github.com/gin-gonic/gin"
)

// ── Automations CRUD ─────────────────────────────────────────

func (s *Server) handleListAutomations(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	rows, err := s.store.Automations.List(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load automations")
		return
	}
	stats, err := s.store.AutomationRuns.GuildRunStats(c.Request.Context(), gidInt)
	if err != nil {
		s.log.Warn("automation run stats failed", "guild", gidInt, "err", err)
		stats = nil
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		h := summarizeAutomation(r)
		addShapeFields(h, r.Definition)
		if st, ok := stats[r.ID]; ok {
			h["runs_24h"] = st.Runs24h
			h["last_run_at"] = st.LastRunAt
		} else if stats != nil {
			h["runs_24h"] = 0
			h["last_run_at"] = nil
		}
		out = append(out, h)
	}
	c.JSON(http.StatusOK, gin.H{
		"automations": out,
		"builtins":    s.builtinSummaries(c, gidInt),
	})
}

func (s *Server) handleGetAutomation(c *gin.Context) {
	id := c.Param("aid")
	if id == "" {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	// Built-in keys (e.g. "welcome.join") resolve to a generated read-only flow.
	if b, ok := s.findBuiltin(c, gidInt, id); ok {
		c.JSON(http.StatusOK, builtinFull(b))
		return
	}
	a, err := s.store.Automations.Get(c.Request.Context(), gidInt, id)
	if err != nil {
		fail(c, http.StatusNotFound, "not found")
		return
	}
	c.JSON(http.StatusOK, fullAutomation(a))
}

type upsertAutomationReq struct {
	ID            string          `json:"id,omitempty"`
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Enabled       bool            `json:"enabled"`
	Status        string          `json:"status,omitempty"`
	TriggerType   string          `json:"trigger_type"`
	TriggerConfig json.RawMessage `json:"trigger_config"`
	Definition    json.RawMessage `json:"definition"`
}

func (s *Server) handleUpsertAutomation(c *gin.Context) {
	var req upsertAutomationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Description == "" {
		req.Description = "Automation"
	}
	def, ok := decodeDefinition(c, req.Definition)
	if !ok {
		return
	}
	tcfg := automations.DecodeTriggerConfig(req.TriggerConfig)
	result := automations.Validate(req.Name, req.TriggerType, tcfg, def)
	if !result.OK {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation failed", "validation": result})
		return
	}
	evType, _ := automations.EventForTrigger(req.TriggerType)

	gidInt, _ := event.ParseID(guildID(c))
	row := store.Automation{
		ID:            req.ID,
		GuildID:       gidInt,
		Name:          strings.TrimSpace(req.Name),
		Description:   req.Description,
		Enabled:       req.Enabled,
		Status:        firstNonEmpty(req.Status, string(automations.StatusDraft)),
		Version:       1,
		TriggerType:   req.TriggerType,
		EventType:     string(evType),
		TriggerConfig: ensureJSON(req.TriggerConfig),
		Definition:    ensureJSON(req.Definition),
	}
	if req.ID != "" {
		if existing, err := s.store.Automations.Get(c.Request.Context(), gidInt, req.ID); err == nil {
			row.Version = existing.Version
			if row.Status == string(automations.StatusPublished) && existing.Status != string(automations.StatusPublished) {
				row.Version = existing.Version + 1
			}
		}
	}
	saved, err := s.store.Automations.Upsert(c.Request.Context(), row)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not save automation")
		return
	}
	if saved.Status == string(automations.StatusPublished) {
		if err := s.store.Automations.PublishVersion(c.Request.Context(), store.AutomationVersion{
			AutomationID:  saved.ID,
			Version:       saved.Version,
			Definition:    saved.Definition,
			TriggerType:   saved.TriggerType,
			TriggerConfig: saved.TriggerConfig,
		}); err != nil {
			s.log.Warn("automation publish version", "err", err)
		}
	}
	s.audit(c, gidInt, "automation.upsert", gin.H{"id": saved.ID, "name": saved.Name, "trigger": saved.TriggerType, "status": saved.Status})
	c.JSON(http.StatusOK, gin.H{"id": saved.ID, "validation": result})
}

func (s *Server) handleValidateAutomation(c *gin.Context) {
	var req upsertAutomationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	def, ok := decodeDefinition(c, req.Definition)
	if !ok {
		return
	}
	tcfg := automations.DecodeTriggerConfig(req.TriggerConfig)
	c.JSON(http.StatusOK, gin.H{"validation": automations.Validate(req.Name, req.TriggerType, tcfg, def)})
}

func (s *Server) handleDeleteAutomation(c *gin.Context) {
	id := c.Param("aid")
	if id == "" {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	if err := s.store.Automations.Delete(c.Request.Context(), gidInt, id); err != nil {
		fail(c, http.StatusInternalServerError, "could not delete automation")
		return
	}
	s.audit(c, gidInt, "automation.delete", gin.H{"id": id})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── Trigger catalogue ────────────────────────────────────────

func (s *Server) handleListTriggers(c *gin.Context) {
	out := make([]gin.H, 0, len(automations.Triggers))
	for _, t := range automations.Triggers {
		filters := make([]string, 0, len(t.Filters))
		for _, f := range t.Filters {
			filters = append(filters, string(f))
		}
		out = append(out, gin.H{
			"key":         t.Key,
			"label":       t.Label,
			"description": t.Description,
			"category":    t.Category,
			"event":       string(t.Event),
			"actor":       t.Actor,
			"has_channel": t.HasChannel,
			"filters":     filters,
		})
	}
	c.JSON(http.StatusOK, gin.H{"triggers": out})
}

// ── Runs (history + per-step timeline) ───────────────────────

func (s *Server) handleListAutomationRuns(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	autoID := c.Query("automation_id")
	limit := 25
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	rows, err := s.store.AutomationRuns.ListByGuild(c.Request.Context(), gidInt, autoID, limit)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load runs")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{
			"id":            r.ID,
			"automation_id": r.AutomationID,
			"version":       r.AutomationVersion,
			"actor_id":      event.FormatID(r.InvokerID),
			"channel_id":    event.FormatID(r.ChannelID),
			"trigger_kind":  r.TriggerKind,
			"status":        r.Status,
			"started_at":    r.StartedAt,
			"completed_at":  r.CompletedAt,
			"error":         r.Error,
		})
	}
	c.JSON(http.StatusOK, gin.H{"runs": out})
}

func (s *Server) handleGetAutomationRun(c *gin.Context) {
	id := c.Param("rid")
	if id == "" {
		fail(c, http.StatusBadRequest, "run id required")
		return
	}
	run, err := s.store.AutomationRuns.Get(c.Request.Context(), id)
	if err != nil {
		fail(c, http.StatusNotFound, "not found")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	if run.GuildID != gidInt {
		fail(c, http.StatusNotFound, "not found")
		return
	}
	logs, _ := s.store.AutomationRuns.ListLogs(c.Request.Context(), run.ID)
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
			"id":            run.ID,
			"automation_id": run.AutomationID,
			"version":       run.AutomationVersion,
			"actor_id":      event.FormatID(run.InvokerID),
			"channel_id":    event.FormatID(run.ChannelID),
			"trigger_kind":  run.TriggerKind,
			"status":        run.Status,
			"started_at":    run.StartedAt,
			"completed_at":  run.CompletedAt,
			"resume_at":     run.ResumeAt,
			"error":         run.Error,
		},
		"logs": logOut,
	})
}

// ── Helpers ──────────────────────────────────────────────────

func summarizeAutomation(r store.Automation) gin.H {
	return gin.H{
		"id":             r.ID,
		"name":           r.Name,
		"description":    r.Description,
		"enabled":        r.Enabled,
		"status":         r.Status,
		"version":        r.Version,
		"trigger_type":   r.TriggerType,
		"trigger_config": jsonRaw(r.TriggerConfig),
		"updated_at":     r.UpdatedAt,
	}
}

func fullAutomation(r store.Automation) gin.H {
	out := summarizeAutomation(r)
	out["definition"] = json.RawMessage(r.Definition)
	out["builtin"] = false
	return out
}

// decodeDefinition unmarshals a definition body, writing a 400 on failure.
func decodeDefinition(c *gin.Context, raw json.RawMessage) (cc.Definition, bool) {
	var def cc.Definition
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &def); err != nil {
			fail(c, http.StatusBadRequest, "invalid definition json")
			return def, false
		}
	}
	return def, true
}

func ensureJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}

// ── Built-ins (read-only, owned by managed features) ─────────

// builtinList builds the guild's built-in automations from the owning features'
// live configs.
func (s *Server) builtinList(c *gin.Context, gidInt int64) []automations.Builtin {
	configs := map[string]json.RawMessage{}
	enabled := map[string]bool{}
	// Load every feature that owns a built-in so its flow renders from the live
	// config (not defaults): welcome, leveling, auto-roles, reaction roles and
	// automod.
	for _, key := range []string{welcome.FeatureKey, leveling.FeatureKey, roles.FeatureKey, roles.ReactionRolesKey, moderation.AutomodKey, giveaway.FeatureKey} {
		if fc, err := s.store.Features.Get(c.Request.Context(), gidInt, key); err == nil {
			configs[key] = fc.Config
			enabled[key] = fc.Enabled
		}
	}
	// Each reaction-role menu contributes its own built-in; a load failure just
	// drops those entries (the rest of the list still serves).
	menus, err := s.store.ReactionRoles.List(c.Request.Context(), gidInt)
	if err != nil {
		s.log.Warn("builtin list: load reaction-role menus failed", "guild", gidInt, "err", err)
	}
	return automations.BuildBuiltins(configs, enabled, menus)
}

func (s *Server) builtinSummaries(c *gin.Context, gidInt int64) []gin.H {
	bs := s.builtinList(c, gidInt)
	out := make([]gin.H, 0, len(bs))
	for _, b := range bs {
		h := builtinSummary(b)
		raw, _ := json.Marshal(b.Definition)
		addShapeFields(h, raw)
		out = append(out, h)
	}
	return out
}

func (s *Server) findBuiltin(c *gin.Context, gidInt int64, key string) (automations.Builtin, bool) {
	if !strings.Contains(key, ".") {
		return automations.Builtin{}, false
	}
	for _, b := range s.builtinList(c, gidInt) {
		if b.Key == key {
			return b, true
		}
	}
	return automations.Builtin{}, false
}

func builtinSummary(b automations.Builtin) gin.H {
	return gin.H{
		"id":           b.Key,
		"name":         b.Name,
		"description":  b.Description,
		"enabled":      b.Enabled,
		"status":       "published",
		"trigger_type": b.TriggerType,
		"builtin":      true,
		"feature_key":  b.FeatureKey,
		"feature_name": b.FeatureName,
		"feature_tab":  b.FeatureTab,
	}
}

func builtinFull(b automations.Builtin) gin.H {
	out := builtinSummary(b)
	raw, _ := json.Marshal(b.Definition)
	out["definition"] = json.RawMessage(raw)
	return out
}
