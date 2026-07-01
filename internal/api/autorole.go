package api

import (
	"encoding/json"
	"net/http"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/roles"
	"github.com/gin-gonic/gin"
)

type autoroleActionsReq struct {
	Tail []cc.Step `json:"tail"` // the post-grant follow-up flow
}

// handleAutoroleActions persists the canvas-authored post-grant follow-up flow
// for auto-roles' built-in automation ("connect a new action after granting the
// roles"). Only Tail is replaced; every other stored field (the roles list,
// IncludeBots, WaitForScreening) stays authoritative from the auto-roles
// settings page. The tail is validated as an event flow (it runs on the bare
// member_join event), so an unrunnable step is rejected here. Mirrors
// handleLevelingActions, minus the per-button click actions (auto-roles posts no
// message, so there are no components to wire).
func (s *Server) handleAutoroleActions(c *gin.Context) {
	var req autoroleActionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if res := cc.ValidateEventFlow(cc.Definition{Steps: req.Tail}, false); !res.OK {
		fail(c, http.StatusBadRequest, "the follow-up flow has an invalid step: "+firstValidationError(res))
		return
	}
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)

	fc, err := s.store.Features.Get(c.Request.Context(), gidInt, roles.FeatureKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load configuration")
		return
	}
	var cfg roles.Config
	if len(fc.Config) > 0 {
		if err := json.Unmarshal(fc.Config, &cfg); err != nil {
			fail(c, http.StatusInternalServerError, "stored configuration is invalid")
			return
		}
	}
	// Replace only the canvas-owned tail; leave the roles list and toggles
	// untouched (owned by the auto-roles settings page).
	cfg.Tail = req.Tail

	raw, err := json.Marshal(cfg)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not encode configuration")
		return
	}
	if err := s.store.Features.Upsert(c.Request.Context(), gidInt, roles.FeatureKey, fc.Enabled, raw); err != nil {
		fail(c, http.StatusInternalServerError, "could not save")
		return
	}
	s.audit(c, gidInt, "feature.update", gin.H{"feature": roles.FeatureKey, "actions": "autorole"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
