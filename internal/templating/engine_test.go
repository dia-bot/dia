package templating

import (
	"context"
	"strings"
	"testing"
)

func sample() *Context {
	return &Context{
		User:    User{ID: "1", Username: "ada", GlobalName: "Ada"},
		Member:  Member{Nick: "", Roles: []string{"10", "20"}},
		Guild:   Guild{ID: "9", Name: "Aurora SMP", MemberCount: 1024},
		Channel: Channel{ID: "5", Name: "welcome"},
		Args:    []string{"hello", "world"},
	}
}

func render(t *testing.T, src string) (string, error) {
	t.Helper()
	return New().Render(context.Background(), src, sample(), nil)
}

type fakeLookup struct{}

func (fakeLookup) Role(nameOrID string) (*RoleInfo, bool) {
	if nameOrID == "Mod" || nameOrID == "10" {
		return &RoleInfo{ID: "10", Name: "Mod", Color: 0xff6363}, true
	}
	return nil, false
}
func (fakeLookup) Channel(nameOrID string) (*ChannelInfo, bool) {
	if nameOrID == "general" || nameOrID == "5" {
		return &ChannelInfo{ID: "5", Name: "general", Type: 0}, true
	}
	return nil, false
}

func TestRenderBasics(t *testing.T) {
	cases := map[string]string{
		`Hi {{.User.GlobalName}}!`:                                 "Hi Ada!",
		`{{upper .User.Username}}`:                                 "ADA",
		`{{.Guild.Name}} has {{.Guild.MemberCount}}`:               "Aurora SMP has 1024",
		`{{add 1 2 3}}`:                                            "6",
		`{{mul 2 (add 3 4)}}`:                                      "14",
		`{{if gt .Guild.MemberCount 1000}}big{{else}}small{{end}}`: "big",
		`{{range .Member.Roles}}{{mentionRole .}} {{end}}`:         "<@&10> <@&20> ",
		`{{default "none" .Member.Nick}}`:                          "none",
		`{{join ", " .Member.Roles}}`:                              "10, 20",
		`{{index .Args 1}}`:                                        "world",
		`{{title "hello there"}}`:                                  "Hello There",
	}
	for src, want := range cases {
		got, err := render(t, src)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", src, err)
			continue
		}
		if got != want {
			t.Errorf("%q = %q, want %q", src, got, want)
		}
	}
}

func TestOutputCapped(t *testing.T) {
	// 4000-char cap: 5000 'x' must error.
	_, err := render(t, `{{repeat 1000 "xxxxx"}}`)
	if err == nil {
		t.Fatal("expected an output-too-long error, got nil")
	}
}

func TestSeqIsBounded(t *testing.T) {
	// seq is capped at maxListLen, so this can't loop unbounded; small output here.
	got, err := render(t, `{{range seq 3}}{{.}}{{end}}`)
	if err != nil || got != "012" {
		t.Fatalf("seq render = %q, err %v", got, err)
	}
}

func TestNoActionFuncs(t *testing.T) {
	// Templates are pure by design: side-effecting functions must not exist —
	// actions are custom-command steps, never template calls.
	for _, fn := range []string{"sendDM", "sendMessage", "addRole", "removeRole", "addReaction"} {
		if _, err := render(t, `{{`+fn+` "1" "x"}}`); err == nil || !strings.Contains(err.Error(), "not defined") {
			t.Errorf("%s: expected a not-defined parse error, got %v", fn, err)
		}
	}
}

func TestLookupFuncs(t *testing.T) {
	out, err := New().Render(context.Background(),
		`{{(getRole "Mod").mention}} in {{(getChannel "general").mention}}`,
		sample(), fakeLookup{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "<@&10> in <#5>" {
		t.Fatalf("lookup render = %q", out)
	}
	// Unknown role → empty fields, no error.
	if out, err := New().Render(context.Background(), `[{{(getRole "Nope").name}}]`, sample(), fakeLookup{}); err != nil || out != "[]" {
		t.Fatalf("unknown role = %q, err %v", out, err)
	}
	// nil lookup → getRole errors (falls back upstream).
	if _, err := New().Render(context.Background(), `{{getRole "Mod"}}`, sample(), nil); err == nil {
		t.Fatal("expected an error when lookup is nil")
	}
}
