package customcommands

import "encoding/json"

// Step kinds — the closed enum of every primitive the program can perform.
// Grouped by what they touch. New kinds added here also need a handler
// registered in exec/registry.go.
const (
	// Reply / message surface
	KindDeferReply       = "defer_reply"
	KindReply            = "reply"
	KindEditReply        = "edit_reply"
	KindSendMessage      = "send_message"
	KindSendDM           = "send_dm"
	KindEmbedSend        = "embed_send"
	KindModalOpen        = "modal_open"
	KindMessageEdit      = "message_edit"
	KindMessageFetch     = "message_fetch"
	KindMessageDelete    = "message_delete"
	KindMessagePurge     = "message_purge"
	KindMessageCrosspost = "message_crosspost"
	KindReactAdd         = "react_add"
	KindReactRemove      = "react_remove"
	KindReactClear       = "react_clear"
	KindPinAdd           = "pin_add"
	KindPinRemove        = "pin_remove"

	// Members & roles
	KindRoleAdd        = "role_add"
	KindRoleRemove     = "role_remove"
	KindMemberNickname = "member_nickname"
	KindMemberKick     = "member_kick"
	KindMemberBan      = "member_ban"
	KindMemberUnban    = "member_unban"
	KindMemberTimeout  = "member_timeout"
	KindMemberFetch    = "member_fetch"

	// Channels / threads / voice
	KindChannelCreate = "channel_create"
	KindChannelEdit   = "channel_edit"
	KindChannelDelete = "channel_delete"
	KindThreadCreate  = "thread_create"
	KindThreadArchive = "thread_archive"
	KindThreadMember  = "thread_member"
	KindInviteCreate  = "invite_create"
	KindVoiceMove     = "voice_move"
	KindVoiceSet      = "voice_set"

	// Image
	KindImageRender = "image_render"
	KindImageAttach = "image_attach"
	KindImageLoad   = "image_load"

	// Data / variables
	KindSetVar     = "set_var"
	KindIncrVar    = "incr_var"
	KindPickRandom = "pick_random"
	KindJSONParse  = "json_parse"
	KindKVGet      = "kv_get"
	KindKVSet      = "kv_set"
	KindKVDelete   = "kv_delete"
	KindHTTPReq    = "http_request"

	// Control flow
	KindIf       = "if"
	KindSwitch   = "switch"
	KindLoop     = "loop"
	KindParallel = "parallel"
	KindWait     = "wait"
	KindWaitFor  = "wait_for"
	KindExit     = "exit"
	KindFail     = "fail"
	KindNoop     = "noop"

	// Sub-flows & audit
	KindRunCommand    = "run_command"
	KindRunAutomation = "run_automation"
	KindAuditNote     = "audit_note"
	KindGiveawayStart = "giveaway_start"
)

// Latency is the worst-case timing class for a step. The validator walks the
// tree at publish time: if any step on the path from root to the first
// user-visible reply is `slow` or `defer`, a `defer_reply` is auto-inserted.
type Latency int

const (
	LatencyInstant Latency = iota // pure compute, no I/O
	LatencyNetwork                // Discord REST or DB read (~50-300 ms)
	LatencySlow                   // image render, http_request (~500 ms - 5 s)
	LatencyDefer                  // wait / wait_for — yields to scheduler
)

// LatencyOf returns the latency class for a step kind.
func LatencyOf(kind string) Latency {
	switch kind {
	case KindNoop, KindSetVar, KindIncrVar, KindIf, KindSwitch, KindLoop,
		KindExit, KindFail, KindAuditNote, KindImageAttach,
		KindPickRandom, KindJSONParse:
		return LatencyInstant
	case KindWait, KindWaitFor:
		return LatencyDefer
	case KindImageRender, KindHTTPReq, KindImageLoad, KindModalOpen, KindParallel:
		return LatencySlow
	default:
		return LatencyNetwork
	}
}

// IsUserVisibleReply reports whether the step kind sends Discord-side output
// the invoker would see as the "first response" — used by the static analyzer
// to decide whether a defer is needed before reaching it.
func IsUserVisibleReply(kind string) bool {
	switch kind {
	case KindReply, KindEditReply, KindEmbedSend, KindSendMessage,
		KindDeferReply, KindModalOpen:
		return true
	}
	return false
}

