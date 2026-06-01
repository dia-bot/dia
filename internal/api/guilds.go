package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/welcome"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/pkg/discordgo"
	"github.com/gin-gonic/gin"
)

// knownFeatures is the set of feature keys the dashboard may configure.
var knownFeatures = map[string]bool{
	"welcome": true, "leveling": true, "autorole": true,
	"moderation": true, "automod": true, "customcommands": true, "reactionroles": true,
}

// botInvitePerms is the permission set requested in the bot invite URL.
const botInvitePerms = discordgo.PermissionViewChannel |
	discordgo.PermissionSendMessages |
	discordgo.PermissionManageMessages |
	discordgo.PermissionEmbedLinks |
	discordgo.PermissionAttachFiles |
	discordgo.PermissionReadMessageHistory |
	discordgo.PermissionKickMembers |
	discordgo.PermissionBanMembers |
	discordgo.PermissionManageRoles |
	discordgo.PermissionModerateMembers

// handleListGuilds returns the guilds the user manages, flagged by whether Dia
// is present (and an invite URL when it is not).
func (s *Server) handleListGuilds(c *gin.Context) {
	sess := currentSession(c)

	var ids []int64
	manageable := make([]UserGuild, 0)
	for _, g := range sess.Guilds {
		if !canManage(sess, g.ID) {
			continue
		}
		manageable = append(manageable, g)
		if id, ok := event.ParseID(g.ID); ok {
			ids = append(ids, id)
		}
	}

	present := map[string]bool{}
	if rows, err := s.store.Guilds.ListByIDs(c.Request.Context(), ids); err == nil {
		for _, r := range rows {
			present[event.FormatID(r.ID)] = true
		}
	}

	out := make([]gin.H, 0, len(manageable))
	for _, g := range manageable {
		item := gin.H{
			"id":          g.ID,
			"name":        g.Name,
			"icon":        g.Icon,
			"icon_url":    discord.GuildIconURL(g.ID, g.Icon, 128),
			"bot_present": present[g.ID],
		}
		if !present[g.ID] {
			item["invite_url"] = s.inviteURL(g.ID)
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"guilds": out})
}

func (s *Server) inviteURL(guildID string) string {
	return fmt.Sprintf(
		"https://discord.com/oauth2/authorize?client_id=%s&scope=bot+applications.commands&permissions=%d&guild_id=%s",
		s.cfg.Discord.ClientID, int64(botInvitePerms), guildID)
}

// handleGetGuild returns the live guild snapshot (meta, channels, roles) plus
// all feature configs.
func (s *Server) handleGetGuild(c *gin.Context) {
	gid := guildID(c)
	snap, err := s.gstate.Snapshot(c.Request.Context(), gid)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load server state")
		return
	}
	// Sort channels/roles for stable dropdowns.
	sort.Slice(snap.Channels, func(i, j int) bool { return snap.Channels[i].Position < snap.Channels[j].Position })
	sort.Slice(snap.Roles, func(i, j int) bool { return snap.Roles[i].Position > snap.Roles[j].Position })

	gidInt, _ := event.ParseID(gid)
	features, _ := s.store.Features.GetAll(c.Request.Context(), gidInt)
	featOut := map[string]gin.H{}
	for k, fc := range features {
		featOut[k] = gin.H{"enabled": fc.Enabled, "config": json.RawMessage(fc.Config)}
	}

	c.JSON(http.StatusOK, gin.H{
		"guild":    snap.Meta,
		"channels": snap.Channels,
		"roles":    snap.Roles,
		"features": featOut,
	})
}

// handleListFeatures returns all feature configs for the guild.
func (s *Server) handleListFeatures(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	features, err := s.store.Features.GetAll(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load features")
		return
	}
	out := map[string]gin.H{}
	for k, fc := range features {
		out[k] = gin.H{"enabled": fc.Enabled, "config": json.RawMessage(fc.Config)}
	}
	c.JSON(http.StatusOK, gin.H{"features": out})
}

