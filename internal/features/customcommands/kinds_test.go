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
