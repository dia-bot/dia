// Type-only mirror of internal/features/giveaway/config.go and the giveaway
// API. Kept in lockstep with the Go side so the dashboard never saves JSONB the
// runtime won't decode. Every embed/announce string is a Go text/template
// rendered against the giveaway scope (see GIVEAWAY_VARS).

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

export interface EmbedConfig {
	color: string;
	title: string;
	description: string;
	footer_text: string;
	thumbnail: string;
	hosted_by_label: string;
	ends_label: string;
	winners_label: string;
	entries_label: string;
	show_timestamp: boolean;
}

export interface ButtonConfig {
	label: string;
	emoji: string;
	// primary | secondary | success | danger (kept as string so the Select binds cleanly).
	style: string;
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

export interface GiveawayConfig {
	manager_roles?: string[];
	default_channel_id?: string;
	default_winner_count: number;
	default_duration: string;
	ping_role_id?: string;
	embed: EmbedConfig;
	button: ButtonConfig;
	announce: AnnounceConfig;
	requirements: RequirementConfig;
	show_entry_count: boolean;
	show_requirements: boolean;
	allow_bots_to_win: boolean;
	exclude_host: boolean;
}

// defaultConfig mirrors giveaway.Default() in Go exactly.
export function defaultConfig(): GiveawayConfig {
	return {
		manager_roles: [],
		default_channel_id: '',
		default_winner_count: 1,
		default_duration: '24h',
		ping_role_id: '',
		embed: {
			color: '#FF6363',
			title: '🎉 {{ .Prize }}',
			description: 'Click the button below to enter!',
			footer_text: '{{ .WinnerCount }} winner(s) · ends',
			thumbnail: '',
			hosted_by_label: 'Hosted by',
			ends_label: 'Ends',
			winners_label: 'Winners',
			entries_label: 'Entries',
			show_timestamp: true
		},
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
		requirements: {},
		show_entry_count: true,
		show_requirements: true,
		allow_bots_to_win: false,
		exclude_host: false
	};
}

// GIVEAWAY_VARS documents the template scope available in every giveaway string,
// for the TemplateField variable picker.
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
	{ token: '{{ .Channel }}', desc: 'The giveaway channel mention' }
];

export type GiveawayStatus = 'scheduled' | 'running' | 'ended' | 'cancelled';

export interface GiveawaySummary {
	id: string;
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
