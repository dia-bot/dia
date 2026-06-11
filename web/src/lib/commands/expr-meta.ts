// Static mirror of the template function registry exposed by
// `internal/templating/funcs.go` + the well-known runtime scope variables
// the executor injects (User, Channel, Input.<arg>, Vars.<var>, Error, …).
//
// The dashboard reads this for autocomplete, reference panels, and the
// "in-scope variables" picker — it's NOT a runtime contract, just the
// editor's hint catalogue. Keep it in lockstep with the Go side; mismatches
// just produce a less helpful suggestion, never a runtime error.

export interface TmplFunc {
	name: string;
	signature: string; // e.g. "upper(s string) string"
	short: string;
	insert: string; // template snippet to insert
	category: 'string' | 'number' | 'collection' | 'time' | 'discord' | 'control';
}

export interface TmplVar {
	path: string; // ".User.ID"
	label: string;
	type: string; // e.g. "string", "Channel", "list<Role>"
	short: string;
}

export interface TmplSnippet {
	id: string;
	label: string;
	insert: string;
	short: string;
}

// ── Functions ─────────────────────────────────────────────────────────────
// Mirrors baseFuncs in internal/templating/funcs.go, plus the lookup funcs.
// Templates are PURE — they read values and format strings; actions are
// custom-command steps, never template calls. Snippets use `$0` only as a
// hint; the editor just inserts the literal string for now.

