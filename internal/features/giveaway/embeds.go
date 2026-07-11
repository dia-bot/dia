package giveaway

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// brandAccent is the fallback embed colour (the rose logo accent) when neither
// the giveaway nor its embed sets one.
const brandAccent = 0xFF6363

// renderEngine is the shared, sandboxed text/template engine used for every
// giveaway string. RenderCardStrict executes a pure template against a data map
// with the safe base funcs (no side effects, output + time capped).
var renderEngine = templating.New()

// scopeData builds the template scope shared by every giveaway string. Winner
// mentions are pre-joined so {{ .Winners }} renders ready-to-post; timestamps are
// pre-formatted as Discord relative/absolute tokens. These are the variables the
// composed embed/announcement templates reference.
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

// ── Live message ─────────────────────────────────────────────────────────────

// buildLiveMessage composes the full giveaway message (content + embeds + Enter
// button) for a running giveaway from its own Spec.
func buildLiveMessage(ctx context.Context, spec Spec, g store.Giveaway, entryCount int, guildName string, memberCount int) *discordgo.MessageSend {
	data := scopeData(g, entryCount, nil, guildName, memberCount)
	send := &discordgo.MessageSend{
		Embeds:     buildLiveEmbeds(ctx, spec, g, data),
		Components: buildComponents(ctx, spec, g, data),
	}
	if c := renderText(ctx, spec.Content, data); c != "" {
		send.Content = c
	}
	return send
}

// buildLiveEmbeds renders every embed the giveaway composes, appending the
// requirement summary to the primary embed when enabled and applying the
// giveaway's image override. Always returns at least one embed so a giveaway is
// never posted as a bare (or empty) message.
func buildLiveEmbeds(ctx context.Context, spec Spec, g store.Giveaway, data map[string]any) []*discordgo.MessageEmbed {
	var out []*discordgo.MessageEmbed
	for i, e := range spec.Embeds {
		em := buildEmbed(ctx, e, data, g.Color)
		if i == 0 && spec.ShowRequirements {
			if s := requirementSummary(decodeRequirements(g.Requirements)); s != "" {
				em.Fields = append(em.Fields, &discordgo.MessageEmbedField{Name: "Requirements", Value: s, Inline: false})
			}
		}
		if g.ImageURL != "" && em.Image == nil {
			em.Image = &discordgo.MessageEmbedImage{URL: g.ImageURL}
		}
		if embedEmpty(em) {
			continue
		}
		out = append(out, em)
	}
	if len(out) == 0 {
		out = append(out, &discordgo.MessageEmbed{Title: fallback(g.Prize, "Giveaway"), Color: embedAccent(spec, g.Color)})
	}
	return out
}

// buildEmbed renders one composed embed (cc.EmbedSpec) against the giveaway
// scope. Mirrors leveling/customcommands embed rendering, but templates every
// string and skips fields whose name or value renders empty (Discord rejects
// empty field parts). overrideColor (the giveaway's colour column) wins over the
// embed's own colour when set.
func buildEmbed(ctx context.Context, e cc.EmbedSpec, data map[string]any, overrideColor string) *discordgo.MessageEmbed {
	em := &discordgo.MessageEmbed{
		Title:       renderText(ctx, e.Title, data),
		Description: renderText(ctx, e.Description, data),
		URL:         renderText(ctx, e.URL, data),
		Color:       resolveColor(overrideColor, e.Color, brandAccent),
	}
	if strings.TrimSpace(e.AuthorName) != "" {
		em.Author = &discordgo.MessageEmbedAuthor{
			Name:    renderText(ctx, e.AuthorName, data),
			IconURL: renderText(ctx, e.AuthorIcon, data),
			URL:     renderText(ctx, e.AuthorURL, data),
		}
	}
	if t := renderText(ctx, e.Thumbnail, data); t != "" {
		em.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: t}
	}
	if u := renderText(ctx, e.ImageURL, data); u != "" {
		em.Image = &discordgo.MessageEmbedImage{URL: u}
	}
	if strings.TrimSpace(e.FooterText) != "" {
		em.Footer = &discordgo.MessageEmbedFooter{Text: renderText(ctx, e.FooterText, data), IconURL: renderText(ctx, e.FooterIcon, data)}
	}
	if e.Timestamp {
		em.Timestamp = time.Now().Format(time.RFC3339)
	}
	for _, f := range e.Fields {
		name := renderText(ctx, f.Name, data)
		val := renderText(ctx, f.Value, data)
		if name == "" || val == "" {
			continue
		}
		em.Fields = append(em.Fields, &discordgo.MessageEmbedField{Name: name, Value: val, Inline: f.Inline})
	}
	return em
}

