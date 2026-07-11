package tickets

import (
	"bytes"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// scope is the data root every ticket-facing string is rendered against. It
// mirrors the customcommands template contract (the same {{ .User.Mention }} /
// {{ .Guild.Name }} placeholders) and adds a {{ .Ticket.* }} namespace. It is
// kept in lockstep with the variable picker in web/src/lib/tickets/types.ts.
type scope struct {
	User    scopeUser // the ticket opener
	Actor   scopeUser // whoever performed the current action (closer / requester)
	Guild   scopeGuild
	Channel scopeChannel
	Ticket  scopeTicket
}

type scopeUser struct {
	ID         string
	Username   string
	GlobalName string
	Mention    string
}

type scopeGuild struct {
	ID          string
	Name        string
	MemberCount int
}

type scopeChannel struct {
	ID      string
	Mention string
}

type scopeTicket struct {
	Number   int
	ID       string
	Subject  string
	Category string
	Channel  string // channel mention
	Claimer  string // mention of the claiming staff member ("" = unclaimed)
	Closer   string // mention of whoever closed / requested the close
	Reason   string // close / close-request reason
	Rating   int    // 1-5 once rated
	Deadline string // Discord timestamp of a pending auto-accept close request
}

// panelScope builds the (opener-less) scope a panel message renders against.
func panelScope(guildID, guildName, channelID string) scope {
	return scope{
		Guild:   scopeGuild{ID: guildID, Name: nonEmpty(guildName, "the server")},
		Channel: channelScope(channelID),
	}
}

// ticketScope builds the scope a ticket's messages render against.
func ticketScope(guildID, guildName string, opener event.User, cat CategoryConfig, t *ticketView) scope {
	s := scope{
		User:    userScope(opener),
		Guild:   scopeGuild{ID: guildID, Name: nonEmpty(guildName, "the server")},
		Channel: channelScope(t.channelID),
		Ticket: scopeTicket{
			Number:   t.number,
			ID:       t.id,
			Subject:  t.subject,
			Category: cat.Label,
			Reason:   t.reason,
			Rating:   t.rating,
		},
	}
	if t.channelID != "" {
		s.Ticket.Channel = "<#" + t.channelID + ">"
	}
	if t.claimerID != "" {
		s.Ticket.Claimer = "<@" + t.claimerID + ">"
	}
	if t.closerID != "" {
		s.Ticket.Closer = "<@" + t.closerID + ">"
	}
	if t.deadline != nil {
		s.Ticket.Deadline = "<t:" + strconv.FormatInt(t.deadline.Unix(), 10) + ":R>"
	}
	return s
}

// withActor returns the scope with .Actor set to the acting user.
func (s scope) withActor(actor event.User) scope {
	s.Actor = userScope(actor)
	return s
}

// ticketView is the minimal ticket data the renderers need, decoupled from the
// store row so both the create path and later rebuilds (claim) can use it. The
// action fields (claimer/closer/reason/rating/deadline) are set only on the
// surfaces where they exist.
type ticketView struct {
	id        string
	number    int
	subject   string
	channelID string
	claimerID string
	closerID  string
	reason    string
	rating    int
	deadline  *time.Time
}

// viewOf builds a ticketView from a store row.
func viewOf(t store.Ticket) ticketView {
	tv := ticketView{id: t.ID, number: t.Number, subject: t.Subject, reason: t.CloseReason, rating: t.Rating}
	if t.ChannelID != 0 {
		tv.channelID = event.FormatID(t.ChannelID)
	}
	if t.ClaimedBy != 0 {
		tv.claimerID = event.FormatID(t.ClaimedBy)
	}
	if t.ClosedBy != 0 {
		tv.closerID = event.FormatID(t.ClosedBy)
	}
	return tv
}

func userScope(u event.User) scopeUser {
	name := u.GlobalName
	if name == "" {
		name = u.Username
	}
	su := scopeUser{ID: u.ID, Username: u.Username, GlobalName: name}
	if u.ID != "" {
		su.Mention = "<@" + u.ID + ">"
	}
	return su
}

func channelScope(id string) scopeChannel {
	c := scopeChannel{ID: id}
	if id != "" {
		c.Mention = "<#" + id + ">"
	}
	return c
}

// render renders raw as a Go text/template against sc. On any parse/exec error it
// returns the raw string so a template typo never blanks a ticket message.
func render(raw string, sc scope) string {
	if !strings.Contains(raw, "{{") {
		return raw
	}
	t, err := template.New("ticket").Option("missingkey=zero").Parse(raw)
	if err != nil {
		return raw
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, sc); err != nil {
		return raw
	}
	return buf.String()
}

// renderEmbed renders a cc.EmbedSpec into a Discord embed, templating every text
// field against sc. Mirrors verification/prompt.go so the dashboard's embed
// editor output renders identically. Returns nil when the embed is empty.
func renderEmbed(e cc.EmbedSpec, sc scope, fallbackColor int) *discordgo.MessageEmbed {
	r := func(s string) string { return render(s, sc) }
	em := &discordgo.MessageEmbed{
		Title:       r(e.Title),
		Description: r(e.Description),
		URL:         r(e.URL),
		Color:       colorInt(e.Color, fallbackColor),
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
	if em.Title == "" && em.Description == "" && len(em.Fields) == 0 && em.Author == nil && em.Image == nil {
		return nil
	}
	return em
}

// renderSpec renders a composed MessageSpec's content + embeds against sc,
// dropping embeds that render empty (Discord rejects them).
func renderSpec(spec MessageSpec, sc scope, fallbackColor int) (string, []*discordgo.MessageEmbed) {
	content := render(spec.Content, sc)
	var embeds []*discordgo.MessageEmbed
	for _, e := range spec.Embeds {
		if em := renderEmbed(e, sc, fallbackColor); em != nil {
			embeds = append(embeds, em)
		}
	}
	return content, embeds
}

// renderSpecRows renders a spec's composed button rows with each click routed:
// a link button opens its (templated) URL; any other button gets a
// tkt:act:<ticketID>:<suffix> custom_id so its click runs the saved automation
// ButtonActions points it at (or is acknowledged silently when unwired). Only
// buttons are meaningful on a ticket surface; selects are skipped.
func renderSpecRows(spec MessageSpec, sc scope, ticketID string) []discordgo.MessageComponent {
	var out []discordgo.MessageComponent
	for _, row := range spec.Components {
		var comps []discordgo.MessageComponent
		for _, c := range row.Components {
			if c.Type != "" && c.Type != "button" {
				continue
			}
			label := render(c.Label, sc)
			if label == "" {
				label = "Button"
			}
			if strings.EqualFold(c.Style, "link") || c.URL != "" {
				url := render(c.URL, sc)
				if url == "" {
					continue
				}
				btn := discordgo.Button{Label: label, Style: discordgo.LinkButton, URL: url, Disabled: c.Disabled}
				if em := ticketEmoji(c.Emoji); em != nil {
					btn.Emoji = em
				}
				comps = append(comps, btn)
				continue
			}
			btn := discordgo.Button{
				Label:    label,
				Style:    buttonStyle(c.Style),
				CustomID: actionButtonID(ticketID, c.CustomIDSuffix),
				Disabled: c.Disabled,
			}
			if em := ticketEmoji(c.Emoji); em != nil {
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

// systemButton builds one system control button, applying the category's
// SystemButton override on top of the built-in default look.
func systemButton(sb SystemButton, defLabel, defEmoji string, defStyle discordgo.ButtonStyle, customID string) discordgo.Button {
	label := strings.TrimSpace(sb.Label)
	if label == "" {
		label = defLabel
	}
	style := defStyle
	if sb.Style != "" {
		style = buttonStyle(sb.Style)
	}
	emoji := sb.Emoji
	if strings.TrimSpace(emoji) == "" {
		emoji = defEmoji
	}
	btn := discordgo.Button{Label: label, Style: style, CustomID: customID}
	if em := ticketEmoji(emoji); em != nil {
		btn.Emoji = em
	}
	return btn
}

// channelName renders a category's name scheme and slugifies the result to
// Discord's channel-name rules (lowercase, hyphenated, <=100 chars).
func channelName(scheme string, sc scope, prefix string, number int) string {
	if strings.TrimSpace(scheme) == "" {
		if prefix == "" {
			prefix = "ticket"
		}
		return slugChannel(prefix + "-" + strconv.Itoa(number))
	}
	return slugChannel(render(scheme, sc))
}

// slugChannel lowercases and hyphenates a string into a valid channel name.
func slugChannel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_':
			b.WriteRune(r)
			lastDash = r == '-'
		case r == ' ':
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
		if b.Len() >= 100 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "ticket"
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

// ticketEmoji parses a dashboard-authored emoji string into a ComponentEmoji:
// a unicode glyph passes through as Name; a custom emoji arrives as "name:id"
// from the shared editor (the pasted "<:name:id>" / "<a:name:id>" forms are
// also tolerated). Empty → nil. Mirrors giveaway's componentEmoji.
func ticketEmoji(s string) *discordgo.ComponentEmoji {
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

func nonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}
