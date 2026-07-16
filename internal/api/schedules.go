package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	sm "github.com/dia-bot/dia/internal/features/schedmessages"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/gin-gonic/gin"
)

// schedJSON shapes one schedule for the dashboard.
func schedJSON(s store.ScheduledMessage) gin.H {
	out := gin.H{
		"id":         strconv.FormatInt(s.ID, 10),
		"name":       s.Name,
		"channel_id": event.FormatID(s.ChannelID),
		"spec":       sm.DecodeSpec(s.Spec),
		"schedule":   sm.DecodeSchedule(s.Schedule),
		"enabled":    s.Enabled,
		"created_at": s.CreatedAt.UnixMilli(),
	}
	if s.NextRunAt != nil {
		out["next_run_at"] = s.NextRunAt.UnixMilli()
	}
	if s.LastRunAt != nil {
		out["last_run_at"] = s.LastRunAt.UnixMilli()
	}
	return out
}

// handleListSchedules returns the guild's scheduled messages.
func (s *Server) handleListSchedules(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	rows, err := s.store.Schedules.ListByGuild(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load schedules")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, schedJSON(r))
	}
	c.JSON(http.StatusOK, gin.H{"schedules": out})
}

type schedReq struct {
	Name      string          `json:"name"`
	ChannelID string          `json:"channel_id"`
	Spec      *sm.MessageSpec `json:"spec"`
	Schedule  *sm.ScheduleDef `json:"schedule"`
	Enabled   *bool           `json:"enabled"`
}

// applySchedReq folds a request into a schedule row, recomputing the durable
// next-run timer whenever the cadence (or the enable switch) changes.
func applySchedReq(row *store.ScheduledMessage, req schedReq) error {
	if req.Name != "" {
		row.Name = strings.TrimSpace(req.Name)
	}
	if req.ChannelID != "" {
		if chID, ok := event.ParseID(req.ChannelID); ok && chID != 0 {
			row.ChannelID = chID
		}
	}
	if req.Spec != nil {
		raw, err := json.Marshal(req.Spec)
		if err != nil {
			return err
		}
		row.Spec = raw
	}
	if req.Schedule != nil {
		if err := req.Schedule.Validate(); err != nil {
			return err
		}
		raw, err := json.Marshal(req.Schedule)
		if err != nil {
			return err
		}
		row.Schedule = raw
	}
	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	}
	def := sm.DecodeSchedule(row.Schedule)
	if next, ok := def.NextRun(time.Now()); ok {
		row.NextRunAt = &next
	} else {
		row.NextRunAt = nil
		if def.Kind == "once" {
			row.Enabled = false
		}
	}
	return nil
}

