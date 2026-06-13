package runtime

import (
	"testing"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/automations"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/store"
)

func TestVoiceTransition(t *testing.T) {
	cases := []struct{ prev, next, want string }{
		{"", "123", "join"},
		{"123", "", "leave"},
		{"123", "456", "move"},
		{"123", "123", ""}, // mute/deafen toggle, no channel change
		{"", "", ""},
	}
	for _, c := range cases {
		if got := voiceTransition(c.prev, c.next); got != c.want {
			t.Errorf("voiceTransition(%q,%q)=%q want %q", c.prev, c.next, got, c.want)
		}
	}
}

func TestRoleChanged(t *testing.T) {
	if !roleChanged([]string{"1", "2"}, "") {
		t.Error("any role change should match an unset watched role")
	}
	if !roleChanged([]string{"1", "2"}, "2") {
		t.Error("watched role present should match")
	}
	if roleChanged([]string{"1", "2"}, "9") {
		t.Error("watched role absent should not match")
	}
	if roleChanged(nil, "1") {
		t.Error("no change should never match")
	}
}

func TestKeywordMatches(t *testing.T) {
	contains := automations.TriggerConfig{Keywords: []string{"hello"}, MatchMode: "contains"}
	if !keywordMatches(contains, "well HELLO there") {
		t.Error("contains should be case-insensitive substring")
	}
	if keywordMatches(contains, "goodbye") {
		t.Error("contains should not match absent keyword")
	}
	equals := automations.TriggerConfig{Keywords: []string{"ping"}, MatchMode: "equals"}
	if !keywordMatches(equals, "PING") {
		t.Error("equals should match the whole content case-insensitively")
	}
	if keywordMatches(equals, "ping pong") {
		t.Error("equals should not match a superset")
	}
	word := automations.TriggerConfig{Keywords: []string{"cat"}, MatchMode: "word"}
	if !keywordMatches(word, "the cat sat") {
		t.Error("word should match a whole word")
	}
	if keywordMatches(word, "category") {
		t.Error("word should not match inside another word")
	}
}

func TestEmojiMatches(t *testing.T) {
	ev := map[string]any{"emoji": "👍", "emoji_name": "thumbsup", "emoji_id": ""}
	if !emojiMatches([]string{"👍"}, ev) {
		t.Error("should match by unicode glyph")
	}
	if !emojiMatches([]string{"thumbsup"}, ev) {
		t.Error("should match by name")
	}
	if emojiMatches([]string{"❤️"}, ev) {
		t.Error("should not match a different emoji")
	}
	custom := map[string]any{"emoji": "<:blob:42>", "emoji_name": "blob", "emoji_id": "42"}
	if !emojiMatches([]string{"42"}, custom) {
		t.Error("should match a custom emoji by id")
	}
}

func TestReactionGlyph(t *testing.T) {
	if g := reactionGlyph(event.Emoji{Name: "👍"}); g != "👍" {
		t.Errorf("unicode glyph = %q", g)
	}
	if g := reactionGlyph(event.Emoji{Name: "blob", ID: "42"}); g != "<:blob:42>" {
		t.Errorf("custom glyph = %q", g)
	}
	if g := reactionGlyph(event.Emoji{Name: "spin", ID: "9", Animated: true}); g != "<a:spin:9>" {
		t.Errorf("animated glyph = %q", g)
	}
}

func TestMatchTransitionGating(t *testing.T) {
	p := &Plugin{}
	ec := &eventContext{voiceKind: "join"}
	joinAuto := store.Automation{TriggerType: "voice_join"}
	if !p.matches(nil, joinAuto, automations.TriggerConfig{}, ec) {
		t.Error("voice_join should match a join transition")
	}
	leaveAuto := store.Automation{TriggerType: "voice_leave"}
	if p.matches(nil, leaveAuto, automations.TriggerConfig{}, ec) {
		t.Error("voice_leave should NOT match a join transition")
	}
}

