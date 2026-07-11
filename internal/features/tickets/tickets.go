package tickets

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/automations/runner"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Plugin implements the ticketing feature.
type Plugin struct {
	// runner launches the optional per-category open/close automations on the
	// shared durable machinery (waits, modals, follow-up clicks resume through the
	// automations plugin), exactly like welcome/roles use it.
	runner *runner.Runner
}

// New returns the tickets plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Tickets",
		Description: "Fully customizable support tickets: panels, private channels or threads, claiming, transcripts, ratings and auto-close.",
		Category:    plugin.CategoryModeration,
	}
}

// Init wires the panel/ticket component + modal handlers, the activity tracker,
// the inactivity sweeper worker and the /ticket + /tickets commands.
func (p *Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	p.runner = runner.New(d)

	reg.Component(componentPrefix, func(c *interactions.Context) error { return p.handleComponent(c, d) })
	reg.Modal(componentPrefix, func(c *interactions.Context) error { return p.handleModal(c, d) })
	reg.OnEvent(event.TypeMessageCreate, func(ctx context.Context, env *event.Envelope) error {
		return p.handleMessage(ctx, d, env)
	})
	reg.Worker("tickets-autoclose", func(ctx context.Context) { p.autoCloseLoop(ctx, d) })

	reg.Command(&interactions.Command{
		Def: interactions.Slash("ticket", "Manage the current ticket",
			interactions.SubCommand("close", "Close this ticket",
				interactions.StringOpt("reason", "Why the ticket is being closed", false)),
			interactions.SubCommand("closerequest", "Ask the opener to confirm closing this ticket",
				interactions.StringOpt("reason", "Why the ticket should be closed", false),
				interactions.WithChoices(interactions.IntOpt("delay", "Close automatically after this long unless the opener objects", false),
					closeRequestDelayChoices()...)),
			interactions.SubCommand("claim", "Claim this ticket so members know who's helping"),
			interactions.SubCommand("unclaim", "Release your claim on this ticket"),
			interactions.SubCommand("add", "Add a member to this ticket",
				interactions.UserOpt("user", "The member to add", true)),
			interactions.SubCommand("remove", "Remove a member from this ticket",
				interactions.UserOpt("user", "The member to remove", true)),
			interactions.SubCommand("rename", "Rename this ticket channel",
				interactions.StringOpt("name", "The new name", true)),
			interactions.SubCommand("note", "Add a private staff note (not shown to the opener)",
				interactions.StringOpt("text", "The note", true)),
			interactions.SubCommand("transcript", "Generate a transcript of this ticket now"),
		),
		Handler: func(c *interactions.Context) error { return p.handleTicketCommand(c, d) },
	})

	reg.Command(&interactions.Command{
		Def: interactions.AdminOnly(interactions.Slash("tickets", "Manage ticket panels",
			interactions.SubCommand("list", "List this server's ticket panels"),
			interactions.SubCommand("post", "Post a ticket panel to a channel",
				interactions.WithAutocomplete(interactions.StringOpt("panel", "Which panel to post", true)),
				interactions.ChannelOpt("channel", "Channel to post the panel in", true)),
		)),
		Handler:      func(c *interactions.Context) error { return p.handleTicketsCommand(c, d) },
		Autocomplete: func(c *interactions.Context) error { return p.autocompletePanel(c, d) },
	})
	return nil
}

// ── Component / modal dispatch ───────────────────────────────

