package templating

import (
	"context"
	"testing"
)

func TestRenderCard(t *testing.T) {
	e := New()
	data := DataFromVars(map[string]string{
		"{user}":          "Ada",
		"{user.name}":     "ada",
		"{user.avatar}":   "https://cdn/avatar.png",
		"{count}":         "1024",
		"{count.ordinal}": "1,024th",
		"{server}":        "Aurora",
		"{rank}":          "1",
	})
	cases := []struct{ name, in, want string }{
		{"username", "Welcome, {{.User.Username}}!", "Welcome, ada!"},
		{"display name", "Hi {{.User.Name}}", "Hi Ada"},
		{"avatar src", "{{.User.Avatar}}", "https://cdn/avatar.png"},
		{"server", "{{.Server.Name}}", "Aurora"},
		{"count", "Member #{{.Count}}", "Member #1024"},
		{"ordinal", "The {{.CountOrdinal}} member", "The 1,024th member"},
		{"conditional", `{{if eq .Rank "1"}}🥇{{else}}—{{end}}`, "🥇"},
		{"pipeline", "{{.User.Name | upper}}", "ADA"},
		{"plain", "No vars here", "No vars here"},
	}
	for _, c := range cases {
		got, err := e.RenderCard(context.Background(), c.in, data)
		if err != nil {
			t.Errorf("%s: error: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: RenderCard(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestRenderCardEmpty(t *testing.T) {
	e := New()
	if got, err := e.RenderCard(context.Background(), "", nil); err != nil || got != "" {
		t.Fatalf("empty: got %q err %v", got, err)
	}
}
