package api

import (
	"encoding/json"
	"net/http"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/leveling"
	"github.com/gin-gonic/gin"
)

// handleLevelingVariables returns the rank-card placeholder tokens for the
// dashboard variable picker (single source of truth: leveling.RankVariables).
func (s *Server) handleLevelingVariables(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"variables": leveling.RankVariables})
}

type levelingActionsReq struct {
	Actions []leveling.ButtonAction `json:"actions"`
	Tail    []cc.Step               `json:"tail"` // the post-announce flow
}

// handleLevelingActions persists the canvas-authored programs for the level-up
// announcement: the per-component click actions (dragging button dots) and the
// post-message tail ("connect a new action after sending the message"). Only
// Actions / Tail are replaced; every other stored field (message, embeds,
// components, rank card) stays authoritative from the composer. The programs are
// validated as event flows — click actions run with the click interaction
// available, the tail on the bare event — so an unrunnable step is rejected
// here. Mirrors handleWelcomeActions, minus the welcome/goodbye + DM tabs
// (leveling's announcement is a single message).
func (s *Server) handleLevelingActions(c *gin.Context) {
	var req levelingActionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	for _, a := range req.Actions {
		if res := cc.ValidateEventFlow(cc.Definition{Steps: a.Steps}, true); !res.OK {
			fail(c, http.StatusBadRequest, "a button action has an invalid step: "+firstValidationError(res))
			return
		}
	}
	if res := cc.ValidateEventFlow(cc.Definition{Steps: req.Tail}, false); !res.OK {
		fail(c, http.StatusBadRequest, "the follow-up flow has an invalid step: "+firstValidationError(res))
		return
	}
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)

	fc, err := s.store.Features.Get(c.Request.Context(), gidInt, leveling.FeatureKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load configuration")
		return
	}
	var cfg leveling.Config
	if len(fc.Config) > 0 {
		if err := json.Unmarshal(fc.Config, &cfg); err != nil {
			fail(c, http.StatusInternalServerError, "stored configuration is invalid")
			return
		}
	}
	// The announcement message is always on the flow, so its click actions are
	// authoritative from the canvas. Replace only Actions / Tail; leave the
	// message, embeds, components and rank card untouched.
	cfg.Actions = req.Actions
	cfg.Tail = req.Tail

	raw, err := json.Marshal(cfg)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not encode configuration")
		return
	}
	if err := s.store.Features.Upsert(c.Request.Context(), gidInt, leveling.FeatureKey, fc.Enabled, raw); err != nil {
		fail(c, http.StatusInternalServerError, "could not save")
		return
	}
	s.audit(c, gidInt, "feature.update", gin.H{"feature": leveling.FeatureKey, "actions": "leveling"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
