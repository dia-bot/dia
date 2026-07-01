package verification

import (
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// buildPrompt composes the persistent gate message: the rich content (Content,
// falling back to the legacy WelcomeText) plus any configured embeds and custom
// component rows, with the persistent "Verify" button (custom_id idStart)
// INJECTED as the first action row. The shared prompt has no specific joiner, so
// every string is templated against an empty user + the guild name (admins write
// guild-level copy). AllowedMentions stays empty so nothing pings.
func buildPrompt(cfg Config, guildID, guildName string) *discordgo.MessageSend {
	body := cfg.Content
	if strings.TrimSpace(body) == "" {
		body = cfg.WelcomeText
	}
	body = renderTemplate(body, event.User{}, guildID, guildName)

	send := &discordgo.MessageSend{
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	}
	if strings.TrimSpace(body) != "" {
		send.Content = body
	}
	for _, e := range cfg.Embeds {
		send.Embeds = append(send.Embeds, buildPromptEmbed(e, guildID, guildName))
	}

	// The Verify button is always row one; the admin's custom rows follow.
	send.Components = append(send.Components, discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{
			Style:    discordgo.SuccessButton,
			Label:    "Verify",
			CustomID: idStart,
			Emoji:    &discordgo.ComponentEmoji{Name: startEmoji},
		},
	}})
	send.Components = append(send.Components, buildPromptComponents(cfg.Components, guildID, guildName)...)

	// A bare prompt with no copy still needs something readable above the button.
	if send.Content == "" && len(send.Embeds) == 0 {
		send.Content = "Click the button below to verify and unlock the server."
	}
	return send
}

// buildPromptEmbed renders one cc.EmbedSpec into a Discord embed, templating
// every text field against the (joiner-less) prompt scope. Mirrors the
// customcommands send_message embed builder so the dashboard's MessageEditor
// output renders identically.
func buildPromptEmbed(e cc.EmbedSpec, guildID, guildName string) *discordgo.MessageEmbed {
	r := func(s string) string { return renderTemplate(s, event.User{}, guildID, guildName) }
	em := &discordgo.MessageEmbed{
		Title:       r(e.Title),
		Description: r(e.Description),
		URL:         r(e.URL),
		Color:       colorInt(e.Color, 0xB244FC),
	}
	if e.AuthorName != "" || e.AuthorIcon != "" || e.AuthorURL != "" {
		em.Author = &discordgo.MessageEmbedAuthor{Name: r(e.AuthorName), IconURL: r(e.AuthorIcon), URL: r(e.AuthorURL)}
	}
	if e.Thumbnail != "" {
		em.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: r(e.Thumbnail)}
	}
	if e.ImageURL != "" {
		em.Image = &discordgo.MessageEmbedImage{URL: r(e.ImageURL)}
	}
	if e.FooterText != "" || e.FooterIcon != "" {
		em.Footer = &discordgo.MessageEmbedFooter{Text: r(e.FooterText), IconURL: r(e.FooterIcon)}
	}
	if e.Timestamp {
		em.Timestamp = time.Now().Format(time.RFC3339)
	}
	for _, f := range e.Fields {
		em.Fields = append(em.Fields, &discordgo.MessageEmbedField{Name: r(f.Name), Value: r(f.Value), Inline: f.Inline})
	}
	return em
}

// buildPromptComponents renders the admin's custom rows. A non-link component's
// click routes to this feature's handler via custom_id "vbtn:<suffix>"; a link
// button carries its URL instead. Rows that render empty (e.g. a string select
// with no options) are dropped so Discord doesn't reject the whole message.
func buildPromptComponents(rows []cc.ComponentRow, guildID, guildName string) []discordgo.MessageComponent {
	out := make([]discordgo.MessageComponent, 0, len(rows))
	for _, row := range rows {
		comps := make([]discordgo.MessageComponent, 0, len(row.Components))
		for _, c := range row.Components {
			if mc := buildPromptComponent(c, guildID, guildName); mc != nil {
				comps = append(comps, mc)
			}
		}
		if len(comps) > 0 {
			out = append(out, discordgo.ActionsRow{Components: comps})
		}
	}
	return out
}