export const TMPL_FUNCTIONS: TmplFunc[] = [
	// Strings
	{ name: 'upper', signature: 'upper(s) string', short: 'Upper-case a string', insert: 'upper ', category: 'string' },
	{ name: 'lower', signature: 'lower(s) string', short: 'Lower-case a string', insert: 'lower ', category: 'string' },
	{ name: 'title', signature: 'title(s) string', short: 'Title-case a string', insert: 'title ', category: 'string' },
	{ name: 'trim', signature: 'trim(s) string', short: 'Trim surrounding whitespace', insert: 'trim ', category: 'string' },
	{ name: 'trimPrefix', signature: 'trimPrefix(s, p) string', short: 'Drop a leading prefix', insert: 'trimPrefix ', category: 'string' },
	{ name: 'trimSuffix', signature: 'trimSuffix(s, p) string', short: 'Drop a trailing suffix', insert: 'trimSuffix ', category: 'string' },
	{ name: 'replace', signature: 'replace(s, old, new) string', short: 'Replace every occurrence', insert: 'replace ', category: 'string' },
	{ name: 'contains', signature: 'contains(s, substr) bool', short: 'True if s contains substr', insert: 'contains ', category: 'string' },
	{ name: 'hasPrefix', signature: 'hasPrefix(s, p) bool', short: 'True if s starts with p', insert: 'hasPrefix ', category: 'string' },
	{ name: 'hasSuffix', signature: 'hasSuffix(s, p) bool', short: 'True if s ends with p', insert: 'hasSuffix ', category: 'string' },
	{ name: 'split', signature: 'split(s, sep) []string', short: 'Split a string on sep', insert: 'split ', category: 'string' },
	{ name: 'join', signature: 'join(sep, parts) string', short: 'Join a list with sep', insert: 'join ', category: 'string' },
	{ name: 'repeat', signature: 'repeat(n, s) string', short: 'Repeat s n times (max 1000)', insert: 'repeat ', category: 'string' },
	{ name: 'slice', signature: 'slice(s, i, j) string', short: 'Sub-string (rune-aware)', insert: 'slice ', category: 'string' },

	// Numbers
	{ name: 'add', signature: 'add(xs...) int', short: 'Sum of integers', insert: 'add ', category: 'number' },
	{ name: 'sub', signature: 'sub(a, b) int', short: 'a − b', insert: 'sub ', category: 'number' },
	{ name: 'mul', signature: 'mul(xs...) int', short: 'Product of integers', insert: 'mul ', category: 'number' },
	{ name: 'div', signature: 'div(a, b) int', short: 'Integer division (0 if b=0)', insert: 'div ', category: 'number' },
	{ name: 'mod', signature: 'mod(a, b) int', short: 'a mod b (0 if b=0)', insert: 'mod ', category: 'number' },
	{ name: 'max', signature: 'max(a, b) int', short: 'Larger of two ints', insert: 'max ', category: 'number' },
	{ name: 'min', signature: 'min(a, b) int', short: 'Smaller of two ints', insert: 'min ', category: 'number' },
	{ name: 'randInt', signature: 'randInt([lo, hi]) int', short: 'Random integer in [lo, hi)', insert: 'randInt ', category: 'number' },
	{ name: 'toInt', signature: 'toInt(v) int', short: 'Coerce a value to int', insert: 'toInt ', category: 'number' },
	{ name: 'toString', signature: 'toString(v) string', short: 'Coerce to string', insert: 'toString ', category: 'number' },
	{ name: 'default', signature: 'default(def, val) any', short: 'def if val is empty', insert: 'default ', category: 'number' },

	// Collections
	{ name: 'list', signature: 'list(xs...) []any', short: 'Make a list', insert: 'list ', category: 'collection' },
	{ name: 'dict', signature: 'dict(k, v, ...) map', short: 'Make a dict (even args)', insert: 'dict ', category: 'collection' },
	{ name: 'seq', signature: 'seq([lo,] hi) []int', short: 'Range [lo, hi)', insert: 'seq ', category: 'collection' },
	{ name: 'in', signature: 'in(item, list) bool', short: 'True if list contains item', insert: 'in ', category: 'collection' },

	// Time
	{ name: 'now', signature: 'now() time', short: 'Current UTC time', insert: 'now', category: 'time' },
	{ name: 'formatTime', signature: 'formatTime(layout, t) string', short: 'Render a time with a layout', insert: 'formatTime ', category: 'time' },

	// Discord — read-only lookups
	{ name: 'mentionUser', signature: 'mentionUser(id) string', short: '<@id>', insert: 'mentionUser ', category: 'discord' },
	{ name: 'mentionRole', signature: 'mentionRole(id) string', short: '<@&id>', insert: 'mentionRole ', category: 'discord' },
	{ name: 'mentionChannel', signature: 'mentionChannel(id) string', short: '<#id>', insert: 'mentionChannel ', category: 'discord' },
	{ name: 'getRole', signature: 'getRole(nameOrID) Role', short: 'Look up a role by name or id', insert: 'getRole ', category: 'discord' },
	{ name: 'getChannel', signature: 'getChannel(nameOrID) Channel', short: 'Look up a channel by name or id', insert: 'getChannel ', category: 'discord' },

	// Control — Go template builtins worth surfacing
	{ name: 'eq', signature: 'eq(a, b) bool', short: 'Equality', insert: 'eq ', category: 'control' },
	{ name: 'ne', signature: 'ne(a, b) bool', short: 'Inequality', insert: 'ne ', category: 'control' },
	{ name: 'lt', signature: 'lt(a, b) bool', short: 'Less than', insert: 'lt ', category: 'control' },
	{ name: 'gt', signature: 'gt(a, b) bool', short: 'Greater than', insert: 'gt ', category: 'control' },
	{ name: 'le', signature: 'le(a, b) bool', short: 'Less or equal', insert: 'le ', category: 'control' },
	{ name: 'ge', signature: 'ge(a, b) bool', short: 'Greater or equal', insert: 'ge ', category: 'control' },
	{ name: 'and', signature: 'and(xs...) any', short: 'Logical AND (short-circuits)', insert: 'and ', category: 'control' },
	{ name: 'or', signature: 'or(xs...) any', short: 'Logical OR (short-circuits)', insert: 'or ', category: 'control' },
	{ name: 'not', signature: 'not(x) bool', short: 'Logical NOT', insert: 'not ', category: 'control' },
	{ name: 'len', signature: 'len(v) int', short: 'Length of string / list / map', insert: 'len ', category: 'control' },
	{ name: 'index', signature: 'index(v, k) any', short: 'Index into list/map', insert: 'index ', category: 'control' },
	{ name: 'printf', signature: 'printf(fmt, xs...) string', short: 'Format like fmt.Sprintf', insert: 'printf ', category: 'control' }
];

