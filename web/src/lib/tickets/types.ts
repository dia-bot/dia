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
	messages: SystemMessages;
}

// SystemMessages overrides the bot's short system replies (mirrors Go
// tickets.SystemMessages). Every value is a Go template rendered against the
// ticket scope; empty keeps the built-in text.
export interface SystemMessages {
	blacklisted: string;
	missing_role: string;
	server_limit: string;
	category_limit: string;
	cooldown: string;
	opened: string;
	open_failed: string;
	closed: string;
	reopened: string;
	close_accepted: string;
	close_denied: string;
}

export function defaultSystemMessages(): SystemMessages {
	return {
		blacklisted: '',
		missing_role: '',
		server_limit: '',
		category_limit: '',
		cooldown: '',
		opened: '',
		open_failed: '',
		closed: '',
		reopened: '',
		close_accepted: '',
		close_denied: ''
	};
}

// SYSTEM_MESSAGE_META drives the settings UI: one entry per overridable reply,
// with the built-in text as the placeholder so admins see what they replace.
export const SYSTEM_MESSAGE_META: {
	key: keyof SystemMessages;
	label: string;
	hint: string;
	placeholder: string;
}[] = [
	{
		key: 'blacklisted',
		label: 'Blacklisted member',
		hint: 'Shown when a blacklisted member tries to open a ticket',
		placeholder: "You're not allowed to open tickets on this server."
	},
	{
		key: 'missing_role',
		label: 'Missing required role',
		hint: 'Shown when the ticket type requires a role the member lacks',
		placeholder: "You don't have the role needed to open this type of ticket."
	},
	{
		key: 'server_limit',
		label: 'Server-wide open limit reached',
		hint: 'Shown when the member hit the max open tickets across all types',
		placeholder: "You've reached the maximum number of open tickets. Please close one before opening another."
	},
	{
		key: 'category_limit',
		label: 'Per-type open limit reached',
		hint: 'Shown when the member already has an open ticket of this type',
		placeholder: 'You already have an open ticket of this type.'
	},
	{
		key: 'cooldown',
		label: 'Opening too fast',
		hint: 'Shown while the per-type cooldown is active',
		placeholder: "You're opening tickets too quickly. Please wait a moment and try again."
	},
	{
		key: 'opened',
		label: 'Ticket opened',
		hint: 'Confirmation after a successful open; {{ .Ticket.Channel }} is the new channel',
		placeholder: 'Opened your ticket: {{ .Ticket.Channel }}'
	},
	{
		key: 'open_failed',
		label: 'Open failed',
		hint: 'Shown when creating the ticket or its channel fails',
		placeholder: 'Something went wrong opening your ticket. Please try again.'
	},
	{
		key: 'closed',
		label: 'Ticket closed',
		hint: 'Confirmation after the close dialog; {{ .Actor.Mention }} is the closer',
		placeholder: 'Ticket closed.'
	},
	{
		key: 'reopened',
		label: 'Reopened card',
		hint: 'Body of the in-channel card after a reopen',
		placeholder: 'Reopened by {{ .Actor.Mention }}.'
	},
	{
		key: 'close_accepted',
		label: 'Close request accepted card',
		hint: 'Body of the in-channel card when a close request is accepted',
		placeholder: 'Accepted by {{ .Actor.Mention }}.'
	},
	{
		key: 'close_denied',
		label: 'Close request declined card',
		hint: 'Body of the in-channel card when the opener keeps the ticket open',
		placeholder: '{{ .Actor.Mention }} kept the ticket open.'
	}
];

