// TypeScript mirror of internal/features/tickets/config.go. Field names must
// match the Go json tags exactly so the editor never emits JSONB the worker
// can't decode (the CLAUDE.md lockstep rule).

import type { TmplVar } from '$lib/commands/expr-meta';

// TicketEmbed mirrors cc.EmbedSpec (the shape EmbedBuilder edits).
export interface TicketEmbed {
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
	fields?: { name: string; value: string; inline?: boolean }[];
}

// ── Feature config (guild_feature_configs, key "tickets") ──
export interface TicketsConfig {
	staff_role_ids: string[];
	log_channel: string;
	transcript_channel: string;
	blacklist_role_ids: string[];
	blacklist_user_ids: string[];
	max_open_per_user: number;
	default_parent_id: string;
	name_prefix: string;
}

export function defaultTicketsConfig(): TicketsConfig {
	return {
		staff_role_ids: [],
		log_channel: '',
		transcript_channel: '',
		blacklist_role_ids: [],
		blacklist_user_ids: [],
		max_open_per_user: 3,
		default_parent_id: '',
		name_prefix: 'ticket'
	};
}

// ── Panel + category config (ticket_panels.config JSONB) ──
export interface MessageSpec {
	content?: string;
	use_embed?: boolean;
	embed: TicketEmbed;
}

export interface FormField {
	id: string;
	label: string;
	placeholder?: string;
	style?: 'short' | 'paragraph';
	required?: boolean;
	min_length?: number;
	max_length?: number;
}

export interface TranscriptConfig {
	enabled?: boolean;
	dm_opener?: boolean;
}

export interface FeedbackConfig {
	enabled?: boolean;
	prompt?: string;
}

export interface AutoCloseConfig {
	enabled?: boolean;
	inactivity_minutes?: number;
	warn_minutes?: number;
}

export interface CategoryConfig {
	id: string;
	label: string;
	emoji?: string;
	description?: string;
	button_style?: string;
	open_mode?: string;
	parent_id?: string;
	name_scheme?: string;
	support_role_ids?: string[];
	ping_role_ids?: string[];
	ping_opener?: boolean;
	required_role_ids?: string[];
	max_open_per_user?: number;
	cooldown_seconds?: number;
	form?: FormField[];
	welcome: MessageSpec;
	transcript: TranscriptConfig;
	feedback: FeedbackConfig;
	auto_close: AutoCloseConfig;
	claim_enabled?: boolean;
	on_open_automation?: string;
	on_close_automation?: string;
}

export interface PanelConfig {
	content?: string;
	embed: TicketEmbed;
	select_placeholder?: string;
	categories: CategoryConfig[];
}

export interface PanelSummary {
	id: string;
	name: string;
	style: string;
	enabled: boolean;
	position: number;
	channel_id: string;
	message_id: string;
	config: PanelConfig;
}

// shortId mints a short, custom_id-safe category key.
function shortId(): string {
	const rand = Math.random().toString(36).slice(2, 8);
	return 'c' + rand;
}

export function newCategory(label = 'Support'): CategoryConfig {
	return {
		id: shortId(),
		label,
		emoji: '🎫',
		description: '',
		button_style: 'primary',
		open_mode: 'channel',
		parent_id: '',
		name_scheme: 'ticket-{{ printf "%04d" .Ticket.Number }}',
		support_role_ids: [],
		ping_role_ids: [],
		ping_opener: false,
		required_role_ids: [],
		max_open_per_user: 0,
		cooldown_seconds: 0,
		form: [],
		welcome: {
			content: '{{ .User.Mention }}',
			use_embed: true,
			embed: {
				title: 'Ticket #{{ .Ticket.Number }}',
				description:
					'Thanks for reaching out, {{ .User.Mention }}. Describe your issue and a staff member will help you soon.',
				color: '#ff6363'
			}
		},
		transcript: { enabled: true, dm_opener: false },
		feedback: { enabled: false, prompt: 'How was your support experience?' },
		auto_close: { enabled: false, inactivity_minutes: 1440, warn_minutes: 60 },
		claim_enabled: true,
		on_open_automation: '',
		on_close_automation: ''
	};
}

export function defaultPanelConfig(): PanelConfig {
	return {
		content: '',
		embed: {
			title: 'Need help?',
			description:
				'Open a ticket and our team will be with you shortly. Pick the option that best fits your request below.',
			color: '#ff6363'
		},
		select_placeholder: 'Choose a ticket type',
		categories: [newCategory('General support')]
	};
}

export function newFormField(): FormField {
	return { id: shortId(), label: '', placeholder: '', style: 'short', required: true };
}

// ── Queue + analytics ──
export interface TicketRow {
	id: string;
	number: number;
	category_id: string;
	category_label: string;
	channel_id: string;
	is_thread: boolean;
	opener_id: string;
	subject: string;
	status: string;
	claimed_by: string;
	opened_at: string;
	closed_at: string | null;
	rating: number;
	transcript_url: string;
}

export interface TicketStats {
	open: number;
	closed: number;
	total: number;
	opened_7d: number;
	closed_7d: number;
	rated: number;
	avg_rating: number;
	avg_first_response_seconds: number;
	avg_resolution_seconds: number;
}

// TICKET_TEMPLATE_VARS drives the variable picker inside the message editors.
export const TICKET_TEMPLATE_VARS: TmplVar[] = [
	{ path: '.User.Mention', label: 'User.Mention', type: 'string', short: 'Mentions the ticket opener' },
	{ path: '.User.Username', label: 'User.Username', type: 'string', short: "Opener's username" },
	{ path: '.User.GlobalName', label: 'User.GlobalName', type: 'string', short: "Opener's display name" },
	{ path: '.Guild.Name', label: 'Guild.Name', type: 'string', short: 'Server name' },
	{ path: '.Channel.Mention', label: 'Channel.Mention', type: 'string', short: 'The ticket channel' },
	{ path: '.Ticket.Number', label: 'Ticket.Number', type: 'int', short: 'The ticket number' },
	{ path: '.Ticket.Subject', label: 'Ticket.Subject', type: 'string', short: 'The ticket subject' },
	{ path: '.Ticket.Category', label: 'Ticket.Category', type: 'string', short: 'The ticket category (type)' }
];
