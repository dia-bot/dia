package api

import (
	"encoding/json"
	"net/http"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/giveaway"
	"github.com/gin-gonic/gin"
)

type giveawayActionsReq struct {
	Tail []cc.Step `json:"tail"` // the post-draw follow-up flow
}

// handleGiveawayActions persists the canvas-authored follow-up flow for the
// giveaway feature's built-in automation ("connect a new action after the
// winners are drawn"). Only Tail is replaced; the preset library, manager roles
// and default preset stay authoritative from the Giveaways settings page. The
// tail is validated as an event flow (it runs on the bare giveaway_ended event),
// so an unrunnable step is rejected here. Mirrors handleAutoroleActions.
func (s *Server) handleGiveawayActions(c *gin.Context) {
	var req giveawayActionsReq
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

	fc, err := s.store.Features.Get(c.Request.Context(), gidInt, giveaway.FeatureKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load configuration")
		return
	}
	cfg := giveaway.Default()
	if len(fc.Config) > 0 {
		if err := json.Unmarshal(fc.Config, &cfg); err != nil {
			fail(c, http.StatusInternalServerError, "stored configuration is invalid")
			return
		}
	}
	// Replace only the canvas-owned tail; the preset library and access settings
	// stay owned by the Giveaways settings page.
	cfg.Tail = req.Tail

	raw, err := json.Marshal(cfg)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not encode configuration")
		return
	}
	if err := s.store.Features.Upsert(c.Request.Context(), gidInt, giveaway.FeatureKey, fc.Enabled, raw); err != nil {
		fail(c, http.StatusInternalServerError, "could not save")
		return
	}
	s.audit(c, gidInt, "feature.update", gin.H{"feature": giveaway.FeatureKey, "actions": "giveaway"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
