// Package runtime is the worker-side glue for the customcommands feature: it
// wires the executor engine to a Plugin (CommandFallback for slash invocation,
// component/modal intercepts for wait_for resume, scheduler worker for wait
// resume) and persists run state to command_runs / command_run_logs.
//
// It lives in a subpackage to avoid an import cycle: customcommands holds the
// data types (Definition, Step, Scope, Expr), customcommands/exec holds the
// engine (imports customcommands), and customcommands/runtime imports both.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/customcommands/exec"
)

// Plugin is the custom-commands feature.
type Plugin struct {
	deps plugin.Deps
	eng  *exec.Engine
}

// New returns the plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         cc.FeatureKey,
		Name:        "Custom Commands",
		Description: "Programmable per-server slash commands with branching, waits, image rendering and component interactions.",
		Category:    plugin.CategoryUtility,
	}
}

// Init wires the runtime engine, the dynamic command fallback, the
// component/modal intercepts and the background scheduler worker.
func (p *Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	p.deps = d
	p.eng = exec.New(exec.Deps{
		Log:     d.Log,
		Discord: &exec.DiscordAdapter{C: d.Discord},
		Store:   &exec.StoreAdapter{S: d.Store},
		Imaging: &exec.ImagingAdapter{R: d.Imaging},
		HTTP:    &exec.HTTPAdapter{Client: &http.Client{Timeout: 10 * time.Second}},
	})

	reg.CommandFallback(p.handleInvoke)
	reg.Component("ccmd:", p.handleResumeComponent)
	reg.Modal("ccmd:", p.handleResumeModal)

	reg.Command(&interactions.Command{
		Def: interactions.AdminOnly(interactions.Slash("customcommands",
			"Manage this server's custom commands",
			interactions.SubCommand("list", "List this server's custom commands"),
		)),
		Handler: p.handleList,
	})

	reg.Worker("ccmd-scheduler", p.runScheduler)
	return nil
}

// ── Slash invocation ────────────────────────────────────────────────────────

func (p *Plugin) handleInvoke(c *interactions.Context) error {
	if c.GuildID == "" {
		return c.RespondEphemeral("Custom commands only work in servers.")
	}
	gid, _ := event.ParseID(c.GuildID)
	cmd, err := p.deps.Store.CustomCommands.GetByName(c.Ctx, gid, c.I.Data.Name)
	if errors.Is(err, store.ErrNotFound) {
		return c.RespondEphemeral("Unknown command.")
	}
	if err != nil {
		return err
	}
	if !cmd.Enabled {
		return c.RespondEphemeral("That command is disabled.")
	}

	var def cc.Definition
	if err := json.Unmarshal(cmd.Definition, &def); err != nil {
		return fmt.Errorf("decode definition: %w", err)
	}

	scope := p.buildScope(c, &def)
	if cmd.RequiresDefer {
		if err := c.Defer(false); err == nil {
			scope.MarkDeferred(true)
		}
	}

	run := &exec.RunState{
		ID:                 newULID(),
		CommandID:          cmd.ID,
		CommandVersion:     cmd.Version,
		GuildID:            c.GuildID,
		InvokerID:          c.User.ID,
		ChannelID:          c.I.ChannelID,
		TriggerKind:        "slash",
		InteractionID:      c.I.ID,
		InteractionToken:   c.I.Token,
		DefinitionSnapshot: cmd.Definition,
	}

	outcome, pause, runErr := p.eng.Run(c.Ctx, run, def, scope)
	if runErr != nil {
		p.deps.Log.Warn("ccmd run error", "command", cmd.Name, "err", runErr)
	}
	p.persistLogs(c.Ctx, run)

	if pause != nil {
		if err := p.persistRunForResume(c.Ctx, run, scope, pause); err != nil {
			p.deps.Log.Warn("ccmd persist run", "err", err)
		}
		return nil
	}
	if outcome.Status == "failed" {
		if !c.Responded() {
			return c.RespondEphemeral("Command failed: " + outcome.Error)
		}
		return nil
	}
	if !c.Responded() {
		return c.RespondEphemeral("Done.")
	}
	return nil
}

