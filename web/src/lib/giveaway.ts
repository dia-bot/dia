// Type-only mirror of internal/features/giveaway/config.go and the giveaway
// API. Kept in lockstep with the Go side so the dashboard never saves JSONB the
// runtime won't decode.
//
// Each giveaway now carries its OWN composed message (the same {content,
// embeds} shape the shared MessageEditor produces and Welcome/Leveling render
// server-side), plus a button, a winner announcement and behaviour toggles. The
// feature config is a library of reusable presets that seed new giveaways. Every
// embed/announce string is a Go text/template over the giveaway scope
// (GIVEAWAY_VARS / GIVEAWAY_SCOPE_VARS).

export const FEATURE = 'giveaway';

export interface BonusEntry {
	role_id: string;
	entries: number;
}

export interface RequirementConfig {
	required_roles?: string[];
	blocked_roles?: string[];
	bypass_roles?: string[];
	bonus_entries?: BonusEntry[];
	min_account_age_days?: number;
	min_member_age_days?: number;
	min_level?: number;
}

// EmbedField / EmbedSpec mirror customcommands.EmbedSpec — the exact shape the
// MessageEditor / EmbedBuilder produce in a step's spec.embeds.
export interface EmbedField {
	name: string;
	value: string;
	inline?: boolean;
}

export interface EmbedSpec {
	title?: string;
	description?: string;
	url?: string;
	color?: string;
	author_name?: string;
	author_icon?: string;
	author_url?: string;
	thumbnail?: string;
	image_url?: string;
	footer_text?: string;
	footer_icon?: string;
	timestamp?: boolean;
	fields?: EmbedField[];
}

export interface ButtonConfig {
	label: string;
	emoji: string;
	// primary | secondary | success | danger (kept as string so a Select binds cleanly).
	style: string;
}

// Component / ComponentRow mirror customcommands.Component — the shape the shared
// MessageEditor writes into spec.components. Giveaway buttons are either the
// entry button (custom_id_suffix === enter_button_suffix) or link buttons.
export interface Component {
	type: string;
	style?: string;
	label?: string;
	emoji?: string;
	custom_id_suffix?: string;
	url?: string;
	disabled?: boolean;
	on_click?: string;
	placeholder?: string;
}

export interface ComponentRow {
	components: Component[];
}

export interface AnnounceConfig {
	message: string;
	ping_winners: boolean;
	jump_button: boolean;
	ended_title: string;
	ended_footer: string;
	no_winners_message: string;
	dm_winners: boolean;
	dm_message: string;
}

// EntryConfig mirrors giveaway.EntryConfig — the ephemeral reply a member gets
// when they click Enter, per outcome. Each is a Go template over the giveaway
// scope plus {{ .Entries }} / {{ .Reason }}. Empty falls back to the built-in
// default on the server, so a member is never left without a reply.
export interface EntryConfig {
	entered: string; // success (has {{ .Entries }})
	left: string; // toggled off (leaves)
	not_eligible: string; // failed a requirement (has {{ .Reason }})
	bots_blocked: string; // a bot clicked
	ended: string; // clicked after it ended
}

// GiveawaySpec is one giveaway's composed presentation + behaviour (stored on
// the giveaway row, and embedded in a preset).
export interface GiveawaySpec {
	content?: string;
	embeds?: EmbedSpec[];
	// Extra composed button rows (links + the entry button). Empty = the system
	// Enter button (styled by `button`) is used.
	components?: ComponentRow[];
	// custom_id_suffix of the composed button that enters the giveaway; '' = use
	// the auto-added system Enter button.
	enter_button_suffix?: string;
	// Maps a composed button's custom_id_suffix to the saved automation it runs on
	// click ("point a button at an automation").
	button_actions?: Record<string, string>;
	button: ButtonConfig;
	announce: AnnounceConfig;
	// Per-outcome ephemeral replies to an Enter click (edited inline in the editor;
	// side effects live in the built-in on-entry automation).
	entry: EntryConfig;
	ping_role_id?: string;
	show_requirements: boolean;
	exclude_host: boolean;
	allow_bots_to_win: boolean;
}

export interface Preset {
	id: string;
	name: string;
	default_channel_id?: string;
	default_duration?: string;
	default_winner_count?: number;
	spec: GiveawaySpec;
	requirements: RequirementConfig;
}

export interface GiveawayConfig {
	manager_roles?: string[];
	presets: Preset[];
	default_preset_id?: string;
}

