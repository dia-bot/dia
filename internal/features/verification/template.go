package verification

import (
	"bytes"
	"strings"
	"text/template"

	"github.com/dia-bot/dia/internal/event"
)

// tmplScope is the tiny scope a verification WelcomeText is rendered against.
// It mirrors the shape used elsewhere in the app (a subset of the custom-command
// scope) so the same {{ .User.Mention }} / {{ .Guild.Name }} placeholders work.
type tmplScope struct {
	User  tmplUser
	Guild tmplGuild
}

type tmplUser struct {
	ID       string
	Username string
	Mention  string
}

type tmplGuild struct {
	ID   string
	Name string
}

// renderTemplate renders raw as a Go text/template against the join scope. On any
// parse/exec error it falls back to the raw string so a typo never blocks the
// gate. Mirrors moderation/actions.go's helper.
func renderTemplate(raw string, u event.User, guildID, guildName string) string {
	if !strings.Contains(raw, "{{") {
		return raw
	}
	t, err := template.New("verify").Option("missingkey=zero").Parse(raw)
	if err != nil {
		return raw
	}
	name := u.GlobalName
	if name == "" {
		name = u.Username
	}
	scope := tmplScope{
		User:  tmplUser{ID: u.ID, Username: name, Mention: "<@" + u.ID + ">"},
		Guild: tmplGuild{ID: guildID, Name: guildName},
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, scope); err != nil {
		return raw
	}
	return buf.String()
}