func TestMatchChannelFilter(t *testing.T) {
	p := &Plugin{}
	ec := &eventContext{channelID: "100"}
	a := store.Automation{TriggerType: "message_create"}
	if p.matches(nil, a, automations.TriggerConfig{Channels: []string{"200"}}, ec) {
		t.Error("channel allowlist should exclude a non-listed channel")
	}
	if !p.matches(nil, a, automations.TriggerConfig{Channels: []string{"100"}}, ec) {
		t.Error("channel allowlist should include a listed channel")
	}
	if !p.matches(nil, a, automations.TriggerConfig{}, ec) {
		t.Error("empty allowlist should match any channel")
	}
	if p.matches(nil, a, automations.TriggerConfig{IgnoreChannels: []string{"100"}}, ec) {
		t.Error("ignore list should drop a listed channel")
	}
}

func TestResumeWaitKind(t *testing.T) {
	if resumeWaitKind(event.TypeMessageCreate) != "message" {
		t.Error("MESSAGE_CREATE should resume message waits")
	}
	if resumeWaitKind(event.TypeReactionAdd) != "reaction" {
		t.Error("MESSAGE_REACTION_ADD should resume reaction waits")
	}
	// Reaction-remove and others don't resume waits.
	if resumeWaitKind(event.TypeReactionRemove) != "" {
		t.Error("MESSAGE_REACTION_REMOVE should not resume waits")
	}
	if resumeWaitKind(event.TypeMemberAdd) != "" {
		t.Error("member add should not resume waits")
	}
}

func TestWaitChannelMatches(t *testing.T) {
	// any: matches everything.
	if !waitChannelMatches(cc.SpecWaitFor{ChannelMode: "any"}, "10", "99") {
		t.Error("any should match any channel")
	}
	if !waitChannelMatches(cc.SpecWaitFor{}, "10", "99") {
		t.Error("unset mode defaults to any")
	}
	// current: only the run's channel.
	if !waitChannelMatches(cc.SpecWaitFor{ChannelMode: "current"}, "10", "10") {
		t.Error("current should match the run channel")
	}
	if waitChannelMatches(cc.SpecWaitFor{ChannelMode: "current"}, "10", "11") {
		t.Error("current should reject a different channel")
	}
	// only: allowlist.
	only := cc.SpecWaitFor{ChannelMode: "only", Channels: []string{"5", "6"}}
	if !waitChannelMatches(only, "10", "6") {
		t.Error("only should match a listed channel")
	}
	if waitChannelMatches(only, "10", "7") {
		t.Error("only should reject an unlisted channel")
	}
	// except: denylist.
	except := cc.SpecWaitFor{ChannelMode: "except", Channels: []string{"5"}}
	if waitChannelMatches(except, "10", "5") {
		t.Error("except should reject a listed channel")
	}
	if !waitChannelMatches(except, "10", "9") {
		t.Error("except should match an unlisted channel")
	}
}

func TestWaitPayloadMessage(t *testing.T) {
	ec := &eventContext{
		channelID: "42",
		user:      event.User{ID: "7"},
		eventMap: map[string]any{
			"content": "hello",
			"message": map[string]any{"id": "99", "channel_id": "42"},
		},
	}
	p := waitPayload("message", ec)
	if p["id"] != "99" || p["content"] != "hello" || p["user_id"] != "7" || p["channel_id"] != "42" {
		t.Fatalf("unexpected message payload: %+v", p)
	}
}

func TestMatchIgnoreBots(t *testing.T) {
	p := &Plugin{}
	ec := &eventContext{user: event.User{ID: "1", Bot: true}}
	a := store.Automation{TriggerType: "message_create"}
	if p.matches(nil, a, automations.TriggerConfig{IgnoreBots: true}, ec) {
		t.Error("ignore_bots should drop a bot actor")
	}
	ec.user.Bot = false
	if !p.matches(nil, a, automations.TriggerConfig{IgnoreBots: true}, ec) {
		t.Error("ignore_bots should keep a human actor")
	}
}
