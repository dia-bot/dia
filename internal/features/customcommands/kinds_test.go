package customcommands

import (
	"encoding/json"
	"testing"
)

func TestClickResponseFor(t *testing.T) {
	s := &SpecWaitFor{
		Response:  ClickResponseSilent,
		Responses: map[string]string{"aa": ClickResponseUpdate},
	}
	if got := s.ResponseFor("aa"); got != ClickResponseUpdate {
		t.Fatalf("suffix override: got %q", got)
	}
	if got := s.ResponseFor("bb"); got != ClickResponseSilent {
		t.Fatalf("listener default: got %q", got)
	}
	if got := (&SpecWaitFor{}).ResponseFor("x"); got != ClickResponseReply {
		t.Fatalf("absent config must mean reply: got %q", got)
	}
}

// The exact JSONB the dashboard's click-line editor writes must round-trip
// into the runtime's resolution (editor ↔ runtime mirror contract).
func TestSpecWaitForDecodesEditorPayload(t *testing.T) {
	raw := `{"trigger":"component","into":"click","timeout":"5m","responses":{"aa111":"update","bb222":"silent"}}`
	var ws SpecWaitFor
	if err := json.Unmarshal([]byte(raw), &ws); err != nil {
		t.Fatal(err)
	}
	if ws.ResponseFor("aa111") != ClickResponseUpdate || ws.ResponseFor("bb222") != ClickResponseSilent {
		t.Fatalf("unexpected resolution: %+v", ws.Responses)
	}
	if ws.ResponseFor("other") != ClickResponseReply {
		t.Fatal("unkeyed suffix must fall back to reply")
	}
}

// Component-level conflicts Discord rejects at send time must surface in
// preflight: noop on a link button, and duplicate static custom ids.
func TestValidateComponentConflicts(t *testing.T) {
	mk := func(comps ...Component) Definition {
		spec, _ := json.Marshal(SpecReply{Content: "x", Components: []ComponentRow{{Components: comps}}})
		return Definition{Steps: []Step{{ID: "s1", Kind: KindReply, Spec: spec}}}
	}
	has := func(r ValidationResult, code string) bool {
		for _, iss := range r.Issues {
			if iss.Code == code {
				return true
			}
		}
		return false
	}

	r := Validate("cmd", mk(Component{Type: "button", Style: "link", URL: "https://x", OnClick: "none"}))
	if !has(r, "on_click_link_conflict") {
		t.Fatalf("link+noop should fail: %+v", r.Issues)
	}

	r = Validate("cmd", mk(
		Component{Type: "button", Style: "primary", Label: "A", CustomIDSuffix: "dup"},
		Component{Type: "button", Style: "secondary", Label: "B", CustomIDSuffix: "dup"},
	))
	if !has(r, "custom_id_duplicate") {
		t.Fatalf("duplicate static suffixes should warn: %+v", r.Issues)
	}

	r = Validate("cmd", mk(
		Component{Type: "button", Style: "primary", Label: "A", CustomIDSuffix: "vote_{{ .Vars.i }}"},
		Component{Type: "button", Style: "secondary", Label: "B", CustomIDSuffix: "vote_{{ .Vars.j }}"},
	))
	if has(r, "custom_id_duplicate") {
		t.Fatalf("templated suffixes must not be compared: %+v", r.Issues)
	}

	r = Validate("cmd", mk(
		Component{Type: "button", Style: "primary", Label: "A", CustomIDSuffix: "a", OnClick: "none"},
		Component{Type: "button", Style: "secondary", Label: "B", CustomIDSuffix: "b"},
	))
	if has(r, "custom_id_duplicate") || has(r, "on_click_link_conflict") {
		t.Fatalf("distinct ids must pass: %+v", r.Issues)
	}
}