export const TMPL_CATEGORIES: { id: TmplFunc['category']; label: string }[] = [
	{ id: 'string', label: 'Strings' },
	{ id: 'number', label: 'Numbers' },
	{ id: 'collection', label: 'Collections' },
	{ id: 'time', label: 'Time' },
	{ id: 'discord', label: 'Discord lookups' },
	{ id: 'control', label: 'Control / logic' }
];

export const TMPL_FUNC_BY_NAME = new Map(TMPL_FUNCTIONS.map((f) => [f.name, f]));

// ── Scope variables ───────────────────────────────────────────────────────
// The runtime injects these under the template's `.` root. Static catalogue
// (the dynamic parts — Input.<arg> and Vars.<var> — are computed by the
// builder from the current command's options + variables).

export const TMPL_STATIC_VARS: TmplVar[] = [
	{ path: '.User.ID', label: 'User.ID', type: 'snowflake', short: "Invoker's user id" },
	{ path: '.User.Username', label: 'User.Username', type: 'string', short: "Invoker's username" },
	{ path: '.User.Mention', label: 'User.Mention', type: 'string', short: '<@id> mention of the invoker' },
	{ path: '.Channel.ID', label: 'Channel.ID', type: 'snowflake', short: 'Channel where the command ran' },
	{ path: '.Channel.Name', label: 'Channel.Name', type: 'string', short: 'Channel name' },
	{ path: '.Guild.ID', label: 'Guild.ID', type: 'snowflake', short: 'Guild id' },
	{ path: '.Guild.Name', label: 'Guild.Name', type: 'string', short: 'Guild name' },
	{ path: '.Now', label: 'Now', type: 'time', short: 'Time the run started (UTC)' }
];

// On_error-scope additions: what the runtime exposes inside an error handler.
export const TMPL_ERROR_VARS: TmplVar[] = [
	{ path: '.Error.Kind', label: 'Error.Kind', type: 'string', short: 'Typed error code, e.g. discord.permission_denied' },
	{ path: '.Error.Message', label: 'Error.Message', type: 'string', short: 'Human-readable error message' },
	{ path: '.Error.Step', label: 'Error.Step', type: 'string', short: 'Step kind that failed' },
	{ path: '.Error.StepID', label: 'Error.StepID', type: 'string', short: 'Step id that failed' },
	{ path: '.Error.Retryable', label: 'Error.Retryable', type: 'bool', short: 'True if the error is transient' }
];

// ── Snippets ──────────────────────────────────────────────────────────────

export const TMPL_SNIPPETS: TmplSnippet[] = [
	{ id: 'if', label: 'If / else', insert: '{{ if  }}{{ else }}{{ end }}', short: 'Conditional branch' },
	{ id: 'range', label: 'Range', insert: '{{ range  }}{{ end }}', short: 'Iterate a list' },
	{ id: 'with', label: 'With', insert: '{{ with  }}{{ end }}', short: 'Rebind the dot' },
	{ id: 'eq', label: 'Equals', insert: '{{ eq  "" }}', short: 'String equality' },
	{ id: 'default', label: 'Default', insert: '{{ default "fallback"  }}', short: 'Fallback when empty' }
];

// ── Error taxonomy ────────────────────────────────────────────────────────
// What the runtime stamps as `.Error.Kind` when a step fails. Each kind has
// a parent group so the case picker can collapse them (discord.* / http.* / …).

export interface ErrorKind {
	id: string; // "discord.permission_denied"
	group: 'discord' | 'http' | 'template' | 'kv' | 'runtime' | 'validation';
	label: string;
	short: string;
	retryable?: boolean;
}

