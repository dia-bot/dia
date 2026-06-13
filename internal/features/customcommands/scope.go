package customcommands

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/guildstate"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/dia-bot/dia/internal/tmpllookup"
)

// Scope is the run-local variable space passed to every step handler and to
// every templated string evaluated during the run. It's shared between the
// pure-template (`{{.User.Username}}`) view and the token-shorthand
// (`{user.mention}`) view; both read the same data.
type Scope struct {
	Data       *ScopeData
	guildState *guildstate.Store
	guildID    string
}

// ScopeData is the JSON-serializable payload of a Scope. Persisted in
// command_runs.scope and restored verbatim when a wait/wait_for resumes.
type ScopeData struct {
	Ctx                ContextVars          `json:"ctx"`
	Input              map[string]any       `json:"input"`
	Vars               map[string]any       `json:"vars"`
	Last               any                  `json:"last,omitempty"`
	PendingAttachments []ScopeAttachment    `json:"pending_attachments,omitempty"`
	ImageBlobs         map[string]ImageBlob `json:"image_blobs,omitempty"`
	Deferred           bool                 `json:"deferred,omitempty"`
	// Replied: the current interaction has had its single initial response
	// (Discord allows exactly one); later replies become follow-ups. Reset
	// on every resume — each component click is a fresh interaction.
	Replied bool `json:"replied,omitempty"`
	// Error is set by the runtime when a step fails and recovery is about
	// to run, so templates inside on_error / on_error_cases can read
	// `.Error.Kind`, `.Error.Message`, etc. Cleared again after the
	// recovery branch finishes (or on the next successful step).
	Error *ErrorInfo `json:"error,omitempty"`
}

// ContextVars are the immutable per-run inputs derived from the trigger.
type ContextVars struct {
	User    ContextUser    `json:"user"`
	Member  ContextMember  `json:"member"`
	Guild   ContextGuild   `json:"guild"`
	Channel ContextChannel `json:"channel"`
	// Now is unix-millis at run start — stable across resumes for repeatable
	// rendering of relative timestamps.
	Now int64 `json:"now"`
}

// ContextUser mirrors event.User minus volatile flags.
type ContextUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name,omitempty"`
	Avatar     string `json:"avatar,omitempty"`
	Mention    string `json:"mention"`
	Bot        bool   `json:"bot,omitempty"`
}

// ContextMember adds the guild-side data (nick, roles).
type ContextMember struct {
	Nick     string   `json:"nick,omitempty"`
	Roles    []string `json:"roles,omitempty"`
	JoinedAt string   `json:"joined_at,omitempty"`
}

// ContextGuild is the invoking guild snapshot.
type ContextGuild struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MemberCount int    `json:"member_count,omitempty"`
}

// ContextChannel is the invoking channel.
type ContextChannel struct {
	ID string `json:"id"`
}

// ScopeAttachment is one attachment queued by image_attach for the next reply.
type ScopeAttachment struct {
	FromVar  string `json:"from_var,omitempty"`
	URL      string `json:"url,omitempty"`
	Filename string `json:"filename"`
}

// ImageBlob is base64-encoded image bytes plus content-type, stored in scope
// so wait/resume can survive a worker restart without losing the buffer.
type ImageBlob struct {
	Bytes       string `json:"bytes"` // base64
	ContentType string `json:"content_type"`
	Filename    string `json:"filename"`
}

// NewScope builds a fresh Scope from a context snapshot.
func NewScope(gs *guildstate.Store, guildID string, ctxVars ContextVars, input map[string]any, varDefaults map[string]any) *Scope {
	if input == nil {
		input = map[string]any{}
	}
	vars := map[string]any{}
	for k, v := range varDefaults {
		vars[k] = v
	}
	return &Scope{
		Data: &ScopeData{
			Ctx:   ctxVars,
			Input: input,
			Vars:  vars,
		},
		guildState: gs,
		guildID:    guildID,
	}
}

// RestoreScope rebuilds a Scope from persisted JSON (resume path).
func RestoreScope(gs *guildstate.Store, guildID string, raw json.RawMessage) (*Scope, error) {
	if len(raw) == 0 {
		return NewScope(gs, guildID, ContextVars{}, nil, nil), nil
	}
	d := &ScopeData{}
	if err := json.Unmarshal(raw, d); err != nil {
		return nil, err
	}
	if d.Input == nil {
		d.Input = map[string]any{}
	}
	if d.Vars == nil {
		d.Vars = map[string]any{}
	}
	return &Scope{Data: d, guildState: gs, guildID: guildID}, nil
}

