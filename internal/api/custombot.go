package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/store"
	"github.com/gin-gonic/gin"
)

// customBotHTTP is a short-timeout client for validating a customer's bot token
// against Discord (GET /users/@me with their token).
var customBotHTTP = &http.Client{Timeout: 8 * time.Second}

// botUser is the subset of Discord's /users/@me we surface to the wizard.
type botUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Avatar   string `json:"avatar"`
	Bot      bool   `json:"bot"`
}

// presenceInput is the presence payload the dashboard sends.
type presenceInput struct {
	Status       string `json:"status"`
	ActivityType int    `json:"activity_type"`
	ActivityText string `json:"activity_text"`
	ActivityURL  string `json:"activity_url"`
}

func normalizePresence(p presenceInput) presenceInput {
	switch p.Status {
	case event.StatusOnline, event.StatusIdle, event.StatusDND, event.StatusInvisible:
	default:
		p.Status = event.StatusOnline
	}
	switch p.ActivityType {
	case event.ActivityPlaying, event.ActivityStreaming, event.ActivityListening,
		event.ActivityWatching, event.ActivityCompeting:
	default:
		p.ActivityType = event.ActivityNone
	}
	p.ActivityText = strings.TrimSpace(p.ActivityText)
	p.ActivityURL = strings.TrimSpace(p.ActivityURL)
	return p
}

// validateBotToken calls GET /users/@me with a bot token, returning the bot's
// identity. A bad token yields a clear 401 error the wizard can surface.
func validateBotToken(ctx context.Context, token string) (botUser, error) {
	var u botUser
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/v10/users/@me", nil)
	if err != nil {
		return u, err
	}
	req.Header.Set("Authorization", "Bot "+token)
	resp, err := customBotHTTP.Do(req)
	if err != nil {
		return u, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return u, fmt.Errorf("Discord rejected that token. Double-check you copied the Bot token (not the client secret) and try Reset Token if needed.")
	}
	if resp.StatusCode != http.StatusOK {
		return u, fmt.Errorf("Discord returned status %d validating the token", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return u, err
	}
	return u, nil
}

// customBotView is the dashboard-safe projection (no secrets).
func (s *Server) customBotView(gid string, row store.CustomBot, ok bool) gin.H {
	out := gin.H{
		"available":  s.cbBox.Enabled(),
		"configured": ok,
		"invite_url": "",
	}
	if !ok {
		return out
	}
	appID := event.FormatID(row.ApplicationID)
	out["configured"] = true
	out["enabled"] = row.Enabled
	out["state"] = row.State
	out["last_error"] = row.LastError
	out["application_id"] = appID
	out["username"] = row.Username
	out["avatar_url"] = discord.AvatarURL(event.FormatID(row.BotUserID), row.Avatar, 128)
	out["commands_synced"] = row.CommandsSynced
	out["presence"] = gin.H{
		"status":        row.PresenceStatus,
		"activity_type": row.ActivityType,
		"activity_text": row.ActivityText,
		"activity_url":  row.ActivityURL,
	}
	out["invite_url"] = customBotInviteURL(appID, gid)
	return out
}

func customBotInviteURL(appID, guildID string) string {
	return fmt.Sprintf(
		"https://discord.com/oauth2/authorize?client_id=%s&scope=bot+applications.commands&permissions=%d&guild_id=%s",
		appID, int64(botInvitePerms), guildID)
}

// GET /custom-bot
func (s *Server) handleGetCustomBot(c *gin.Context) {
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)
	row, ok, err := s.store.CustomBots.Get(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load custom bot")
		return
	}
	c.JSON(http.StatusOK, s.customBotView(gid, row, ok))
}

type validateReq struct {
	Token string `json:"token"`
}

// POST /custom-bot/validate — preview a token's bot without saving it.
func (s *Server) handleValidateCustomBot(c *gin.Context) {
	if !s.cbBox.Enabled() {
		fail(c, http.StatusServiceUnavailable, "custom bots are not enabled on this instance")
		return
	}
	var req validateReq
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Token) == "" {
		fail(c, http.StatusBadRequest, "a bot token is required")
		return
	}
	u, err := validateBotToken(c.Request.Context(), strings.TrimSpace(req.Token))
	if err != nil {
		fail(c, http.StatusBadRequest, err.Error())
		return
	}
	if !u.Bot {
		fail(c, http.StatusBadRequest, "that token belongs to a user account, not a bot")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"application_id": u.ID,
		"username":       u.Username,
		"avatar_url":     discord.AvatarURL(u.ID, u.Avatar, 128),
	})
}

type saveCustomBotReq struct {
	Token    string         `json:"token"`
	Presence *presenceInput `json:"presence"`
}

