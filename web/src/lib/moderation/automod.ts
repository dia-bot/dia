// Automod schema + catalogues. This MIRRORS internal/features/moderation/{config,catalog}.go;
// keep field names (json tags) and trigger/action keys in lockstep with Go. The
// catalogues below drive the dashboard rule editor: each trigger/action declares
// the fields it reads so a generic renderer can build its form.

// ── Schema (mirror of the Go JSONB) ──────────────────────────────────────────

export type MatchMode = 'substring' | 'word' | 'wildcard';
export type LinkMode = 'all' | 'allowlist' | 'blocklist';
export type NameScan = 'username' | 'nick' | 'both';

export interface RuleTrigger {
	type: TriggerKey;
	words?: string[];
	match_mode?: MatchMode;
	patterns?: string[];
	allow_list?: string[];
	link_mode?: LinkMode;
	domains?: string[];
	limit?: number;
	min_length?: number;
	count?: number;
	window?: number;
	everyone?: boolean;
	roles?: boolean;
	scan?: NameScan;
}

export interface RuleExempt {
	roles?: string[];
	channels?: string[];
}

export interface RuleAction {
	type: ActionKey;
	duration?: number;
	delete_days?: number;
	role_id?: string;
	message?: string;
	channel?: string;
	delete_after?: number;
	points?: number;
	// id of the saved automation launched when type == 'run_automation'.
	automation_id?: string;
	reason?: string;
}

export interface AutomodRule {
	id: string;
	name: string;
	enabled: boolean;
	trigger: RuleTrigger;
	exempt: RuleExempt;
	actions: RuleAction[];
}

export interface EscalationTier {
	points: number;
	action: 'timeout' | 'kick' | 'ban' | 'run_automation';
	duration?: number;
	// id of the saved automation launched when action == 'run_automation'.
	automation?: string;
}

export interface Escalation {
	enabled: boolean;
	decay_hours: number;
	tiers: EscalationTier[];
}

// RaidConfig mirrors moderation.RaidConfig (Go). The anti-raid join-velocity
// guard: when more than `threshold` members join within `window` seconds the
// guild enters raid mode and joiners are actioned until it auto-calms.
export interface RaidConfig {
	enabled: boolean;
	window: number;
	threshold: number;
	action: 'kick' | 'ban' | 'timeout';
	timeout_seconds?: number;
	only_new_accounts: boolean;
	new_account_hours?: number;
	alert_channel?: string;
}

export interface AutomodConfig {
	rules: AutomodRule[];
	escalation: Escalation;
	raid: RaidConfig;
	exempt_roles: string[];
	exempt_channels: string[];
	ignore_bots: boolean;
	ignore_mods: boolean;
	alert_channel: string;
}

// ── Trigger keys / catalogue ─────────────────────────────────────────────────

export type TriggerKey =
	| 'words'
	| 'regex'
	| 'invites'
	| 'links'
	| 'scam_links'
	| 'spam'
	| 'duplicates'
	| 'mentions'
	| 'mass_mention'
	| 'caps'
	| 'emojis'
	| 'newlines'
	| 'zalgo'
	| 'spoilers'
	| 'attachments'
	| 'account_age'
	| 'name';

export type ActionKey =
	| 'delete'
	| 'warn'
	| 'timeout'
	| 'kick'
	| 'ban'
	| 'add_role'
	| 'remove_role'
	| 'send_message'
	| 'dm'
	| 'add_points'
	| 'run_automation';

// A field descriptor drives a generic form control in the rule editor.
export type FieldType =
	| 'text'
	| 'textarea'
	| 'number'
	| 'words' // chip list
	| 'select'
	| 'toggle'
	| 'role'
	| 'channel'
	| 'duration' // seconds, rendered as a friendly duration control
	| 'automation'; // pick a saved automation flow

export interface FieldSpec {
	key: string; // matches a RuleTrigger / RuleAction json field
	label: string;
	type: FieldType;
	hint?: string;
	placeholder?: string;
	min?: number;
	max?: number;
	suffix?: string;
	options?: { value: string; label: string }[];
	// show this field only when another field has one of these values
	showWhen?: { key: string; in: (string | number | boolean)[] };
}

export type TriggerSurface = 'message' | 'member';

export interface TriggerSpec {
	key: TriggerKey;
	label: string;
	short: string; // one-liner for the picker
	description: string;
	category: 'Content' | 'Spam & flood' | 'Mentions' | 'Formatting' | 'Members';
	surface: TriggerSurface;
	icon: string; // lucide icon name
	fields: FieldSpec[];
	defaults: Partial<RuleTrigger>;
}