// ── Resume on component / modal ─────────────────────────────────────────────

func (p *Plugin) handleResumeComponent(c *interactions.Context) error {
	return p.resume(c, "component")
}
func (p *Plugin) handleResumeModal(c *interactions.Context) error {
	return p.resume(c, "modal")
}

func (p *Plugin) resume(c *interactions.Context, kind string) error {
	cid := c.CustomID()
	run, err := p.deps.Store.CommandRuns.FindWaitingForComponent(c.Ctx, cid)
	if errors.Is(err, store.ErrNotFound) {
		return c.RespondEphemeral("This interaction is no longer active.")
	}
	if err != nil {
		return err
	}
	if run.AwaitingUserID != 0 && c.User.ID != "" {
		uid, _ := event.ParseID(c.User.ID)
		if uid != run.AwaitingUserID {
			return c.RespondEphemeral("Only the original invoker can use this.")
		}
	}
	claimed, err := p.deps.Store.CommandRuns.ClaimResume(c.Ctx, run.ID)
	if err != nil || !claimed {
		return c.RespondEphemeral("Already processed.")
	}

	var def cc.Definition
	if err := json.Unmarshal(run.DefinitionSnapshot, &def); err != nil {
		return err
	}
	scope, err := cc.RestoreScope(p.deps.GuildState, c.GuildID, run.Scope)
	if err != nil {
		return err
	}

	payload := map[string]any{
		"kind":      kind,
		"custom_id": cid,
		"user_id":   c.User.ID,
	}
	// The bare suffix ("approve" from ccmd:<run>:approve) — what per-button
	// switches branch on.
	if i := strings.LastIndex(cid, ":"); i >= 0 && i < len(cid)-1 {
		payload["id"] = cid[i+1:]
	}
	if kind == "component" {
		payload["values"] = c.ComponentValues()
	}
	if kind == "modal" {
		values := map[string]string{}
		for _, row := range c.I.Data.Components {
			for _, comp := range row.Components {
				values[comp.CustomID] = comp.Value
			}
		}
		payload["fields"] = values
	}
	scope.Set("trigger", payload)

	var cursor []cc.CursorFrame
	if len(run.Cursor) > 0 {
		_ = json.Unmarshal(run.Cursor, &cursor)
	}

	// The awaiting step, located precisely via the cursor: it names where the
	// payload lands (`into`) and how to acknowledge the click. The click
	// router's hidden listener has NO custom_id suffix, so a suffix scan
	// can't find it; only the cursor can.
	awaitInto := ""
	mode := cc.ClickResponseReply
	if st := exec.StepAtCursor(def.Steps, cursor); st != nil && len(st.Spec) > 0 {
		switch st.Kind {
		case cc.KindWaitFor:
			var ws cc.SpecWaitFor
			if json.Unmarshal(st.Spec, &ws) == nil {
				awaitInto = ws.Into
				if kind == "component" {
					suffix, _ := payload["id"].(string)
					mode = ws.ResponseFor(suffix)
				}
			}
		case cc.KindModalOpen:
			var ms cc.SpecModalOpen
			if json.Unmarshal(st.Spec, &ms) == nil {
				awaitInto = ms.Into
			}
		}
	}
	// Land the payload under the awaiting step's `into` name, so flows
	// reference {{ .Vars.<into>.id }} the way the editor promises. Runs
	// persisted before cursors were captured fall back to a suffix scan.
	if awaitInto == "" {
		if i := strings.LastIndex(cid, ":"); i >= 0 && i < len(cid)-1 {
			awaitInto = findAwaitInto(def.Steps, cid[i+1:])
		}
	}
	if awaitInto != "" {
		scope.Set(awaitInto, payload)
	}

	// A component click is a FRESH interaction with its own single response.
	// The awaiting wait_for says HOW to acknowledge it (per clicked button):
	//
	//   reply (default): defer visibly; the first reply edits the
	//   "thinking" message and later ones follow up.
	//   update: defer silently; the first reply EDITS the clicked message
	//   in place (a deferred-update's @original is the component's message).
	//   silent: defer silently and mark replied, so nothing shows unless a
	//   later step posts a follow-up.
	switch mode {
	case cc.ClickResponseSilent:
		_ = c.DeferUpdate()
		scope.MarkDeferred(true)
		scope.MarkReplied(true)
	case cc.ClickResponseUpdate:
		_ = c.DeferUpdate()
		scope.MarkDeferred(true)
		scope.MarkReplied(false)
	default:
		_ = c.Defer(false)
		scope.MarkDeferred(true)
		scope.MarkReplied(false)
	}

	resumeRun := &exec.RunState{
		ID:                 run.ID,
		CommandID:          run.CommandID,
		CommandVersion:     run.CommandVersion,
		GuildID:            event.FormatID(run.GuildID),
		InvokerID:          event.FormatID(run.InvokerID),
		ChannelID:          event.FormatID(run.ChannelID),
		TriggerKind:        run.TriggerKind,
		InteractionID:      c.I.ID,
		InteractionToken:   c.I.Token,
		DefinitionSnapshot: run.DefinitionSnapshot,
	}
	resumeRun.SetCursor(cursor)

	outcome, pause, runErr := p.eng.Resume(c.Ctx, resumeRun, def, scope, cursor)
	if runErr != nil {
		p.deps.Log.Warn("ccmd resume error", "run", run.ID, "err", runErr)
	}
	p.persistLogs(c.Ctx, resumeRun)

	if pause != nil {
		_ = p.persistRunForResume(c.Ctx, resumeRun, scope, pause)
		return nil
	}
	_ = p.deps.Store.CommandRuns.MarkComplete(c.Ctx, run.ID, outcome.Status, outcome.Error)
	return nil
}