// Marshal returns the scope's JSON payload.
func (s *Scope) Marshal() (json.RawMessage, error) {
	if s == nil {
		return json.RawMessage("{}"), nil
	}
	return json.Marshal(s.Data)
}

// Set assigns a variable.
func (s *Scope) Set(name string, value any) {
	if s == nil || s.Data == nil || name == "" {
		return
	}
	s.Data.Vars[name] = value
	s.Data.Last = value
}

// Get reads a variable (or returns nil).
func (s *Scope) Get(name string) any {
	if s == nil || s.Data == nil {
		return nil
	}
	return s.Data.Vars[name]
}

// Lookup returns a templating.Lookup for getRole/getChannel — backed by the
// Redis guild snapshot, same as welcome and customcommands v1.
func (s *Scope) Lookup() templating.Lookup {
	if s == nil || s.guildState == nil || s.guildID == "" {
		return nil
	}
	return tmpllookup.New(context.Background(), s.guildState, s.guildID)
}

// TemplateContext builds the templating.Context view (`.` root for templates).
func (s *Scope) TemplateContext() *templating.Context {
	if s == nil || s.Data == nil {
		return &templating.Context{}
	}
	d := s.Data
	ctx := &templating.Context{
		User: templating.User{
			ID:         d.Ctx.User.ID,
			Username:   d.Ctx.User.Username,
			GlobalName: d.Ctx.User.GlobalName,
			Avatar:     d.Ctx.User.Avatar,
			Bot:        d.Ctx.User.Bot,
		},
		Member: templating.Member{
			User: templating.User{
				ID:         d.Ctx.User.ID,
				Username:   d.Ctx.User.Username,
				GlobalName: d.Ctx.User.GlobalName,
				Avatar:     d.Ctx.User.Avatar,
			},
			Nick:     d.Ctx.Member.Nick,
			Roles:    d.Ctx.Member.Roles,
			JoinedAt: d.Ctx.Member.JoinedAt,
		},
		Guild: templating.Guild{
			ID:          d.Ctx.Guild.ID,
			Name:        d.Ctx.Guild.Name,
			MemberCount: d.Ctx.Guild.MemberCount,
		},
		Channel: templating.Channel{ID: d.Ctx.Channel.ID},
		// Args carries declared input names so admins can iterate them in templates.
		Args: inputNames(d.Input),
	}
	if d.Error != nil {
		ctx.Error = templating.ErrorInfo{
			Kind:      d.Error.Kind,
			Message:   d.Error.Message,
			Step:      d.Error.Step,
			StepID:    d.Error.StepID,
			Retryable: d.Error.Retryable,
		}
	}
	return ctx
}

// SetErrorInfo flags the scope as being inside an on_error subtree.
// Recovery templates read `.Error.*` for typed dispatch / message reuse.
func (s *Scope) SetErrorInfo(info ErrorInfo) {
	if s == nil || s.Data == nil {
		return
	}
	cp := info
	s.Data.Error = &cp
}

// ClearErrorInfo wipes the scope's `.Error` after the recovery branch
// completes so a downstream successful step doesn't see stale state.
func (s *Scope) ClearErrorInfo() {
	if s == nil || s.Data == nil {
		return
	}
	s.Data.Error = nil
}

// Tokens builds the brace-delimited shorthand map used after template render.
func (s *Scope) Tokens() map[string]string {
	if s == nil || s.Data == nil {
		return nil
	}
	d := s.Data
	tokens := map[string]string{
		"{user.mention}":  d.Ctx.User.Mention,
		"{user.id}":       d.Ctx.User.ID,
		"{user.name}":     d.Ctx.User.Username,
		"{user.username}": d.Ctx.User.Username,
		"{user.global}":   d.Ctx.User.GlobalName,
		"{user.avatar}":   d.Ctx.User.Avatar,
		"{user}":          displayName(d.Ctx.User),
		"{server}":        d.Ctx.Guild.Name,
		"{server.name}":   d.Ctx.Guild.Name,
		"{server.id}":     d.Ctx.Guild.ID,
		"{count}":         strconv.Itoa(d.Ctx.Guild.MemberCount),
		"{channel.id}":    d.Ctx.Channel.ID,
		"{channel}":       "<#" + d.Ctx.Channel.ID + ">",
	}
	// Expose input options as {input.name}.
	for k, v := range d.Input {
		tokens["{input."+k+"}"] = anyToString(v)
	}
	// Expose run-vars as {vars.name}.
	for k, v := range d.Vars {
		tokens["{vars."+k+"}"] = anyToString(v)
	}
	return tokens
}