const MATCH_MODE_OPTIONS = [
	{ value: 'substring', label: 'Contains (substring)' },
	{ value: 'word', label: 'Whole word' },
	{ value: 'wildcard', label: 'Wildcard (* matches anything)' }
];

export const TRIGGERS: TriggerSpec[] = [
	{
		key: 'words',
		label: 'Blocked words',
		short: 'Words or phrases to filter',
		description: 'Flags messages containing any of the listed words or phrases. Choose how strictly each entry is matched.',
		category: 'Content',
		surface: 'message',
		icon: 'CaseSensitive',
		fields: [
			{ key: 'words', label: 'Blocked words', type: 'words', hint: 'Press enter or comma to add each term.' },
			{ key: 'match_mode', label: 'Match', type: 'select', options: MATCH_MODE_OPTIONS },
			{ key: 'allow_list', label: 'Allowed exceptions', type: 'words', hint: 'Terms that should never trip this rule.' }
		],
		defaults: { match_mode: 'substring' }
	},
	{
		key: 'regex',
		label: 'Regex filter',
		short: 'Custom regular expressions',
		description: 'Power-user matching: flags messages matching any RE2 pattern. Great for structured spam, leetspeak, or formats.',
		category: 'Content',
		surface: 'message',
		icon: 'Regex',
		fields: [
			{ key: 'patterns', label: 'Patterns', type: 'words', hint: 'One RE2 pattern per entry, e.g. (?i)free\\s*nitro' },
			{ key: 'allow_list', label: 'Allowed exceptions', type: 'words' }
		],
		defaults: {}
	},
	{
		key: 'invites',
		label: 'Discord invites',
		short: 'Block invite links to other servers',
		description: 'Flags any Discord invite link. Allow specific servers by listing their invite codes or server IDs.',
		category: 'Content',
		surface: 'message',
		icon: 'Link2Off',
		fields: [
			{ key: 'allow_list', label: 'Allowed invites', type: 'words', hint: 'Invite codes or server IDs that are allowed.' }
		],
		defaults: {}
	},
	{
		key: 'links',
		label: 'Links',
		short: 'Control which URLs are allowed',
		description: 'Filters messages containing URLs. Block all links, allow only listed domains, or block only listed domains.',
		category: 'Content',
		surface: 'message',
		icon: 'Globe',
		fields: [
			{
				key: 'link_mode',
				label: 'Mode',
				type: 'select',
				options: [
					{ value: 'all', label: 'Block all links' },
					{ value: 'allowlist', label: 'Allow only these domains' },
					{ value: 'blocklist', label: 'Block only these domains' }
				]
			},
			{ key: 'allow_list', label: 'Allowed domains', type: 'words', showWhen: { key: 'link_mode', in: ['allowlist'] } },
			{ key: 'domains', label: 'Blocked domains', type: 'words', showWhen: { key: 'link_mode', in: ['blocklist'] } }
		],
		defaults: { link_mode: 'all' }
	},
	{
		key: 'scam_links',
		label: 'Scam & phishing links',
		short: 'Block known malicious domains (live threat feed)',
		description:
			'Checks links against a continuously-updated list of known phishing and scam domains (Discord nitro scams, IP loggers, token stealers). The list syncs automatically in the background, so it catches new campaigns without you maintaining it.',
		category: 'Content',
		surface: 'message',
		icon: 'ShieldAlert',
		fields: [
			{
				key: 'allow_list',
				label: 'Allowed domains',
				type: 'words',
				hint: 'Domains to never flag even if they appear on the feed (rare).'
			}
		],
		defaults: {}
	},
	{
		key: 'spam',
		label: 'Message spam',
		short: 'Too many messages, too fast',
		description: 'Flags a user who sends more than the allowed number of messages within a short window (across any channel).',
		category: 'Spam & flood',
		surface: 'message',
		icon: 'Zap',
		fields: [
			{ key: 'count', label: 'Messages', type: 'number', min: 2, max: 50, suffix: 'messages' },
			{ key: 'window', label: 'Within', type: 'number', min: 1, max: 120, suffix: 'seconds' }
		],
		defaults: { count: 5, window: 5 }
	},
	{
		key: 'duplicates',
		label: 'Repeated messages',
		short: 'Same message sent over and over',
		description: 'Flags a user repeating the same message content within a window.',
		category: 'Spam & flood',
		surface: 'message',
		icon: 'Copy',
		fields: [
			{ key: 'count', label: 'Repeats', type: 'number', min: 2, max: 20, suffix: 'times' },
			{ key: 'window', label: 'Within', type: 'number', min: 1, max: 300, suffix: 'seconds' }
		],
		defaults: { count: 3, window: 30 }
	},
	{
		key: 'mentions',
		label: 'Mention spam',
		short: 'Too many user mentions in one message',
		description: 'Flags a single message that pings more than the allowed number of distinct users.',
		category: 'Mentions',
		surface: 'message',
		icon: 'AtSign',
		fields: [{ key: 'limit', label: 'Max mentions', type: 'number', min: 1, max: 50, suffix: 'mentions' }],
		defaults: { limit: 6 }
	},
	{
		key: 'mass_mention',
		label: '@everyone / role pings',
		short: 'Block @everyone, @here and role pings',
		description: 'Flags messages that ping @everyone/@here and/or roles. Useful for stopping ping raids.',
		category: 'Mentions',
		surface: 'message',
		icon: 'Megaphone',
		fields: [
			{ key: 'everyone', label: 'Catch @everyone / @here', type: 'toggle' },
			{ key: 'roles', label: 'Catch role pings', type: 'toggle' },
			{ key: 'limit', label: 'Allowed before tripping', type: 'number', min: 0, max: 20, hint: '0 = trip on the first one.' }
		],
		defaults: { everyone: true, roles: false, limit: 0 }
	},
	{
		key: 'caps',
		label: 'Excessive caps',
		short: 'SHOUTING in capital letters',
		description: 'Flags messages that are mostly uppercase. Short messages are ignored to avoid false positives.',
		category: 'Formatting',
		surface: 'message',
		icon: 'ArrowBigUpDash',
		fields: [
			{ key: 'limit', label: 'Max uppercase', type: 'number', min: 50, max: 100, suffix: '%' },
			{ key: 'min_length', label: 'Ignore shorter than', type: 'number', min: 1, max: 200, suffix: 'chars' }
		],
		defaults: { limit: 70, min_length: 10 }
	},
	{
		key: 'emojis',
		label: 'Excessive emoji',
		short: 'Too many emoji in one message',
		description: 'Flags messages packed with more than the allowed number of emoji (unicode and custom).',
		category: 'Formatting',
		surface: 'message',
		icon: 'Smile',
		fields: [{ key: 'limit', label: 'Max emoji', type: 'number', min: 1, max: 100, suffix: 'emoji' }],
		defaults: { limit: 8 }
	},
	{
		key: 'newlines',
		label: 'Wall of text',
		short: 'Too many line breaks',
		description: 'Flags messages with more than the allowed number of newlines (spammy multi-line walls).',
		category: 'Formatting',
		surface: 'message',
		icon: 'WrapText',
		fields: [{ key: 'limit', label: 'Max newlines', type: 'number', min: 1, max: 100, suffix: 'lines' }],
		defaults: { limit: 12 }
	},
	{
		key: 'zalgo',
		label: 'Disruptive text',
		short: 'Zalgo / glitch text',
		description: 'Flags messages with an unusual amount of combining marks (zalgo / glitch text used to disrupt chat).',
		category: 'Formatting',
		surface: 'message',
		icon: 'Sparkles',
		fields: [{ key: 'limit', label: 'Max combining marks', type: 'number', min: 10, max: 100, suffix: '%', hint: 'Leave at the default unless you see false positives.' }],
		defaults: { limit: 50 }
	},
	{
		key: 'spoilers',
		label: 'Spoiler spam',
		short: 'Too many spoiler tags',
		description: 'Flags messages with more than the allowed number of ||spoiler|| spans.',
		category: 'Formatting',
		surface: 'message',
		icon: 'EyeOff',
		fields: [{ key: 'limit', label: 'Max spoilers', type: 'number', min: 1, max: 50, suffix: 'spoilers' }],
		defaults: { limit: 6 }
	},
	{
		key: 'attachments',
		label: 'Attachment flood',
		short: 'Too many attachments at once',
		description: 'Flags messages carrying more than the allowed number of attachments.',
		category: 'Formatting',
		surface: 'message',
		icon: 'Paperclip',
		fields: [{ key: 'limit', label: 'Max attachments', type: 'number', min: 1, max: 10, suffix: 'files' }],
		defaults: { limit: 5 }
	},
	{
		key: 'account_age',
		label: 'New account gate',
		short: 'Catch very new accounts on join',
		description: 'Flags members whose Discord account is younger than the threshold when they join. Pair with a quarantine role.',
		category: 'Members',
		surface: 'member',
		icon: 'UserPlus',
		fields: [{ key: 'limit', label: 'Minimum account age', type: 'number', min: 1, max: 8760, suffix: 'hours' }],
		defaults: { limit: 24 }
	},
	{
		key: 'name',
		label: 'Username filter',
		short: 'Block bad usernames / nicknames',
		description: 'Flags members whose username or nickname matches blocked words or patterns, on join and on change.',
		category: 'Members',
		surface: 'member',
		icon: 'UserX',
		fields: [
			{ key: 'words', label: 'Blocked words', type: 'words' },
			{ key: 'patterns', label: 'Patterns (regex)', type: 'words' },
			{
				key: 'scan',
				label: 'Scan',
				type: 'select',
				options: [
					{ value: 'both', label: 'Username and nickname' },
					{ value: 'username', label: 'Username only' },
					{ value: 'nick', label: 'Nickname only' }
				]
			}
		],
		defaults: { scan: 'both', match_mode: 'substring' }
	}
];