// ── List management command ─────────────────────────────────────────────────

func (p *Plugin) handleList(c *interactions.Context) error {
	gid, _ := event.ParseID(c.GuildID)
	cmds, err := p.deps.Store.CustomCommands.List(c.Ctx, gid)
	if err != nil {
		return err
	}
	if len(cmds) == 0 {
		return c.RespondEphemeral("No custom commands yet. Create them on the dashboard.")
	}
	var b strings.Builder
	for _, cmd := range cmds {
		state := "🟢 enabled"
		if !cmd.Enabled {
			state = "⚪ disabled"
		}
		fmt.Fprintf(&b, "`/%s` — %s · v%d (%s)\n", cmd.Name, state, cmd.Version, cmd.Status)
		if cmd.Description != "" {
			fmt.Fprintf(&b, "> %s\n", cmd.Description)
		}
	}
	return c.RespondEphemeral(b.String())
}

// ── Scheduler worker (resumes wait runs when deadlines pass) ────────────────

func (p *Plugin) runScheduler(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.drainDueWaits(ctx)
		}
	}
}

func (p *Plugin) drainDueWaits(ctx context.Context) {
	runs, err := p.deps.Store.CommandRuns.DueWaits(ctx, 20)
	if err != nil {
		p.deps.Log.Warn("ccmd: scan due waits", "err", err)
		return
	}
	for _, r := range runs {
		claimed, err := p.deps.Store.CommandRuns.ClaimResume(ctx, r.ID)
		if err != nil || !claimed {
			continue
		}
		p.resumeWait(ctx, r)
	}
}

