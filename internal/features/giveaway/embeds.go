package giveaway

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// brandAccent is the fallback embed colour (the rose logo accent) when neither
// the giveaway nor the feature config sets one.
const brandAccent = 0xFF6363

// renderEngine is the shared, sandboxed text/template engine used for every
// giveaway string. RenderCard executes a pure template against a data map with
// the safe base funcs (no side effects, output + time capped).
var renderEngine = templating.New()

// scopeData builds the template scope shared by every giveaway string. Winner
// mentions are pre-joined so {{ .Winners }} renders ready-to-post; timestamps are
// pre-formatted as Discord relative/absolute tokens.
func scopeData(g store.Giveaway, entryCount int, winnerMentions []string, guildName string, memberCount int) map[string]any {
	host := ""
	if g.HostID != 0 {
		host = "<@" + event.FormatID(g.HostID) + ">"
	}
	return map[string]any{
		"Prize":       g.Prize,
		"Description": g.Description,
		"WinnerCount": g.WinnerCount,
		"EntryCount":  entryCount,
		"Host":        host,
		"Winners":     strings.Join(winnerMentions, ", "),
		"WinnerList":  strings.Join(winnerMentions, "\n"),
		"Ends":        discordTS(g.EndsAt, "R"),
		"EndsAt":      discordTS(g.EndsAt, "F"),
		"Server":      guildName,
		"MemberCount": memberCount,
		"Channel":     "<#" + event.FormatID(g.ChannelID) + ">",
	}
}

// renderText executes a giveaway template against the scope, falling back to the
// raw source on a parse/exec error so a typo degrades to the literal text rather
// than an empty embed. Strict mode (missingkey=error) makes a key typo like
// {{ .Winnner }} an error that triggers the fallback, instead of silently
// emitting "<no value>" into a public message.
func renderText(ctx context.Context, src string, data map[string]any) string {
	if strings.TrimSpace(src) == "" {
		return ""
	}
	out, err := renderEngine.RenderCardStrict(ctx, src, data)
	if err != nil {
		return src
	}
	return strings.TrimSpace(out)
}

// discordTS formats a time as a Discord timestamp token (<t:unix:style>), which
// the client renders in the viewer's own timezone (R = relative "in 2 hours",
// F = long date+time).
func discordTS(t time.Time, style string) string {
	return "<t:" + strconv.FormatInt(t.Unix(), 10) + ":" + style + ">"
}

// buildLiveMessage composes the full giveaway message (embed + Enter button) for
// a running giveaway.
func buildLiveMessage(ctx context.Context, cfg Config, g store.Giveaway, entryCount int, guildName string, memberCount int) *discordgo.MessageSend {
	data := scopeData(g, entryCount, nil, guildName, memberCount)
	send := &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{buildLiveEmbed(ctx, cfg, g, data, entryCount)},
		Components: enterComponents(cfg, g.ID),
	}
	return send
}

// buildLiveEmbed renders the live giveaway embed with its inline info grid.
func buildLiveEmbed(ctx context.Context, cfg Config, g store.Giveaway, data map[string]any, entryCount int) *discordgo.MessageEmbed {
	e := cfg.Embed
	em := &discordgo.MessageEmbed{
		Title:       fallback(renderText(ctx, e.Title, data), g.Prize),
		Description: renderText(ctx, e.Description, data),
		Color:       giveawayColor(g, cfg),
	}
	if e.Thumbnail != "" {
		em.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: e.Thumbnail}
	}
	if g.ImageURL != "" {
		em.Image = &discordgo.MessageEmbedImage{URL: g.ImageURL}
	}

	if g.HostID != 0 {
		em.Fields = append(em.Fields, inlineField(orLabel(e.HostedByLabel, "Hosted by"), "<@"+event.FormatID(g.HostID)+">"))
	}
	em.Fields = append(em.Fields, inlineField(orLabel(e.EndsLabel, "Ends"), discordTS(g.EndsAt, "R")))
	em.Fields = append(em.Fields, inlineField(orLabel(e.WinnersLabel, "Winners"), strconv.Itoa(g.WinnerCount)))
	if cfg.ShowEntryCount {
		em.Fields = append(em.Fields, inlineField(orLabel(e.EntriesLabel, "Entries"), strconv.Itoa(entryCount)))
	}
	if cfg.ShowRequirements {
		if s := requirementSummary(decodeRequirements(g.Requirements)); s != "" {
			em.Fields = append(em.Fields, &discordgo.MessageEmbedField{Name: "Requirements", Value: s, Inline: false})
		}
	}

	if ft := renderText(ctx, e.FooterText, data); ft != "" {
		em.Footer = &discordgo.MessageEmbedFooter{Text: ft}
	}
	if e.ShowTimestamp {
		em.Timestamp = g.EndsAt.Format(time.RFC3339)
	}
	return em
}