// IsControl reports whether the kind is control flow (recurses into children).
func IsControl(kind string) bool {
	switch kind {
	case KindIf, KindSwitch, KindLoop, KindParallel:
		return true
	}
	return false
}

// ── Per-kind spec structs ────────────────────────────────────────────────────
// Each step's Spec JSONB is decoded into one of these by the step handler.
// Templated strings render at runtime against the current Scope.

// MsgMentions controls which mentions in a message actually ping. Nil means the
// safe default: only user mentions ping, while @everyone/@here and role mentions
// render but stay inert. Set fields to opt specific kinds back in.
type MsgMentions struct {
	Users    bool `json:"users,omitempty"`
	Roles    bool `json:"roles,omitempty"`
	Everyone bool `json:"everyone,omitempty"`
}

// SpecReply is the spec for a `reply` step.
type SpecReply struct {
	Content         string          `json:"content,omitempty"`
	Ephemeral       bool            `json:"ephemeral,omitempty"`
	Embeds          []EmbedSpec     `json:"embeds,omitempty"`
	Components      []ComponentRow  `json:"components,omitempty"`
	Attachments     []AttachmentRef `json:"attachments,omitempty"`
	AllowedMentions *MsgMentions    `json:"allowed_mentions,omitempty"`
}

// SpecEditReply is the spec for an `edit_reply` step.
type SpecEditReply struct {
	Content         string          `json:"content,omitempty"`
	Embeds          []EmbedSpec     `json:"embeds,omitempty"`
	Components      []ComponentRow  `json:"components,omitempty"`
	Attachments     []AttachmentRef `json:"attachments,omitempty"`
	AllowedMentions *MsgMentions    `json:"allowed_mentions,omitempty"`
}

// SpecDeferReply is the spec for a `defer_reply` step.
type SpecDeferReply struct {
	Ephemeral bool `json:"ephemeral,omitempty"`
}

// SpecSendMessage is the spec for a `send_message` step.
type SpecSendMessage struct {
	Channel         Expr            `json:"channel"`
	Content         string          `json:"content,omitempty"`
	Embeds          []EmbedSpec     `json:"embeds,omitempty"`
	Components      []ComponentRow  `json:"components,omitempty"`
	Attachments     []AttachmentRef `json:"attachments,omitempty"`
	Into            string          `json:"into,omitempty"`
	ReplyTo         Expr            `json:"reply_to,omitempty"` // message id to reply to
	AllowedMentions *MsgMentions    `json:"allowed_mentions,omitempty"`
}

// SpecSendDM is the spec for a `send_dm` step. DMs carry the full message
// surface — embeds, component rows (buttons / selects route back via the
// same ccmd custom_id scheme) and attachments.
type SpecSendDM struct {
	User            Expr            `json:"user"`
	Content         string          `json:"content,omitempty"`
	Embeds          []EmbedSpec     `json:"embeds,omitempty"`
	Components      []ComponentRow  `json:"components,omitempty"`
	Attachments     []AttachmentRef `json:"attachments,omitempty"`
	AllowedMentions *MsgMentions    `json:"allowed_mentions,omitempty"`
}

// SpecEmbedSend is sugar over send_message for a single-embed message.
type SpecEmbedSend struct {
	Channel         Expr         `json:"channel"`
	Embed           EmbedSpec    `json:"embed"`
	Into            string       `json:"into,omitempty"`
	AllowedMentions *MsgMentions `json:"allowed_mentions,omitempty"`
}