func (p *Plugin) resumeWait(ctx context.Context, r store.CommandRun) {
	var def cc.Definition
	if err := json.Unmarshal(r.DefinitionSnapshot, &def); err != nil {
		_ = p.deps.Store.CommandRuns.MarkComplete(ctx, r.ID, "failed", err.Error())
		return
	}
	scope, err := cc.RestoreScope(p.deps.GuildState, event.FormatID(r.GuildID), r.Scope)
	if err != nil {
		_ = p.deps.Store.CommandRuns.MarkComplete(ctx, r.ID, "failed", err.Error())
		return
	}
	var cursor []cc.CursorFrame
	_ = json.Unmarshal(r.Cursor, &cursor)

	run := &exec.RunState{
		ID:             r.ID,
		CommandID:      r.CommandID,
		CommandVersion: r.CommandVersion,
		GuildID:        event.FormatID(r.GuildID),
		InvokerID:      event.FormatID(r.InvokerID),
		ChannelID:      event.FormatID(r.ChannelID),
		TriggerKind:    r.TriggerKind,
		// The original interaction context: a wait_for is capped at 10
		// minutes while the token lives ~15, so on_timeout steps can still
		// reply or edit the original message.
		InteractionID:      r.InteractionID,
		InteractionToken:   r.InteractionToken,
		InteractionExpires: r.InteractionExpires,
		DefinitionSnapshot: r.DefinitionSnapshot,
	}
	run.SetCursor(cursor)

	// A plain `wait` (sleep) resumes its continuation; an EVENT wait landing
	// here means the deadline passed without the event, so the on_timeout
	// branch runs and the continuation does not.
	timedOut := r.AwaitingKind != ""
	var outcome exec.Outcome
	var pause *exec.PauseError
	var runErr error
	if timedOut {
		outcome, pause, runErr = p.eng.ResumeTimedOut(ctx, run, def, scope, cursor)
	} else {
		outcome, pause, runErr = p.eng.Resume(ctx, run, def, scope, cursor)
	}
	if runErr != nil {
		p.deps.Log.Warn("ccmd scheduler resume", "run", r.ID, "err", runErr)
	}
	p.persistLogs(ctx, run)
	if pause != nil {
		_ = p.persistRunForResume(ctx, run, scope, pause)
		return
	}
	_ = p.deps.Store.CommandRuns.MarkComplete(ctx, r.ID, outcome.Status, outcome.Error)
}

// ── Scope build + persistence helpers ───────────────────────────────────────

func (p *Plugin) buildScope(c *interactions.Context, def *cc.Definition) *cc.Scope {
	gid, _ := event.ParseID(c.GuildID)
	guildName := "the server"
	memberCount := 0
	if g, err := p.deps.Store.Guilds.Get(c.Ctx, gid); err == nil {
		if g.Name != "" {
			guildName = g.Name
		}
		memberCount = g.MemberCount
	}
	ctxVars := cc.BuildContext(c.GuildID, c.I.ChannelID, c.User, c.I.Member, cc.ContextGuild{
		ID: c.GuildID, Name: guildName, MemberCount: memberCount,
	}, time.Now().UnixMilli())

	input := readSlashOptions(c, def)
	defaults := defaultVars(def)
	return cc.NewScope(p.deps.GuildState, c.GuildID, ctxVars, input, defaults)
}

func readSlashOptions(c *interactions.Context, def *cc.Definition) map[string]any {
	out := map[string]any{}
	if def == nil {
		return out
	}
	opts := c.Options()
	for _, o := range def.Options {
		switch o.Kind {
		case "string":
			out[o.Name] = opts.String(o.Name)
		case "int", "integer":
			out[o.Name] = opts.Int(o.Name)
		case "bool", "boolean":
			out[o.Name] = opts.Bool(o.Name)
		case "user", "role", "channel", "mentionable", "attachment":
			out[o.Name] = opts.Snowflake(o.Name)
		case "number":
			out[o.Name] = opts.Float(o.Name)
		default:
			out[o.Name] = opts.String(o.Name)
		}
	}
	return out
}

func defaultVars(def *cc.Definition) map[string]any {
	out := map[string]any{}
	if def == nil {
		return out
	}
	for _, v := range def.Variables {
		if len(v.Default) == 0 {
			continue
		}
		var any any
		_ = json.Unmarshal(v.Default, &any)
		out[v.Name] = any
	}
	return out
}