export const TRIGGERS_BY_KEY: Record<TriggerKey, TriggerSpec> = Object.fromEntries(
	TRIGGERS.map((t) => [t.key, t])
) as Record<TriggerKey, TriggerSpec>;

// ── Action catalogue ─────────────────────────────────────────────────────────

export interface ActionSpec {
	key: ActionKey;
	label: string;
	short: string;
	icon: string;
	tone: 'neutral' | 'warn' | 'danger'; // for chip colouring
	// 'message' actions only make sense for message triggers (e.g. delete);
	// 'any' actions apply to both surfaces.
	surface: 'message' | 'any';
	fields: FieldSpec[];
	defaults: Partial<RuleAction>;
}

export const ACTIONS: ActionSpec[] = [
	{
		key: 'delete',
		label: 'Delete message',
		short: 'Remove the offending message',
		icon: 'Trash2',
		tone: 'neutral',
		surface: 'message',
		fields: [],
		defaults: {}
	},
	{
		key: 'warn',
		label: 'Warn',
		short: 'Log a warning case (and DM the user)',
		icon: 'TriangleAlert',
		tone: 'warn',
		surface: 'any',
		fields: [{ key: 'reason', label: 'Reason', type: 'text', placeholder: 'Why was this warned?' }],
		defaults: {}
	},
	{
		key: 'timeout',
		label: 'Timeout',
		short: 'Temporarily mute the member',
		icon: 'Clock',
		tone: 'warn',
		surface: 'any',
		fields: [
			{ key: 'duration', label: 'Duration', type: 'duration', suffix: 'seconds' },
			{ key: 'reason', label: 'Reason', type: 'text' }
		],
		defaults: { duration: 600 }
	},
	{
		key: 'kick',
		label: 'Kick',
		short: 'Remove the member (can rejoin)',
		icon: 'LogOut',
		tone: 'danger',
		surface: 'any',
		fields: [{ key: 'reason', label: 'Reason', type: 'text' }],
		defaults: {}
	},
	{
		key: 'ban',
		label: 'Ban',
		short: 'Remove and block the member',
		icon: 'Ban',
		tone: 'danger',
		surface: 'any',
		fields: [
			{ key: 'delete_days', label: 'Delete message history', type: 'number', min: 0, max: 7, suffix: 'days' },
			{ key: 'duration', label: 'Temp-ban (0 = permanent)', type: 'duration', suffix: 'seconds' },
			{ key: 'reason', label: 'Reason', type: 'text' }
		],
		defaults: { delete_days: 1 }
	},
	{
		key: 'add_role',
		label: 'Add role',
		short: 'Grant a role (e.g. mute / quarantine)',
		icon: 'ShieldPlus',
		tone: 'neutral',
		surface: 'any',
		fields: [{ key: 'role_id', label: 'Role', type: 'role' }],
		defaults: {}
	},
	{
		key: 'remove_role',
		label: 'Remove role',
		short: 'Revoke a role',
		icon: 'ShieldMinus',
		tone: 'neutral',
		surface: 'any',
		fields: [{ key: 'role_id', label: 'Role', type: 'role' }],
		defaults: {}
	},
	{
		key: 'send_message',
		label: 'Send a message',
		short: 'Post a notice in a channel',
		icon: 'MessageSquare',
		tone: 'neutral',
		surface: 'any',
		fields: [
			{ key: 'message', label: 'Message', type: 'textarea', hint: 'Supports {{ .User.Mention }} and other template values.' },
			{ key: 'channel', label: 'Channel', type: 'channel', hint: 'Leave empty to post in the channel that triggered the rule.' },
			{ key: 'delete_after', label: 'Auto-delete after', type: 'number', min: 0, max: 3600, suffix: 'seconds', hint: '0 = keep it.' }
		],
		defaults: {}
	},
	{
		key: 'dm',
		label: 'DM the user',
		short: 'Send the offender a private message',
		icon: 'Mail',
		tone: 'neutral',
		surface: 'any',
		fields: [{ key: 'message', label: 'Message', type: 'textarea', hint: 'Supports template values like {{ .Guild.Name }}.' }],
		defaults: {}
	},
	{
		key: 'add_points',
		label: 'Add infraction points',
		short: 'Feed the escalation ladder',
		icon: 'Flame',
		tone: 'warn',
		surface: 'any',
		fields: [{ key: 'points', label: 'Points', type: 'number', min: 1, max: 20, suffix: 'points' }],
		defaults: { points: 1 }
	},
	{
		key: 'run_automation',
		label: 'Run automation',
		short: 'Launch an automation flow',
		icon: 'Zap',
		tone: 'neutral',
		surface: 'any',
		fields: [
			{
				key: 'automation_id',
				label: 'Automation',
				type: 'automation',
				hint: 'The flow to launch when this rule fires. It gets the offending member as .User and the rule details as .Event.*'
			}
		],
		defaults: {}
	}
];

