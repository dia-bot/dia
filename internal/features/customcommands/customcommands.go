// Package customcommands handles invocation of admin-defined per-guild slash
// commands. The commands themselves are created/edited on the dashboard and
// registered with Discord by the API when saved; this worker plugin only
// resolves and renders an invocation (via a dynamic command fallback) and
// provides a /customcommands list management command.
package customcommands

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Plugin implements the custom-commands feature.
type Plugin struct{}

// New returns the custom-commands plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Custom Commands",
		Description: "Admin-defined per-server slash commands with custom text/embed responses.",
		Category:    plugin.CategoryUtility,
	}
}

// Init wires the dynamic command fallback (handles invocation of admin-defined
// commands) and the /customcommands management command.
func (*Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	reg.CommandFallback(func(c *interactions.Context) error {
		return handleInvoke(c, d)
	})

	reg.Command(&interactions.Command{
		Def: interactions.AdminOnly(interactions.Slash("customcommands",
			"Manage this server's custom commands",
			interactions.SubCommand("list", "List this server's custom commands"),
		)),
		Handler: func(c *interactions.Context) error { return handleList(c, d) },
	})
	return nil
}

// handleInvoke resolves the invoked (unknown) application command against the
// guild's custom_commands and renders its configured response.
func handleInvoke(c *interactions.Context, d plugin.Deps) error {
	if c.GuildID == "" {
		return c.RespondEphemeral("Custom commands only work in servers.")
	}
	gid, ok := event.ParseID(c.GuildID)
	if !ok {
		return c.RespondEphemeral("Custom commands only work in servers.")
	}

	cmd, err := d.Store.CustomCommands.GetByName(c.Ctx, gid, c.I.Data.Name)
	if errors.Is(err, store.ErrNotFound) {
		return c.RespondEphemeral("Unknown command.")
	}
	if err != nil {
		return err
	}
	if !cmd.Enabled {
		return c.RespondEphemeral("That command is disabled.")
	}

	var resp Response
	if len(cmd.Response) > 0 {
		if err := json.Unmarshal(cmd.Response, &resp); err != nil {
			return err
		}
	}

	v := vars{user: c.User, server: guildName(c.Ctx, d, gid)}

	data := &discordgo.InteractionResponseData{}
	if resp.Content != "" {
		data.Content = v.apply(resp.Content)
	}
	if resp.Embed != nil {
		embed := &discordgo.MessageEmbed{
			Title:       v.apply(resp.Embed.Title),
			Description: v.apply(resp.Embed.Description),
			Color:       colorInt(resp.Embed.Color, 0xB244FC),
		}
		if url := v.apply(resp.Embed.ImageURL); url != "" {
			embed.Image = &discordgo.MessageEmbedImage{URL: url}
		}
		data.Embeds = []*discordgo.MessageEmbed{embed}
	}
	// Nothing configured — avoid an empty (rejected) response.
	if data.Content == "" && len(data.Embeds) == 0 {
		return c.RespondEphemeral("This command has no response configured yet.")
	}
	if resp.Ephemeral {
		data.Flags = discordgo.MessageFlagsEphemeral
	}
	return c.RespondData(data)
}

// handleList renders the guild's custom commands and their enabled state.
func handleList(c *interactions.Context, d plugin.Deps) error {
	gid, ok := event.ParseID(c.GuildID)
	if !ok {
		return c.RespondEphemeral("Custom commands only work in servers.")
	}
	cmds, err := d.Store.CustomCommands.List(c.Ctx, gid)
	if err != nil {
		return err
	}
	if len(cmds) == 0 {
		return c.RespondEphemeral("No custom commands yet. Create them on the dashboard.")
	}
	sort.Slice(cmds, func(i, j int) bool { return cmds[i].Name < cmds[j].Name })

	var b strings.Builder
	for _, cmd := range cmds {
		state := "🟢 enabled"
		if !cmd.Enabled {
			state = "⚪ disabled"
		}
		b.WriteString("`/")
		b.WriteString(cmd.Name)
		b.WriteString("` — ")
		b.WriteString(state)
		if cmd.Description != "" {
			b.WriteString("\n> ")
			b.WriteString(cmd.Description)
		}
		b.WriteString("\n")
	}

	embed := &discordgo.MessageEmbed{
		Title:       "Custom Commands",
		Description: b.String(),
		Color:       0xB244FC,
		Footer:      &discordgo.MessageEmbedFooter{Text: "Create and edit commands on the dashboard."},
	}
	return c.RespondEmbed(true, embed)
}

func guildName(ctx context.Context, d plugin.Deps, guildID int64) string {
	if g, err := d.Store.Guilds.Get(ctx, guildID); err == nil && g.Name != "" {
		return g.Name
	}
	return "the server"
}

// vars holds the substitution context for response templates.
type vars struct {
	user   event.User
	server string
}

func (v vars) apply(s string) string {
	if s == "" {
		return ""
	}
	name := v.user.GlobalName
	if name == "" {
		name = v.user.Username
	}
	return strings.NewReplacer(
		"{user.mention}", "<@"+v.user.ID+">",
		"{username}", v.user.Username,
		"{user}", name,
		"{server}", v.server,
	).Replace(s)
}

// colorInt converts a #RRGGBB string to a Discord embed color int.
func colorInt(hex string, fallback int) int {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return fallback
	}
	n, err := strconv.ParseInt(hex, 16, 32)
	if err != nil {
		return fallback
	}
	return int(n)
}
