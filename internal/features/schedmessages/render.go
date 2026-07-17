package schedmessages

import (
	"context"
	"strconv"
	"strings"
	"time"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// renderComposed renders a schedule's composed MessageSpec into the
// outgoing message: templated content and embeds plus composed button rows.
// Mirrors tickets' renderSpec/renderSpecRows so the shared dashboard editor's
// output posts identically across features.
func renderComposed(ctx context.Context, tmpl *templating.Engine, spec MessageSpec, data map[string]any, schedID int64, fallbackColor int) (string, []*discordgo.MessageEmbed, []discordgo.MessageComponent) {
	r := func(s string) string { return renderLine(ctx, tmpl, s, data) }

	content := r(spec.Content)
	var embeds []*discordgo.MessageEmbed
	for _, e := range spec.Embeds {
		if em := renderComposedEmbed(e, r, fallbackColor); em != nil {
			embeds = append(embeds, em)
		}
	}
	rows := renderComposedRows(spec, r, schedID)
	return content, embeds, rows
}

// renderLine renders one templated string against the schedule scope,
// returning the raw string on any template error so a typo never blanks a
// scheduled message.
func renderLine(ctx context.Context, tmpl *templating.Engine, src string, data map[string]any) string {
	if !strings.Contains(src, "{{") {
		return src
	}
	out, err := tmpl.RenderCard(ctx, src, data)
	if err != nil {
		return src
	}
	return out
}

// renderComposedEmbed renders one cc.EmbedSpec, templating every text field.
// Returns nil when the embed renders empty (Discord rejects empty embeds).
func renderComposedEmbed(e cc.EmbedSpec, r func(string) string, fallbackColor int) *discordgo.MessageEmbed {
	em := &discordgo.MessageEmbed{
		Title:       r(e.Title),
		Description: r(e.Description),
		URL:         r(e.URL),
		Color:       colorInt(e.Color, fallbackColor),
	}
	if e.AuthorName != "" || e.AuthorIcon != "" || e.AuthorURL != "" {
		em.Author = &discordgo.MessageEmbedAuthor{Name: r(e.AuthorName), IconURL: r(e.AuthorIcon), URL: r(e.AuthorURL)}
	}
	// Image URLs are templated (the default message uses {{ .Image }}), so
	// check the rendered value: an update without a thumbnail must not produce
	// an embed image with an empty URL (Discord rejects it).
	if u := r(e.Thumbnail); u != "" {
		em.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: u}
	}
	if u := r(e.ImageURL); u != "" {
		em.Image = &discordgo.MessageEmbedImage{URL: u}
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
	if em.Title == "" && em.Description == "" && len(em.Fields) == 0 && em.Author == nil && em.Image == nil {
		return nil
	}
	return em
}

// renderComposedRows renders the spec's button rows: a link button opens its
// templated URL; any other button gets a sched:act:<schedID>:<suffix>
// custom_id so its click runs the saved automation ButtonActions points it at
// (or is acknowledged with a notice when unwired). Selects have no meaning on
// a scheduled message and are skipped.
func renderComposedRows(spec MessageSpec, r func(string) string, schedID int64) []discordgo.MessageComponent {
	var out []discordgo.MessageComponent
	for _, row := range spec.Components {
		var comps []discordgo.MessageComponent
		for _, c := range row.Components {
			if c.Type != "" && c.Type != "button" {
				continue
			}
			label := r(c.Label)
			if label == "" {
				label = "Button"
			}
			if strings.EqualFold(c.Style, "link") || c.URL != "" {
				url := r(c.URL)
				if url == "" {
					continue
				}
				btn := discordgo.Button{Label: label, Style: discordgo.LinkButton, URL: url, Disabled: c.Disabled}
				if em := componentEmoji(c.Emoji); em != nil {
					btn.Emoji = em
				}
				comps = append(comps, btn)
				continue
			}
			btn := discordgo.Button{
				Label:    label,
				Style:    buttonStyle(c.Style),
				CustomID: actionCustomID(schedID, c.CustomIDSuffix),
				Disabled: c.Disabled,
			}
			if em := componentEmoji(c.Emoji); em != nil {
				btn.Emoji = em
			}
			comps = append(comps, btn)
		}
		if len(comps) > 0 {
			out = append(out, discordgo.ActionsRow{Components: comps})
		}
	}
	return out
}

// buttonStyle maps a config style string to a Discord button style.
func buttonStyle(s string) discordgo.ButtonStyle {
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

// componentEmoji parses a dashboard-authored emoji string into a
// ComponentEmoji: a unicode glyph passes through as Name; a custom emoji
// arrives as "name:id" (pasted "<:name:id>" / "<a:name:id>" forms tolerated).
// Empty means nil. Mirrors tickets' ticketEmoji.
func componentEmoji(s string) *discordgo.ComponentEmoji {
	s = strings.Trim(strings.TrimSpace(s), "<>")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ":")
	last := parts[len(parts)-1]
	if len(parts) >= 2 && isSnowflakeID(last) {
		e := &discordgo.ComponentEmoji{Name: parts[len(parts)-2], ID: last}
		if len(parts) >= 3 && parts[0] == "a" {
			e.Animated = true
		}
		return e
	}
	return &discordgo.ComponentEmoji{Name: s}
}

func isSnowflakeID(s string) bool {
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
