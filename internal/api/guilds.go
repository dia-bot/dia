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
	"github.com/dia-bot/dia/internal/features/giveaway"
	"github.com/dia-bot/dia/internal/features/leveling"
	"github.com/dia-bot/dia/internal/features/moderation"
	"github.com/dia-bot/dia/internal/features/roles"
	sn "github.com/dia-bot/dia/internal/features/socialnotifications"
	"github.com/dia-bot/dia/internal/features/statschannels"
	"github.com/dia-bot/dia/internal/features/welcome"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/pkg/discordgo"
	"github.com/gin-gonic/gin"
)

// knownFeatures is the set of feature keys the dashboard may configure.
var knownFeatures = map[string]bool{
	"welcome": true, "leveling": true, "autorole": true,
	"moderation": true, "automod": true, "verification": true, "logging": true,
	"customcommands": true, "reactionroles": true, "tickets": true, "giveaway": true,
	"social": true, "stats": true, "scheduler": true,
}

// botInvitePerms is the permission requested in the bot invite URL: Administrator,
// so Dia is granted full access in the server it's added to.
const botInvitePerms = discordgo.PermissionAdministrator

// handleListGuilds returns the guilds the user manages, flagged by whether Dia
// is present (and an invite URL when it is not).
func (s *Server) handleListGuilds(c *gin.Context) {
	ctx := c.Request.Context()
	sess := currentSession(c)

	// Refresh-on-empty: a session created when Discord's /users/@me/guilds
	// fetch hiccuped (rate-limit, transient 5xx) ends up with an empty
	// Guilds slice and the dashboard's server switcher renders blank
	// forever. Try once more with the user's access token before serving;
	// if it succeeds, persist back so the next request is free.
	if len(sess.Guilds) == 0 && sess.AccessToken != "" {
		var fresh []UserGuild
		if err := discordGet(ctx, sess.AccessToken, "/users/@me/guilds", &fresh); err == nil && len(fresh) > 0 {
			sess.Guilds = fresh
			if _, token, ok := s.sessionFromCookie(c); ok {
				_ = s.sessions.save(ctx, token, sess)
			}
		} else if err != nil {
			s.log.Warn("refresh user guilds", "err", err)
		}
	}

	// Collect ids for the bulk presence lookup across ALL the user's guilds — a
	// feature manager may not be a server admin, so we can't pre-filter to
	// canManage here anymore.
	var ids []int64
	for _, g := range sess.Guilds {
		if id, ok := event.ParseID(g.ID); ok {
			ids = append(ids, id)
		}
	}

	// Presence comes from two sources OR'd together: the gateway-sourced guilds
	// table, and the bot's live guild list from Discord (cached). The latter is
	// authoritative and independent of the GUILD_CREATE pipeline, so the dashboard
	// reflects the bot's real membership even when gateway events lag or drop.
	// IMPORTANT: the bulk lookup + live botSet must agree with the middleware's
	// botInGuild helper; otherwise the dashboard shows "Add" for guilds that
	// would actually pass the guild gate. We pre-warm the live set once, then use
	// it inline so each item is computed the same way the middleware does.
	present := map[string]bool{}
	if rows, err := s.store.Guilds.ListByIDs(ctx, ids); err == nil {
		for _, r := range rows {
			present[event.FormatID(r.ID)] = true
		}
	}
	botSet := s.botGuildIDs(ctx)

	out := make([]gin.H, 0, len(sess.Guilds))
	for _, g := range sess.Guilds {
		admin := canManage(sess, g.ID)
		inGuild := present[g.ID] || botSet[g.ID]
		// Per-guild fallback: when a guild was missed by both signals (race
		// between OAuth list refresh and worker syncing GUILD_CREATE rows),
		// hit Postgres directly. Cheap; only runs for the misses.
		if !inGuild {
			if id, ok := event.ParseID(g.ID); ok {
				if _, err := s.store.Guilds.Get(ctx, id); err == nil {
					inGuild = true
					present[g.ID] = true
				}
			}
		}
		if !admin {
			// A non-admin sees the server only when the bot is present AND they
			// manage at least one delegated feature there (so it stays out of the
			// switcher for everyone else).
			if !inGuild || !s.accessFor(ctx, sess, g.ID).any() {
				continue
			}
		}
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

	// Honour the cache only when it actually contains data. An empty cached
	// slice (which can happen if Redis was previously poisoned, or if the
	// SetJSON path got called with empty input by a future caller) would
	// otherwise short-circuit the live fetch and falsely report "bot is in
	// no servers" for the next 10 seconds.
	var cached []string
	if err := s.cache.GetJSON(ctx, cacheKey, &cached); err == nil && len(cached) > 0 {
		for _, id := range cached {
			set[id] = true
		}
		return set
	}
	if s.cfg.Discord.Token == "" {
		s.log.Warn("botGuildIDs: DISCORD_TOKEN empty, can't list bot guilds")
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
		resp, err := botListHTTP.Do(req)
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
		// Cache for a full minute: this list rarely changes (bot joining or
		// leaving a guild fires gateway events the worker handles), and the
		// dashboard hits this on every page load. 10s caused every reload to
		// race against Discord's REST budget.
		_ = s.cache.SetJSON(ctx, cacheKey, ids, 60*time.Second)
		s.log.Info("botGuildIDs: refreshed bot guild list", "count", len(ids))
	} else {
		s.log.Warn("botGuildIDs: Discord returned 0 guilds for the bot — token wrong or bot really is in no servers")
	}
	return set
}

// botInGuild reports whether Dia is present in the given guild. It ORs the
// gateway-sourced guilds table with the live `GET /users/@me/guilds` set so that
// dashboard auth doesn't 404 when gateway events lag (or when a process was
// restarted with a fresh DB but the bot is still in the server). Matches the
// permissive presence logic in handleListGuilds.
func (s *Server) botInGuild(ctx context.Context, gid string) bool {
	if id, ok := event.ParseID(gid); ok {
		if _, err := s.store.Guilds.Get(ctx, id); err == nil {
			return true
		}
	}
	return s.botGuildIDs(ctx)[gid]
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

	// Meta fallback chain: the freshest source is the Redis snapshot populated
	// from GUILD_CREATE, but until the worker has handshaken (or after a fresh
	// data wipe) Redis can be empty even when the bot is in the guild. Fall
	// back to the Postgres guilds row, then to the user's session guild list,
	// so the dashboard header always renders a real name / icon.
	if snap.Meta.ID == "" {
		snap.Meta.ID = gid
	}
	if snap.Meta.Name == "" {
		if g, gerr := s.store.Guilds.Get(c.Request.Context(), gidInt); gerr == nil {
			if g.Name != "" {
				snap.Meta.Name = g.Name
			}
			if snap.Meta.Icon == "" && g.Icon != "" {
				snap.Meta.Icon = g.Icon
			}
			if snap.Meta.OwnerID == "" && g.OwnerID != 0 {
				snap.Meta.OwnerID = event.FormatID(g.OwnerID)
			}
			if snap.Meta.MemberCount == 0 && g.MemberCount > 0 {
				snap.Meta.MemberCount = g.MemberCount
			}
		}
	}
	if snap.Meta.Name == "" {
		if sess := currentSession(c); sess != nil {
			for _, ug := range sess.Guilds {
				if ug.ID == gid {
					snap.Meta.Name = ug.Name
					if snap.Meta.Icon == "" {
						snap.Meta.Icon = ug.Icon
					}
					break
				}
			}
		}
	}
	// A non-admin feature manager only sees the config of the features they can
	// manage — never another feature's config.
	acc := accessFromCtx(c)
	features, _ := s.store.Features.GetAll(c.Request.Context(), gidInt)
	featOut := map[string]gin.H{}
	for k, fc := range features {
		if !acc.can(k) {
			continue
		}
		featOut[k] = gin.H{"enabled": fc.Enabled, "config": json.RawMessage(fc.Config)}
	}
	accFeatures := map[string]bool{}
	for k := range acc.Features {
		accFeatures[k] = true
	}

	c.JSON(http.StatusOK, gin.H{
		"guild":    snap.Meta,
		"channels": snap.Channels,
		"roles":    snap.Roles,
		"features": featOut,
		"access":   gin.H{"admin": acc.Admin, "features": accFeatures},
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
	// A non-admin manager may only read the config of a feature they manage.
	if !accessFromCtx(c).can(key) {
		fail(c, http.StatusForbidden, "you don't have access to this feature")
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
	// Automod configs are structurally validated before they're persisted so the
	// editor can surface field-level problems and the runtime never decodes a
	// rule set it can't execute.
	if key == moderation.AutomodKey && len(req.Config) > 0 {
		var cfg moderation.AutomodConfig
		if err := json.Unmarshal(req.Config, &cfg); err != nil {
			fail(c, http.StatusBadRequest, "invalid automod config")
			return
		}
		if errs := moderation.ValidateAutomod(cfg); len(errs) > 0 {
			c.JSON(http.StatusBadRequest, gin.H{"errors": errs})
			return
		}
	}
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)
	// Welcome's, leveling's, auto-roles', automod's, giveaways' and social's
	// canvas-owned programs (button click actions and/or the follow-up flows)
	// are owned by the automation flow (saved via /welcome/actions,
	// /leveling/actions, /autorole/actions, /automod/rules/:rid/actions,
	// /giveaway/actions or /social-actions), not the settings page. Keep the
	// stored copy authoritative so a settings save can't clobber a flow wired
	// meanwhile on the canvas.
	if len(req.Config) > 0 && (key == welcome.FeatureKey || key == leveling.FeatureKey || key == roles.FeatureKey || key == moderation.AutomodKey || key == giveaway.FeatureKey || key == sn.FeatureKey || key == statschannels.FeatureKey) {
		if existing, err := s.store.Features.Get(c.Request.Context(), gidInt, key); err == nil && len(existing.Config) > 0 {
			switch key {
			case welcome.FeatureKey:
				req.Config = welcome.MergeStoredActions(req.Config, existing.Config)
			case leveling.FeatureKey:
				req.Config = leveling.MergeStoredActions(req.Config, existing.Config)
			case roles.FeatureKey:
				req.Config = roles.MergeStoredActions(req.Config, existing.Config)
			case moderation.AutomodKey:
				req.Config = moderation.MergeStoredRuleTails(req.Config, existing.Config)
			case giveaway.FeatureKey:
				req.Config = giveaway.MergeStoredTail(req.Config, existing.Config)
			case sn.FeatureKey:
				req.Config = sn.MergeStoredTail(req.Config, existing.Config)
			case statschannels.FeatureKey:
				req.Config = statschannels.MergeStoredTail(req.Config, existing.Config)
			}
		}
	}
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