// buildPromptComponent renders one custom component, or nil to skip it when it
// would make Discord reject the message (a string select with no options).
func buildPromptComponent(c cc.Component, guildID, guildName string) discordgo.MessageComponent {
	r := func(s string) string { return renderTemplate(s, event.User{}, guildID, guildName) }
	routeID := vbtnPrefix + c.CustomIDSuffix
	switch c.Type {
	case "select_string":
		if len(c.Options) == 0 {
			return nil
		}
		opts := make([]discordgo.SelectMenuOption, 0, len(c.Options))
		for _, o := range c.Options {
			so := discordgo.SelectMenuOption{Label: r(o.Label), Value: o.Value, Description: r(o.Description), Default: o.Default}
			if o.Emoji != "" {
				so.Emoji = promptEmoji(o.Emoji)
			}
			opts = append(opts, so)
		}
		return discordgo.SelectMenu{
			MenuType:    discordgo.StringSelectMenu,
			CustomID:    routeID,
			Placeholder: r(c.Placeholder),
			Options:     opts,
			MinValues:   c.MinValues,
			MaxValues:   promptIntOrZero(c.MaxValues),
			Disabled:    c.Disabled,
		}
	case "select_user":
		return discordgo.SelectMenu{MenuType: discordgo.UserSelectMenu, CustomID: routeID, Placeholder: r(c.Placeholder), Disabled: c.Disabled}
	case "select_role":
		return discordgo.SelectMenu{MenuType: discordgo.RoleSelectMenu, CustomID: routeID, Placeholder: r(c.Placeholder), Disabled: c.Disabled}
	case "select_channel":
		return discordgo.SelectMenu{MenuType: discordgo.ChannelSelectMenu, CustomID: routeID, Placeholder: r(c.Placeholder), Disabled: c.Disabled}
	default: // button
		style := promptButtonStyle(c.Style)
		btn := discordgo.Button{Label: r(c.Label), Style: style, Disabled: c.Disabled}
		// Discord rejects a button that carries both a URL and a custom_id: a link
		// button is its URL, anything else routes its click to the handler.
		if style == discordgo.LinkButton {
			btn.URL = r(c.URL)
		} else {
			btn.CustomID = routeID
		}
		if c.Emoji != "" {
			btn.Emoji = promptEmoji(c.Emoji)
		}
		return btn
	}
}

func promptButtonStyle(s string) discordgo.ButtonStyle {
	switch strings.ToLower(s) {
	case "primary":
		return discordgo.PrimaryButton
	case "success":
		return discordgo.SuccessButton
	case "danger":
		return discordgo.DangerButton
	case "link":
		return discordgo.LinkButton
	}
	return discordgo.SecondaryButton
}

func promptIntOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
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

// promptEmoji turns the editor's emoji string into Discord's shape: a unicode
// glyph passes through as Name; a custom emoji arrives as "name:id" (also
// tolerated: "a:name:id" for animated, or the full "<a:name:id>" paste).
func promptEmoji(s string) *discordgo.ComponentEmoji {
	s = strings.Trim(strings.TrimSpace(s), "<>")
	parts := strings.Split(s, ":")
	last := parts[len(parts)-1]
	if len(parts) >= 2 && promptIsSnowflake(last) {
		e := &discordgo.ComponentEmoji{Name: parts[len(parts)-2], ID: last}
		if len(parts) >= 3 && parts[0] == "a" {
			e.Animated = true
		}
		return e
	}
	return &discordgo.ComponentEmoji{Name: s}
}

func promptIsSnowflake(s string) bool {
	if len(s) < 15 || len(s) > 21 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