// handleGetFeature returns one feature's config.
func (s *Server) handleGetFeature(c *gin.Context) {
	key := c.Param("key")
	if !knownFeatures[key] {
		fail(c, http.StatusNotFound, "unknown feature")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	fc, err := s.store.Features.Get(c.Request.Context(), gidInt, key)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load feature")
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": fc.Enabled, "config": json.RawMessage(fc.Config)})
}

type putFeatureReq struct {
	Enabled bool            `json:"enabled"`
	Config  json.RawMessage `json:"config"`
}

// handlePutFeature saves a feature config.
func (s *Server) handlePutFeature(c *gin.Context) {
	key := c.Param("key")
	if !knownFeatures[key] {
		fail(c, http.StatusNotFound, "unknown feature")
		return
	}
	var req putFeatureReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Config) > 0 && !json.Valid(req.Config) {
		fail(c, http.StatusBadRequest, "config is not valid JSON")
		return
	}
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)
	if err := s.store.Features.Upsert(c.Request.Context(), gidInt, key, req.Enabled, req.Config); err != nil {
		fail(c, http.StatusInternalServerError, "could not save")
		return
	}
	s.audit(c, gidInt, "feature.update", gin.H{"feature": key, "enabled": req.Enabled})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleWelcomePresets returns the built-in welcome card presets.
func (s *Server) handleWelcomePresets(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"presets": welcome.Presets})
}

type welcomePreviewReq struct {
	Background   imaging.Background `json:"background"`
	AccentColor  string             `json:"accent_color"`
	TextColor    string             `json:"text_color"`
	SubTextColor string             `json:"sub_text_color"`
	Title        string             `json:"title"`
	Subtitle     string             `json:"subtitle"`
	Footer       string             `json:"footer"`
	AvatarURL    string             `json:"avatar_url"`
	Username     string             `json:"username"`
	Count        int                `json:"count"`
}

// handleWelcomePreview renders a live welcome-card preview for the dashboard.
func (s *Server) handleWelcomePreview(c *gin.Context) {
	var req welcomePreviewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Username == "" {
		req.Username = "NewMember"
	}
	if req.Count == 0 {
		req.Count = 1024
	}
	sub := newSampleVars(req.Username, s.guildName(c), req.Count)
	png, err := s.imaging.RenderWelcome(c.Request.Context(), imaging.WelcomeInput{
		Background:   req.Background,
		AccentColor:  req.AccentColor,
		TextColor:    req.TextColor,
		SubTextColor: req.SubTextColor,
		AvatarURL:    req.AvatarURL,
		Title:        sub.Replace(orDefault(req.Title, "Welcome, {user}!")),
		Subtitle:     sub.Replace(orDefault(req.Subtitle, "You're member #{count}")),
		Footer:       sub.Replace(req.Footer),
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "render failed")
		return
	}
	c.Data(http.StatusOK, "image/png", png)
}

type rankPreviewReq struct {
	Background   imaging.Background `json:"background"`
	AccentColor  string             `json:"accent_color"`
	TextColor    string             `json:"text_color"`
	SubTextColor string             `json:"sub_text_color"`
	BarColor     string             `json:"bar_color"`
	BarBgColor   string             `json:"bar_bg_color"`
	AvatarURL    string             `json:"avatar_url"`
	Username     string             `json:"username"`
	Rank         int                `json:"rank"`
	Level        int                `json:"level"`
	LevelXP      int64              `json:"level_xp"`
	NeededXP     int64              `json:"needed_xp"`
	TotalXP      int64              `json:"total_xp"`
}

// handleRankPreview renders a live rank-card preview for the dashboard.
func (s *Server) handleRankPreview(c *gin.Context) {
	var req rankPreviewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Username == "" {
		req.Username = "Member"
	}
	if req.NeededXP == 0 {
		req.NeededXP = 1000
	}
	png, err := s.imaging.RenderRank(c.Request.Context(), imaging.RankInput{
		Background:   req.Background,
		AccentColor:  req.AccentColor,
		TextColor:    req.TextColor,
		SubTextColor: req.SubTextColor,
		BarColor:     req.BarColor,
		BarBgColor:   req.BarBgColor,
		AvatarURL:    req.AvatarURL,
		Username:     req.Username,
		Rank:         orDefaultInt(req.Rank, 1),
		Level:        orDefaultInt(req.Level, 5),
		LevelXP:      req.LevelXP,
		NeededXP:     req.NeededXP,
		TotalXP:      req.TotalXP,
	})
	if err != nil {
		fail(c, http.StatusInternalServerError, "render failed")
		return
	}
	c.Data(http.StatusOK, "image/png", png)
}

func (s *Server) guildName(c *gin.Context) string {
	if gidInt, ok := event.ParseID(guildID(c)); ok {
		if g, err := s.store.Guilds.Get(c.Request.Context(), gidInt); err == nil && g.Name != "" {
			return g.Name
		}
	}
	return "your server"
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func orDefaultInt(n, def int) int {
	if n == 0 {
		return def
	}
	return n
}