// handleCreateSchedule stores a new scheduled message.
func (s *Server) handleCreateSchedule(c *gin.Context) {
	var req schedReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Schedule == nil {
		fail(c, http.StatusBadRequest, "pick a schedule")
		return
	}
	chID, ok := event.ParseID(req.ChannelID)
	if !ok || chID == 0 {
		fail(c, http.StatusBadRequest, "pick a channel to post in")
		return
	}
	if req.Spec == nil || req.Spec.Empty() {
		fail(c, http.StatusBadRequest, "compose the message to post")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	row := store.ScheduledMessage{GuildID: gidInt, Enabled: true, ChannelID: chID}
	if err := applySchedReq(&row, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if row.Name == "" {
		row.Name = "Scheduled message"
	}
	created, err := s.store.Schedules.Create(c.Request.Context(), row)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not save the schedule")
		return
	}
	// First schedule auto-enables the feature so it actually posts; an
	// explicit off stays off.
	if _, ferr := s.store.Features.Get(c.Request.Context(), gidInt, sm.FeatureKey); ferr != nil {
		_ = s.store.Features.Upsert(c.Request.Context(), gidInt, sm.FeatureKey, true, []byte("{}"))
	}
	s.audit(c, gidInt, "schedule.create", gin.H{"schedule": created.ID, "name": created.Name})
	c.JSON(http.StatusOK, gin.H{"schedule": schedJSON(created)})
}

// handleUpdateSchedule saves the editable fields of one schedule.
func (s *Server) handleUpdateSchedule(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	sid, err := strconv.ParseInt(c.Param("sid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid schedule id")
		return
	}
	row, ok, err := s.store.Schedules.Get(c.Request.Context(), gidInt, sid)
	if err != nil || !ok {
		fail(c, http.StatusNotFound, "schedule not found")
		return
	}
	var req schedReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if err := applySchedReq(&row, req); err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.Schedules.Update(c.Request.Context(), row); err != nil {
		fail(c, http.StatusInternalServerError, "could not save the schedule")
		return
	}
	s.audit(c, gidInt, "schedule.update", gin.H{"schedule": row.ID, "name": row.Name})
	c.JSON(http.StatusOK, gin.H{"schedule": schedJSON(row)})
}

// handleDeleteSchedule removes one schedule.
func (s *Server) handleDeleteSchedule(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	sid, err := strconv.ParseInt(c.Param("sid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid schedule id")
		return
	}
	if err := s.store.Schedules.Delete(c.Request.Context(), gidInt, sid); err != nil {
		fail(c, http.StatusInternalServerError, "could not delete the schedule")
		return
	}
	s.audit(c, gidInt, "schedule.delete", gin.H{"schedule": sid})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleSendSchedule posts one schedule immediately, using the exact runtime
// composition. The timer is untouched.
func (s *Server) handleSendSchedule(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	sid, err := strconv.ParseInt(c.Param("sid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid schedule id")
		return
	}
	ctx := c.Request.Context()
	row, ok, err := s.store.Schedules.Get(ctx, gidInt, sid)
	if err != nil || !ok {
		fail(c, http.StatusNotFound, "schedule not found")
		return
	}
	data := map[string]any{
		"Name":  row.Name,
		"Date":  time.Now().UTC().Format("January 2, 2006"),
		"Guild": gin.H{"Name": "", "MemberCount": 0},
	}
	if g, err := s.store.Guilds.Get(ctx, gidInt); err == nil {
		data["Guild"] = gin.H{"Name": g.Name, "MemberCount": g.MemberCount}
	}
	send, err := sm.Build(ctx, templating.New(), row, data)
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.discord.SendMessage(event.FormatID(row.ChannelID), send); err != nil {
		fail(c, http.StatusBadGateway, "could not send: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type schedActionsReq struct {
	Tail []cc.Step `json:"tail"` // the canvas-authored follow-up flow
}

// handleSchedulerActions persists the canvas-authored follow-up flow for the
// scheduler's built-in "Scheduled message sent" automation. Mirrors
// handleSocialActions.
func (s *Server) handleSchedulerActions(c *gin.Context) {
	var req schedActionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if res := cc.ValidateEventFlow(cc.Definition{Steps: req.Tail}, false); !res.OK {
		fail(c, http.StatusBadRequest, "the follow-up flow has an invalid step: "+firstValidationError(res))
		return
	}
	gidInt, _ := event.ParseID(guildID(c))

	fc, err := s.store.Features.Get(c.Request.Context(), gidInt, sm.FeatureKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load configuration")
		return
	}
	cfg := sm.Default()
	if len(fc.Config) > 0 {
		if err := json.Unmarshal(fc.Config, &cfg); err != nil {
			fail(c, http.StatusInternalServerError, "stored configuration is invalid")
			return
		}
	}
	cfg.Tail = req.Tail

	raw, err := json.Marshal(cfg)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not encode configuration")
		return
	}
	if err := s.store.Features.Upsert(c.Request.Context(), gidInt, sm.FeatureKey, fc.Enabled, raw); err != nil {
		fail(c, http.StatusInternalServerError, "could not save")
		return
	}
	s.audit(c, gidInt, "feature.update", gin.H{"feature": sm.FeatureKey, "actions": "scheduler.sent"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
