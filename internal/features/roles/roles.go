// Package roles provides autorole (roles granted automatically when a member
// joins) and reaction/self-assign role menus (buttons or a string select the
// bot posts and reacts to). Menu definitions live in the reaction_role_menus
// table (authored on the dashboard); this feature posts them and handles clicks.
package roles

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Plugin implements the roles feature.
type Plugin struct{}

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
func (*Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	reg.OnEvent(event.TypeMemberAdd, func(ctx context.Context, env *event.Envelope) error {
		return handleMemberAdd(ctx, d, env)
	})
	reg.OnEvent(event.TypeMemberUpdate, func(ctx context.Context, env *event.Envelope) error {
		return handleMemberUpdate(ctx, d, env)
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

func handleMemberAdd(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
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
	return applyAutoroles(ctx, d, ma.GuildID, ma.Member.User.ID, cfg.Roles)
}

func handleMemberUpdate(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
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
	return applyAutoroles(ctx, d, mu.GuildID, mu.Member.User.ID, cfg.Roles)
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

	menu, menuOpts, err := loadMenu(c.Ctx, d, menuID)
	if err != nil {
		return c.RespondEphemeral("No menu found with ID " + strconv.FormatInt(menuID, 10) + ".")
	}
	gid, _ := event.ParseID(c.GuildID)
	if menu.GuildID != gid {
		return c.RespondEphemeral("That menu belongs to another server.")
	}
	if len(menuOpts) == 0 {
		return c.RespondEphemeral("That menu has no options yet — add some on the dashboard first.")
	}

	send := &discordgo.MessageSend{
		Components: buildComponents(menu, menuOpts),
	}
	if menu.Title != "" {
		send.Embeds = []*discordgo.MessageEmbed{{
			Title:       menu.Title,
			Description: menuDescription(menuOpts),
			Color:       0xB244FC,
		}}
	} else {
		send.Content = "Pick your roles:"
	}

	msg, err := d.Discord.SendMessage(channelID, send)
	if err != nil {
		return c.RespondEphemeral("Failed to post the menu: " + err.Error())
	}
	chID, _ := event.ParseID(msg.ChannelID)
	if chID == 0 {
		chID, _ = event.ParseID(channelID)
	}
	msgID, _ := event.ParseID(msg.ID)
	if err := d.Store.ReactionRoles.SetMessage(c.Ctx, menu.ID, chID, msgID); err != nil {
		return err
	}
	return c.RespondEphemeral(fmt.Sprintf("Posted menu #%d to <#%s>.", menu.ID, channelID))
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
	menu, err := d.Store.ReactionRoles.Get(ctx, id)
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