// EmbedSpec is one Discord embed with templated fields.
type EmbedSpec struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	URL         string       `json:"url,omitempty"`
	Color       string       `json:"color,omitempty"` // hex
	AuthorName  string       `json:"author_name,omitempty"`
	AuthorIcon  string       `json:"author_icon,omitempty"`
	AuthorURL   string       `json:"author_url,omitempty"`
	Thumbnail   string       `json:"thumbnail,omitempty"`
	ImageURL    string       `json:"image_url,omitempty"`
	FooterText  string       `json:"footer_text,omitempty"`
	FooterIcon  string       `json:"footer_icon,omitempty"`
	Timestamp   bool         `json:"timestamp,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
}

// EmbedField is one name/value row in an embed.
type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// ComponentRow is one row of message components attached to a reply.
type ComponentRow struct {
	Components []Component `json:"components"`
}

// NoopCustomIDPrefix marks components whose clicks the worker acknowledges
// silently and statelessly (DEFERRED_UPDATE_MESSAGE): no run lookup, no
// steps, no expiry. The segment can never collide with a run id.
const NoopCustomIDPrefix = "ccmd:noop:"

// Component is one button or select inside a row.
type Component struct {
	Type           string         `json:"type"`            // button | select_string | select_user | select_role | select_channel
	Style          string         `json:"style,omitempty"` // primary | secondary | success | danger | link
	Label          string         `json:"label,omitempty"`
	Emoji          string         `json:"emoji,omitempty"`
	CustomIDSuffix string         `json:"custom_id_suffix,omitempty"` // routed back to the run via ccmd:<run_id>:<suffix>
	URL            string         `json:"url,omitempty"`              // link buttons only
	Disabled       bool           `json:"disabled,omitempty"`
	Placeholder    string         `json:"placeholder,omitempty"`
	Options        []SelectOption `json:"options,omitempty"` // string select
	MinValues      *int           `json:"min_values,omitempty"`
	MaxValues      *int           `json:"max_values,omitempty"`

	// OnClick "none" makes a button decorative: every click is acknowledged
	// silently and nothing ever runs. Such buttons keep working long after
	// the run is gone (the custom_id carries no run reference).
	OnClick string `json:"on_click,omitempty"` // "" (routed) | "none"
}

// SelectOption is one option in a string-select component.
type SelectOption struct {
	Label       string `json:"label"`
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	Emoji       string `json:"emoji,omitempty"`
	Default     bool   `json:"default,omitempty"`
}

// AttachmentRef references an attachment to attach to the next message-shaped
// step — either a literal URL or a scope variable holding bytes (from
// image_render / image_load).
type AttachmentRef struct {
	FromVar  string `json:"from_var,omitempty"`
	URL      string `json:"url,omitempty"`
	Filename string `json:"filename,omitempty"`
}

// SpecReactAdd / SpecReactRemove
type SpecReact struct {
	Channel Expr   `json:"channel"`
	Message Expr   `json:"message"`
	Emoji   string `json:"emoji"`
	User    Expr   `json:"user,omitempty"` // remove someone else's reaction
}

// SpecReactClear removes every reaction (or one emoji's reactions) from a message.
type SpecReactClear struct {
	Channel Expr   `json:"channel"`
	Message Expr   `json:"message"`
	Emoji   string `json:"emoji,omitempty"` // empty = clear all
}

// SpecMessageEdit edits an existing message the bot sent — the full message
// surface, so status boards / panels can be updated in place. Target "reply"
// edits the command's own interaction reply (subsumes the old edit_reply);
// otherwise Channel+Message locate the message.
type SpecMessageEdit struct {
	Target          string         `json:"target,omitempty"` // "" (specific message) | "reply"
	Channel         Expr           `json:"channel,omitempty"`
	Message         Expr           `json:"message,omitempty"`
	Content         string         `json:"content,omitempty"`
	Embeds          []EmbedSpec    `json:"embeds,omitempty"`
	Components      []ComponentRow `json:"components,omitempty"`
	AllowedMentions *MsgMentions   `json:"allowed_mentions,omitempty"`
}

// SpecMessageFetch reads an existing message into scope so conditions can
// branch on its content / author / state.
type SpecMessageFetch struct {
	Channel Expr   `json:"channel"`
	Message Expr   `json:"message"`
	Into    string `json:"into"`
}

// SpecMessagePurge bulk-deletes recent messages in a channel with optional
// filters. Messages older than 14 days are skipped (Discord limit).
type SpecMessagePurge struct {
	Channel  Expr   `json:"channel"`
	Limit    int    `json:"limit"` // 1..100
	FromUser Expr   `json:"from_user,omitempty"`
	BotsOnly bool   `json:"bots_only,omitempty"`
	Contains string `json:"contains,omitempty"` // templated substring filter
	Reason   string `json:"reason,omitempty"`
	Into     string `json:"into,omitempty"` // deleted count
}

// SpecMessageCrosspost publishes an announcement-channel message to followers.
type SpecMessageCrosspost struct {
	Channel Expr `json:"channel"`
	Message Expr `json:"message"`
}

// SpecMemberFetch reads another member into scope (roles, joined_at, nick…)
// so flows can gate on properties of someone other than the invoker.
type SpecMemberFetch struct {
	User Expr   `json:"user"`
	Into string `json:"into"`
}

// SpecVoiceSet server-mutes / deafens a member (pointers = leave unchanged).
type SpecVoiceSet struct {
	User   Expr   `json:"user"`
	Mute   *bool  `json:"mute,omitempty"`
	Deafen *bool  `json:"deafen,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// SpecThreadMember adds or removes a member from a thread (ticket flows).
