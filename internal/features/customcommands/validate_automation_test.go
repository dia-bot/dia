package customcommands

import (
	"encoding/json"
	"testing"
)

func spec(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	return b
}

func hasCode(r ValidationResult, code string) bool {
	for _, i := range r.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func TestAutomationReplyAtRootFails(t *testing.T) {
	def := Definition{Steps: []Step{
		{ID: "r", Kind: KindReply, Spec: spec(t, SpecReply{Content: "hi"})},
	}}
	r := ValidateAutomation(def)
	if r.OK {
		t.Fatal("reply at root (no interaction) should fail")
	}
	if !hasCode(r, "needs_interaction") {
		t.Fatalf("expected needs_interaction, got %+v", r.Issues)
	}
}

func TestAutomationReplyAfterComponentWaitOK(t *testing.T) {
	def := Definition{Steps: []Step{
		{ID: "m", Kind: KindSendMessage, Spec: spec(t, SpecSendMessage{Channel: Expr{Src: "{{ .Channel.ID }}"}, Content: "click"})},
		{ID: "w", Kind: KindWaitFor, Spec: spec(t, SpecWaitFor{Trigger: "component", Timeout: "30s", Into: "click"})},
		{ID: "r", Kind: KindReply, Spec: spec(t, SpecReply{Content: "thanks"})},
	}}
	r := ValidateAutomation(def)
	if !r.OK {
		t.Fatalf("reply after a component wait should be valid, got %+v", r.Issues)
	}
}

func TestAutomationReplyInTimeoutFails(t *testing.T) {
	def := Definition{Steps: []Step{
		{ID: "w", Kind: KindWaitFor, Spec: spec(t, SpecWaitFor{
			Trigger:   "component",
			Timeout:   "30s",
			Into:      "click",
			OnTimeout: []Step{{ID: "r", Kind: KindReply, Spec: spec(t, SpecReply{Content: "too late"})}},
		})},
	}}
	r := ValidateAutomation(def)
	if r.OK {
		t.Fatal("reply inside on_timeout (no interaction) should fail")
	}
	if !hasCode(r, "needs_interaction") {
		t.Fatalf("expected needs_interaction in on_timeout, got %+v", r.Issues)
	}
}

func TestAutomationWaitTooLongWarns(t *testing.T) {
	def := Definition{Steps: []Step{
		{ID: "w", Kind: KindWaitFor, Spec: spec(t, SpecWaitFor{Trigger: "component", Timeout: "5m", Into: "click"})},
	}}
	r := ValidateAutomation(def)
	if !hasCode(r, "wait_too_long") {
		t.Fatalf("expected wait_too_long warning, got %+v", r.Issues)
	}
	// A warning shouldn't block publishing.
	if !r.OK {
		t.Fatalf("a long-wait warning should not fail validation, got %+v", r.Issues)
	}
}

func TestAutomationReplyInComponentSwitchOK(t *testing.T) {
	// The click-router shape: message → wait_for(component) → switch on click id,
	// with the reply inside a case. Reply is valid because the switch is the
	// wait's continuation.
	def := Definition{Steps: []Step{
		{ID: "m", Kind: KindSendMessage, Spec: spec(t, SpecSendMessage{Channel: Expr{Src: "{{ .Channel.ID }}"}, Content: "vote"})},
		{ID: "w", Kind: KindWaitFor, Spec: spec(t, SpecWaitFor{Trigger: "component", Timeout: "30s", Into: "click"})},
		{ID: "s", Kind: KindSwitch, Spec: spec(t, SpecSwitch{On: Expr{Src: "{{ .Vars.click.id }}"}}), Cases: []SwitchCase{
			{When: Expr{Src: "yes"}, Do: []Step{{ID: "r", Kind: KindReply, Spec: spec(t, SpecReply{Content: "ok"})}}},
		}},
	}}
	r := ValidateAutomation(def)
	if !r.OK {
		t.Fatalf("reply in a click-router switch case should be valid, got %+v", r.Issues)
	}
}