func (p *Plugin) persistRunForResume(ctx context.Context, run *exec.RunState, scope *cc.Scope, pause *exec.PauseError) error {
	scopeJSON, _ := scope.Marshal()
	// The cursor on the pause, NOT run.Cursor(): the walker's frames have
	// already unwound by the time the pause reaches us.
	cursorJSON, _ := json.Marshal(pause.Cursor)
	gid, _ := event.ParseID(run.GuildID)
	uid, _ := event.ParseID(run.InvokerID)
	chID, _ := event.ParseID(run.ChannelID)
	awUID, _ := event.ParseID(pause.AwaitingUserID)
	r := store.CommandRun{
		ID:                 run.ID,
		CommandID:          run.CommandID,
		CommandVersion:     run.CommandVersion,
		GuildID:            gid,
		InvokerID:          uid,
		ChannelID:          chID,
		TriggerKind:        run.TriggerKind,
		InteractionID:      run.InteractionID,
		InteractionToken:   run.InteractionToken,
		InteractionExpires: run.InteractionExpires,
		Scope:              scopeJSON,
		Cursor:             cursorJSON,
		Status:             "waiting",
		ResumeAt:           pause.ResumeAt,
		AwaitingCustomID:   pause.AwaitingCustomID,
		AwaitingUserID:     awUID,
		AwaitingKind:       pause.AwaitingKind,
		DefinitionSnapshot: run.DefinitionSnapshot,
	}
	if err := p.deps.Store.CommandRuns.Insert(ctx, r); err == nil {
		return nil
	}
	return p.deps.Store.CommandRuns.UpdateState(ctx, r.ID, scopeJSON, cursorJSON,
		"waiting", pause.ResumeAt, pause.AwaitingCustomID, awUID, pause.AwaitingKind)
}

func (p *Plugin) persistLogs(ctx context.Context, run *exec.RunState) {
	for _, l := range run.Logs() {
		l.RunID = run.ID
		if err := p.deps.Store.CommandRuns.AppendLog(ctx, store.CommandRunLog{
			RunID:      l.RunID,
			StepID:     l.StepID,
			StepKind:   l.StepKind,
			CursorPath: l.CursorPath,
			DurationMs: l.DurationMs,
			Status:     l.Status,
			Input:      l.Input,
			Output:     l.Output,
			Error:      l.Error,
		}); err != nil {
			p.deps.Log.Debug("ccmd log write", "err", err)
		}
	}
}

// newULID returns a sortable, opaque identifier. Time-prefixed + nanos suffix
// is unique enough within a worker without pulling in oklog/ulid.
func newULID() string {
	ts := time.Now().UnixNano()
	return fmt.Sprintf("R%013xR%08x", ts/1_000_000, ts&0xFFFFFFFF)
}

// findAwaitInto locates the wait_for / modal_open step whose custom_id suffix
// matches and returns its `into` variable name — the resume payload lands
// under that name in addition to the legacy "trigger" variable.
func findAwaitInto(steps []cc.Step, suffix string) string {
	for i := range steps {
		s := &steps[i]
		if (s.Kind == cc.KindWaitFor || s.Kind == cc.KindModalOpen) && len(s.Spec) > 0 {
			var spec struct {
				CustomIDSuffix string `json:"custom_id_suffix"`
				Into           string `json:"into"`
			}
			if json.Unmarshal(s.Spec, &spec) == nil &&
				spec.CustomIDSuffix == suffix && spec.Into != "" {
				return spec.Into
			}
		}
		for _, br := range [][]cc.Step{s.Then, s.Else, s.Default, s.OnError} {
			if v := findAwaitInto(br, suffix); v != "" {
				return v
			}
		}
		for _, cse := range s.Cases {
			if v := findAwaitInto(cse.Do, suffix); v != "" {
				return v
			}
		}
		for _, ec := range s.OnErrorCases {
			if v := findAwaitInto(ec.Do, suffix); v != "" {
				return v
			}
		}
		if s.Kind == cc.KindParallel && len(s.Spec) > 0 {
			var ps cc.SpecParallel
			if json.Unmarshal(s.Spec, &ps) == nil {
				for _, br := range ps.Branches {
					if v := findAwaitInto(br, suffix); v != "" {
						return v
					}
				}
			}
		}
		if s.Kind == cc.KindWaitFor && len(s.Spec) > 0 {
			var ws cc.SpecWaitFor
			if json.Unmarshal(s.Spec, &ws) == nil {
				if v := findAwaitInto(ws.OnTimeout, suffix); v != "" {
					return v
				}
			}
		}
	}
	return ""
}
