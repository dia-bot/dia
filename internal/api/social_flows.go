package api

import (
	"encoding/json"
	"net/http"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	sn "github.com/dia-bot/dia/internal/features/socialnotifications"
	"github.com/gin-gonic/gin"
)

type socialActionsReq struct {
	Tail []cc.Step `json:"tail"` // the canvas-authored follow-up flow
}

// handleSocialActions persists the canvas-authored follow-up flow for the
// social feature's built-in "Announce social updates" automation ("connect a
// new action after the announcement posts"). It runs on the bare social_update
// event, so the tail is validated as an event flow. Mirrors
// handleGiveawayActions: only the canvas-owned Tail is replaced; the rest of
// the stored config stays authoritative.
func (s *Server) handleSocialActions(c *gin.Context) {
	var req socialActionsReq
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

	fc, err := s.store.Features.Get(c.Request.Context(), gidInt, sn.FeatureKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load configuration")
		return
	}
	cfg := sn.Default()
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
	if err := s.store.Features.Upsert(c.Request.Context(), gidInt, sn.FeatureKey, fc.Enabled, raw); err != nil {
		fail(c, http.StatusInternalServerError, "could not save")
		return
	}
	s.audit(c, gidInt, "feature.update", gin.H{"feature": sn.FeatureKey, "actions": "social.update"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
