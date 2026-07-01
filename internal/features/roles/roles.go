// Package roles provides autorole (roles granted automatically when a member
// joins) and reaction/self-assign role menus (buttons or a string select the
// bot posts and reacts to). Menu definitions live in the reaction_role_menus
// table (authored on the dashboard); this feature posts them and handles clicks.
package roles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/automations/runner"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Sentinel errors PostMenu returns so callers can map them to the right response.
var (
	ErrMenuWrongGuild = errors.New("menu belongs to another server")
	ErrMenuNoOptions  = errors.New("menu has no options")
)

// Plugin implements the roles feature.
type Plugin struct {
	// runner runs the durable post-grant follow-up flow auto-roles owns
	// (Config.Tail) on the shared automations machinery: it persists a parked run
	// to automation_runs and emits "auto:" components, so any waits, modals and
	// follow-up clicks resume through the automations plugin's handlers + the wait
	// scheduler. Auto-roles sends no message of its own — it grants the configured
	// roles, then hands off to this tail.
	runner *runner.Runner
}

// New returns the roles plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Roles",
		Description: "Automatically assign roles on join and let members self-assign roles via buttons or menus.",
		Category:    plugin.CategoryEngagement,
	}
}

// Init wires the autorole join/update handlers, the reaction-role component
// handlers and the /reactionroles admin command.
func (p *Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	p.runner = runner.New(d)

	reg.OnEvent(event.TypeMemberAdd, func(ctx context.Context, env *event.Envelope) error {
		return p.handleMemberAdd(ctx, d, env)
	})
	reg.OnEvent(event.TypeMemberUpdate, func(ctx context.Context, env *event.Envelope) error {
		return p.handleMemberUpdate(ctx, d, env)
	})

	// All reaction-role components share the "rr:" prefix; one handler routes
	// buttons vs. selects by their custom_id.
	reg.Component(componentPrefix, func(c *interactions.Context) error {
		return handleComponent(c, d)
	})

	reg.Command(&interactions.Command{
		Def: interactions.AdminOnly(interactions.Slash("reactionroles",
			"Manage self-assign reaction-role menus",
			interactions.SubCommand("list", "List this server's reaction-role menus"),
			interactions.SubCommand("post", "Post a menu to a channel",
				interactions.IntOpt("id", "Menu ID (see /reactionroles list)", true),
				interactions.ChannelOpt("channel", "Channel to post the menu in", true),
			),
			interactions.SubCommand("delete", "Delete a menu",
				interactions.IntOpt("id", "Menu ID (see /reactionroles list)", true),
			),
		)),
		Handler: func(c *interactions.Context) error { return handleCommand(c, d) },
	})
	return nil
}

// ── Autorole events ──────────────────────────────────────────

