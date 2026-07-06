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

// TestRenderCardKV covers getKV / getGuildKV backed by the ctx lookup, and the
// no-lookup fallback (getKV → "").
func TestRenderCardKV(t *testing.T) {
	e := New()
	data := DataFromVars(map[string]string{})
	kv := func(scope, key string) (string, bool) {
		switch scope + "/" + key {
		case "member/coins":
			return "150", true
		case "guild/theme":
			return "#FFD700", true
		}
		return "", false
	}
	ctx := WithCardKV(context.Background(), kv)

	cases := []struct{ name, in, want string }{
		{"member kv", "{{ getKV \"coins\" }}", "150"},
		{"guild kv", "{{ getGuildKV \"theme\" }}", "#FFD700"},
		{"missing key", "{{ getKV \"nope\" }}", ""},
		{"kv in math", "{{ if gt (toInt (getKV \"coins\")) 100 }}rich{{ else }}poor{{ end }}", "rich"},
	}
	for _, c := range cases {
		got, err := e.RenderCard(ctx, c.in, data)
		if err != nil {
			t.Errorf("%s: error: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: = %q, want %q", c.name, got, c.want)
		}
	}

	// No lookup on ctx → getKV renders empty, no error.
	if got, err := e.RenderCard(context.Background(), "{{ getKV \"coins\" }}", data); err != nil || got != "" {
		t.Fatalf("no-lookup: got %q err %v", got, err)
	}
}