// ── Ended / cancelled state ──────────────────────────────────────────────────

// buildEndedEmbed composes the compact ended-state card (title + winners +
// footer) from the giveaway's Announce config, keeping the giveaway's colour and
// image. The Enter button is dropped by the caller.
func buildEndedEmbed(ctx context.Context, spec Spec, g store.Giveaway, winnerMentions []string, entryCount, memberCount int, guildName string) *discordgo.MessageEmbed {
	data := scopeData(g, entryCount, winnerMentions, guildName, memberCount)
	a := spec.Announce
	em := &discordgo.MessageEmbed{
		Title: fallback(renderText(ctx, a.EndedTitle, data), g.Prize),
		Color: embedAccent(spec, g.Color),
	}
	if g.ImageURL != "" {
		em.Image = &discordgo.MessageEmbedImage{URL: g.ImageURL}
	}
	winnersVal := "No valid entries."
	if len(winnerMentions) > 0 {
		winnersVal = strings.Join(winnerMentions, ", ")
	}
	em.Fields = append(em.Fields, &discordgo.MessageEmbedField{Name: "Winners", Value: winnersVal, Inline: false})
	if g.HostID != 0 {
		em.Fields = append(em.Fields, inlineField("Hosted by", "<@"+event.FormatID(g.HostID)+">"))
	}
	if ft := renderText(ctx, a.EndedFooter, data); ft != "" {
		em.Footer = &discordgo.MessageEmbedFooter{Text: ft}
	}
	em.Timestamp = time.Now().Format(time.RFC3339)
	return em
}

// buildCancelledEmbed composes the dimmed cancelled-state card.
func buildCancelledEmbed(ctx context.Context, spec Spec, g store.Giveaway, guildName string, memberCount int) *discordgo.MessageEmbed {
	data := scopeData(g, 0, nil, guildName, memberCount)
	title := g.Prize
	if len(spec.Embeds) > 0 {
		title = fallback(renderText(ctx, spec.Embeds[0].Title, data), g.Prize)
	}
	return &discordgo.MessageEmbed{
		Title:       title,
		Description: "🚫 This giveaway was cancelled.",
		Color:       embedAccent(spec, g.Color),
	}
}

// ── Components ───────────────────────────────────────────────────────────────

// buildComponents renders the giveaway's action rows. When the spec composes its
// own buttons, they're rendered with their custom_ids remapped to this feature's
// routes: the entry button (custom_id_suffix == EnterButtonSuffix) gets the
// enter custom_id, link buttons keep their (templated) URL, and any other button
// gets an action custom_id so a click still routes back here. When no entry
// button is composed, the styled system Enter button is appended so a giveaway
// is always enterable. Labels/URLs are templated against the giveaway scope.
func buildComponents(ctx context.Context, spec Spec, g store.Giveaway, data map[string]any) []discordgo.MessageComponent {
	// Only when the message composes no buttons at all (a legacy or fully-cleared
	// giveaway) does the styled system Enter button stand in, so a giveaway is
	// never left with no way to enter. When the composer HAS buttons, nothing is
	// added: the composed buttons are exactly what posts.
	if len(spec.Components) == 0 {
		return enterComponents(spec, g.ID)
	}
	return renderComponentRows(ctx, spec.Components, spec, g.ID, data)
}

// renderComponentRows renders composed button rows (the giveaway message's own,
// or an entry reply's) with each button's click routed: the entry button
// (custom_id_suffix == EnterButtonSuffix) gets the enter custom_id, link buttons
// keep their (templated) URL, and any other button gets an action custom_id so
// its click runs the saved automation Spec.ButtonActions points it at.
func renderComponentRows(ctx context.Context, rows []cc.ComponentRow, spec Spec, giveawayID string, data map[string]any) []discordgo.MessageComponent {
	var out []discordgo.MessageComponent
	for _, row := range rows {
		var comps []discordgo.MessageComponent
		for _, c := range row.Components {
			if c.Type != "" && c.Type != "button" {
				continue // only buttons are meaningful on a giveaway surface
			}
			isEnter := c.CustomIDSuffix != "" && c.CustomIDSuffix == spec.EnterButtonSuffix
			btn := giveawayButton(ctx, c, giveawayID, isEnter, data)
			if btn == nil {
				continue
			}
			comps = append(comps, btn)
		}
		if len(comps) > 0 {
			out = append(out, discordgo.ActionsRow{Components: comps})
		}
	}
	return out
}