export function defaultTicketsConfig(): TicketsConfig {
	return {
		staff_role_ids: [],
		log_channel: '',
		transcript_channel: '',
		blacklist_role_ids: [],
		blacklist_user_ids: [],
		max_open_per_user: 3,
		default_parent_id: '',
		name_prefix: 'ticket',
		messages: defaultSystemMessages()
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
	// button_bindings maps a composed button's custom_id_suffix to the SYSTEM
	// action it performs on this surface (welcome: claim/close; closed:
	// reopen/delete/transcript; close request: accept/deny). A binding wins over
	// button_actions for the same suffix.
	button_bindings?: Record<string, string>;
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
	// Automation hooks: a saved automation launched at each lifecycle moment
	// (mirrors the Go On*Automation fields; empty = no hook).
	on_open_automation?: string;
	on_claim_automation?: string;
	on_close_request_automation?: string;
	on_reopen_automation?: string;
	on_close_automation?: string;
	on_rate_automation?: string;
}

export interface PanelConfig {
	content?: string;
	embeds?: TicketEmbed[];
	select_placeholder?: string;
	categories: CategoryConfig[];
	// User-composed panel button rows: a button either opens the ticket type
	// button_bindings maps its suffix to, runs the automation button_actions
	// maps it to, or opens a link. With none composed the classic generated
	// per-type buttons (or dropdown) are used. Always present after
	// normalizePanelConfig / defaultPanelConfig so the editor can bind into them.
	components: TicketComponentRow[];
	button_bindings: Record<string, string>; // suffix → category id
	button_actions: Record<string, string>; // suffix → automation id
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
	return { content: '', embeds: [], components: [], button_actions: {}, button_bindings: {} };
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
		button_actions: { ...(m?.button_actions ?? {}) },
		button_bindings: { ...(m?.button_bindings ?? {}) }
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
			// Real, visible Claim/Close buttons in the composition (edited in the
			// preview like any other button); button_bindings routes their clicks
			// to the native handlers. Mirrors Go DefaultCategory.
			components: [
				{
					components: [
						{ type: 'button', style: 'success', label: 'Claim', emoji: '🙋', custom_id_suffix: 'claim' },
						{ type: 'button', style: 'danger', label: 'Close', emoji: '🔒', custom_id_suffix: 'close' }
					]
				}
			],
			button_actions: {},
			button_bindings: { claim: 'claim', close: 'close' }
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
		on_claim_automation: '',
		on_close_request_automation: '',
		on_reopen_automation: '',
		on_close_automation: '',
		on_rate_automation: ''
	};
}

export function defaultPanelConfig(): PanelConfig {
	const cat = newCategory('General support');
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
		categories: [cat],
		// A real, visible open button in the composition, bound to the category
		// (mirrors Go DefaultPanelConfig).
		components: [
			{
				components: [
					{ type: 'button', style: 'primary', label: cat.label, emoji: '🎫', custom_id_suffix: cat.id }
				]
			}
		],
		button_bindings: { [cat.id]: cat.id },
		button_actions: {}
	};
}

export function newFormField(): FormField {
	return { id: shortId(), label: '', placeholder: '', style: 'short', required: true };
}

// mergeButtons fills in any missing system-button overrides.
export function mergeButtons(b: Partial<ControlButtons> | undefined): ControlButtons {
	const d = defaultControlButtons();
	return {
		claim: { ...d.claim, ...(b?.claim ?? {}) },
		close: { ...d.close, ...(b?.close ?? {}) },
		reopen: { ...d.reopen, ...(b?.reopen ?? {}) },
		delete: { ...d.delete, ...(b?.delete ?? {}) },
		transcript: { ...d.transcript, ...(b?.transcript ?? {}) }
	};
}

// normalizeCategory upgrades a stored category to the current shape (folding
// the legacy single-embed welcome into embeds, mirroring the Go decoder).
export function normalizeCategory(c: Partial<CategoryConfig>): CategoryConfig {
	const base = newCategory();
	return {
		...base,
		...c,
		welcome: normalizeMessageSpec(c.welcome ?? base.welcome),
		closed: normalizeMessageSpec(c.closed),
		close_request: normalizeMessageSpec(c.close_request),
		buttons: mergeButtons(c.buttons),
		transcript: { ...base.transcript, ...(c.transcript ?? {}) },
		feedback: { ...base.feedback, ...(c.feedback ?? {}), message: normalizeMessageSpec(c.feedback?.message) },
		auto_close: {
			...base.auto_close,
			...(c.auto_close ?? {}),
			warn_message: normalizeMessageSpec(c.auto_close?.warn_message)
		},
		form: c.form ?? []
	};
}

// normalizePanelConfig upgrades a stored panel config (legacy single embed →
// embeds) and normalizes every category.
export function normalizePanelConfig(pc: Partial<PanelConfig> | undefined): PanelConfig {
	const d = defaultPanelConfig();
	let embeds = (pc?.embeds ?? []).map((e) => ({ ...e }));
	if (embeds.length === 0 && pc?.embed && Object.keys(pc.embed).length > 0) {
		embeds = [{ ...pc.embed }]; // legacy single-embed panel
	}
	if (!pc) embeds = d.embeds ?? [];
	return {
		content: pc?.content ?? '',
		embeds,
		select_placeholder: pc?.select_placeholder ?? d.select_placeholder,
		categories: (pc?.categories ?? []).map(normalizeCategory),
		components: (pc?.components ?? []).map((r) => ({
			components: (r.components ?? []).map((c) => ({ ...c }))
		})),
		button_bindings: { ...(pc?.button_bindings ?? {}) },
		button_actions: { ...(pc?.button_actions ?? {}) }
	};
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
