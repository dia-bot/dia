package api

import (
	"encoding/json"
	"net/http"

	"github.com/dia-bot/dia/internal/event"
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
	if req.Kind == "goodbye" {
		mc = cfg.Goodbye
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
		WithFonts(fonts)

	send, err := welcome.BuildMessage(c.Request.Context(), s.imaging, mc, v)
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