type SpecThreadMember struct {
	Thread Expr   `json:"thread"`
	User   Expr   `json:"user"`
	Action string `json:"action"` // add | remove
}

// SpecInviteCreate mints an invite for a channel; the code/URL land in scope.
type SpecInviteCreate struct {
	Channel   Expr   `json:"channel"`
	MaxAge    string `json:"max_age,omitempty"` // Go duration; empty = server default
	MaxUses   int    `json:"max_uses,omitempty"`
	Temporary bool   `json:"temporary,omitempty"`
	Unique    bool   `json:"unique,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Into      string `json:"into"`
}

// SpecPickRandom picks one (or N) random entries from a list value — lists
// from scope, or a string split on newlines / commas.
type SpecPickRandom struct {
	From  Expr   `json:"from"`
	Count int    `json:"count,omitempty"` // default 1
	Into  string `json:"into"`
}

// SpecJSONParse parses a JSON string into a structured scope value so
// `.Vars.x.field` and loops work over it.
type SpecJSONParse struct {
	Value Expr   `json:"value"`
	Into  string `json:"into"`
}

// SpecMessageOp is shared by message_delete / pin_add / pin_remove.
type SpecMessageOp struct {
	Channel Expr   `json:"channel"`
	Message Expr   `json:"message"`
	Reason  string `json:"reason,omitempty"`
}

// SpecRole is shared by role_add / role_remove.
type SpecRole struct {
	User   Expr   `json:"user"`
	Role   Expr   `json:"role"`
	Reason string `json:"reason,omitempty"`
}

// SpecMember is shared by member_nickname / kick / ban / timeout.
type SpecMember struct {
	User              Expr   `json:"user"`
	Nickname          string `json:"nickname,omitempty"`
	Reason            string `json:"reason,omitempty"`
	DeleteMessageDays int    `json:"delete_message_days,omitempty"` // ban only
	Duration          string `json:"duration,omitempty"`            // timeout
}

// SpecChannelCreate is the spec for channel_create.
type SpecChannelCreate struct {
	Name             string `json:"name"`
	Type             string `json:"type"` // text | voice | category | announcement | forum | stage
	Parent           Expr   `json:"parent,omitempty"`
	Topic            string `json:"topic,omitempty"`
	NSFW             bool   `json:"nsfw,omitempty"`
	RateLimitPerUser int    `json:"rate_limit_per_user,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Into             string `json:"into,omitempty"`
}