func (p *Plugin) handleMemberAdd(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	ma, err := plugin.DecodeData[event.MemberAdd](env)
	if err != nil {
		return err
	}
	gid, _ := event.ParseID(ma.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || !enabled {
		return err
	}
	if len(cfg.Roles) == 0 {
		return nil
	}
	if ma.Member.Pending && cfg.WaitForScreening {
		// Member still behind membership screening; grant later on update.
		return nil
	}
	if ma.Member.User.Bot && !cfg.IncludeBots {
		return nil
	}
	if err := applyAutoroles(ctx, d, ma.GuildID, ma.Member.User.ID, cfg.Roles); err != nil {
		return err
	}
	// Run the post-grant follow-up flow once the roles are on. The scope mirrors
	// an automations member_join run — same .User / .Member / .Guild / .Event
	// vars (member_count + pending) — so a tail authored on the canvas behaves
	// exactly like a hand-built member_join automation.
	p.runTail(ctx, d, cfg, ma.GuildID, &ma.Member,
		map[string]any{"member_count": ma.MemberCount, "pending": ma.Member.Pending})
	return nil
}

func (p *Plugin) handleMemberUpdate(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	mu, err := plugin.DecodeData[event.MemberUpdate](env)
	if err != nil {
		return err
	}
	gid, _ := event.ParseID(mu.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || !enabled {
		return err
	}
	// Only relevant when waiting for screening: grant once the member has
	// finished membership screening (no longer pending).
	if !cfg.WaitForScreening || mu.Member.Pending || len(cfg.Roles) == 0 {
		return nil
	}
	if mu.Member.User.Bot && !cfg.IncludeBots {
		return nil
	}
	if err := applyAutoroles(ctx, d, mu.GuildID, mu.Member.User.ID, cfg.Roles); err != nil {
		return err
	}
	// Same member_join scope as the join path: screening has completed, so the
	// member is no longer pending. MemberUpdate carries no member count, so it's
	// filled from the guild store (0 when unavailable).
	p.runTail(ctx, d, cfg, mu.GuildID, &mu.Member,
		map[string]any{"member_count": guildMemberCount(ctx, d, gid), "pending": false})
	return nil
}

// runTail runs the post-grant follow-up flow (cfg.Tail) as a durable automation
// run once the configured roles have been granted. Labelled "autorole.join" with
// TriggerKind "member_join" so it shares the flow's KV scope and Runs filter and
// reads like the built-in automation the canvas shows. Nothing runs (and nothing
// persists) when the tail is empty.
func (p *Plugin) runTail(ctx context.Context, d plugin.Deps, cfg Config, guildID string, member *event.Member, eventMap map[string]any) {
	if len(cfg.Tail) == 0 || p.runner == nil || member == nil {
		return
	}
	gid, _ := event.ParseID(guildID)
	guildCtx := cc.ContextGuild{ID: guildID, Name: "the server"}
	if g, err := d.Store.Guilds.Get(ctx, gid); err == nil {
		if g.Name != "" {
			guildCtx.Name = g.Name
		}
		guildCtx.MemberCount = g.MemberCount
	}
	// Auto-roles posts no message, so there's no anchoring channel; the tail's
	// .Channel.* falls back to the member's context. BuildContext tolerates an
	// empty channel id.
	ctxVars := cc.BuildContext(guildID, "", member.User, member, guildCtx, time.Now().UnixMilli())
	scope := cc.NewScope(d.GuildState, guildID, ctxVars, nil, nil)
	scope.SetEvent(eventMap)
	p.runner.Start(ctx, runner.Meta{
		AutomationID: "autorole.join",
		Version:      1,
		GuildID:      guildID,
		InvokerID:    member.User.ID,
		ActorID:      member.User.ID,
		TriggerKind:  "member_join",
	}, cc.Definition{Steps: cfg.Tail}, scope)
}

// guildMemberCount reads the guild's cached member count (0 when unavailable),
// so the screening-completed member_update path can present the same
// member_count .Event var as the join path.
func guildMemberCount(ctx context.Context, d plugin.Deps, gid int64) int {
	if g, err := d.Store.Guilds.Get(ctx, gid); err == nil {
		return g.MemberCount
	}
	return 0
}

// MergeStoredActions returns the incoming auto-roles config JSON with the
// canvas-owned field (the post-grant follow-up flow, Tail) replaced by the
// stored config's. The auto-roles settings page owns the roles list and toggles;
// the follow-up flow is owned by the automation canvas (saved via
// /autorole/actions), so a settings save must not overwrite it with its
// (possibly stale, or absent) copy. On any decode/encode error the incoming
// config is returned unchanged.
func MergeStoredActions(incoming, stored []byte) []byte {
	var in, st Config
	if json.Unmarshal(incoming, &in) != nil || json.Unmarshal(stored, &st) != nil {
		return incoming
	}
	in.Tail = st.Tail
	out, err := json.Marshal(in)
	if err != nil {
		return incoming
	}
	return out
}

// applyAutoroles grants each configured role, collecting (but not aborting on)
// per-role errors.
func applyAutoroles(ctx context.Context, d plugin.Deps, guildID, userID string, roles []string) error {
	var errs []error
	for _, role := range roles {
		if role == "" {
			continue
		}
		if err := d.Discord.AddRole(guildID, userID, role, "autorole"); err != nil {
			errs = append(errs, fmt.Errorf("add role %s: %w", role, err))
		}
	}
	return errors.Join(errs...)
}

// ── Reaction-role components ─────────────────────────────────

func handleComponent(c *interactions.Context, d plugin.Deps) error {
	customID := c.CustomID()
	switch {
	case strings.HasPrefix(customID, buttonPrefix):
		return handleButton(c, d, customID)
	case strings.HasPrefix(customID, selectPrefix):
		return handleSelect(c, d, customID)
	default:
		return nil // stale / unknown component
	}
}

func handleButton(c *interactions.Context, d plugin.Deps, customID string) error {
	menuID, roleID, ok := parseButtonID(customID)
	if !ok {
		return c.RespondEphemeral("That button is no longer valid.")
	}
	menu, opts, err := loadMenu(c.Ctx, d, menuID)
	if err != nil {
		return c.RespondEphemeral("That menu no longer exists.")
	}
	if _, ok := optionByRole(opts, roleID); !ok {
		return c.RespondEphemeral("That role is no longer part of this menu.")
	}
	added, removed, err := applyMode(c, d, menu, opts, []string{roleID})
	if err != nil {
		return err
	}
	return c.RespondEphemeral(changeSummary(added, removed))
}

func handleSelect(c *interactions.Context, d plugin.Deps, customID string) error {
	menuID, ok := parseSelectID(customID)
	if !ok {
		return c.RespondEphemeral("That menu is no longer valid.")
	}
	menu, opts, err := loadMenu(c.Ctx, d, menuID)
	if err != nil {
		return c.RespondEphemeral("That menu no longer exists.")
	}
	// Keep only the selected values that actually belong to this menu.
	var chosen []string
	for _, v := range c.ComponentValues() {
		if _, ok := optionByRole(opts, v); ok {
			chosen = append(chosen, v)
		}
	}
	added, removed, err := applyMode(c, d, menu, opts, chosen)
	if err != nil {
		return err
	}
	return c.RespondEphemeral(changeSummary(added, removed))
}

// applyMode mutates the invoking member's roles according to the menu mode and
// returns the role IDs added and removed. The member's current roles come from
// the interaction (c.I.Member.Roles).
func applyMode(c *interactions.Context, d plugin.Deps, menu store.ReactionRoleMenu, opts []Option, chosen []string) (added, removed []string, err error) {
	current := map[string]bool{}
	if c.I.Member != nil {
		for _, r := range c.I.Member.Roles {
			current[r] = true
		}
	}
	chosenSet := map[string]bool{}
	for _, r := range chosen {
		chosenSet[r] = true
	}

	guildID := c.GuildID
	userID := userIDOf(c)

	add := func(roleID string) {
		if current[roleID] {
			return
		}
		if e := d.Discord.AddRole(guildID, userID, roleID, "reaction role"); e != nil {
			err = errors.Join(err, e)
			return
		}
		current[roleID] = true
		added = append(added, roleID)
	}
	remove := func(roleID string) {
		if !current[roleID] {
			return
		}
		if e := d.Discord.RemoveRole(guildID, userID, roleID, "reaction role"); e != nil {
			err = errors.Join(err, e)
			return
		}
		delete(current, roleID)
		removed = append(removed, roleID)
	}

	switch menu.Mode {
	case ModeUnique:
		// Remove the menu's other option roles, then add the chosen ones.
		for _, o := range opts {
			if !chosenSet[o.RoleID] {
				remove(o.RoleID)
			}
		}
		for _, roleID := range chosen {
			add(roleID)
		}
	case ModeVerify:
		// Only ever add.
		for _, roleID := range chosen {
			add(roleID)
		}
	default: // ModeToggle
		for _, roleID := range chosen {
			if current[roleID] {
				remove(roleID)
			} else {
				add(roleID)
			}
		}
	}
	return added, removed, err
}

func userIDOf(c *interactions.Context) string {
	if c.User.ID != "" {
		return c.User.ID
	}
	if c.I.Member != nil {
		return c.I.Member.User.ID
	}
	return ""
}

func changeSummary(added, removed []string) string {
	var b strings.Builder
	if len(added) > 0 {
		b.WriteString("Added " + mentionRoles(added))
	}
	if len(removed) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Removed " + mentionRoles(removed))
	}
	if b.Len() == 0 {
		return "No changes — your roles are already up to date."
	}
	return b.String()
}

func mentionRoles(ids []string) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, "<@&"+id+">")
	}
	return strings.Join(parts, ", ")
}