// defaultSpec mirrors giveaway.defaultSpec() in Go.
export function defaultSpec(): GiveawaySpec {
	return {
		content: '',
		embeds: [
			{
				color: '#FF6363',
				title: '🎉 {{ .Prize }}',
				description: 'Click the button below to enter!',
				fields: [
					{ name: 'Hosted by', value: '{{ .Host }}', inline: true },
					{ name: 'Ends', value: '{{ .Ends }}', inline: true },
					{ name: 'Winners', value: '{{ .WinnerCount }}', inline: true },
					{ name: 'Entries', value: '{{ .EntryCount }}', inline: true }
				]
			}
		],
		// A real, visible Enter button (edited in the message preview); nothing is
		// added invisibly. enter_button_suffix points at it.
		components: [
			{
				components: [
					{ type: 'button', style: 'primary', label: 'Enter Giveaway', emoji: '🎉', custom_id_suffix: 'enter' }
				]
			}
		],
		enter_button_suffix: 'enter',
		button_actions: {},
		button: { label: 'Enter Giveaway', emoji: '🎉', style: 'primary' },
		announce: {
			message: 'Congratulations {{ .Winners }}! You won **{{ .Prize }}** 🎉',
			ping_winners: true,
			jump_button: true,
			ended_title: '🎉 {{ .Prize }}',
			ended_footer: 'Ended',
			no_winners_message: 'Not enough valid entries to draw a winner for **{{ .Prize }}**.',
			dm_winners: false,
			dm_message:
				'🎉 You won **{{ .Prize }}** in {{ .Server }}! Contact the host {{ .Host }} to claim your prize.'
		},
		// Mirrors giveaway.defaultEntry() in Go — keep the copy in lockstep.
		entry: {
			entered:
				"🎉 You're entered into the giveaway for **{{ .Prize }}**!{{ if gt .Entries 1 }} You have **{{ .Entries }}** entries.{{ end }}",
			left: "You've left the giveaway for **{{ .Prize }}**.",
			not_eligible: '❌ {{ .Reason }}',
			bots_blocked: "Bots can't enter this giveaway.",
			ended: 'This giveaway has already ended.'
		},
		ping_role_id: '',
		show_requirements: true,
		exclude_host: false,
		allow_bots_to_win: false
	};
}

// defaultPreset mirrors giveaway.defaultPreset() in Go.
export function defaultPreset(): Preset {
	return {
		id: 'default',
		name: 'Classic',
		default_channel_id: '',
		default_duration: '24h',
		default_winner_count: 1,
		spec: defaultSpec(),
		requirements: {}
	};
}

// defaultConfig mirrors giveaway.Default() in Go.
export function defaultConfig(): GiveawayConfig {
	return {
		manager_roles: [],
		presets: [defaultPreset()],
		default_preset_id: 'default'
	};
}

// newPresetID mints a client-side preset id (stable enough for a small library).
export function newPresetID(): string {
	const ts = Date.now().toString(36);
	const r = Math.floor(Math.random() * 0xffffff).toString(36);
	return `p${ts}${r}`;
}

// parseDurationSeconds parses a compact duration ("30m", "2h", "3d", "1w",
// "1d12h") to seconds, mirroring giveaway.parseGiveawayDuration in Go. Returns 0
// when the input is empty or malformed.
export function parseDurationSeconds(s: string): number {
	const str = (s || '').trim().toLowerCase();
	if (!str) return 0;
	const units: Record<string, number> = { s: 1, m: 60, h: 3600, d: 86400, w: 604800 };
	let total = 0;
	let num = '';
	for (const ch of str) {
		if (ch >= '0' && ch <= '9') {
			num += ch;
			continue;
		}
		if (!num || !(ch in units)) return 0;
		total += parseInt(num, 10) * units[ch];
		num = '';
	}
	if (num) return 0;
	return total;
}