// SpecChannelEdit edits an existing channel.
type SpecChannelEdit struct {
	Channel          Expr   `json:"channel"`
	Name             string `json:"name,omitempty"`
	Topic            string `json:"topic,omitempty"`
	RateLimitPerUser *int   `json:"rate_limit_per_user,omitempty"`
	NSFW             *bool  `json:"nsfw,omitempty"`
	Parent           Expr   `json:"parent,omitempty"`
	Locked           *bool  `json:"locked,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

// SpecChannelDelete deletes a channel or closes a thread.
type SpecChannelDelete struct {
	Channel Expr   `json:"channel"`
	Reason  string `json:"reason,omitempty"`
}

// SpecThreadCreate creates a thread.
type SpecThreadCreate struct {
	Channel        Expr   `json:"channel"`
	Message        Expr   `json:"message,omitempty"` // thread-from-message
	Name           string `json:"name"`
	AutoArchiveMin int    `json:"auto_archive_minutes,omitempty"`
	Private        bool   `json:"private,omitempty"`
	Invitable      bool   `json:"invitable,omitempty"`
	Into           string `json:"into,omitempty"`
}

// SpecThreadArchive archives or locks a thread.
type SpecThreadArchive struct {
	Thread Expr   `json:"thread"`
	Locked bool   `json:"locked,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// SpecVoiceMove moves a member between voice channels (or disconnects when Channel is nil).
type SpecVoiceMove struct {
	User    Expr `json:"user"`
	Channel Expr `json:"channel,omitempty"`
}

// SpecImageRender renders a Studio template to PNG and stows bytes in scope.
type SpecImageRender struct {
	TemplateID int64             `json:"template_id"`
	Vars       map[string]string `json:"vars,omitempty"` // each value is a templated string
	Into       string            `json:"into"`
}

// SpecImageAttach attaches a scope-resident image to the next reply.
type SpecImageAttach struct {
	FromVar  string `json:"from_var"`
	Filename string `json:"filename,omitempty"`
}

// SpecImageLoad loads an image from a URL into scope.
type SpecImageLoad struct {
	Source   Expr   `json:"source"`
	Into     string `json:"into"`
	MaxBytes int    `json:"max_bytes,omitempty"`
}

// SpecSetVar / SpecIncrVar are pure scope mutations.
type SpecSetVar struct {
	Name  string `json:"name"`
	Value Expr   `json:"value"`
}

type SpecIncrVar struct {
	Name string  `json:"name"`
	By   float64 `json:"by"`
}

// SpecKV is shared by kv_get / kv_set / kv_delete.
type SpecKV struct {
	Key     string          `json:"key"`                // templated
	Scope   string          `json:"scope"`              // guild | member
	OwnerID Expr            `json:"owner_id,omitempty"` // for scope=member; defaults to ctx.user.id
	Value   Expr            `json:"value,omitempty"`
	Default json.RawMessage `json:"default,omitempty"`
	TTL     string          `json:"ttl,omitempty"` // "1h", "7d" (parsed by time.ParseDuration)
	Into    string          `json:"into,omitempty"`
	// Shared stores/reads in the guild-SHARED card namespace (command_id ""), so
	// Card Studio formulas can read the value via getKV / getGuildKV. Off = the
	// value stays private to this command.
	Shared bool `json:"shared,omitempty"`
}

// SpecHTTP makes an outbound HTTP(S) request (SSRF-guarded).
type SpecHTTP struct {
	Method    string            `json:"method,omitempty"`
	URL       string            `json:"url"` // templated
	Headers   map[string]string `json:"headers,omitempty"`
	Body      Expr              `json:"body,omitempty"`
	TimeoutMs int               `json:"timeout_ms,omitempty"`
	Into      string            `json:"into,omitempty"`
	ParseJSON bool              `json:"parse_json,omitempty"`
}

// SpecIf is the spec for an `if` step.
type SpecIf struct {
	Cond Expr `json:"cond"`
}

// SpecSwitch is the spec for a `switch` step (cases live on Step.Cases).
type SpecSwitch struct {
	On Expr `json:"on"`
}

// SpecLoop is the spec for a `loop` step (body lives on Step.Then).
type SpecLoop struct {
	Over    Expr   `json:"over"`
	As      string `json:"as"`
	IndexAs string `json:"index_as,omitempty"`
	MaxIter int    `json:"max_iter,omitempty"` // default 100, hard cap 1000
}

// SpecParallel forks branches concurrently.
type SpecParallel struct {
	Branches [][]Step `json:"branches"`
	Join     string   `json:"join,omitempty"` // all (default) | race
}

// SpecWait pauses the run for a fixed duration.
type SpecWait struct {
	Duration string `json:"duration"` // parsed by time.ParseDuration
}

// Click-response modes: how the bot acknowledges the component interaction
// that resumes a wait_for, before any steps run.
//
//   - reply (default): deferred channel message; Discord shows the bot
//     thinking until the flow's first Message step replies.
//   - update: deferred update; nothing shows at the click, the flow's first
//     Message step rewrites the clicked message in place.
//   - silent: deferred update marked replied; nothing shows at the click
//     and any later Message step posts a fresh follow-up.
const (
	ClickResponseReply  = "reply"
	ClickResponseUpdate = "update"
	ClickResponseSilent = "silent"
)

// SpecWaitFor parks the run until a Discord event matches.
type SpecWaitFor struct {
	Trigger        string `json:"trigger"` // component | modal | message | reaction
	CustomIDSuffix string `json:"custom_id_suffix,omitempty"`
	FromUser       Expr   `json:"from_user,omitempty"` // restrict who can satisfy this wait
	Channel        Expr   `json:"channel,omitempty"`   // legacy single-channel filter
	// ChannelMode + Channels scope which channels satisfy a message/reaction
	// wait: "any" (default) | "current" (the run's channel) | "only" (in
	// Channels) | "except" (anywhere but Channels). Channels are concrete ids.
	ChannelMode string   `json:"channel_mode,omitempty"`
	Channels    []string `json:"channels,omitempty"`
	// Emoji optionally restricts a reaction wait to one emoji (glyph, name or id).
	Emoji     string `json:"emoji,omitempty"`
	Timeout   string `json:"timeout"` // parsed by time.ParseDuration
	Into      string `json:"into,omitempty"`
	OnTimeout []Step `json:"on_timeout,omitempty"`

	// Response is the click-response mode for this listener; Responses
	// overrides it per clicked button suffix (the click-router shares one
	// listener across a message's buttons, but each button keeps its own
	// behaviour). Templated suffixes can't be matched here and fall back to
	// Response.
	Response  string            `json:"response,omitempty"`
	Responses map[string]string `json:"responses,omitempty"`
}

// ResponseFor resolves the click-response mode for a clicked button suffix.
func (s *SpecWaitFor) ResponseFor(suffix string) string {
	if m, ok := s.Responses[suffix]; ok && m != "" {
		return m
	}
	if s.Response != "" {
		return s.Response
	}
	return ClickResponseReply
}

// SpecExit / SpecFail terminate the run.
type SpecExit struct {
	Reason string `json:"reason,omitempty"`
}
type SpecFail struct {
	Message string `json:"message,omitempty"`
}

// SpecRunCommand invokes another command in the same run.
type SpecRunCommand struct {
	Command      string          `json:"command"`
	Args         json.RawMessage `json:"args,omitempty"` // object {name: value}; string values are templated
	InheritScope bool            `json:"inherit_scope,omitempty"`
}

// SpecRunAutomation launches another automation in the same run: the target's
// step program walks inline against the current scope, so any flow (a custom
// command, an automation, or a welcome flow) can reuse an automation as a
// subroutine. The target is referenced by its automation id.
type SpecRunAutomation struct {
	Automation string `json:"automation"`
}

// SpecAuditNote writes a row to dashboard_audit_log.
type SpecAuditNote struct {
	Action string `json:"action"`
	Detail Expr   `json:"detail,omitempty"`
}

// SpecGiveawayStart starts a giveaway from a saved preset (managed on the
// Giveaways dashboard, which owns the composed message + the draw). Blank
// overrides fall back to the preset's defaults; the new giveaway's id is written
// to Into.
type SpecGiveawayStart struct {
	Preset   string `json:"preset,omitempty"`   // preset id (templated)
	Prize    Expr   `json:"prize"`              // required
	Channel  Expr   `json:"channel,omitempty"`  // snowflake / #mention; default = preset channel
	Duration Expr   `json:"duration,omitempty"` // e.g. "24h", "3d"; default = preset duration
	Winners  Expr   `json:"winners,omitempty"`  // int; default = preset winners
	Into     string `json:"into,omitempty"`     // var to receive the new giveaway id
}

// SpecModalOpen opens a modal in response to the interaction; the result lands
// in scope under Into.
type SpecModalOpen struct {
	Title          string       `json:"title"`
	CustomIDSuffix string       `json:"custom_id_suffix"`
	Fields         []ModalField `json:"fields"`
	Timeout        string       `json:"timeout,omitempty"` // optional wait timeout
	Into           string       `json:"into,omitempty"`
}

// ModalField is one input row in a modal.
type ModalField struct {
	CustomID    string `json:"custom_id"`
	Label       string `json:"label"`
	Style       string `json:"style,omitempty"` // short | paragraph
	Required    bool   `json:"required,omitempty"`
	MinLength   int    `json:"min_length,omitempty"`
	MaxLength   int    `json:"max_length,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Value       string `json:"value,omitempty"`
}
