package tickets

import (
	"bytes"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// scope is the data root every ticket-facing string is rendered against. It
// mirrors the customcommands template contract (the same {{ .User.Mention }} /
// {{ .Guild.Name }} placeholders) and adds a {{ .Ticket.* }} namespace. It is
// kept in lockstep with the variable picker in web/src/lib/tickets/types.ts.
type scope struct {
	User    scopeUser
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
		},
	}
	if t.channelID != "" {
		s.Ticket.Channel = "<#" + t.channelID + ">"
	}
	return s
}

// ticketView is the minimal ticket data the renderers need, decoupled from the
// store row so both the create path and later rebuilds (claim) can use it.
type ticketView struct {
	id        string
	number    int
	subject   string
	channelID string
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

// ticketEmoji parses a dashboard-authored emoji string (unicode glyph or a
// "<:name:id>" / "<a:name:id>" custom emoji) into a ComponentEmoji, or nil.
func ticketEmoji(s string) *discordgo.ComponentEmoji {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		body := strings.Trim(s, "<>")
		animated := strings.HasPrefix(body, "a:")
		body = strings.TrimPrefix(body, "a:")
		parts := strings.Split(body, ":")
		if len(parts) == 2 {
			return &discordgo.ComponentEmoji{Name: parts[0], ID: parts[1], Animated: animated}
		}
	}
	return &discordgo.ComponentEmoji{Name: s}
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
