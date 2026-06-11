// Package templating is Dia's sandboxed template engine: Go text/template
// with a curated function map, a guild/member context, and hard safety
// limits — an execution timeout, a capped output size and bounded loop
// helpers. Templates are PURE: they read values and format strings, never
// perform actions — side effects belong to custom-command steps. It powers
// welcome/goodbye messages, custom commands and any other place an admin
// writes dynamic text.
package templating

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"text/template"
	"time"
)

const (
	defaultMaxOutput = 4000                   // ~Discord message length
	defaultTimeout   = 500 * time.Millisecond // per-render wall-clock budget
	maxListLen       = 1000                   // cap for seq/list helpers (bounds loops)
)

// ErrOutputTooLong is returned when a template writes past the output cap.
var ErrOutputTooLong = errors.New("template output exceeded the limit")

// ── Context (the template root / dot) ───────────────────────────────────────

type User struct {
	ID         string
	Username   string
	GlobalName string
	Avatar     string
	Bot        bool
}

type Member struct {
	User     User
	Nick     string
	Roles    []string
	JoinedAt string
}

type Guild struct {
	ID          string
	Name        string
	Icon        string
	OwnerID     string
	MemberCount int
}

type Channel struct {
	ID   string
	Name string
	Type int
}

// Context is exposed to templates as `.` (e.g. {{.User.Username}}, {{.Guild.Name}}).
type Context struct {
	User    User
	Member  Member
	Guild   Guild
	Channel Channel
	Args    []string
	// Error is populated inside an on_error subtree so the recovery
	// branch's templates can dispatch on `.Error.Kind`, surface
	// `.Error.Message`, etc. Outside an error scope it's zero-valued.
	Error ErrorInfo
}

// ErrorInfo mirrors customcommands.ErrorInfo for template consumption.
// Kept here so the templating package stays leaf-level (no upward import).
type ErrorInfo struct {
	Kind      string
	Message   string
	Step      string
	StepID    string
	Retryable bool
}

// ── Read-only guild data (safe; the getRole/getChannel funcs use this) ────────

// RoleInfo / ChannelInfo are the small read-only shapes the lookup returns.
type RoleInfo struct {
	ID    string
	Name  string
	Color int
}

type ChannelInfo struct {
	ID   string
	Name string
	Type int
}

// Lookup resolves guild roles/channels by id or name for the getRole/getChannel
// template functions. It has no side effects. A nil Lookup disables those
// functions (e.g. in dashboard previews), where they return a friendly error.
type Lookup interface {
	Role(nameOrID string) (*RoleInfo, bool)
	Channel(nameOrID string) (*ChannelInfo, bool)
}

// ── Engine ──────────────────────────────────────────────────────────────────

type Engine struct {
	maxOutput int
	timeout   time.Duration
}

// New returns an engine with the default safety limits.
func New() *Engine {
	return &Engine{maxOutput: defaultMaxOutput, timeout: defaultTimeout}
}

// Render executes src with data as the root. lookup may be nil to disable
// getRole/getChannel. The returned string is the produced output (possibly
// partial on error).
func (e *Engine) Render(ctx context.Context, src string, data *Context, lookup Lookup) (string, error) {
	if src == "" {
		return "", nil
	}
	tmpl, err := template.New("dia").Funcs(e.funcMap(lookup)).Option("missingkey=zero").Parse(src)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}

	cctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	buf := &limitedBuffer{max: e.maxOutput}
	done := make(chan error, 1)
	go func() { done <- tmpl.Execute(buf, data) }()

	select {
	case err := <-done:
		if err != nil {
			return buf.String(), fmt.Errorf("template error: %w", err)
		}
		return buf.String(), nil
	case <-cctx.Done():
		// Loops are bounded by capped helpers + the output cap, so this is a
		// backstop; the lingering goroutine finishes against those bounds.
		return "", fmt.Errorf("template timed out after %s", e.timeout)
	}
}

// limitedBuffer caps total bytes written so a template can't produce unbounded
// output (or spin writing forever).
type limitedBuffer struct {
	buf bytes.Buffer
	max int
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.max {
		return 0, ErrOutputTooLong
	}
	return w.buf.Write(p)
}

func (w *limitedBuffer) String() string { return w.buf.String() }