// ── /reactionroles command ───────────────────────────────────

func handleCommand(c *interactions.Context, d plugin.Deps) error {
	sub := c.Subcommand()
	if len(sub) == 0 {
		return c.RespondEphemeral("Unknown subcommand.")
	}
	switch sub[0] {
	case "list":
		return handleList(c, d)
	case "post":
		return handlePost(c, d)
	case "delete":
		return handleDelete(c, d)
	default:
		return c.RespondEphemeral("Unknown subcommand.")
	}
}

func handleList(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	menus, err := d.Store.ReactionRoles.List(c.Ctx, gid)
	if err != nil {
		return err
	}
	if len(menus) == 0 {
		return c.RespondEphemeral("No reaction-role menus yet. Create one on the dashboard, then post it with `/reactionroles post`.")
	}
	embed := &discordgo.MessageEmbed{
		Title: "Reaction-role menus",
		Color: 0xB244FC,
	}
	for _, m := range menus {
		opts, _ := decodeOptions(m.Options)
		title := m.Title
		if title == "" {
			title = "(untitled)"
		}
		var val strings.Builder
		fmt.Fprintf(&val, "Mode: `%s` · %d option(s)", modeLabel(m.Mode), len(opts))
		if m.MessageID != 0 && m.ChannelID != 0 {
			fmt.Fprintf(&val, "\n[Posted message](%s)", messageLink(m.GuildID, m.ChannelID, m.MessageID))
		} else {
			val.WriteString("\nNot posted yet")
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:  fmt.Sprintf("#%d — %s", m.ID, title),
			Value: val.String(),
		})
	}
	return c.RespondEmbed(true, embed)
}