func (p *Plugin) handleComponent(c *interactions.Context, d plugin.Deps) error {
	action, args := parseID(c.CustomID())
	switch action {
	case "open":
		if len(args) < 2 {
			return c.DeferUpdate()
		}
		return p.handleOpen(c, d, args[0], args[1])
	case "sel":
		if len(args) < 1 {
			return c.DeferUpdate()
		}
		vals := c.ComponentValues()
		if len(vals) == 0 {
			return c.DeferUpdate()
		}
		return p.handleOpen(c, d, args[0], vals[0])
	case "close":
		return p.handleCloseButton(c, d, arg0(args))
	case "claim":
		return p.handleClaim(c, d, arg0(args), true)
	case "unclaim":
		return p.handleClaim(c, d, arg0(args), false)
	case "reopen":
		return p.handleReopen(c, d, arg0(args))
	case "delete":
		return p.handleDelete(c, d, arg0(args))
	case "crok":
		return p.handleCloseReqAccept(c, d, arg0(args))
	case "crno":
		return p.handleCloseReqDeny(c, d, arg0(args))
	case "act":
		if len(args) < 2 {
			return c.DeferUpdate()
		}
		return p.handleActionButton(c, d, args[0], args[1])
	case "pact":
		if len(args) < 2 {
			return c.DeferUpdate()
		}
		return p.handlePanelAction(c, d, args[0], args[1])
	case "transcript":
		return p.handleTranscriptButton(c, d, arg0(args))
	case "rate":
		if len(args) < 2 {
			return c.DeferUpdate()
		}
		return p.handleRate(c, d, args[0], args[1])
	default:
		return c.DeferUpdate()
	}
}

func (p *Plugin) handleModal(c *interactions.Context, d plugin.Deps) error {
	action, args := parseID(c.CustomID())
	switch action {
	case "form":
		if len(args) < 2 {
			return nil
		}
		return p.handleFormSubmit(c, d, args[0], args[1])
	case "closeform":
		return p.handleCloseSubmit(c, d, arg0(args))
	default:
		return nil
	}
}