// GIVEAWAY_VARS documents the template scope available in every giveaway string,
// for the TemplateField variable picker (announce / DM fields).
export const GIVEAWAY_VARS: { token: string; desc: string }[] = [
	{ token: '{{ .Prize }}', desc: 'The prize' },
	{ token: '{{ .Description }}', desc: 'The giveaway description' },
	{ token: '{{ .WinnerCount }}', desc: 'Number of winners' },
	{ token: '{{ .EntryCount }}', desc: 'Number of entrants' },
	{ token: '{{ .Host }}', desc: 'The host mention' },
	{ token: '{{ .Winners }}', desc: 'Winner mentions, comma-separated (ended only)' },
	{ token: '{{ .WinnerList }}', desc: 'Winner mentions, one per line (ended only)' },
	{ token: '{{ .Ends }}', desc: 'Relative end time (e.g. "in 2 hours")' },
	{ token: '{{ .EndsAt }}', desc: 'Absolute end time' },
	{ token: '{{ .Server }}', desc: 'The server name' },
	{ token: '{{ .MemberCount }}', desc: 'Server member count' },
	{ token: '{{ .Channel }}', desc: 'The giveaway channel mention' },
	{ token: '{{ .Entries }}', desc: 'The entrant’s weighted tickets (entry reply)' },
	{ token: '{{ .Reason }}', desc: 'Why entry was denied (entry reply)' }
];

// GIVEAWAY_SCOPE_VARS is the same scope shaped as ExprScope.extraVars so the
// MessageEditor's variable picker offers the giveaway variables.
export const GIVEAWAY_SCOPE_VARS: { path: string; label: string; type: string; short: string }[] = [
	{ path: '.Prize', label: 'Prize', type: 'string', short: 'The prize' },
	{ path: '.Description', label: 'Description', type: 'string', short: 'The giveaway description' },
	{ path: '.WinnerCount', label: 'WinnerCount', type: 'int', short: 'Number of winners' },
	{ path: '.EntryCount', label: 'EntryCount', type: 'int', short: 'Number of entrants' },
	{ path: '.Host', label: 'Host', type: 'string', short: 'The host mention' },
	{ path: '.Winners', label: 'Winners', type: 'string', short: 'Winner mentions (ended only)' },
	{ path: '.WinnerList', label: 'WinnerList', type: 'string', short: 'Winners, one per line' },
	{ path: '.Ends', label: 'Ends', type: 'string', short: 'Relative end time' },
	{ path: '.EndsAt', label: 'EndsAt', type: 'string', short: 'Absolute end time' },
	{ path: '.Server', label: 'Server', type: 'string', short: 'The server name' },
	{ path: '.MemberCount', label: 'MemberCount', type: 'int', short: 'Server member count' },
	{ path: '.Channel', label: 'Channel', type: 'string', short: 'The giveaway channel mention' },
	{ path: '.Entries', label: 'Entries', type: 'int', short: 'Entrant’s weighted tickets (entry reply)' },
	{ path: '.Reason', label: 'Reason', type: 'string', short: 'Denial reason (entry reply)' }
];

// GIVEAWAY_SAMPLE is realistic sample data for the giveaway scope, passed to the
// server "Test render" so a preview of a giveaway string ({{ .Prize }},
// {{ .Winners }}, …) resolves against the SAME card engine the giveaway uses at
// runtime — rather than the default user/guild scope, where those fields don't
// exist and the render errors ("can't evaluate field Prize"). Keep the keys in
// lockstep with scopeData() in internal/features/giveaway/embeds.go.
export const GIVEAWAY_SAMPLE: Record<string, unknown> = {
	Prize: 'Discord Nitro (1 month)',
	Description: 'A month of Nitro, on the house.',
	WinnerCount: 2,
	EntryCount: 84,
	Host: '@you',
	Winners: '@alex, @sam',
	WinnerList: '@alex\n@sam',
	Ends: 'in 2 hours',
	EndsAt: 'July 7, 2026 6:00 PM',
	Server: 'Your Server',
	MemberCount: 1024,
	Channel: '#giveaways',
	Entries: 3,
	Reason: 'You need the @Member role to enter.'
};

export type GiveawayStatus = 'draft' | 'scheduled' | 'running' | 'ended' | 'cancelled';

export interface GiveawaySummary {
	id: string;
	name: string;
	channel_id: string;
	message_id: string;
	prize: string;
	description: string;
	winner_count: number;
	host_id: string;
	status: GiveawayStatus;
	image_url: string;
	color: string;
	winners: string[];
	entry_count: number;
	requirements: RequirementConfig;
	starts_at: string;
	ends_at: string;
	ended_at: string | null;
	created_at: string;
}

// GiveawayDetail is a summary plus the full composed spec, for the editor.
export interface GiveawayDetail extends GiveawaySummary {
	spec: GiveawaySpec;
}
