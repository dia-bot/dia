package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/roles"
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
	// Usage stats are decoration: the list still serves without them.
	stats, err := s.store.CommandRuns.GuildRunStats(c.Request.Context(), gidInt)
	if err != nil {
		s.log.Warn("custom command run stats failed", "guild", gidInt, "err", err)
		stats = nil
	}
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		h := summarizeCommand(r)
		addShapeFields(h, r.Definition)
		if st, ok := stats[r.ID]; ok {
			h["runs_24h"] = st.Runs24h
			h["last_run_at"] = st.LastRunAt
		} else if stats != nil {
			h["runs_24h"] = 0
			h["last_run_at"] = nil
		}
		out = append(out, h)
	}
	c.JSON(http.StatusOK, gin.H{"commands": out})
}

func (s *Server) handleGetCommand(c *gin.Context) {
	id := c.Param("cid")
	if id == "" {
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
	ID          string          `json:"id,omitempty"`
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
	if req.ID != "" {
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
	s.syncGuildCommandsAsync(gid, gidInt)
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
	id := c.Param("cid")
	if id == "" {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)
	if err := s.store.CustomCommands.Delete(c.Request.Context(), gidInt, id); err != nil {
		fail(c, http.StatusInternalServerError, "could not delete command")
		return
	}
	s.syncGuildCommandsAsync(gid, gidInt)
	s.audit(c, gidInt, "command.delete", gin.H{"id": id})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── Command groups (organizational folders) ────────────────

func groupJSON(g store.CommandGroup) gin.H {
	return gin.H{"id": g.ID, "name": g.Name, "position": g.Position, "created_at": g.CreatedAt}
}

func (s *Server) handleListCommandGroups(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	rows, err := s.store.CommandGroups.List(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load groups")
		return
	}
	out := make([]gin.H, 0, len(rows))
	for _, g := range rows {
		out = append(out, groupJSON(g))
	}
	c.JSON(http.StatusOK, gin.H{"groups": out})
}

type groupReq struct {
	Name string `json:"name"`
}

func cleanGroupName(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 40 {
		s = s[:40]
	}
	return s
}

func (s *Server) handleCreateCommandGroup(c *gin.Context) {
	var req groupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	name := cleanGroupName(req.Name)
	if name == "" {
		fail(c, http.StatusBadRequest, "group name required")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	g, err := s.store.CommandGroups.Create(c.Request.Context(), gidInt, name)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not create group")
		return
	}
	s.audit(c, gidInt, "command_group.create", gin.H{"id": g.ID, "name": name})
	c.JSON(http.StatusOK, groupJSON(g))
}

func (s *Server) handleRenameCommandGroup(c *gin.Context) {
	id := c.Param("gid")
	if id == "" {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req groupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	name := cleanGroupName(req.Name)
	if name == "" {
		fail(c, http.StatusBadRequest, "group name required")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	if err := s.store.CommandGroups.Rename(c.Request.Context(), gidInt, id, name); err != nil {
		fail(c, http.StatusNotFound, "group not found")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *Server) handleDeleteCommandGroup(c *gin.Context) {
	id := c.Param("gid")
	if id == "" {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	if err := s.store.CommandGroups.Delete(c.Request.Context(), gidInt, id); err != nil {
		fail(c, http.StatusInternalServerError, "could not delete group")
		return
	}
	s.audit(c, gidInt, "command_group.delete", gin.H{"id": id})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type reorderGroupsReq struct {
	IDs []string `json:"ids"`
}

func (s *Server) handleReorderCommandGroups(c *gin.Context) {
	var req reorderGroupsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	if err := s.store.CommandGroups.Reorder(c.Request.Context(), gidInt, req.IDs); err != nil {
		fail(c, http.StatusInternalServerError, "could not reorder groups")
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type setGroupReq struct {
	GroupID *string `json:"group_id"`
}

func (s *Server) handleSetCommandGroup(c *gin.Context) {
	id := c.Param("cid")
	if id == "" {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req setGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	// A non-nil group must belong to this guild.
	if req.GroupID != nil {
		groups, err := s.store.CommandGroups.List(c.Request.Context(), gidInt)
		if err != nil {
			fail(c, http.StatusInternalServerError, "could not verify group")
			return
		}
		ok := false
		for _, g := range groups {
			if g.ID == *req.GroupID {
				ok = true
				break
			}
		}
		if !ok {
			fail(c, http.StatusBadRequest, "unknown group")
			return
		}
	}
	if err := s.store.CustomCommands.SetGroup(c.Request.Context(), gidInt, id, req.GroupID); err != nil {
		fail(c, http.StatusNotFound, "command not found")
		return
	}
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
		"group_id":       r.GroupID,
		"updated_at":     r.UpdatedAt,
	}
}

func fullCommand(r store.CustomCommand) gin.H {
	out := summarizeCommand(r)
	out["definition"] = json.RawMessage(r.Definition)
	return out
}

// ── Flow shape (the overview's miniature canvases) ──────────

// shapeNode is the thumbnail's entire input; layout happens client-side.
type shapeNode struct {
	K string        `json:"k"`           // step kind
	C [][]shapeNode `json:"c,omitempty"` // one array per non-empty control branch, in order
	E bool          `json:"e,omitempty"` // has an on-error router
}

const (
	shapeMaxNodes = 40
	shapeMaxDepth = 6
)

// addShapeFields derives the overview's cheap structural fields from the
// definition JSONB: total step count, slash property count, and a compact
// capped shape for the flow thumbnail.
func addShapeFields(h gin.H, raw json.RawMessage) {
	var def cc.Definition
	if len(raw) == 0 || json.Unmarshal(raw, &def) != nil {
		return
	}
	b := &shapeBuilder{}
	shape := b.walk(def.Steps, 0)
	h["step_count"] = countShapeSteps(def.Steps)
	h["option_count"] = len(def.Options)
	h["flow_shape"] = shape
	h["shape_more"] = b.dropped
}

type shapeBuilder struct {
	nodes   int
	dropped int
}

func (b *shapeBuilder) walk(steps []cc.Step, depth int) []shapeNode {
	out := make([]shapeNode, 0, len(steps))
	for i := range steps {
		s := &steps[i]
		if b.nodes >= shapeMaxNodes || depth >= shapeMaxDepth {
			// Count only what the thumbnail WOULD draw (control branches, not
			// error-handler bodies), so "+n more" matches the missing nodes.
			b.dropped += countDrawable(steps[i:])
			break
		}
		b.nodes++
		n := shapeNode{
			K: s.Kind,
			E: s.OnError != nil || len(s.OnErrorCases) > 0,
		}
		for _, br := range branchesOf(s) {
			if len(br) == 0 {
				continue
			}
			n.C = append(n.C, b.walk(br, depth+1))
		}
		out = append(out, n)
	}
	return out
}

// branchesOf lists a step's control branches in display order. Error
// handlers are not branches here; they surface as the dashed rail flag.
func branchesOf(s *cc.Step) [][]cc.Step {
	switch s.Kind {
	case cc.KindIf:
		return [][]cc.Step{s.Then, s.Else}
	case cc.KindSwitch:
		out := make([][]cc.Step, 0, len(s.Cases)+1)
		for _, cse := range s.Cases {
			out = append(out, cse.Do)
		}
		return append(out, s.Default)
	case cc.KindLoop:
		return [][]cc.Step{s.Then}
	case cc.KindParallel:
		var ps cc.SpecParallel
		if len(s.Spec) > 0 && json.Unmarshal(s.Spec, &ps) == nil {
			return ps.Branches
		}
	}
	return nil
}

// countShapeSteps counts every step in the live tree, error-handler bodies
// included (scratch is the caller's concern and excluded by construction).
func countShapeSteps(steps []cc.Step) int {
	n := 0
	for i := range steps {
		s := &steps[i]
		n++
		for _, br := range branchesOf(s) {
			n += countShapeSteps(br)
		}
		n += countShapeSteps(s.OnError)
		for _, ec := range s.OnErrorCases {
			n += countShapeSteps(ec.Do)
		}
	}
	return n
}

// countDrawable counts the nodes the thumbnail walk would emit: control
// branches only, error handlers surface as a flag, never their own nodes.
func countDrawable(steps []cc.Step) int {
	n := 0
	for i := range steps {
		s := &steps[i]
		n++
		for _, br := range branchesOf(s) {
			n += countDrawable(br)
		}
	}
	return n
}

// syncGuildCommands rebuilds the guild's slash-command set from all enabled
// custom commands and registers it with Discord. Dia's built-in commands are
// registered globally, so a guild's command slots hold only custom commands.
// syncGuildCommandsAsync registers the guild's slash commands with Discord
// WITHOUT blocking the HTTP response: a save/publish must not hang on a slow
// or unreachable Discord API (that surfaced as a 10s client timeout). The
// registration runs on its own bounded context and only logs failures, which
// matches the synchronous version's error handling.
func (s *Server) syncGuildCommandsAsync(gid string, gidInt int64) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("custom command sync: panic", "guild", gid, "err", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.syncGuildCommands(ctx, gid, gidInt)
	}()
}

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

type menuActionsReq struct {
	Tail []cc.Step `json:"tail"` // the post-pick follow-up flow
}

// handleMenuActions persists the canvas-authored follow-up flow for one
// reaction-role menu's built-in automation ("connect a new action after the
// roles are applied"). Only the menu's tail is written (via SetTail); the
// title, mode and options stay authoritative from the reaction-roles page,
// whose upsert never carries the tail. The tail is validated as an event flow
// (it runs on the bare pick event), so an unrunnable step is rejected here.
// Mirrors handleAutoroleActions, but stores per menu row instead of in the
// feature config.
func (s *Server) handleMenuActions(c *gin.Context) {
	menuID, err := strconv.ParseInt(c.Param("mid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req menuActionsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if res := cc.ValidateEventFlow(cc.Definition{Steps: req.Tail}, false); !res.OK {
		fail(c, http.StatusBadRequest, "the follow-up flow has an invalid step: "+firstValidationError(res))
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	menu, err := s.store.ReactionRoles.Get(c.Request.Context(), menuID)
	if err != nil || menu.GuildID != gidInt {
		fail(c, http.StatusNotFound, "menu not found")
		return
	}
	if req.Tail == nil {
		req.Tail = []cc.Step{} // marshal to [], never null
	}
	raw, err := json.Marshal(req.Tail)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not encode flow")
		return
	}
	if err := s.store.ReactionRoles.SetTail(c.Request.Context(), gidInt, menuID, raw); err != nil {
		fail(c, http.StatusInternalServerError, "could not save")
		return
	}
	s.audit(c, gidInt, "reactionrole.actions", gin.H{"id": menuID})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

type postMenuReq struct {
	ChannelID string `json:"channel_id"`
}

// handlePostMenu posts a saved reaction-role menu to a channel from the
// dashboard, reusing the same build + send + record path as /reactionroles post.
func (s *Server) handlePostMenu(c *gin.Context) {
	menuID, err := strconv.ParseInt(c.Param("mid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid id")
		return
	}
	var req postMenuReq
	if err := c.ShouldBindJSON(&req); err != nil || req.ChannelID == "" {
		fail(c, http.StatusBadRequest, "channel_id is required")
		return
	}
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)
	msgID, err := roles.PostMenu(c.Request.Context(), s.discord, s.store, gid, req.ChannelID, menuID)
	if err != nil {
		switch {
		case errors.Is(err, roles.ErrMenuWrongGuild), errors.Is(err, roles.ErrMenuNoOptions):
			fail(c, http.StatusBadRequest, err.Error())
		case errors.Is(err, store.ErrNotFound):
			fail(c, http.StatusNotFound, "menu not found")
		default:
			fail(c, http.StatusBadGateway, "could not post menu: "+err.Error())
		}
		return
	}
	s.audit(c, gidInt, "reactionrole.post", gin.H{"menu": menuID, "channel": req.ChannelID})
	c.JSON(http.StatusOK, gin.H{"ok": true, "message_id": msgID})
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