// PUT /custom-bot — validate + encrypt + store the identity (does not enable).
func (s *Server) handleSaveCustomBot(c *gin.Context) {
	if !s.cbBox.Enabled() {
		fail(c, http.StatusServiceUnavailable, "custom bots are not enabled on this instance")
		return
	}
	var req saveCustomBotReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	ctx := c.Request.Context()
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)

	// A save with no token keeps the stored one and only updates presence.
	token := strings.TrimSpace(req.Token)
	existing, hasExisting, _ := s.store.CustomBots.Get(ctx, gidInt)

	var u botUser
	if token != "" {
		var err error
		u, err = validateBotToken(ctx, token)
		if err != nil {
			fail(c, http.StatusBadRequest, err.Error())
			return
		}
		if !u.Bot {
			fail(c, http.StatusBadRequest, "that token belongs to a user account, not a bot")
			return
		}
	} else if !hasExisting {
		fail(c, http.StatusBadRequest, "a bot token is required")
		return
	}

	row := store.CustomBot{GuildID: gidInt}
	if hasExisting {
		row = existing
	}
	if token != "" {
		enc, err := s.cbBox.EncryptString(token)
		if err != nil {
			fail(c, http.StatusInternalServerError, "could not secure the token")
			return
		}
		appID, _ := event.ParseID(u.ID)
		row.ApplicationID = appID
		row.BotUserID = appID
		row.Username = u.Username
		row.Avatar = u.Avatar
		row.TokenEnc = enc
	}
	p := normalizePresence(presenceValueOr(req.Presence, existing, hasExisting))
	row.PresenceStatus = p.Status
	row.ActivityType = p.ActivityType
	row.ActivityText = p.ActivityText
	row.ActivityURL = p.ActivityURL

	if err := s.store.CustomBots.Upsert(ctx, row); err != nil {
		fail(c, http.StatusInternalServerError, "could not save the custom bot")
		return
	}
	// If it's already enabled, a new token/presence should take effect: restart
	// the connection (remove then ensure) so a token change is picked up.
	if row.Enabled {
		_ = s.custombot.RemoveApp(row.ApplicationID)
		_ = s.custombot.EnsureGuild(ctx, gidInt)
	}
	s.audit(c, gidInt, "custombot.save", gin.H{"application_id": event.FormatID(row.ApplicationID)})

	fresh, ok, _ := s.store.CustomBots.Get(ctx, gidInt)
	c.JSON(http.StatusOK, s.customBotView(gid, fresh, ok))
}

func presenceValueOr(p *presenceInput, existing store.CustomBot, hasExisting bool) presenceInput {
	if p != nil {
		return *p
	}
	if hasExisting {
		return presenceInput{
			Status:       existing.PresenceStatus,
			ActivityType: existing.ActivityType,
			ActivityText: existing.ActivityText,
			ActivityURL:  existing.ActivityURL,
		}
	}
	return presenceInput{Status: event.StatusOnline, ActivityType: event.ActivityNone}
}

// POST /custom-bot/enable
func (s *Server) handleEnableCustomBot(c *gin.Context) {
	ctx := c.Request.Context()
	gidInt, _ := event.ParseID(guildID(c))
	row, ok, err := s.store.CustomBots.Get(ctx, gidInt)
	if err != nil || !ok {
		fail(c, http.StatusBadRequest, "set up the custom bot before enabling it")
		return
	}
	if err := s.store.CustomBots.SetEnabled(ctx, gidInt, true); err != nil {
		fail(c, http.StatusInternalServerError, "could not enable")
		return
	}
	if err := s.custombot.EnsureGuild(ctx, gidInt); err != nil {
		s.log.Warn("custombot ensure failed", "guild", gidInt, "err", err)
	}
	s.audit(c, gidInt, "custombot.enable", gin.H{"application_id": event.FormatID(row.ApplicationID)})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /custom-bot/disable
func (s *Server) handleDisableCustomBot(c *gin.Context) {
	ctx := c.Request.Context()
	gidInt, _ := event.ParseID(guildID(c))
	row, ok, _ := s.store.CustomBots.Get(ctx, gidInt)
	if err := s.store.CustomBots.SetEnabled(ctx, gidInt, false); err != nil {
		fail(c, http.StatusInternalServerError, "could not disable")
		return
	}
	if ok {
		_ = s.custombot.RemoveApp(row.ApplicationID)
	}
	s.audit(c, gidInt, "custombot.disable", nil)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// PUT /custom-bot/presence
func (s *Server) handleCustomBotPresence(c *gin.Context) {
	ctx := c.Request.Context()
	gidInt, _ := event.ParseID(guildID(c))
	var req presenceInput
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	p := normalizePresence(req)
	if err := s.store.CustomBots.SetPresence(ctx, gidInt, p.Status, p.ActivityType, p.ActivityText, p.ActivityURL); err != nil {
		fail(c, http.StatusInternalServerError, "could not save presence")
		return
	}
	if err := s.custombot.Presence(ctx, gidInt); err != nil {
		s.log.Warn("custombot presence publish failed", "guild", gidInt, "err", err)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DELETE /custom-bot
func (s *Server) handleDeleteCustomBot(c *gin.Context) {
	ctx := c.Request.Context()
	gidInt, _ := event.ParseID(guildID(c))
	row, ok, _ := s.store.CustomBots.Get(ctx, gidInt)
	if ok {
		_ = s.custombot.RemoveApp(row.ApplicationID)
	}
	if err := s.store.CustomBots.Delete(ctx, gidInt); err != nil {
		fail(c, http.StatusInternalServerError, "could not remove the custom bot")
		return
	}
	s.audit(c, gidInt, "custombot.delete", nil)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
