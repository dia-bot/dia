package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strconv"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
	"github.com/gin-gonic/gin"
)

// ── Leveling ─────────────────────────────────────────────────

func (s *Server) handleLeaderboard(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	rows, err := s.store.Levels.Leaderboard(c.Request.Context(), gidInt, 25, 0)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load leaderboard")
		return
	}
	entries := make([]gin.H, 0, len(rows))
	for i, r := range rows {
		entries = append(entries, gin.H{
			"rank": i + 1, "user_id": event.FormatID(r.UserID),
			"level": r.Level, "xp": r.XP, "messages": r.Messages,
		})
	}
	c.JSON(http.StatusOK, gin.H{"entries": entries})
}

func (s *Server) handleListRewards(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	rows, err := s.store.Levels.ListRewards(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load rewards")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, gin.H{"level": r.Level, "role_id": event.FormatID(r.RoleID), "remove_previous": r.RemovePrevious})
	}
	c.JSON(http.StatusOK, gin.H{"rewards": out})
}

type setRewardReq struct {
	Level          int    `json:"level"`
	RoleID         string `json:"role_id"`
	RemovePrevious bool   `json:"remove_previous"`
}

func (s *Server) handleSetReward(c *gin.Context) {
	var req setRewardReq
	if err := c.ShouldBindJSON(&req); err != nil || req.Level < 1 {
		fail(c, http.StatusBadRequest, "invalid reward")
		return
	}
	roleID, ok := event.ParseID(req.RoleID)
	if !ok {
		fail(c, http.StatusBadRequest, "invalid role id")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	if err := s.store.Levels.SetReward(c.Request.Context(), store.LevelReward{
		GuildID: gidInt, Level: req.Level, RoleID: roleID, RemovePrevious: req.RemovePrevious,
	}); err != nil {
		fail(c, http.StatusInternalServerError, "could not save reward")
		return
	}
	s.audit(c, gidInt, "level_reward.set", gin.H{"level": req.Level, "role_id": req.RoleID})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleDeleteReward(c *gin.Context) {
	level, err := strconv.Atoi(c.Param("level"))
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid level")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	if err := s.store.Levels.DeleteReward(c.Request.Context(), gidInt, level); err != nil {
		fail(c, http.StatusInternalServerError, "could not delete reward")
		return
	}
	s.audit(c, gidInt, "level_reward.delete", gin.H{"level": level})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── Custom commands (v2 — programmable Step[] tree) ─────────

var commandNameRe = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

func (s *Server) handleListCommands(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	rows, err := s.store.CustomCommands.List(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load commands")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, summarizeCommand(r))
	}
	c.JSON(http.StatusOK, gin.H{"commands": out})
}

func (s *Server) handleGetCommand(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("cid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	cmd, err := s.store.CustomCommands.Get(c.Request.Context(), gidInt, id)
	if err != nil {
		fail(c, http.StatusNotFound, "not found")
		return
	}
	c.JSON(http.StatusOK, fullCommand(cmd))
}

type upsertCommandReq struct {
	ID          int64           `json:"id,omitempty"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Enabled     bool            `json:"enabled"`
	Status      string          `json:"status,omitempty"`
	Definition  json.RawMessage `json:"definition"`
}

func (s *Server) handleUpsertCommand(c *gin.Context) {
	var req upsertCommandReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if !commandNameRe.MatchString(req.Name) {
		fail(c, http.StatusBadRequest, "name must be 1-32 chars, lowercase letters/numbers/-/_")
		return
	}
	if req.Description == "" {
		req.Description = "Custom command"
	}
	var def cc.Definition
	if len(req.Definition) > 0 {
		if err := json.Unmarshal(req.Definition, &def); err != nil {
			fail(c, http.StatusBadRequest, "invalid definition json")
			return
		}
	}
	result := cc.Validate(req.Name, def)
	if !result.OK {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation failed", "validation": result})
		return
	}

	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)
	row := store.CustomCommand{
		ID:            req.ID,
		GuildID:       gidInt,
		Name:          req.Name,
		Description:   req.Description,
		Enabled:       req.Enabled,
		Status:        firstNonEmpty(req.Status, string(cc.StatusDraft)),
		Version:       1,
		RequiresDefer: result.RequiresDefer,
		Definition:    req.Definition,
	}
	// If the row already exists keep its version and bump on publish only.
	if req.ID != 0 {
		if existing, err := s.store.CustomCommands.Get(c.Request.Context(), gidInt, req.ID); err == nil {
			row.Version = existing.Version
			if row.Status == string(cc.StatusPublished) && existing.Status != string(cc.StatusPublished) {
				row.Version = existing.Version + 1
			}
		}
	}
	saved, err := s.store.CustomCommands.Upsert(c.Request.Context(), row)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not save command")
		return
	}
	if saved.Status == string(cc.StatusPublished) {
		if err := s.store.CustomCommands.PublishVersion(c.Request.Context(), store.CustomCommandVersion{
			CommandID:  saved.ID,
			Version:    saved.Version,
			Definition: saved.Definition,
		}); err != nil {
			s.log.Warn("publish version", "err", err)
		}
	}
	s.syncGuildCommands(c.Request.Context(), gid, gidInt)
	s.audit(c, gidInt, "command.upsert", gin.H{"name": req.Name, "id": saved.ID, "status": saved.Status})
	c.JSON(http.StatusOK, gin.H{"id": saved.ID, "validation": result})
}

func (s *Server) handleValidateCommand(c *gin.Context) {
	var req upsertCommandReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	var def cc.Definition
	if len(req.Definition) > 0 {
		if err := json.Unmarshal(req.Definition, &def); err != nil {
			fail(c, http.StatusBadRequest, "invalid definition json")
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"validation": cc.Validate(req.Name, def)})
}

func (s *Server) handleDeleteCommand(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("cid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)
	if err := s.store.CustomCommands.Delete(c.Request.Context(), gidInt, id); err != nil {
		fail(c, http.StatusInternalServerError, "could not delete command")
		return
	}
	s.syncGuildCommands(c.Request.Context(), gid, gidInt)
	s.audit(c, gidInt, "command.delete", gin.H{"id": id})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func summarizeCommand(r store.CustomCommand) gin.H {
	return gin.H{
		"id":             r.ID,
		"name":           r.Name,
		"description":    r.Description,
		"enabled":        r.Enabled,
		"status":         r.Status,
		"version":        r.Version,
		"requires_defer": r.RequiresDefer,
		"updated_at":     r.UpdatedAt,
	}
}

func fullCommand(r store.CustomCommand) gin.H {
	out := summarizeCommand(r)
	out["definition"] = json.RawMessage(r.Definition)
	return out
}

// syncGuildCommands rebuilds the guild's slash-command set from all enabled
// custom commands and registers it with Discord. Dia's built-in commands are
// registered globally, so a guild's command slots hold only custom commands.
func (s *Server) syncGuildCommands(ctx context.Context, gid string, gidInt int64) {
	rows, err := s.store.CustomCommands.List(ctx, gidInt)
	if err != nil {
		s.log.Warn("custom command sync: list failed", "guild", gid, "err", err)
		return
	}
	defs := make([]*discordgo.ApplicationCommand, 0, len(rows))
	for _, r := range rows {
		if !r.Enabled {
			continue
		}
		var d cc.Definition
		_ = json.Unmarshal(r.Definition, &d)
		defs = append(defs, &discordgo.ApplicationCommand{
			Type:        discordgo.ChatApplicationCommand,
			Name:        r.Name,
			Description: r.Description,
			Options:     buildSlashOptions(d.Options),
		})
	}
	if _, err := s.discord.BulkOverwriteGuildCommands(gid, defs); err != nil {
		s.log.Warn("custom command sync: register failed", "guild", gid, "err", err)
	}
}

func buildSlashOptions(opts []cc.CommandOption) []*discordgo.ApplicationCommandOption {
	// Discord rejects registrations where a required option follows an optional
	// one; sort stably so a stored out-of-order definition still syncs.
	ordered := make([]cc.CommandOption, len(opts))
	copy(ordered, opts)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Required && !ordered[j].Required
	})
	out := make([]*discordgo.ApplicationCommandOption, 0, len(ordered))
	for _, o := range ordered {
		opt := &discordgo.ApplicationCommandOption{
			Type:         slashOptKind(o.Kind),
			Name:         o.Name,
			Description:  o.Description,
			Required:     o.Required,
			Autocomplete: o.Autocomplete,
			MinValue:     o.MinValue,
		}
		if o.MaxValue != nil {
			opt.MaxValue = *o.MaxValue
		}
		if o.MinLength != nil {
			opt.MinLength = o.MinLength
		}
		if o.MaxLength != nil {
			opt.MaxLength = *o.MaxLength
		}
		if len(o.ChannelTypes) > 0 {
			cts := make([]discordgo.ChannelType, 0, len(o.ChannelTypes))
			for _, ct := range o.ChannelTypes {
				cts = append(cts, discordgo.ChannelType(ct))
			}
			opt.ChannelTypes = cts
		}
		if len(o.Choices) > 0 {
			for _, c := range o.Choices {
				var v interface{}
				_ = json.Unmarshal(c.Value, &v)
				opt.Choices = append(opt.Choices, &discordgo.ApplicationCommandOptionChoice{
					Name:  c.Name,
					Value: v,
				})
			}
		}
		out = append(out, opt)
	}
	return out
}

func slashOptKind(k string) discordgo.ApplicationCommandOptionType {
	switch k {
	case "int", "integer":
		return discordgo.ApplicationCommandOptionInteger
	case "bool", "boolean":
		return discordgo.ApplicationCommandOptionBoolean
	case "user":
		return discordgo.ApplicationCommandOptionUser
	case "role":
		return discordgo.ApplicationCommandOptionRole
	case "channel":
		return discordgo.ApplicationCommandOptionChannel
	case "mentionable":
		return discordgo.ApplicationCommandOptionMentionable
	case "number":
		return discordgo.ApplicationCommandOptionNumber
	case "attachment":
		return discordgo.ApplicationCommandOptionAttachment
	}
	return discordgo.ApplicationCommandOptionString
}

// ── Reaction role menus ──────────────────────────────────────

func (s *Server) handleListMenus(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	rows, err := s.store.ReactionRoles.List(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load menus")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, m := range rows {
		out = append(out, gin.H{
			"id": m.ID, "title": m.Title, "mode": m.Mode,
			"channel_id": event.FormatID(m.ChannelID), "message_id": event.FormatID(m.MessageID),
			"options": json.RawMessage(m.Options),
		})
	}
	c.JSON(http.StatusOK, gin.H{"menus": out})
}

type upsertMenuReq struct {
	ID      int64           `json:"id"`
	Title   string          `json:"title"`
	Mode    string          `json:"mode"`
	Options json.RawMessage `json:"options"`
}

func (s *Server) handleUpsertMenu(c *gin.Context) {
	var req upsertMenuReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Mode == "" {
		req.Mode = "toggle"
	}
	gidInt, _ := event.ParseID(guildID(c))
	menu := store.ReactionRoleMenu{ID: req.ID, GuildID: gidInt, Title: req.Title, Mode: req.Mode, Options: req.Options}
	if req.ID == 0 {
		created, err := s.store.ReactionRoles.Create(c.Request.Context(), menu)
		if err != nil {
			fail(c, http.StatusInternalServerError, "could not create menu")
			return
		}
		s.audit(c, gidInt, "reactionrole.create", gin.H{"id": created.ID})
		c.JSON(http.StatusOK, gin.H{"id": created.ID})
		return
	}
	if err := s.store.ReactionRoles.Update(c.Request.Context(), menu); err != nil {
		fail(c, http.StatusInternalServerError, "could not update menu")
		return
	}
	s.audit(c, gidInt, "reactionrole.update", gin.H{"id": req.ID})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleDeleteMenu(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("mid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	if err := s.store.ReactionRoles.Delete(c.Request.Context(), gidInt, id); err != nil {
		fail(c, http.StatusInternalServerError, "could not delete menu")
		return
	}
	s.audit(c, gidInt, "reactionrole.delete", gin.H{"id": id})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── Moderation ───────────────────────────────────────────────

func (s *Server) handleListCases(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	var userFilter *int64
	if u := c.Query("user"); u != "" {
		if id, ok := event.ParseID(u); ok {
			userFilter = &id
		}
	}
	rows, err := s.store.Moderation.ListCases(c.Request.Context(), gidInt, userFilter, 50, 0)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load cases")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, cse := range rows {
		out = append(out, gin.H{
			"case": cse.CaseNumber, "action": cse.Action,
			"user_id": event.FormatID(cse.UserID), "moderator_id": event.FormatID(cse.ModeratorID),
			"reason": cse.Reason, "created_at": cse.CreatedAt, "expires_at": cse.ExpiresAt, "active": cse.Active,
		})
	}
	c.JSON(http.StatusOK, gin.H{"cases": out})
}
