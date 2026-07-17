package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/statschannels"
	"github.com/dia-bot/dia/pkg/discordgo"
	"github.com/gin-gonic/gin"
)

type statsChannelReq struct {
	Name string `json:"name"`
}

// handleCreateStatsChannel creates a locked voice channel for a stats counter
// (members can see the live value in the name but can't join) and returns its
// id for the counter config.
func (s *Server) handleCreateStatsChannel(c *gin.Context) {
	var req statsChannelReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "📊 Server Stats"
	}
	if r := []rune(name); r != nil && len(r) > 100 {
		name = string(r[:100])
	}
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)

	ch, err := s.discord.CreateChannel(gid, discordgo.GuildChannelCreateData{
		Name: name,
		Type: discordgo.ChannelTypeGuildVoice,
		PermissionOverwrites: []*discordgo.PermissionOverwrite{{
			// The @everyone role shares the guild's id: hide the join button.
			ID:   gid,
			Type: discordgo.PermissionOverwriteTypeRole,
			Deny: discordgo.PermissionVoiceConnect,
		}},
	}, "stats counter channel")
	if err != nil {
		fail(c, http.StatusBadGateway, "could not create the channel: "+err.Error())
		return
	}
	s.audit(c, gidInt, "stats.channel_create", gin.H{"channel": ch.ID, "name": name})
	c.JSON(http.StatusOK, gin.H{"channel_id": ch.ID})
}

type statsActionsReq struct {
	Tail []cc.Step `json:"tail"` // the canvas-authored follow-up flow
}

// handleStatsActions persists the canvas-authored follow-up flow for the stats
// feature's built-in "Member milestone" automation. Mirrors
// handleSocialActions: only the canvas-owned Tail is replaced.
func (s *Server) handleStatsActions(c *gin.Context) {
	var req statsActionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if res := cc.ValidateEventFlow(cc.Definition{Steps: req.Tail}, false); !res.OK {
		fail(c, http.StatusBadRequest, "the follow-up flow has an invalid step: "+firstValidationError(res))
		return
	}
	gidInt, _ := event.ParseID(guildID(c))

	fc, err := s.store.Features.Get(c.Request.Context(), gidInt, statschannels.FeatureKey)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load configuration")
		return
	}
	cfg := statschannels.Default()
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
	if err := s.store.Features.Upsert(c.Request.Context(), gidInt, statschannels.FeatureKey, fc.Enabled, raw); err != nil {
		fail(c, http.StatusInternalServerError, "could not save")
		return
	}
	s.audit(c, gidInt, "feature.update", gin.H{"feature": statschannels.FeatureKey, "actions": "stats.milestone"})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