export const ACTIONS_BY_KEY: Record<ActionKey, ActionSpec> = Object.fromEntries(
	ACTIONS.map((a) => [a.key, a])
) as Record<ActionKey, ActionSpec>;

// Actions valid for a given trigger surface (message triggers can delete; member triggers can't).
export function actionsForSurface(surface: TriggerSurface): ActionSpec[] {
	return ACTIONS.filter((a) => a.surface === 'any' || a.surface === surface);
}

// ── Helpers ──────────────────────────────────────────────────────────────────

let ruleSeq = 0;
export function newRuleId(): string {
	ruleSeq += 1;
	return `rule_${Date.now().toString(36)}${ruleSeq.toString(36)}`;
}

export function newRule(type: TriggerKey): AutomodRule {
	const spec = TRIGGERS_BY_KEY[type];
	return {
		id: newRuleId(),
		name: spec.label,
		enabled: true,
		trigger: { type, ...spec.defaults },
		exempt: {},
		actions: spec.surface === 'message' ? [{ type: 'delete' }, { type: 'warn' }] : [{ type: 'add_role' }]
	};
}

export function defaultConfig(): AutomodConfig {
	return {
		rules: [],
		escalation: {
			enabled: true,
			decay_hours: 24,
			tiers: [
				{ points: 3, action: 'timeout', duration: 600 },
				{ points: 5, action: 'timeout', duration: 3600 },
				{ points: 8, action: 'kick' },
				{ points: 12, action: 'ban' }
			]
		},
		raid: {
			enabled: false,
			window: 10,
			threshold: 8,
			action: 'kick',
			only_new_accounts: true,
			new_account_hours: 72
		},
		exempt_roles: [],
		exempt_channels: [],
		ignore_bots: true,
		ignore_mods: true,
		alert_channel: ''
	};
}

