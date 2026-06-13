// Type-only mirror of internal/features/customcommands/{config,kinds}.go.
// The dashboard's source of truth for what a custom command can contain;
// kept in lockstep with the Go side so the editor never produces JSONB the
// runtime won't understand.

export type Status = 'draft' | 'published' | 'archived';

export interface Expr {
	lang?: 'tmpl' | 'literal';
	src?: string;
	value?: unknown;
}

export interface SwitchCase {
	when: Expr;
	do: Step[];
}

// ErrorCase is one arm of a typed on_error dispatch. `when` is a list of
// segment-glob patterns (`discord.*`, `*.timeout`, exact codes, or `*`).
// The runtime picks the first arm whose patterns match the failure kind.
export interface ErrorCase {
	when: string[];
	do: Step[];
}

export interface Step {
	id: string;
	kind: string;
	spec?: unknown;
	then?: Step[];
	else?: Step[];
	cases?: SwitchCase[];
	default?: Step[];
	on_error?: Step[];
	on_error_cases?: ErrorCase[];
}

export interface CommandOption {
	kind: string;
	name: string;
	description: string;
	required?: boolean;
	// Numeric (int/number) bounds.
	min_value?: number;
	max_value?: number;
	// String length bounds.
	min_length?: number;
	max_length?: number;
	// Runtime answers suggestion requests; mutually exclusive with choices.
	// Only valid on string/int/number.
	autocomplete?: boolean;
	// Discord channel type ids. Only valid on channel options.
	channel_types?: number[];
	choices?: { name: string; value: unknown }[];
}

// Slash-arg kind catalogue (mirrors what validOptionKind() / slashOptKind()
// accept on the Go side). The dashboard uses this list to drive the
// argument editor — labels, icons, and which per-type fields are legal.
export interface SlashOptionKindMeta {
	id: string;
	label: string;
	icon: string; // lucide-svelte component name
	short: string;
	bucket: 'string' | 'numeric' | 'channel' | 'other';
	// What configuration applies to this kind.
	supportsLength?: boolean;
	supportsValueBounds?: boolean;
	supportsChannelTypes?: boolean;
	supportsChoices?: boolean;
	supportsAutocomplete?: boolean;
}

export const SLASH_OPTION_KINDS: SlashOptionKindMeta[] = [
	{
		id: 'string',
		label: 'Text',
		icon: 'Type',
		short: 'A free-form line of text',
		bucket: 'string',
		supportsLength: true,
		supportsChoices: true,
		supportsAutocomplete: true
	},
	{
		id: 'int',
		label: 'Integer',
		icon: 'Hash',
		short: 'A whole number',
		bucket: 'numeric',
		supportsValueBounds: true,
		supportsChoices: true,
		supportsAutocomplete: true
	},
	{
		id: 'number',
		label: 'Number',
		icon: 'Hash',
		short: 'A decimal number',
		bucket: 'numeric',
		supportsValueBounds: true,
		supportsChoices: true,
		supportsAutocomplete: true
	},
	{
		id: 'bool',
		label: 'True/False',
		icon: 'ToggleLeft',
		short: 'A yes / no value',
		bucket: 'other'
	},
	{
		id: 'user',
		label: 'User',
		icon: 'User',
		short: 'A server member',
		bucket: 'other'
	},
	{
		id: 'role',
		label: 'Role',
		icon: 'Shield',
		short: 'A server role',
		bucket: 'other'
	},
	{
		id: 'channel',
		label: 'Channel',
		icon: 'Hash',
		short: 'A channel reference',
		bucket: 'channel',
		supportsChannelTypes: true
	},
	{
		id: 'mentionable',
		label: 'Mentionable',
		icon: 'AtSign',
		short: 'A user or role',
		bucket: 'other'
	},
	{
		id: 'attachment',
		label: 'Attachment',
		icon: 'Paperclip',
		short: 'A file upload',
		bucket: 'other'
	}
];

export const SLASH_OPTION_KIND_BY_ID = new Map(SLASH_OPTION_KINDS.map((k) => [k.id, k]));

// Discord channel type ids the slash-arg picker supports.
export const SLASH_CHANNEL_TYPES: { id: number; label: string }[] = [
	{ id: 0, label: 'Text' },
	{ id: 2, label: 'Voice' },
	{ id: 4, label: 'Category' },
	{ id: 5, label: 'Announcement' },
	{ id: 13, label: 'Stage' },
	{ id: 15, label: 'Forum' },
	{ id: 16, label: 'Media' },
	{ id: 10, label: 'Announce thread' },
	{ id: 11, label: 'Public thread' },
	{ id: 12, label: 'Private thread' }
];

