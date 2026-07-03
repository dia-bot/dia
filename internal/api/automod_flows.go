package api

import (
	"encoding/json"
	"net/http"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/moderation"
	"github.com/gin-gonic/gin"
)

type automodRuleActionsReq struct {
	Tail []cc.Step `json:"tail"` // the post-actions follow-up flow
}

// handleAutomodRuleActions persists the canvas-authored follow-up flow for one
// Dia automod rule's built-in automation ("connect a new action after the
// rule's actions apply"). Only the rule's Tail is replaced; the trigger,
// actions and toggles stay authoritative from the Automod tab, whose saves
// pass through MergeStoredRuleTails so they can't clobber a flow wired here.
// The tail is validated as an event flow (it runs on the bare automod hit), so
// an unrunnable step is rejected here. Mirrors handleMenuActions, but stores
// into the automod feature config instead of a menu row. (Discord-native rules
// live under /automod-rules and are unrelated.)
func (s *Server) handleAutomodRuleActions(c *gin.Context) {
	ruleID := c.Param("rid")
	if ruleID == "" {
		fail(c, http.StatusBadRequest, "invalid rule id")
		return
	}
	var req automodRuleActionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if res := cc.ValidateEventFlow(cc.Definition{Steps: req.Tail}, false); !res.OK {
		fail(c, http.StatusBadRequest, "the follow-up flow has an invalid step: "+firstValidationError(res))
		return
	}
	gidInt, _ := event.ParseID(guildID(c))

	fc, err := s.store.Features.Get(c.Request.Context(), gidInt, moderation.AutomodKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load configuration")
		return
	}
	// Decode over the defaults (mirroring the built-in list) so a tail can be
	// wired onto a starter rule before the Automod page has ever been saved.
	cfg := moderation.DefaultAutomod()
	if len(fc.Config) > 0 {
		if err := json.Unmarshal(fc.Config, &cfg); err != nil {
			fail(c, http.StatusInternalServerError, "stored configuration is invalid")
			return
		}
	}
	idx := -1
	for i := range cfg.Rules {
		if cfg.Rules[i].ID == ruleID {
			idx = i
			break
		}
	}
	if idx < 0 {
		fail(c, http.StatusNotFound, "rule not found")
		return
	}
	// Replace only the canvas-owned tail; leave the rule's trigger, actions and
	// exemptions untouched (owned by the Automod tab).
	cfg.Rules[idx].Tail = req.Tail

	raw, err := json.Marshal(cfg)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not encode configuration")
		return
	}
	if err := s.store.Features.Upsert(c.Request.Context(), gidInt, moderation.AutomodKey, fc.Enabled, raw); err != nil {
		fail(c, http.StatusInternalServerError, "could not save")
		return
	}
	s.audit(c, gidInt, "automod.actions", gin.H{"id": ruleID})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
