package api

import (
	"encoding/json"
	"net/http"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/welcome"
	"github.com/dia-bot/dia/internal/tmpllookup"
	"github.com/gin-gonic/gin"
)

type welcomeTestReq struct {
	Kind string `json:"kind"` // "welcome" (default) or "goodbye"
}

// handleWelcomeTest sends a real test welcome/goodbye message to its configured
// channel, using the logged-in admin as the sample member. It reuses
// welcome.BuildMessage, so the dashboard test is byte-for-byte what the bot
// posts at runtime.
func (s *Server) handleWelcomeTest(c *gin.Context) {
	var req welcomeTestReq
	_ = c.ShouldBindJSON(&req)

	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)

	fc, err := s.store.Features.Get(c.Request.Context(), gidInt, welcome.FeatureKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load configuration")
		return
	}
	var cfg welcome.Config
	if len(fc.Config) > 0 {
		if err := json.Unmarshal(fc.Config, &cfg); err != nil {
			fail(c, http.StatusInternalServerError, "stored configuration is invalid")
			return
		}
	}

	mc := cfg.Welcome
	tab := "welcome"
	if req.Kind == "goodbye" {
		mc = cfg.Goodbye
		tab = "goodbye"
	}
	if mc.ChannelID == "" {
		fail(c, http.StatusBadRequest, "no channel is set for this message yet")
		return
	}

	sess := currentSession(c)
	user := event.User{
		ID:         sess.UserID,
		Username:   sess.Username,
		GlobalName: sess.GlobalName,
		Avatar:     sess.Avatar,
	}

	count := 0
	if snap, err := s.gstate.Snapshot(c.Request.Context(), gid); err == nil {
		count = snap.Meta.MemberCount
	}
	fonts, _ := s.store.Uploads.FontMap(c.Request.Context(), gidInt)
	v := welcome.NewVars(user, gid, s.guildName(c), count).
		WithLookup(tmpllookup.New(c.Request.Context(), s.gstate, gid)).
		WithFonts(fonts).
		WithServerIcon(s.guildIconURL(c))

	send, err := welcome.BuildMessage(c.Request.Context(), s.imaging, mc, v, tab)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not render the message")
		return
	}
	if _, err := s.discord.SendMessage(mc.ChannelID, send); err != nil {
		fail(c, http.StatusBadGateway, "Discord rejected the message: "+err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleWelcomeVariables returns the supported template placeholders for the
// dashboard's variable picker (single source of truth on the Go side).
func (s *Server) handleWelcomeVariables(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"variables": welcome.Variables})
}

// firstValidationError returns the message of the first error-severity issue in
// a validation result (for a 400 the dashboard can show), or a generic fallback.
func firstValidationError(res cc.ValidationResult) string {
	for _, i := range res.Issues {
		if i.Severity == "error" {
			return i.Message
		}
	}
	return "it can't run as configured"
}

type welcomeActionsReq struct {
	Kind      string                 `json:"kind"` // "welcome" (default) or "goodbye"
	Actions   []welcome.ButtonAction `json:"actions"`
	DMActions []welcome.ButtonAction `json:"dm_actions"`
	Tail      []cc.Step              `json:"tail"` // the post-message flow
}

// handleWelcomeActions persists the canvas-authored programs for one tab: the
// per-component click actions (dragging button dots) and the post-message tail
// ("connect a new action after sending the message"). Only the named tab's
// Actions / Tail are replaced; every other stored field (message, embeds, card,
// components, the other tab) is preserved untouched. The programs are validated
// as event flows — click actions run with the click interaction available, the
// tail on the bare event — so an unrunnable step is rejected here.
func (s *Server) handleWelcomeActions(c *gin.Context) {
	var req welcomeActionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	for _, a := range append(append([]welcome.ButtonAction{}, req.Actions...), req.DMActions...) {
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

	fc, err := s.store.Features.Get(c.Request.Context(), gidInt, welcome.FeatureKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load configuration")
		return
	}
	var cfg welcome.Config
	if len(fc.Config) > 0 {
		if err := json.Unmarshal(fc.Config, &cfg); err != nil {
			fail(c, http.StatusInternalServerError, "stored configuration is invalid")
			return
		}
	}
	// The channel message is always on the flow, so its actions are authoritative
	// from the canvas. The DM node only appears when the DM is enabled and has
	// components, so only then can the canvas see (and overwrite) its actions;
	// otherwise keep the stored DM actions so toggling the DM off in the composer
	// doesn't let a later canvas save silently blank them.
	if req.Kind == "goodbye" {
		cfg.Goodbye.Actions = req.Actions
		cfg.Goodbye.Tail = req.Tail
		if cfg.Goodbye.DM.Enabled && len(cfg.Goodbye.DM.Components) > 0 {
			cfg.Goodbye.DM.Actions = req.DMActions
		}
	} else {
		cfg.Welcome.Actions = req.Actions
		cfg.Welcome.Tail = req.Tail
		if cfg.Welcome.DM.Enabled && len(cfg.Welcome.DM.Components) > 0 {
			cfg.Welcome.DM.Actions = req.DMActions
		}
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not encode configuration")
		return
	}
	if err := s.store.Features.Upsert(c.Request.Context(), gidInt, welcome.FeatureKey, fc.Enabled, raw); err != nil {
		fail(c, http.StatusInternalServerError, "could not save")
		return
	}
	s.audit(c, gidInt, "feature.update", gin.H{"feature": welcome.FeatureKey, "actions": req.Kind})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