export interface VarDecl {
	name: string;
	type: string;
	default?: unknown;
	scope: string;
}

export interface Trigger {
	kind: string;
	prefix?: string;
	event?: string;
	cron?: string;
	timezone?: string;
	channel?: string;
}

export interface Cooldown {
	scope: string;
	seconds: number;
}

export interface Definition {
	options?: CommandOption[];
	permissions?: string;
	cooldown?: Cooldown;
	variables?: VarDecl[];
	triggers?: Trigger[];
	steps?: Step[];
	// Disconnected chains: canvas islands the runtime never executes.
	// Detaching a line parks the downstream steps here until reconnected.
	scratch?: Step[][];
	ui_hints?: Record<string, unknown>;
}

// ShapeNode is the compact structural sketch of a step the list API derives
// for the overview's flow thumbnails: kind, non-empty control branches in
// display order, and whether an on-error router hangs off it.
export interface ShapeNode {
	k: string;
	c?: ShapeNode[][];
	e?: boolean;
}

export interface CommandSummary {
	id: string;
	name: string;
	description: string;
	enabled: boolean;
	status: Status;
	version: number;
	requires_defer: boolean;
	updated_at: string;
	// Optional enrichments (older APIs may omit them; the overview degrades).
	step_count?: number;
	option_count?: number;
	flow_shape?: ShapeNode[];
	shape_more?: number;
	runs_24h?: number;
	last_run_at?: string | null;
	group_id?: string | null;
}

export interface CommandGroup {
	id: string;
	name: string;
	position: number;
	created_at?: string;
}

export interface ValidationIssue {
	severity: 'error' | 'warning';
	path: string;
	code: string;
	message: string;
}

export interface ValidationResult {
	ok: boolean;
	issues: ValidationIssue[];
	requires_defer: boolean;
	step_count: number;
}

// ── Step-kind metadata (the palette) ────────────────────────────────────────

export type StepCategory =
	| 'reply'
	| 'messages'
	| 'members'
	| 'channels'
	| 'data'
	| 'flow'
	| 'image';

export interface StepKindMeta {
	kind: string;
	category: StepCategory;
	label: string;
	short: string; // one-line description for palette
	icon: string; // lucide-svelte component name
	// Hidden kinds stay decodable/renderable (old flows keep working) but the
	// picker stops offering them — they're subsumed by another step.
	hidden?: boolean;
}