// Short human summary of a rule's trigger for cards (no editor needed).
export function triggerSummary(t: RuleTrigger): string {
	switch (t.type) {
		case 'words':
			return `${t.words?.length ?? 0} blocked word(s)`;
		case 'regex':
			return `${t.patterns?.length ?? 0} pattern(s)`;
		case 'invites':
			return 'Discord invite links';
		case 'scam_links':
			return 'known phishing/scam domains';
		case 'links':
			return t.link_mode === 'allowlist'
				? `Only ${t.allow_list?.length ?? 0} domain(s) allowed`
				: t.link_mode === 'blocklist'
					? `${t.domains?.length ?? 0} domain(s) blocked`
					: 'All links blocked';
		case 'spam':
			return `${t.count ?? 0} messages / ${t.window ?? 0}s`;
		case 'duplicates':
			return `${t.count ?? 0} repeats / ${t.window ?? 0}s`;
		case 'mentions':
			return `> ${t.limit ?? 0} mentions`;
		case 'mass_mention':
			return [t.everyone && '@everyone/@here', t.roles && 'role pings'].filter(Boolean).join(' + ') || 'mass mentions';
		case 'caps':
			return `> ${t.limit ?? 0}% caps`;
		case 'emojis':
			return `> ${t.limit ?? 0} emoji`;
		case 'newlines':
			return `> ${t.limit ?? 0} lines`;
		case 'zalgo':
			return 'disruptive text';
		case 'spoilers':
			return `> ${t.limit ?? 0} spoilers`;
		case 'attachments':
			return `> ${t.limit ?? 0} attachments`;
		case 'account_age':
			return `account < ${t.limit ?? 0}h old`;
		case 'name':
			return `${(t.words?.length ?? 0) + (t.patterns?.length ?? 0)} name filter(s)`;
		default:
			return t.type;
	}
}