func arg0(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// ── Activity tracking (auto-close clock + first-response time) ─

func (p *Plugin) handleMessage(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	m, err := plugin.DecodeData[event.Message](env)
	if err != nil || m.Author.Bot {
		return err
	}
	gid, ok := event.ParseID(m.GuildID)
	if !ok {
		return nil
	}
	chID, ok := event.ParseID(m.ChannelID)
	if !ok {
		return nil
	}
	t, err := d.Store.Tickets.GetTicketByChannel(ctx, gid, chID)
	if err != nil {
		return nil // not a ticket channel (the common case) — cheap indexed miss
	}
	_ = d.Store.Tickets.TouchActivity(ctx, chID)

	// Stamp first-response the first time a staff member replies (analytics).
	if t.FirstResponseAt == nil && m.Author.ID != event.FormatID(t.OpenerID) {
		cfg, cat := p.resolveTicketConfig(ctx, d, gid, t)
		if isStaffMember(cfg, cat, m.Member) {
			_ = d.Store.Tickets.SetFirstResponse(ctx, t.ID)
		}
	}
	return nil
}

// ── /tickets admin command ───────────────────────────────────

func (p *Plugin) handleTicketsCommand(c *interactions.Context, d plugin.Deps) error {
	sub := c.Subcommand()
	if len(sub) == 0 {
		return c.RespondEphemeral("Unknown subcommand.")
	}
	switch sub[0] {
	case "list":
		return p.handlePanelList(c, d)
	case "post":
		return p.handlePanelPost(c, d)
	default:
		return c.RespondEphemeral("Unknown subcommand.")
	}
}

func (p *Plugin) handlePanelList(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	panels, err := d.Store.Tickets.ListPanels(c.Ctx, gid)
	if err != nil {
		return err
	}
	if len(panels) == 0 {
		return c.RespondEphemeral("No ticket panels yet. Create one on the dashboard, then post it with `/tickets post`.")
	}
	embed := &discordgo.MessageEmbed{Title: "Ticket panels", Color: brandColor}
	for _, panel := range panels {
		pc := DecodePanel(panel.Config)
		name := panel.Name
		if name == "" {
			name = "(untitled)"
		}
		var b strings.Builder
		b.WriteString(strconv.Itoa(len(pc.Categories)) + " categor")
		if len(pc.Categories) == 1 {
			b.WriteString("y")
		} else {
			b.WriteString("ies")
		}
		b.WriteString(" · style `" + panel.Style + "`")
		if panel.MessageID != 0 && panel.ChannelID != 0 {
			b.WriteString("\nPosted in <#" + event.FormatID(panel.ChannelID) + ">")
		} else {
			b.WriteString("\nNot posted yet")
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{Name: name, Value: b.String()})
	}
	return c.RespondEmbed(true, embed)
}

func (p *Plugin) handlePanelPost(c *interactions.Context, d plugin.Deps) error {
	opts := c.Options()
	panelID := opts.String("panel")
	channelID := opts.Snowflake("channel")
	if channelID == "" {
		return c.RespondEphemeral("Please choose a channel to post the panel in.")
	}
	_, err := PostPanel(c.Ctx, d.Discord, d.Store, c.GuildID, channelID, panelID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return c.RespondEphemeral("That panel no longer exists.")
	case errors.Is(err, ErrPanelNoCategories):
		return c.RespondEphemeral("That panel has no categories yet — add some on the dashboard first.")
	case err != nil:
		return c.RespondEphemeral("Failed to post the panel: " + err.Error())
	}
	return c.RespondEphemeral("Posted the panel to <#" + channelID + ">.")
}

func (p *Plugin) autocompletePanel(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	panels, err := d.Store.Tickets.ListPanels(c.Ctx, gid)
	if err != nil {
		return c.Autocomplete(nil)
	}
	_, focused := c.Options().Focused()
	focused = strings.ToLower(focused)
	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, panel := range panels {
		name := panel.Name
		if name == "" {
			name = "(untitled)"
		}
		if focused != "" && !strings.Contains(strings.ToLower(name), focused) {
			continue
		}
		choices = append(choices, interactions.Choice(name, panel.ID))
		if len(choices) >= 25 {
			break
		}
	}
	return c.Autocomplete(choices)
}

// ── shared helpers ───────────────────────────────────────────

// interactionUser returns the acting user for a component/modal/command
// interaction (Member.User in a guild, User in a DM).
func interactionUser(c *interactions.Context) event.User {
	if u, ok := c.I.Actor(); ok {
		return u
	}
	return c.User
}

// resolveTicketConfig loads the feature config and the ticket's originating
// category. The category is zero when the panel was since edited/deleted; every
// caller tolerates that.
func (p *Plugin) resolveTicketConfig(ctx context.Context, d plugin.Deps, gid int64, t store.Ticket) (Config, CategoryConfig) {
	cfg, _, _ := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	var cat CategoryConfig
	if t.PanelID != "" {
		if panel, err := d.Store.Tickets.GetPanel(ctx, gid, t.PanelID); err == nil {
			if c, ok := DecodePanel(panel.Config).Category(t.CategoryID); ok {
				cat = c
			}
		}
	}
	return cfg, cat
}

// isStaffMember reports whether a member holds a support/staff role for a ticket.
func isStaffMember(cfg Config, cat CategoryConfig, member *event.Member) bool {
	if member == nil {
		return false
	}
	have := map[string]bool{}
	for _, r := range member.Roles {
		have[r] = true
	}
	for _, r := range cfg.StaffRoles(cat) {
		if have[r] {
			return true
		}
	}
	return false
}

// guildName fetches a guild's display name (falls back to "the server").
func guildName(ctx context.Context, d plugin.Deps, gid int64) string {
	if g, err := d.Store.Guilds.Get(ctx, gid); err == nil && g.Name != "" {
		return g.Name
	}
	return "the server"
}

// openerUser rebuilds the opener's user from a ticket row, including the display
// name captured at open time, so later rebuilds (claim/unclaim) and transcripts
// render {{ .User.Username }} / {{ .User.GlobalName }} the same as the first post.
func openerUser(t store.Ticket) event.User {
	return event.User{ID: event.FormatID(t.OpenerID), Username: t.OpenerUsername, GlobalName: t.OpenerGlobalName}
}
