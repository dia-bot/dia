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
	Tail []cc.Step `json:"tail"` // the canvas-authored follow-up flow
}

// handleGiveawayActions persists the canvas-authored follow-up flow for the
// giveaway feature's built-in "Draw giveaway winners" automation ("connect a new
// action after the winners are drawn"). It runs on the bare giveaway_ended event.
func (s *Server) handleGiveawayActions(c *gin.Context) {
	s.saveGiveawayFlow(c, "ended")
}

// handleGiveawayEntryActions persists the canvas-authored follow-up flow for the
// giveaway feature's built-in "On giveaway entry" automation ("connect a new
// action after a member enters"). It runs on the giveaway_entered event.
func (s *Server) handleGiveawayEntryActions(c *gin.Context) {
	s.saveGiveawayFlow(c, "entry")
}

// saveGiveawayFlow replaces exactly one of the giveaway config's canvas-owned
// tails (which=="ended" → Tail, which=="entry" → EntryTail), leaving the other
// tail, the preset library, manager roles and default preset authoritative from
// the Giveaways settings page and the other flow. The tail is validated as an
// event flow (it runs on the bare event), so an unrunnable step is rejected here.
// Mirrors handleAutoroleActions.
func (s *Server) saveGiveawayFlow(c *gin.Context, which string) {
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
	// Replace only the one canvas-owned tail; the preset library, access settings
	// and the other built-in's tail stay untouched.
	if which == "entry" {
		cfg.EntryTail = req.Tail
	} else {
		cfg.Tail = req.Tail
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not encode configuration")
		return
	}
	if err := s.store.Features.Upsert(c.Request.Context(), gidInt, giveaway.FeatureKey, fc.Enabled, raw); err != nil {
		fail(c, http.StatusInternalServerError, "could not save")
		return
	}
	s.audit(c, gidInt, "feature.update", gin.H{"feature": giveaway.FeatureKey, "actions": "giveaway." + which})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
