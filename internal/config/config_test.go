package config

import "testing"

func TestStripInlineComment(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain value", "DIA_EVENTS", "DIA_EVENTS"},
		{"trailing comment", "DIA_EVENTS        # stream name", "DIA_EVENTS"},
		{"tab before comment", "DIA_EVENTS\t# stream name", "DIA_EVENTS"},
		{"comment only", "# just a comment", ""},
		{"url fragment kept", "http://x:8080#frag", "http://x:8080#frag"},
		{"db url untouched", "postgres://dia:dia@localhost:5432/dia?sslmode=disable", "postgres://dia:dia@localhost:5432/dia?sslmode=disable"},
		{"quoted hash literal", `"value # not a comment"`, `"value # not a comment"`},
		{"quoted then comment", `"value" # comment`, `"value"`},
		{"single quoted hash literal", `'a # b'`, `'a # b'`},
		{"empty", "", ""},
		{"hash no space kept", "abc#def", "abc#def"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stripInlineComment(c.in); got != c.want {
				t.Errorf("stripInlineComment(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