// renderComposedEmbeds renders a composed surface's embeds (an entry reply's,
// or the winner DM's) against its scope, dropping any that render empty
// (Discord rejects empty embeds). The giveaway's colour override applies like
// on the live message.
func renderComposedEmbeds(ctx context.Context, embeds []cc.EmbedSpec, data map[string]any, overrideColor string) []*discordgo.MessageEmbed {
	var out []*discordgo.MessageEmbed
	for _, e := range embeds {
		em := buildEmbed(ctx, e, data, overrideColor)
		if embedEmpty(em) {
			continue
		}
		out = append(out, em)
	}
	return out
}

// giveawayButton renders one composed button, routing its click. A link button
// keeps its URL; the entry button gets the enter custom_id; every other button
// gets an action custom_id (giveaway:act:<id>:<suffix>) so the handler can react.
func giveawayButton(ctx context.Context, c cc.Component, giveawayID string, isEnter bool, data map[string]any) discordgo.MessageComponent {
	label := renderText(ctx, c.Label, data)
	if label == "" {
		label = "Button"
	}
	// Link button: no custom_id, just the (templated) URL.
	if strings.EqualFold(c.Style, "link") || c.URL != "" {
		url := renderText(ctx, c.URL, data)
		if url == "" {
			return nil
		}
		btn := discordgo.Button{Label: label, Style: discordgo.LinkButton, URL: url, Disabled: c.Disabled}
		if em := componentEmoji(c.Emoji); em != nil {
			btn.Emoji = em
		}
		return btn
	}
	customID := actionCustomID(giveawayID, c.CustomIDSuffix)
	if isEnter {
		customID = enterCustomID(giveawayID)
	}
	btn := discordgo.Button{
		Label:    label,
		Style:    buttonStyle(c.Style),
		CustomID: customID,
		Disabled: c.Disabled,
	}
	if em := componentEmoji(c.Emoji); em != nil {
		btn.Emoji = em
	}
	return btn
}

// enterComponents builds the single Enter button row. Its custom_id embeds the
// giveaway id so a click resolves directly (no message-id round-trip).
func enterComponents(spec Spec, giveawayID string) []discordgo.MessageComponent {
	label := spec.Button.Label
	if strings.TrimSpace(label) == "" {
		label = "Enter Giveaway"
	}
	btn := discordgo.Button{
		Label:    label,
		Style:    buttonStyle(spec.Button.Style),
		CustomID: enterCustomID(giveawayID),
	}
	if em := componentEmoji(spec.Button.Emoji); em != nil {
		btn.Emoji = em
	}
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{btn}}}
}

// jumpComponents builds a "Jump to giveaway" link button row for the winner
// announcement (empty when jump is off or the message isn't posted yet).
func jumpComponents(spec Spec, g store.Giveaway) []discordgo.MessageComponent {
	if !spec.Announce.JumpButton || g.MessageID == 0 {
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

func fallback(v, alt string) string {
	if strings.TrimSpace(v) == "" {
		return alt
	}
	return v
}

// embedEmpty reports whether a rendered embed carries nothing Discord would
// display (so it can be dropped rather than rejected).
func embedEmpty(em *discordgo.MessageEmbed) bool {
	return em.Title == "" && em.Description == "" && len(em.Fields) == 0 &&
		em.Image == nil && em.Thumbnail == nil && em.Author == nil && em.Footer == nil
}

// embedAccent resolves the giveaway's effective accent colour: the giveaway's
// colour override, else the primary embed's colour, else the brand accent.
func embedAccent(spec Spec, override string) int {
	embedColor := ""
	if len(spec.Embeds) > 0 {
		embedColor = spec.Embeds[0].Color
	}
	return resolveColor(override, embedColor, brandAccent)
}

// resolveColor picks the first valid hex colour from override then base, falling
// back to def.
func resolveColor(override, base string, def int) int {
	if c := colorInt(override, -1); c >= 0 {
		return c
	}
	return colorInt(base, def)
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
