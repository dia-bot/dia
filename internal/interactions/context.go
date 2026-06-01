// Package interactions is Dia's slash-command-native interaction framework: a
// router that dispatches application commands, message components and modals to
// handlers, plus an ergonomic Context for reading options and responding.
package interactions

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Context carries everything a handler needs to read an interaction and respond.
type Context struct {
	Ctx     context.Context
	I       *event.Interaction
	Client  *discord.Client
	Log     *slog.Logger
	GuildID string
	User    event.User

	responded bool
}

func (c *Context) ref() discord.InteractionRef {
	return discord.InteractionRef{ID: c.I.ID, AppID: c.I.ApplicationID, Token: c.I.Token}
}

// Responded reports whether an initial response has been sent.
func (c *Context) Responded() bool { return c.responded }

// ── Responding ───────────────────────────────────────────────

// Respond replies with a (public) text message.
func (c *Context) Respond(content string) error {
	return c.respond(&discordgo.InteractionResponseData{Content: content})
}

// RespondEphemeral replies with a message only the invoker can see.
func (c *Context) RespondEphemeral(content string) error {
	return c.respond(&discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral})
}

// RespondEmbed replies with one or more embeds.
func (c *Context) RespondEmbed(ephemeral bool, embeds ...*discordgo.MessageEmbed) error {
	d := &discordgo.InteractionResponseData{Embeds: embeds}
	if ephemeral {
		d.Flags = discordgo.MessageFlagsEphemeral
	}
	return c.respond(d)
}

// RespondData replies with a fully-formed response data block.
func (c *Context) RespondData(d *discordgo.InteractionResponseData) error { return c.respond(d) }

func (c *Context) respond(d *discordgo.InteractionResponseData) error {
	err := c.Client.Respond(c.ref(), &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: d,
	})
	if err == nil {
		c.responded = true
	}
	return err
}

// Defer acknowledges now; follow up later (for slow work like image rendering).
func (c *Context) Defer(ephemeral bool) error {
	err := c.Client.Defer(c.ref(), ephemeral)
	if err == nil {
		c.responded = true
	}
	return err
}

// Followup sends a follow-up after Defer.
func (c *Context) Followup(params *discordgo.WebhookParams) (*discordgo.Message, error) {
	return c.Client.Followup(c.ref(), params)
}

// FollowupContent is a convenience follow-up with just text.
func (c *Context) FollowupContent(content string) (*discordgo.Message, error) {
	return c.Client.Followup(c.ref(), &discordgo.WebhookParams{Content: content})
}

// Edit edits the original (deferred) response.
func (c *Context) Edit(edit *discordgo.WebhookEdit) (*discordgo.Message, error) {
	return c.Client.EditResponse(c.ref(), edit)
}

// UpdateMessage responds to a component interaction by editing its message.
func (c *Context) UpdateMessage(d *discordgo.InteractionResponseData) error {
	err := c.Client.Respond(c.ref(), &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseUpdateMessage,
		Data: d,
	})
	if err == nil {
		c.responded = true
	}
	return err
}

// RespondModal opens a modal in response to the interaction.
func (c *Context) RespondModal(customID, title string, rows []discordgo.MessageComponent) error {
	err := c.Client.Respond(c.ref(), &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{CustomID: customID, Title: title, Components: rows},
	})
	if err == nil {
		c.responded = true
	}
	return err
}

// Autocomplete responds to an autocomplete interaction with choices.
func (c *Context) Autocomplete(choices []*discordgo.ApplicationCommandOptionChoice) error {
	return c.Client.Respond(c.ref(), &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{Choices: choices},
	})
}

// ── Reading component / modal data ───────────────────────────

// CustomID returns the component or modal custom_id.
func (c *Context) CustomID() string { return c.I.Data.CustomID }

// ComponentValues returns the selected values of a select-menu interaction.
func (c *Context) ComponentValues() []string { return c.I.Data.Values }

// ModalValue returns a submitted modal input by its custom_id.
func (c *Context) ModalValue(customID string) string {
	for _, row := range c.I.Data.Components {
		for _, comp := range row.Components {
			if comp.CustomID == customID {
				return comp.Value
			}
		}
	}
	return ""
}

// ── Reading command options ──────────────────────────────────

// Options returns an option reader resolving sub-commands automatically.
func (c *Context) Options() Options {
	_, leaf := flatten(c.I.Data.Options)
	byName := make(map[string]event.InteractionOption, len(leaf))
	for _, o := range leaf {
		byName[o.Name] = o
	}
	return Options{byName: byName, resolved: c.I.Data.Resolved}
}

// Subcommand returns the invoked sub-command path (e.g. ["rewards","add"]), or nil.
func (c *Context) Subcommand() []string {
	path, _ := flatten(c.I.Data.Options)
	return path
}

// flatten drills through sub-command/group options, returning the path taken and
// the leaf options of the actually-invoked (sub)command.
func flatten(opts []event.InteractionOption) (path []string, leaf []event.InteractionOption) {
	leaf = opts
	for len(leaf) == 1 && (leaf[0].Type == event.OptSubCommand || leaf[0].Type == event.OptSubCommandGroup) {
		path = append(path, leaf[0].Name)
		leaf = leaf[0].Options
	}
	return path, leaf
}

// Options reads command option values by name.
type Options struct {
	byName   map[string]event.InteractionOption
	resolved *event.Resolved
}

// Has reports whether an option was provided.
func (o Options) Has(name string) bool { _, ok := o.byName[name]; return ok }

// String returns a string option (or "").
func (o Options) String(name string) string {
	var s string
	o.decode(name, &s)
	return s
}

// Int returns an integer option (or 0).
func (o Options) Int(name string) int64 {
	var n int64
	o.decode(name, &n)
	return n
}

// Float returns a number option (or 0).
func (o Options) Float(name string) float64 {
	var f float64
	o.decode(name, &f)
	return f
}

// Bool returns a boolean option (or false).
func (o Options) Bool(name string) bool {
	var b bool
	o.decode(name, &b)
	return b
}

// Snowflake returns a user/role/channel/mentionable option's ID (or "").
func (o Options) Snowflake(name string) string {
	var s string
	o.decode(name, &s)
	return s
}

// User resolves a user option to its object via the resolved data.
func (o Options) User(name string) (event.User, bool) {
	id := o.Snowflake(name)
	if id == "" || o.resolved == nil {
		return event.User{}, false
	}
	u, ok := o.resolved.Users[id]
	return u, ok
}

// Role resolves a role option to its object via the resolved data.
func (o Options) Role(name string) (event.Role, bool) {
	id := o.Snowflake(name)
	if id == "" || o.resolved == nil {
		return event.Role{}, false
	}
	r, ok := o.resolved.Roles[id]
	return r, ok
}

// Channel resolves a channel option to its object via the resolved data.
func (o Options) Channel(name string) (event.Channel, bool) {
	id := o.Snowflake(name)
	if id == "" || o.resolved == nil {
		return event.Channel{}, false
	}
	ch, ok := o.resolved.Channels[id]
	return ch, ok
}

// Focused returns the name of the currently-focused option (for autocomplete).
func (o Options) Focused() (string, string) {
	for name, opt := range o.byName {
		if opt.Focused {
			var s string
			_ = json.Unmarshal(opt.Value, &s)
			return name, s
		}
	}
	return "", ""
}

func (o Options) decode(name string, dst any) {
	opt, ok := o.byName[name]
	if !ok || len(opt.Value) == 0 {
		return
	}
	_ = json.Unmarshal(opt.Value, dst)
}