export const STEP_KINDS: StepKindMeta[] = [
	// Replying to the invocation
	{ kind: 'reply', category: 'reply', label: 'Reply', short: 'Reply to the slash invocation.', icon: 'MessageSquare' },
	{ kind: 'edit_reply', category: 'reply', label: 'Edit reply', short: 'Legacy — use Edit message with target “the reply”.', icon: 'SquarePen', hidden: true },
	{ kind: 'defer_reply', category: 'reply', label: 'Defer reply', short: 'Acknowledge — buys time for slow steps.', icon: 'Clock' },
	{ kind: 'send_dm', category: 'reply', label: 'Send DM', short: 'DM a user — embeds, buttons, files.', icon: 'Mail' },
	{ kind: 'modal_open', category: 'reply', label: 'Open modal', short: 'Prompt the user with a form.', icon: 'TextCursorInput' },

	// Messages anywhere
	{ kind: 'send_message', category: 'messages', label: 'Send message', short: 'Post a message to any channel.', icon: 'Send' },
	{ kind: 'embed_send', category: 'messages', label: 'Send embed', short: 'Legacy — Send message carries embeds.', icon: 'LayoutTemplate', hidden: true },
	{ kind: 'message_edit', category: 'messages', label: 'Edit message', short: 'Update an existing message in place.', icon: 'PencilLine' },
	{ kind: 'message_fetch', category: 'messages', label: 'Fetch message', short: 'Read a message into a variable.', icon: 'FileSearch' },
	{ kind: 'message_delete', category: 'messages', label: 'Delete message', short: 'Remove a message.', icon: 'Trash2' },
	{ kind: 'message_purge', category: 'messages', label: 'Purge messages', short: 'Bulk-delete recent messages with filters.', icon: 'Trash' },
	{ kind: 'message_crosspost', category: 'messages', label: 'Publish message', short: 'Crosspost an announcement to followers.', icon: 'Megaphone' },
	{ kind: 'react_add', category: 'messages', label: 'Add reaction', short: 'React to a message.', icon: 'SmilePlus' },
	{ kind: 'react_remove', category: 'messages', label: 'Remove reaction', short: 'Remove a reaction.', icon: 'Frown' },
	{ kind: 'react_clear', category: 'messages', label: 'Clear reactions', short: 'Clear all (or one emoji’s) reactions.', icon: 'CircleOff' },
	{ kind: 'pin_add', category: 'messages', label: 'Pin message', short: 'Pin a message.', icon: 'Pin' },
	{ kind: 'pin_remove', category: 'messages', label: 'Unpin message', short: 'Unpin a message.', icon: 'PinOff' },

	// Members & roles
	{ kind: 'role_add', category: 'members', label: 'Add role', short: 'Grant a role to a member.', icon: 'UserPlus' },
	{ kind: 'role_remove', category: 'members', label: 'Remove role', short: 'Revoke a role from a member.', icon: 'UserMinus' },
	{ kind: 'member_fetch', category: 'members', label: 'Fetch member', short: 'Read roles / join date into a variable.', icon: 'UserSearch' },
	{ kind: 'member_nickname', category: 'members', label: 'Set nickname', short: "Set a member's server nickname.", icon: 'Tag' },
	{ kind: 'member_timeout', category: 'members', label: 'Timeout member', short: 'Mute a member for a duration.', icon: 'Timer' },
	{ kind: 'member_kick', category: 'members', label: 'Kick member', short: 'Kick a member from the server.', icon: 'LogOut' },
	{ kind: 'member_ban', category: 'members', label: 'Ban member', short: 'Ban a user.', icon: 'Ban' },
	{ kind: 'member_unban', category: 'members', label: 'Unban member', short: 'Unban a user by id.', icon: 'UserCheck' },
	{ kind: 'voice_move', category: 'members', label: 'Move voice', short: 'Move a member between voice channels.', icon: 'Move' },
	{ kind: 'voice_set', category: 'members', label: 'Mute / deafen', short: 'Server-mute or deafen a member.', icon: 'MicOff' },

	// Channels & threads
	{ kind: 'channel_create', category: 'channels', label: 'Create channel', short: 'Create a new channel.', icon: 'SquarePlus' },
	{ kind: 'channel_edit', category: 'channels', label: 'Edit channel', short: 'Modify a channel.', icon: 'Pencil' },
	{ kind: 'channel_delete', category: 'channels', label: 'Delete channel', short: 'Delete a channel or close a thread.', icon: 'SquareX' },
	{ kind: 'thread_create', category: 'channels', label: 'Create thread', short: 'Open a thread.', icon: 'GitBranch' },
	{ kind: 'thread_member', category: 'channels', label: 'Thread member', short: 'Add or remove someone from a thread.', icon: 'Users' },
	{ kind: 'thread_archive', category: 'channels', label: 'Archive thread', short: 'Archive (and optionally lock) a thread.', icon: 'Archive' },
	{ kind: 'invite_create', category: 'channels', label: 'Create invite', short: 'Mint an invite link into a variable.', icon: 'Link' },

	// Image
	{ kind: 'image_render', category: 'image', label: 'Render image', short: 'Render a Studio template to PNG.', icon: 'ImagePlus' },
	{ kind: 'image_attach', category: 'image', label: 'Attach image', short: 'Legacy — attach via the message composer instead.', icon: 'Paperclip', hidden: true },
	{ kind: 'image_load', category: 'image', label: 'Load image', short: 'Download an image from a URL.', icon: 'ImageDown' },

	// Data
	{ kind: 'set_var', category: 'data', label: 'Set variable', short: 'Assign a value.', icon: 'Variable' },
	{ kind: 'incr_var', category: 'data', label: 'Increment variable', short: 'Add to a numeric variable.', icon: 'CirclePlus' },
	{ kind: 'pick_random', category: 'data', label: 'Pick random', short: 'Random entry from a list — 8ball, giveaways.', icon: 'Dices' },
	{ kind: 'json_parse', category: 'data', label: 'Parse JSON', short: 'Turn a JSON string into a structured value.', icon: 'Braces' },
	{ kind: 'kv_get', category: 'data', label: 'KV get', short: 'Read a durable value.', icon: 'Database' },
	{ kind: 'kv_set', category: 'data', label: 'KV set', short: 'Write a durable value.', icon: 'Save' },
	{ kind: 'kv_delete', category: 'data', label: 'KV delete', short: 'Delete a durable value.', icon: 'Eraser' },
	{ kind: 'http_request', category: 'data', label: 'HTTP request', short: 'Outbound HTTP call.', icon: 'Globe' },

	// Flow
	{ kind: 'if', category: 'flow', label: 'If', short: 'Conditional branch.', icon: 'Split' },
	{ kind: 'switch', category: 'flow', label: 'Switch', short: 'Multi-arm branch on a value.', icon: 'GitMerge' },
	{ kind: 'loop', category: 'flow', label: 'Loop', short: 'Iterate over a list.', icon: 'RotateCw' },
	{ kind: 'parallel', category: 'flow', label: 'Parallel', short: 'Fork branches concurrently.', icon: 'Columns2' },
	{ kind: 'wait', category: 'flow', label: 'Wait', short: 'Pause the run for a duration.', icon: 'Hourglass' },
	{ kind: 'wait_for', category: 'flow', label: 'Wait for', short: 'Plumbing for click paths; created by dragging a button dot.', icon: 'Radar', hidden: true },
	{ kind: 'exit', category: 'flow', label: 'Exit', short: 'End the run successfully.', icon: 'CircleCheck' },
	{ kind: 'fail', category: 'flow', label: 'Fail', short: 'Abort with an error.', icon: 'CircleAlert' },
	{ kind: 'run_command', category: 'flow', label: 'Run command', short: 'Invoke another custom command.', icon: 'Terminal' },
	{ kind: 'audit_note', category: 'flow', label: 'Audit note', short: 'Record an entry in the audit log.', icon: 'FileText' }
];