export const ERROR_KINDS: ErrorKind[] = [
	// Discord API failures
	{ id: 'discord.permission_denied', group: 'discord', label: 'Permission denied', short: 'Dia lacks the permission to perform the action.' },
	{ id: 'discord.rate_limited', group: 'discord', label: 'Rate limited', short: 'Discord rate-limited the request.', retryable: true },
	{ id: 'discord.unknown_user', group: 'discord', label: 'Unknown user', short: 'The targeted user does not exist or left the server.' },
	{ id: 'discord.unknown_role', group: 'discord', label: 'Unknown role', short: 'The targeted role does not exist.' },
	{ id: 'discord.unknown_channel', group: 'discord', label: 'Unknown channel', short: 'The targeted channel does not exist.' },
	{ id: 'discord.unknown_message', group: 'discord', label: 'Unknown message', short: 'The targeted message does not exist or was deleted.' },
	{ id: 'discord.unavailable', group: 'discord', label: 'Discord unavailable', short: 'Discord returned a 5xx.', retryable: true },

	// HTTP step failures
	{ id: 'http.timeout', group: 'http', label: 'HTTP timeout', short: 'The outbound HTTP request timed out.', retryable: true },
	{ id: 'http.status_4xx', group: 'http', label: 'HTTP 4xx', short: 'The outbound call returned a client error.' },
	{ id: 'http.status_5xx', group: 'http', label: 'HTTP 5xx', short: 'The outbound call returned a server error.', retryable: true },
	{ id: 'http.connection', group: 'http', label: 'HTTP connection refused', short: 'Could not reach the host.', retryable: true },
	{ id: 'http.parse', group: 'http', label: 'HTTP parse failure', short: 'Response body could not be decoded.' },

	// Templates / expressions
	{ id: 'template.parse', group: 'template', label: 'Template parse error', short: 'Expression source is malformed.' },
	{ id: 'template.eval', group: 'template', label: 'Template eval error', short: 'A function inside an expression failed.' },
	{ id: 'template.budget', group: 'template', label: 'Template budget exceeded', short: 'Expression ran out of CPU / output budget.' },

	// KV store
	{ id: 'kv.not_found', group: 'kv', label: 'KV miss', short: 'The KV key has no value.' },
	{ id: 'kv.conflict', group: 'kv', label: 'KV conflict', short: 'Concurrent writers raced on the same key.', retryable: true },

	// Runtime / validation
	{ id: 'runtime.budget_exceeded', group: 'runtime', label: 'Run budget exceeded', short: 'The command exceeded its action / step budget.' },
	{ id: 'runtime.timeout', group: 'runtime', label: 'Run timeout', short: 'The command exceeded its wall-clock limit.' },
	{ id: 'validation.invalid_argument', group: 'validation', label: 'Invalid argument', short: 'An expression produced a value the step rejected.' }
];

export const ERROR_KIND_BY_ID = new Map(ERROR_KINDS.map((k) => [k.id, k]));

export const ERROR_GROUPS: { id: ErrorKind['group']; label: string }[] = [
	{ id: 'discord', label: 'Discord API' },
	{ id: 'http', label: 'HTTP requests' },
	{ id: 'template', label: 'Expressions' },
	{ id: 'kv', label: 'KV store' },
	{ id: 'runtime', label: 'Runtime / limits' },
	{ id: 'validation', label: 'Validation' }
];

// ── Editor scope context ──────────────────────────────────────────────────
// The editor page sets this so any deeply-nested <ExprField> can show
// in-scope variables (slash args + declared vars) without prop drilling.

import type { CommandOption, Step, VarDecl } from './types';

export interface ExprScope {
	options: CommandOption[];
	variables: VarDecl[];
	// The live step tree — lets fields offer "reference a previous step"
	// pickers (e.g. the message sent by an earlier Send-message step).
	steps?: Step[];
}

export const EXPR_SCOPE_CTX = Symbol('dia.expr-scope');
