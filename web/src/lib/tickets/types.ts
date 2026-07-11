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

// Component / ComponentRow mirror cc.Component — the shape the shared
// MessageEditor writes into spec.components. Ticket buttons are either links
// or run the saved automation button_actions points their suffix at.
export interface TicketComponent {
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

export interface TicketComponentRow {
	components: TicketComponent[];
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

// MessageSpec mirrors tickets.MessageSpec: one fully-composed message (content
// + embeds + extra button rows, the same shape the shared MessageEditor
// produces). button_actions maps a composed button's custom_id_suffix to the
// saved automation it runs on click. The legacy {use_embed, embed} fields are
// accepted on load (normalizeMessageSpec folds them in) but never written.
export interface MessageSpec {
	content?: string;
	embeds?: TicketEmbed[];
	components?: TicketComponentRow[];
	button_actions?: Record<string, string>;
	// Legacy (read-only): pre-rework panels stored a single embed.
	use_embed?: boolean;
	embed?: TicketEmbed;
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
	message: MessageSpec;
	thanks_message?: string;
}

export interface AutoCloseConfig {
	enabled?: boolean;
	inactivity_minutes?: number;
	warn_minutes?: number;
	warn_message: MessageSpec;
}

// SystemButton restyles one built-in control button (empty = default look).
export interface SystemButton {
	label?: string;
	emoji?: string;
	style?: string;
	hide?: boolean;
}

export interface ControlButtons {
	claim: SystemButton;
	close: SystemButton;
	reopen: SystemButton;
	delete: SystemButton;
	transcript: SystemButton;
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
	closed: MessageSpec;
	close_request: MessageSpec;
	buttons: ControlButtons;
	transcript: TranscriptConfig;
	feedback: FeedbackConfig;
	auto_close: AutoCloseConfig;
	claim_enabled?: boolean;
	on_open_automation?: string;
	on_close_automation?: string;
}

export interface PanelConfig {
	content?: string;
	embeds?: TicketEmbed[];
	select_placeholder?: string;
	categories: CategoryConfig[];
	// Legacy (read-only): pre-rework panels stored a single embed.
	embed?: TicketEmbed;
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

export function emptyMessageSpec(): MessageSpec {
	return { content: '', embeds: [], components: [], button_actions: {} };
}

export function emptySystemButton(): SystemButton {
	return { label: '', emoji: '', style: '', hide: false };
}

export function defaultControlButtons(): ControlButtons {
	return {
		claim: emptySystemButton(),
		close: emptySystemButton(),
		reopen: emptySystemButton(),
		delete: emptySystemButton(),
		transcript: emptySystemButton()
	};
}

// normalizeMessageSpec folds the legacy {use_embed, embed} shape into embeds
// (mirrors MessageSpec.UnmarshalJSON on the Go side) and guarantees arrays.
export function normalizeMessageSpec(m: Partial<MessageSpec> | undefined): MessageSpec {
	const spec: MessageSpec = {
		content: m?.content ?? '',
		embeds: (m?.embeds ?? []).map((e) => ({ ...e })),
		components: (m?.components ?? []).map((r) => ({
			components: (r.components ?? []).map((c) => ({ ...c }))
		})),
		button_actions: { ...(m?.button_actions ?? {}) }
	};
	if (spec.embeds!.length === 0 && m?.use_embed && m.embed && Object.keys(m.embed).length > 0) {
		spec.embeds = [{ ...m.embed }];
	}
	return spec;
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
			embeds: [
				{
					title: 'Ticket #{{ .Ticket.Number }}',
					description:
						'Thanks for reaching out, {{ .User.Mention }}. Describe your issue and a staff member will help you soon.',
					color: '#ff6363'
				}
			],
			components: [],
			button_actions: {}
		},
		closed: emptyMessageSpec(),
		close_request: emptyMessageSpec(),
		buttons: defaultControlButtons(),
		transcript: { enabled: true, dm_opener: false },
		feedback: {
			enabled: false,
			prompt: 'How was your support experience?',
			message: emptyMessageSpec(),
			thanks_message: ''
		},
		auto_close: {
			enabled: false,
			inactivity_minutes: 1440,
			warn_minutes: 60,
			warn_message: emptyMessageSpec()
		},
		claim_enabled: true,
		on_open_automation: '',
		on_close_automation: ''
	};
}

export function defaultPanelConfig(): PanelConfig {
	return {
		content: '',
		embeds: [
			{
				title: 'Need help?',
				description:
					'Open a ticket and our team will be with you shortly. Pick the option that best fits your request below.',
				color: '#ff6363'
			}
		],
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
// Kept in lockstep with the scope struct in tickets/template.go.
export const TICKET_TEMPLATE_VARS: TmplVar[] = [
	{ path: '.User.Mention', label: 'User.Mention', type: 'string', short: 'Mentions the ticket opener' },
	{ path: '.User.Username', label: 'User.Username', type: 'string', short: "Opener's username" },
	{ path: '.User.GlobalName', label: 'User.GlobalName', type: 'string', short: "Opener's display name" },
	{ path: '.Actor.Mention', label: 'Actor.Mention', type: 'string', short: 'Whoever performed the action (closer / requester)' },
	{ path: '.Guild.Name', label: 'Guild.Name', type: 'string', short: 'Server name' },
	{ path: '.Channel.Mention', label: 'Channel.Mention', type: 'string', short: 'The ticket channel' },
	{ path: '.Ticket.Number', label: 'Ticket.Number', type: 'int', short: 'The ticket number' },
	{ path: '.Ticket.Subject', label: 'Ticket.Subject', type: 'string', short: 'The ticket subject' },
	{ path: '.Ticket.Category', label: 'Ticket.Category', type: 'string', short: 'The ticket category (type)' },
	{ path: '.Ticket.Claimer', label: 'Ticket.Claimer', type: 'string', short: 'Mention of the claiming staff member' },
	{ path: '.Ticket.Closer', label: 'Ticket.Closer', type: 'string', short: 'Mention of whoever closed the ticket' },
	{ path: '.Ticket.Reason', label: 'Ticket.Reason', type: 'string', short: 'The close / close-request reason' },
	{ path: '.Ticket.Rating', label: 'Ticket.Rating', type: 'int', short: 'The rating 1-5 (feedback surfaces)' },
	{ path: '.Ticket.Deadline', label: 'Ticket.Deadline', type: 'string', short: 'When a close request auto-accepts (relative timestamp)' }
];
