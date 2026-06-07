package welcome

import (
	"context"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/templating"
)

// Vars is the substitution context for welcome/goodbye templates. Construct it
// with NewVars; the fields are intentionally private so apply() stays the single
// place that knows how a member/guild maps to placeholder values.
type Vars struct {
	user    event.User
	guildID string
	server  string
	count   int
	lookup  templating.Lookup // read-only guild data for getRole/getChannel; nil in previews
	fonts   map[string]string // guild custom fonts (family → URL) for the card renderer
}

// NewVars builds a template context for a member in a guild.
func NewVars(user event.User, guildID, server string, count int) Vars {
	return Vars{user: user, guildID: guildID, server: server, count: count}
}

// WithLookup attaches a read-only guild-data lookup (for getRole/getChannel) so
// the API "send test" path resolves them exactly like the live worker does.
func (v Vars) WithLookup(l templating.Lookup) Vars {
	v.lookup = l
	return v
}

// WithFonts attaches the guild's custom fonts (family → URL) so a studio card
// that uses an uploaded font renders with it.
func (v Vars) WithFonts(f map[string]string) Vars {
	v.fonts = f
	return v
}

// Variable documents one supported placeholder (the dashboard renders these in
// its variable picker; this slice is the single source of truth).
type Variable struct {
	Token string `json:"token"`
	Desc  string `json:"desc"`
}

// Variables lists every placeholder apply() understands.
var Variables = []Variable{
	{"{user}", "Member's display name"},
	{"{user.mention}", "Pings the member"},
	{"{user.name}", "Username"},
	{"{username}", "Username (alias)"},
	{"{user.id}", "Member ID"},
	{"{user.avatar}", "Avatar image URL"},
	{"{server}", "Server name"},
	{"{server.id}", "Server ID"},
	{"{count}", "Member count"},
	{"{count.ordinal}", "Member count, like 1,024th"},
}

func (v Vars) displayName() string {
	if v.user.GlobalName != "" {
		return v.user.GlobalName
	}
	return v.user.Username
}

// apply substitutes every placeholder in s. Order matters: longer, more
// specific tokens (e.g. {user.mention}) are listed before their prefixes
// ({user}) so the Replacer matches them first.
func (v Vars) apply(s string) string {
	if s == "" {
		return ""
	}
	return strings.NewReplacer(
		"{user.mention}", "<@"+v.user.ID+">",
		"{user.name}", v.user.Username,
		"{username}", v.user.Username,
		"{user.id}", v.user.ID,
		"{user.avatar}", discord.AvatarURL(v.user.ID, v.user.Avatar, 256),
		"{user}", v.displayName(),
		"{server.id}", v.guildID,
		"{server}", v.server,
		"{count.ordinal}", ordinal(v.count),
		"{count}", strconv.Itoa(v.count),
	).Replace(s)
}

// tmplContext is the data root (.) for the template engine.
func (v Vars) tmplContext() *templating.Context {
	u := templating.User{
		ID:         v.user.ID,
		Username:   v.user.Username,
		GlobalName: v.user.GlobalName,
		Avatar:     discord.AvatarURL(v.user.ID, v.user.Avatar, 256),
		Bot:        v.user.Bot,
	}
	return &templating.Context{
		User:   u,
		Member: templating.Member{User: u},
		Guild:  templating.Guild{ID: v.guildID, Name: v.server, MemberCount: v.count},
	}
}

// render runs the pure template engine (logic + functions, no actions) then the
// {token} shorthands, so messages can use {{ }} logic as well as {tokens}.
func (v Vars) render(s string) string {
	if s == "" {
		return ""
	}
	return templating.RenderMessage(context.Background(), s, v.tmplContext(), v.lookup, v.Map())
}

// Map returns the placeholder→value map used by the layout renderer (which
// substitutes tokens in text/image layers). Tokens are distinct (the closing
// brace disambiguates {user} from {user.name}), so replacement order is safe.
func (v Vars) Map() map[string]string {
	return map[string]string{
		"{user.mention}":  "<@" + v.user.ID + ">",
		"{user.name}":     v.user.Username,
		"{username}":      v.user.Username,
		"{user.id}":       v.user.ID,
		"{user.avatar}":   discord.AvatarURL(v.user.ID, v.user.Avatar, 256),
		"{user}":          v.displayName(),
		"{server.id}":     v.guildID,
		"{server}":        v.server,
		"{count.ordinal}": ordinal(v.count),
		"{count}":         strconv.Itoa(v.count),
	}
}

// ordinal renders a comma-grouped count with its English ordinal suffix.
func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return commaInt(n) + suffix
}

// commaInt inserts thousands separators (1024 -> "1,024").
func commaInt(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}
