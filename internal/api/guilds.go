package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

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

// botInvitePerms is the permission requested in the bot invite URL: Administrator,
// so Dia is granted full access in the server it's added to.
const botInvitePerms = discordgo.PermissionAdministrator

// handleListGuilds returns the guilds the user manages, flagged by whether Dia
// is present (and an invite URL when it is not).
func (s *Server) handleListGuilds(c *gin.Context) {
	ctx := c.Request.Context()
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

	// Presence comes from two sources OR'd together: the gateway-sourced guilds
	// table, and the bot's live guild list from Discord (cached). The latter is
	// authoritative and independent of the GUILD_CREATE pipeline, so the dashboard
	// reflects the bot's real membership even when gateway events lag or drop.
	present := map[string]bool{}
	if rows, err := s.store.Guilds.ListByIDs(ctx, ids); err == nil {
		for _, r := range rows {
			present[event.FormatID(r.ID)] = true
		}
	}
	botSet := s.botGuildIDs(ctx)

	out := make([]gin.H, 0, len(manageable))
	for _, g := range manageable {
		inGuild := present[g.ID] || botSet[g.ID]
		item := gin.H{
			"id":          g.ID,
			"name":        g.Name,
			"icon":        g.Icon,
			"icon_url":    discord.GuildIconURL(g.ID, g.Icon, 128),
			"bot_present": inGuild,
		}
		if !inGuild {
			item["invite_url"] = s.inviteURL(g.ID)
		}
		out = append(out, item)
	}
	c.JSON(http.StatusOK, gin.H{"guilds": out})
}

// botGuildIDs returns the set of guild IDs the bot belongs to, read straight from
// Discord (GET /users/@me/guilds with the bot token) and cached in Redis for a
// short window. Authoritative and independent of the gateway's GUILD_CREATE
// pipeline. On any error it returns an empty set, so callers fall back to the DB.
func (s *Server) botGuildIDs(ctx context.Context) map[string]bool {
	const cacheKey = "bot:guilds"
	set := map[string]bool{}

	var cached []string
	if err := s.cache.GetJSON(ctx, cacheKey, &cached); err == nil {
		for _, id := range cached {
			set[id] = true
		}
		return set
	}
	if s.cfg.Discord.Token == "" {
		return set
	}

	var ids []string
	after := ""
	for page := 0; page < 100; page++ { // safety cap: 100 * 200 = 20k guilds
		url := "https://discord.com/api/v10/users/@me/guilds?limit=200"
		if after != "" {
			url += "&after=" + after
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			break
		}
		req.Header.Set("Authorization", "Bot "+s.cfg.Discord.Token)
		resp, err := oauthHTTP.Do(req)
		if err != nil {
			s.log.Warn("list bot guilds", "err", err)
			break
		}
		var guilds []struct {
			ID string `json:"id"`
		}
		if resp.StatusCode == http.StatusOK {
			err = json.NewDecoder(resp.Body).Decode(&guilds)
		} else {
			err = fmt.Errorf("status %d", resp.StatusCode)
		}
		resp.Body.Close()
		if err != nil {
			s.log.Warn("list bot guilds", "err", err)
			break
		}
		for _, g := range guilds {
			set[g.ID] = true
			ids = append(ids, g.ID)
		}
		if len(guilds) < 200 {
			break
		}
		after = guilds[len(guilds)-1].ID
	}

	if len(ids) > 0 {
		_ = s.cache.SetJSON(ctx, cacheKey, ids, 10*time.Second)
	}
	return set
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

// guildIconURL returns the guild's icon CDN URL (or "" if it has none), for the
// {{.Server.Icon}} card variable.
func (s *Server) guildIconURL(c *gin.Context) string {
	gid := guildID(c)
	if gidInt, ok := event.ParseID(gid); ok {
		if g, err := s.store.Guilds.Get(c.Request.Context(), gidInt); err == nil {
			return discord.GuildIconURL(gid, g.Icon, 256)
		}
	}
	return ""
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
