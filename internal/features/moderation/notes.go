package moderation

import (
	"fmt"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// ── Mod notes ────────────────────────────────────────────────
//
// A "note" is a non-punitive case (action == "note") attached to a member: it
// records context for staff without DMing the user or expiring. /note adds one,
// /notes lists a member's notes.

func handleNote(c *interactions.Context, d plugin.Deps) error {
	opts := c.Options()
	target, ok := opts.User("user")
	if !ok {
		return c.RespondEphemeral("You must specify a user to add a note to.")
	}
	text := strings.TrimSpace(opts.String("text"))
	if text == "" {
		return c.RespondEphemeral("A note needs some text.")
	}

	gid, _ := event.ParseID(c.GuildID)
	uid, _ := event.ParseID(target.ID)
	modID, _ := event.ParseID(c.User.ID)

	created, err := d.Store.Moderation.CreateCase(c.Ctx, store.ModCase{
		GuildID:     gid,
		UserID:      uid,
		ModeratorID: modID,
		Action:      "note",
		Reason:      text,
		Active:      false,
	})
	if err != nil {
		return c.RespondEphemeral("Failed to record note: " + err.Error())
	}
	publishEvent(c.Ctx, d, event.TypeModerationAction, c.GuildID, event.ModerationAction{
		GuildID:    c.GuildID,
		Action:     "note",
		Reason:     text,
		User:       target,
		Moderator:  c.User,
		CaseNumber: created.CaseNumber,
	})
	return c.RespondEphemeral(fmt.Sprintf("Note added — case #%d.", created.CaseNumber))
}

func handleNotes(c *interactions.Context, d plugin.Deps) error {
	target, ok := c.Options().User("user")
	if !ok {
		return c.RespondEphemeral("You must specify a user.")
	}
	gid, _ := event.ParseID(c.GuildID)
	uid, ok := event.ParseID(target.ID)
	if !ok {
		return c.RespondEphemeral("Invalid user.")
	}
	cases, err := d.Store.Moderation.ListCases(c.Ctx, gid, &uid, 50, 0)
	if err != nil {
		return err
	}
	var notes []store.ModCase
	for _, mc := range cases {
		if mc.Action == "note" {
			notes = append(notes, mc)
		}
	}
	if len(notes) == 0 {
		return c.RespondEphemeral("No notes for that member.")
	}
	return c.RespondEmbed(true, notesEmbed(target, notes))
}

func notesEmbed(target event.User, notes []store.ModCase) *discordgo.MessageEmbed {
	var b strings.Builder
	for _, mc := range notes {
		line := fmt.Sprintf("`#%d` <@%s> — %s", mc.CaseNumber, event.FormatID(mc.ModeratorID), truncate(mc.Reason, 180))
		b.WriteString(line)
		b.WriteString("\n")
	}
	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("Notes for %s (%d)", userName(target), len(notes)),
		Description: b.String(),
		Color:       0x5865F2,
	}
}

// ── Reason-template autocomplete ─────────────────────────────

// reasonAutocomplete is the shared autocomplete handler wired onto the reason /
// text option of the mod commands. It offers the guild's configured
// ReasonTemplates, filtered by the focused input (case-insensitive substring),
// capped at Discord's 25-choice limit. Graceful when none are configured.
func reasonAutocomplete(c *interactions.Context, d plugin.Deps) error {
	_, input := c.Options().Focused()
	gid, ok := event.ParseID(c.GuildID)
	if !ok {
		return c.Autocomplete(nil)
	}
	cfg, _, _ := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	needle := strings.ToLower(strings.TrimSpace(input))

	var choices []*discordgo.ApplicationCommandOptionChoice
	for _, tmpl := range cfg.ReasonTemplates {
		t := strings.TrimSpace(tmpl)
		if t == "" {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(t), needle) {
			continue
		}
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{Name: truncate(t, 100), Value: t})
		if len(choices) >= 25 {
			break
		}
	}
	return c.Autocomplete(choices)
}