func handlePost(c *interactions.Context, d plugin.Deps) error {
	opts := c.Options()
	menuID := opts.Int("id")
	channelID := opts.Snowflake("channel")
	if channelID == "" {
		return c.RespondEphemeral("Please choose a channel to post the menu in.")
	}

	_, err := PostMenu(c.Ctx, d.Discord, d.Store, c.GuildID, channelID, menuID)
	switch {
	case errors.Is(err, ErrMenuWrongGuild):
		return c.RespondEphemeral("That menu belongs to another server.")
	case errors.Is(err, ErrMenuNoOptions):
		return c.RespondEphemeral("That menu has no options yet — add some on the dashboard first.")
	case errors.Is(err, store.ErrNotFound):
		return c.RespondEphemeral("No menu found with ID " + strconv.FormatInt(menuID, 10) + ".")
	case err != nil:
		return c.RespondEphemeral("Failed to post the menu: " + err.Error())
	}
	return c.RespondEphemeral(fmt.Sprintf("Posted menu #%d to <#%s>.", menuID, channelID))
}

// PostMenu builds and sends a reaction-role menu to channelID, then records the
// posted message id. It is guild-scoped: a menu owned by another guild is refused
// (ErrMenuWrongGuild) before anything is sent. Shared by the /reactionroles post
// command and the dashboard post endpoint.
func PostMenu(ctx context.Context, dc *discord.Client, st *store.Store, guildID, channelID string, menuID int64) (string, error) {
	menu, opts, err := loadMenuFromStore(ctx, st, menuID)
	if err != nil {
		return "", err
	}
	gid, _ := event.ParseID(guildID)
	if menu.GuildID != gid {
		return "", ErrMenuWrongGuild
	}
	if len(opts) == 0 {
		return "", ErrMenuNoOptions
	}
	msg, err := dc.SendMessage(channelID, buildMenuMessage(menu, opts))
	if err != nil {
		return "", err
	}
	chID, _ := event.ParseID(msg.ChannelID)
	if chID == 0 {
		chID, _ = event.ParseID(channelID)
	}
	msgID, _ := event.ParseID(msg.ID)
	if err := st.ReactionRoles.SetMessage(ctx, menu.ID, chID, msgID); err != nil {
		return "", err
	}
	return msg.ID, nil
}

func handleDelete(c *interactions.Context, d plugin.Deps) error {
	menuID := c.Options().Int("id")
	gid, _ := event.ParseID(c.GuildID)
	if err := d.Store.ReactionRoles.Delete(c.Ctx, gid, menuID); err != nil {
		return err
	}
	return c.RespondEphemeral(fmt.Sprintf("Deleted menu #%d (any posted message is left in place).", menuID))
}

// ── helpers ──────────────────────────────────────────────────

// loadMenu fetches a menu and decodes its options.
func loadMenu(ctx context.Context, d plugin.Deps, id int64) (store.ReactionRoleMenu, []Option, error) {
	return loadMenuFromStore(ctx, d.Store, id)
}

// loadMenuFromStore is the store-only variant of loadMenu, so callers without
// plugin.Deps (the dashboard API) can reuse the same load + decode path.
func loadMenuFromStore(ctx context.Context, st *store.Store, id int64) (store.ReactionRoleMenu, []Option, error) {
	menu, err := st.ReactionRoles.Get(ctx, id)
	if err != nil {
		return store.ReactionRoleMenu{}, nil, err
	}
	opts, err := decodeOptions(menu.Options)
	if err != nil {
		return menu, nil, err
	}
	return menu, opts, nil
}

func menuDescription(opts []Option) string {
	var b strings.Builder
	for _, o := range opts {
		if o.Emoji != "" {
			b.WriteString(o.Emoji + " ")
		}
		b.WriteString("<@&" + o.RoleID + ">")
		if o.Description != "" {
			b.WriteString(" — " + o.Description)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func modeLabel(mode string) string {
	switch mode {
	case ModeUnique, ModeVerify, ModeToggle:
		return mode
	default:
		return ModeToggle
	}
}

func messageLink(guildID, channelID, messageID int64) string {
	return fmt.Sprintf("https://discord.com/channels/%d/%d/%d", guildID, channelID, messageID)
}