// QueueAttachment enqueues an attachment for the next user-visible message step.
func (s *Scope) QueueAttachment(a ScopeAttachment) {
	if s == nil || s.Data == nil {
		return
	}
	s.Data.PendingAttachments = append(s.Data.PendingAttachments, a)
}

// DrainAttachments returns and clears the pending attachment queue.
func (s *Scope) DrainAttachments() []ScopeAttachment {
	if s == nil || s.Data == nil {
		return nil
	}
	out := s.Data.PendingAttachments
	s.Data.PendingAttachments = nil
	return out
}

// SetImageBlob stores image bytes (base64) in scope.
func (s *Scope) SetImageBlob(name string, blob ImageBlob) {
	if s == nil || s.Data == nil {
		return
	}
	if s.Data.ImageBlobs == nil {
		s.Data.ImageBlobs = map[string]ImageBlob{}
	}
	s.Data.ImageBlobs[name] = blob
	s.Data.Vars[name] = map[string]any{
		"filename":     blob.Filename,
		"content_type": blob.ContentType,
	}
}

// ImageBlob retrieves a stored image bytes blob by variable name.
func (s *Scope) ImageBlob(name string) (ImageBlob, bool) {
	if s == nil || s.Data == nil || s.Data.ImageBlobs == nil {
		return ImageBlob{}, false
	}
	b, ok := s.Data.ImageBlobs[name]
	return b, ok
}

// MarkDeferred records that the interaction was deferred (so subsequent
// "reply" steps degrade to edit_reply).
func (s *Scope) MarkDeferred(yes bool) {
	if s != nil && s.Data != nil {
		s.Data.Deferred = yes
	}
}

// Deferred reports whether Defer has been called for the interaction.
func (s *Scope) Deferred() bool { return s != nil && s.Data != nil && s.Data.Deferred }

// MarkReplied records that the interaction's single initial response was
// sent (or edited in); further replies must be follow-up messages.
func (s *Scope) MarkReplied(yes bool) {
	if s != nil && s.Data != nil {
		s.Data.Replied = yes
	}
}

// Replied reports whether the current interaction already got its response.
func (s *Scope) Replied() bool { return s != nil && s.Data != nil && s.Data.Replied }

// CardVars produces the map[string]string that image_render hands to the
// imaging.Renderer. Each ctx value is exposed under its canonical key plus
// the brace-shorthand alias.
func (s *Scope) CardVars() map[string]string {
	tokens := s.Tokens()
	if tokens == nil {
		tokens = map[string]string{}
	}
	// Add bare-key aliases (without braces) so a template var named "user.name"
	// resolves whether the admin writes {user.name} or user.name in a layout
	// variable.
	out := make(map[string]string, len(tokens)*2)
	for k, v := range tokens {
		out[k] = v
		bare := k
		if len(bare) > 2 && bare[0] == '{' && bare[len(bare)-1] == '}' {
			bare = bare[1 : len(bare)-1]
		}
		out[bare] = v
	}
	return out
}

// ── helpers ──────────────────────────────────────────────────────────────────

func displayName(u ContextUser) string {
	if u.GlobalName != "" {
		return u.GlobalName
	}
	if u.Username != "" {
		return u.Username
	}
	return "user"
}

func inputNames(m map[string]any) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func anyToString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// BuildContext composes a ContextVars from interaction data.
func BuildContext(guildID, channelID string, user event.User, member *event.Member, guild ContextGuild, nowMs int64) ContextVars {
	cv := ContextVars{
		User: ContextUser{
			ID:         user.ID,
			Username:   user.Username,
			GlobalName: user.GlobalName,
			Avatar:     user.Avatar,
			Mention:    "<@" + user.ID + ">",
			Bot:        user.Bot,
		},
		Guild:   guild,
		Channel: ContextChannel{ID: channelID},
		Now:     nowMs,
	}
	if member != nil {
		cv.Member = ContextMember{
			Nick:     member.Nick,
			Roles:    member.Roles,
			JoinedAt: member.JoinedAt,
		}
	}
	if cv.Guild.ID == "" {
		cv.Guild.ID = guildID
	}
	return cv
}