// buildEndedMessage composes the ended-state message: the winners embed and,
// unless it's a link-less config, an optional "Jump to giveaway" button. The
// Enter button is removed.
func buildEndedEmbed(ctx context.Context, cfg Config, g store.Giveaway, winnerMentions []string, entryCount, memberCount int, guildName string) *discordgo.MessageEmbed {
	data := scopeData(g, entryCount, winnerMentions, guildName, memberCount)
	a := cfg.Announce
	em := &discordgo.MessageEmbed{
		Title: fallback(renderText(ctx, a.EndedTitle, data), g.Prize),
		Color: giveawayColor(g, cfg),
	}
	if g.ImageURL != "" {
		em.Image = &discordgo.MessageEmbedImage{URL: g.ImageURL}
	}
	winnersVal := "No valid entries."
	if len(winnerMentions) > 0 {
		winnersVal = strings.Join(winnerMentions, ", ")
	}
	em.Fields = append(em.Fields,
		&discordgo.MessageEmbedField{Name: orLabel(cfg.Embed.WinnersLabel, "Winners"), Value: winnersVal, Inline: false},
	)
	if g.HostID != 0 {
		em.Fields = append(em.Fields, inlineField(orLabel(cfg.Embed.HostedByLabel, "Hosted by"), "<@"+event.FormatID(g.HostID)+">"))
	}
	if cfg.ShowEntryCount {
		em.Fields = append(em.Fields, inlineField(orLabel(cfg.Embed.EntriesLabel, "Entries"), strconv.Itoa(entryCount)))
	}
	if ft := renderText(ctx, a.EndedFooter, data); ft != "" {
		em.Footer = &discordgo.MessageEmbedFooter{Text: ft}
	}
	em.Timestamp = time.Now().Format(time.RFC3339)
	return em
}

// enterComponents builds the single Enter button row. Its custom_id embeds the
// giveaway id so a click resolves directly (no message-id round-trip).
func enterComponents(cfg Config, giveawayID string) []discordgo.MessageComponent {
	label := cfg.Button.Label
	if strings.TrimSpace(label) == "" {
		label = "Enter Giveaway"
	}
	btn := discordgo.Button{
		Label:    label,
		Style:    buttonStyle(cfg.Button.Style),
		CustomID: enterCustomID(giveawayID),
	}
	if em := componentEmoji(cfg.Button.Emoji); em != nil {
		btn.Emoji = em
	}
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{btn}}}
}

// jumpComponents builds a "Jump to giveaway" link button row for the winner
// announcement (empty when jump is off or the message isn't posted yet).
func jumpComponents(cfg Config, g store.Giveaway) []discordgo.MessageComponent {
	if !cfg.Announce.JumpButton || g.MessageID == 0 {
		return nil
	}
	url := "https://discord.com/channels/" + event.FormatID(g.GuildID) + "/" +
		event.FormatID(g.ChannelID) + "/" + event.FormatID(g.MessageID)
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{Label: "Jump to giveaway", Style: discordgo.LinkButton, URL: url},
	}}}
}

// ── small helpers ────────────────────────────────────────────────────────────

func inlineField(name, value string) *discordgo.MessageEmbedField {
	return &discordgo.MessageEmbedField{Name: name, Value: value, Inline: true}
}

func orLabel(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func fallback(v, alt string) string {
	if strings.TrimSpace(v) == "" {
		return alt
	}
	return v
}

func giveawayColor(g store.Giveaway, cfg Config) int {
	if c := colorInt(g.Color, -1); c >= 0 {
		return c
	}
	return colorInt(cfg.Embed.Color, brandAccent)
}

// colorInt converts a #RRGGBB string to a Discord embed colour int (fallback on
// a malformed/empty value).
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

func buttonStyle(s string) discordgo.ButtonStyle {
	switch buttonComponentStyle(s) {
	case "secondary":
		return discordgo.SecondaryButton
	case "success":
		return discordgo.SuccessButton
	case "danger":
		return discordgo.DangerButton
	default:
		return discordgo.PrimaryButton
	}
}

// componentEmoji turns the editor's emoji string into Discord's component shape:
// a unicode glyph passes through as Name; a custom emoji arrives as "name:id"
// (also tolerated: the full "<a:name:id>" paste) and splits into Name + ID +
// Animated. Empty → nil (no emoji).
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
