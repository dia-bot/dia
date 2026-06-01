package moderation

import (
	"fmt"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// actionLogEmbed builds the embed posted to the configured log channel for a
// freshly-applied action.
func actionLogEmbed(mc store.ModCase, target, moderator event.User, expiresAt *time.Time) *discordgo.MessageEmbed {
	fields := []*discordgo.MessageEmbedField{
		{Name: "User", Value: mention(event.FormatID(mc.UserID), userName(target)), Inline: true},
		{Name: "Moderator", Value: mention(event.FormatID(mc.ModeratorID), userName(moderator)), Inline: true},
		{Name: "Reason", Value: nonEmpty(mc.Reason), Inline: false},
	}
	if expiresAt != nil {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Expires",
			Value:  fmt.Sprintf("<t:%d:R>", expiresAt.Unix()),
			Inline: true,
		})
	}
	return &discordgo.MessageEmbed{
		Title:     fmt.Sprintf("%s — Case #%d", actionTitle(mc.Action), mc.CaseNumber),
		Color:     actionColor(mc.Action),
		Fields:    fields,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// caseEmbed renders a single case for /case.
func caseEmbed(mc store.ModCase) *discordgo.MessageEmbed {
	fields := []*discordgo.MessageEmbedField{
		{Name: "Action", Value: actionTitle(mc.Action), Inline: true},
		{Name: "User", Value: "<@" + event.FormatID(mc.UserID) + ">", Inline: true},
		{Name: "Moderator", Value: "<@" + event.FormatID(mc.ModeratorID) + ">", Inline: true},
		{Name: "Reason", Value: nonEmpty(mc.Reason), Inline: false},
	}
	if mc.DurationSeconds > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Duration",
			Value:  humanDuration(time.Duration(mc.DurationSeconds) * time.Second),
			Inline: true,
		})
	}
	if mc.ExpiresAt != nil {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "Expires",
			Value:  fmt.Sprintf("<t:%d:R>", mc.ExpiresAt.Unix()),
			Inline: true,
		})
	}
	status := "Resolved"
	if mc.Active {
		status = "Active"
	}
	fields = append(fields, &discordgo.MessageEmbedField{Name: "Status", Value: status, Inline: true})

	return &discordgo.MessageEmbed{
		Title:     fmt.Sprintf("Case #%d", mc.CaseNumber),
		Color:     actionColor(mc.Action),
		Fields:    fields,
		Timestamp: mc.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// casesEmbed renders a compact list for /cases.
func casesEmbed(cases []store.ModCase) *discordgo.MessageEmbed {
	var b strings.Builder
	for _, mc := range cases {
		line := fmt.Sprintf("`#%d` **%s** — <@%s>", mc.CaseNumber, actionTitle(mc.Action), event.FormatID(mc.UserID))
		if mc.Reason != "" {
			line += " — " + truncate(mc.Reason, 60)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}
	return &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("Moderation Cases (%d)", len(cases)),
		Description: b.String(),
		Color:       0x5865F2,
	}
}

func mention(id, name string) string {
	if id == "" || id == "0" {
		return name
	}
	return fmt.Sprintf("<@%s> (%s)", id, name)
}

func userName(u event.User) string {
	if u.GlobalName != "" {
		return u.GlobalName
	}
	if u.Username != "" {
		return u.Username
	}
	return "Unknown"
}

func nonEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "No reason provided"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func humanDuration(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		days := int(d.Hours()) / 24
		return fmt.Sprintf("%dd", days)
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
