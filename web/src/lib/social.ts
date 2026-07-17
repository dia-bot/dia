// Types for the social alerts feature, mirroring internal/api/social.go and
// internal/features/socialnotifications/spec.go. Field names must match the Go
// json tags exactly so the editor never emits JSONB the worker can't decode.

import type { TmplVar } from '$lib/commands/expr-meta';

// SOCIAL_TEMPLATE_VARS is the announcement template scope, kept in lockstep
// with the data map in socialnotifications.BuildAnnouncement. It seeds the
// expression picker wherever a social message is composed.
export const SOCIAL_TEMPLATE_VARS: TmplVar[] = [
	{ path: '.Account', label: 'Account', type: 'string', short: 'The followed account name' },
	{ path: '.AccountURL', label: 'AccountURL', type: 'string', short: "Link to the account's page" },
	{ path: '.Platform', label: 'Platform', type: 'string', short: 'The platform name (Twitch, YouTube, ...)' },
	{ path: '.Kind', label: 'Kind', type: 'string', short: 'What happened (live_start, live_end, new_video, new_post)' },
	{ path: '.Title', label: 'Title', type: 'string', short: 'The stream, video or post title' },
	{ path: '.URL', label: 'URL', type: 'string', short: 'Link to the stream, video or post' },
	{ path: '.Game', label: 'Game', type: 'string', short: 'The stream game or category ("" if none)' },
	{ path: '.Description', label: 'Description', type: 'string', short: 'A short excerpt or description ("" if none)' },
	{ path: '.Image', label: 'Image', type: 'string', short: 'Thumbnail image URL ("" if none)' }
];

// SOCIAL_KINDS labels each update kind for the editor. live_end announcements
// are opt-in (mirrors SubSpec.Announces on the Go side).
export const SOCIAL_KINDS: Record<string, { label: string; hint: string; defaultOn: boolean }> = {
	live_start: { label: 'Goes live', hint: 'When the account starts streaming.', defaultOn: true },
	live_end: { label: 'Stream ends', hint: 'When the stream ends. Off unless you turn it on.', defaultOn: false },
	new_video: { label: 'New upload', hint: 'When a new video is published.', defaultOn: true },
	new_post: { label: 'New post', hint: 'When a new post or feed entry is published.', defaultOn: true }
};

// SocialCapability is one provider tile: whether this deployment has the
// credentials to offer it (available) or it's locked (coming_soon).
export interface SocialCapability {
	provider: string;
	name: string;
	status: 'available' | 'coming_soon';
	input?: string;
	kinds?: string[];
}

// SocialEmbedField / SocialEmbed mirror cc.EmbedSpec — the exact shape the
// shared MessageEditor / EmbedBuilder produce in a step's spec.embeds.
export interface SocialEmbedField {
	name: string;
	value: string;
	inline?: boolean;
}

export interface SocialEmbed {
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
	fields?: SocialEmbedField[];
}

// SocialComponent / SocialComponentRow mirror cc.Component — the shape the
// shared MessageEditor writes into spec.components. Announcement buttons are
// either link buttons ("Watch stream") or action buttons wired to a saved
// automation via button_actions.
export interface SocialComponent {
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

export interface SocialComponentRow {
	components: SocialComponent[];
}

// SocialMessageSpec mirrors socialnotifications.MessageSpec: one composed
// announcement (content + embeds + button rows). Empty means the provider's
// default announcement (the legacy template + brand-embed path).
export interface SocialMessageSpec {
	content?: string;
	embeds?: SocialEmbed[];
	components?: SocialComponentRow[];
	// custom_id_suffix → saved automation id its click runs.
	button_actions?: Record<string, string>;
}

// SocialKindConfig mirrors socialnotifications.KindConfig: what happens for
// one update kind (live_start, live_end, new_video, new_post).
export interface SocialKindConfig {
	disabled?: boolean;
	message?: SocialMessageSpec;
}

// SocialSubSpec mirrors socialnotifications.SubSpec (the spec JSONB).
export interface SocialSubSpec {
	kinds?: Record<string, SocialKindConfig>;
}

export function emptySocialMessageSpec(): SocialMessageSpec {
	return { content: '', embeds: [], components: [], button_actions: {} };
}

// DEFAULT_KIND_LINES mirrors socialnotifications.defaultTemplate: the
// announcement line the bot sends when no custom message is stored.
const DEFAULT_KIND_LINES: Record<string, string> = {
	live_start:
		'🔴 **{{ .Account }}** is now live{{ if .Game }} playing **{{ .Game }}**{{ end }}{{ if .Title }}: {{ .Title }}{{ end }}',
	live_end: '⬛ **{{ .Account }}** just went offline. Thanks for watching!',
	new_video: '▶️ **{{ .Account }}** uploaded a new video: **{{ .Title }}**',
	new_post: '📣 **{{ .Account }}** posted{{ if .Title }}: {{ .Title }}{{ end }}'
};

// defaultSocialMessage builds the composed equivalent of the bot's default
// announcement for one kind: the standard line plus (when embed is on) the
// brand card the legacy path renders, so the editor opens showing exactly
// what the bot would send today, ready to be tweaked.
export function defaultSocialMessage(
	kind: string,
	opts: { platform: string; color: string; embed: boolean }
): SocialMessageSpec {
	const msg = emptySocialMessageSpec();
	msg.content = DEFAULT_KIND_LINES[kind] ?? DEFAULT_KIND_LINES.new_post;
	if (!opts.embed) {
		msg.content += '\n{{ .URL }}';
		return msg;
	}
	msg.embeds = [
		{
			author_name: '{{ .Account }}',
			author_url: '{{ .AccountURL }}',
			title: '{{ .Title }}',
			url: '{{ .URL }}',
			description: '{{ .Description }}',
			image_url: '{{ .Image }}',
			color: opts.color,
			footer_text: opts.platform,
			timestamp: true
		}
	];
	return msg;
}

// SocialSubscription is one followed account (snowflakes as strings).
export interface SocialSubscription {
	id: string;
	provider: string;
	account_id: string;
	account_name: string;
	account_url: string;
	channel_id: string;
	ping_role_id: string;
	template: string;
	embed: boolean;
	spec: SocialSubSpec;
	enabled: boolean;
	live: boolean;
	hook_status: string;
	last_error: string;
	created_at: number;
}

export interface SocialList {
	capabilities: SocialCapability[];
	subscriptions: SocialSubscription[];
	limit: number;
}

// The editable fields sent on create / update. spec omitted/undefined leaves
// the stored per-kind spec untouched.
export interface SocialSubInput {
	provider?: string;
	account?: string;
	channel_id?: string;
	ping_role_id?: string;
	template?: string;
	embed?: boolean;
	spec?: SocialSubSpec;
	enabled?: boolean;
}