export const STEP_KIND_BY_KIND = new Map(STEP_KINDS.map((k) => [k.kind, k]));

export const STEP_CATEGORIES: { id: StepCategory; label: string }[] = [
	{ id: 'reply', label: 'Reply' },
	{ id: 'messages', label: 'Messages' },
	{ id: 'members', label: 'Members & roles' },
	{ id: 'channels', label: 'Channels & threads' },
	{ id: 'image', label: 'Image' },
	{ id: 'data', label: 'Data & variables' },
	{ id: 'flow', label: 'Flow control' }
];

// newStepID returns a short stable id (sortable, not crypto).
export function newStepID(): string {
	const ts = Date.now().toString(36);
	const r = Math.floor(Math.random() * 0xffffff).toString(36);
	return `S${ts}${r}`;
}

// newStep returns a Step with a sensible default spec for the kind.
export function newStep(kind: string): Step {
	const step: Step = { id: newStepID(), kind };
	switch (kind) {
		case 'reply':
			step.spec = { content: 'Hello {{ .User.Mention }}!' };
			break;
		case 'edit_reply':
			step.spec = { content: '' };
			break;
		case 'send_message':
			step.spec = { channel: { src: '{channel.id}' }, content: '' };
			break;
		case 'send_dm':
			step.spec = { user: { src: '{{ .User.ID }}' }, content: '' };
			break;
		case 'embed_send':
			step.spec = { channel: { src: '{channel.id}' }, embed: { title: '', description: '' } };
			break;
		case 'modal_open':
			step.spec = {
				title: 'Tell us more',
				custom_id_suffix: 'form',
				into: 'form',
				fields: [{ custom_id: 'answer', label: 'Your answer', style: 'short', required: true }]
			};
			break;
		case 'react_add':
		case 'react_remove':
			step.spec = { channel: { src: '{channel.id}' }, message: { src: '' }, emoji: '👍' };
			break;
		case 'react_clear':
			step.spec = { channel: { src: '{channel.id}' }, message: { src: '' } };
			break;
		case 'message_edit':
			step.spec = { channel: { src: '{channel.id}' }, message: { src: '' }, content: '' };
			break;
		case 'message_fetch':
			step.spec = { channel: { src: '{channel.id}' }, message: { src: '' }, into: 'msg' };
			break;
		case 'message_delete':
		case 'pin_add':
		case 'pin_remove':
			step.spec = { channel: { src: '{channel.id}' }, message: { src: '' } };
			break;
		case 'message_purge':
			step.spec = { channel: { src: '{channel.id}' }, limit: 50 };
			break;
		case 'message_crosspost':
			step.spec = { channel: { src: '{channel.id}' }, message: { src: '' } };
			break;
		case 'member_fetch':
			step.spec = { user: { src: '{{ .User.ID }}' }, into: 'member' };
			break;
		case 'voice_set':
			step.spec = { user: { src: '{{ .User.ID }}' }, mute: true };
			break;
		case 'thread_member':
			step.spec = { thread: { src: '' }, user: { src: '{{ .User.ID }}' }, action: 'add' };
			break;
		case 'invite_create':
			step.spec = { channel: { src: '{channel.id}' }, max_age: '24h', max_uses: 1, into: 'invite' };
			break;
		case 'pick_random':
			step.spec = { from: { lang: 'tmpl', src: '' }, into: 'picked' };
			break;
		case 'json_parse':
			step.spec = { value: { lang: 'tmpl', src: '' }, into: 'parsed' };
			break;
		case 'role_add':
		case 'role_remove':
			step.spec = { user: { src: '{{ .User.ID }}' }, role: { src: '' } };
			break;
		case 'member_timeout':
			step.spec = { user: { src: '{{ .User.ID }}' }, duration: '10m', reason: '' };
			break;
		case 'member_ban':
			step.spec = { user: { src: '{{ .User.ID }}' }, reason: '', delete_message_days: 0 };
			break;
		case 'member_kick':
		case 'member_unban':
			step.spec = { user: { src: '{{ .User.ID }}' }, reason: '' };
			break;
		case 'member_nickname':
			step.spec = { user: { src: '{{ .User.ID }}' }, nickname: '', reason: '' };
			break;
		case 'channel_create':
			step.spec = { name: 'new-channel', type: 'text', into: 'created_channel' };
			break;
		case 'channel_edit':
			step.spec = { channel: { src: '{channel.id}' } };
			break;
		case 'channel_delete':
			step.spec = { channel: { src: '' } };
			break;
		case 'thread_create':
			step.spec = { channel: { src: '{channel.id}' }, name: 'New thread', auto_archive_minutes: 60, into: 'thread' };
			break;
		case 'thread_archive':
			step.spec = { thread: { src: '' } };
			break;
		case 'voice_move':
			step.spec = { user: { src: '{{ .User.ID }}' }, channel: { src: '' } };
			break;
		case 'image_render':
			step.spec = { template_id: 0, into: 'card_png', vars: {} };
			break;
		case 'image_attach':
			step.spec = { from_var: 'card_png', filename: 'card.png' };
			break;
		case 'image_load':
			step.spec = { source: { src: '' }, into: 'loaded', max_bytes: 8388608 };
			break;
		case 'set_var':
			step.spec = { name: 'my_var', value: { lang: 'tmpl', src: '' } };
			break;
		case 'incr_var':
			step.spec = { name: 'counter', by: 1 };
			break;
		case 'kv_get':
			step.spec = { key: 'my_key', scope: 'guild', into: 'value' };
			break;
		case 'kv_set':
			step.spec = { key: 'my_key', scope: 'guild', value: { lang: 'tmpl', src: '' } };
			break;
		case 'kv_delete':
			step.spec = { key: 'my_key', scope: 'guild' };
			break;
		case 'http_request':
			step.spec = { method: 'GET', url: 'https://example.com', timeout_ms: 5000, into: 'response', parse_json: true };
			break;
		case 'if':
			step.spec = { cond: { lang: 'tmpl', src: '{{ eq .User.Username "" }}' } };
			step.then = [];
			step.else = [];
			break;
		case 'switch':
			step.spec = { on: { lang: 'tmpl', src: '' } };
			step.cases = [];
			step.default = [];
			break;
		case 'loop':
			step.spec = { over: { lang: 'tmpl', src: '' }, as: 'item' };
			step.then = [];
			break;
		case 'parallel':
			step.spec = { branches: [[], []], join: 'all' };
			break;
		case 'wait':
			step.spec = { duration: '30s' };
			break;
		case 'wait_for':
			step.spec = { trigger: 'component', timeout: '10m', custom_id_suffix: 'click', into: 'click' };
			break;
		case 'exit':
			step.spec = { reason: '' };
			break;
		case 'fail':
			step.spec = { message: '' };
			break;
		case 'run_command':
			step.spec = { command: '', inherit_scope: true };
			break;
		case 'audit_note':
			step.spec = { action: 'note', detail: { lang: 'tmpl', src: '' } };
			break;
		case 'defer_reply':
		default:
			step.spec = {};
	}
	return step;
}
