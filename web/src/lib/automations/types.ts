// Type-only mirror of internal/features/automations/{automations,triggers}.go
// and the automation API. Kept in lockstep with the Go side so the editor never
// produces JSONB the runtime won't understand. The step program reuses the
// custom-command Definition/Step types verbatim.

import type { Definition } from '$lib/commands/types';
import type { TmplVar } from '$lib/commands/expr-meta';

export type AutomationStatus = 'draft' | 'published' | 'archived';

export type TriggerFilter =
	| 'channels'
	| 'roles'
	| 'ignore_bots'
	| 'keywords'
	| 'emojis'
	| 'role'
	| 'social_accounts'
	| 'social_kinds'
	| 'cooldown';

export interface TriggerConfig {
	channels?: string[];
	ignore_channels?: string[];
	roles?: string[];
	ignore_roles?: string[];
	ignore_bots?: boolean;
	keywords?: string[];
	match_mode?: 'contains' | 'equals' | 'word';
	emojis?: string[];
	role?: string;
	// social_update scoping: followed-account subscription ids and update kinds.
	subscriptions?: string[];
	kinds?: string[];
	cooldown?: { scope: 'user' | 'channel' | 'guild'; seconds: number };
}

export interface TriggerKindMeta {
	key: string;
	label: string;
	description: string;
	category: string;
	event: string;
	actor: string;
	hasChannel: boolean;
	filters: TriggerFilter[];
	// eventVars are the `.Event.*` fields this trigger exposes, shown in the
	// expression picker on top of the always-available User/Guild/Channel context.
	eventVars: TmplVar[];
}

export const TRIGGER_CATEGORIES: { id: string; label: string }[] = [
	{ id: 'members', label: 'Members' },
	{ id: 'roles', label: 'Roles' },
	{ id: 'messages', label: 'Messages' },
	{ id: 'reactions', label: 'Reactions' },
	{ id: 'voice', label: 'Voice' },
	{ id: 'moderation', label: 'Moderation' },
	{ id: 'tickets', label: 'Tickets' },
	{ id: 'channels', label: 'Channels & threads' },
	{ id: 'giveaways', label: 'Giveaways' },
	{ id: 'social', label: 'Social' }
];

const v = (path: string, type: string, short: string): TmplVar => ({
	path,
	label: path.replace(/^\./, ''),
	type,
	short
});

const MEMBER_EVENT_VARS: TmplVar[] = [
	v('.Event.roles', 'list', "The member's current role ids"),
	v('.Event.added_roles', 'list', 'Role ids just added'),
	v('.Event.removed_roles', 'list', 'Role ids just removed'),
	v('.Event.nick', 'string', "The member's nickname"),
	v('.Event.premium_since', 'string', 'When they started boosting ("" if not)'),
	v('.Event.boosting', 'bool', 'True if currently boosting')
];

const LEVELUP_EVENT_VARS: TmplVar[] = [
	v('.Event.level', 'int', 'The level the member just reached'),
	v('.Event.new_level', 'int', 'The new level (same as level)'),
	v('.Event.xp', 'int', "The member's total XP"),
	v('.Event.rank', 'int', "The member's leaderboard position"),
	v('.Event.channel_id', 'snowflake', 'The channel they leveled up in')
];

const RR_PICK_EVENT_VARS: TmplVar[] = [
	v('.Event.menu_id', 'string', 'The reaction-role menu id (decimal string)'),
	v('.Event.menu_title', 'string', "The menu's title"),
	v('.Event.mode', 'string', 'The menu mode (toggle, unique or verify)'),
	v('.Event.values', 'list', 'Role ids the member picked (before the mode was applied)'),
	v('.Event.added', 'list', 'Role ids just granted by this pick'),
	v('.Event.removed', 'list', 'Role ids just removed by this pick')
];

const MESSAGE_EVENT_VARS: TmplVar[] = [
	v('.Event.content', 'string', 'The message content'),
	v('.Event.message.id', 'snowflake', 'The message id'),
	v('.Event.message.channel_id', 'snowflake', 'The message channel id'),
	v('.Event.message.attachment_count', 'int', 'Number of attachments'),
	v('.Event.message.mention_everyone', 'bool', 'Whether it pinged @everyone')
];

const REACTION_EVENT_VARS: TmplVar[] = [
	v('.Event.emoji', 'string', 'The reaction emoji (glyph or <:name:id>)'),
	v('.Event.emoji_name', 'string', 'The emoji name'),
	v('.Event.emoji_id', 'snowflake', 'The custom emoji id ("" for unicode)'),
	v('.Event.message.id', 'snowflake', 'The reacted message id'),
	v('.Event.message.channel_id', 'snowflake', 'The message channel id')
];

const VOICE_EVENT_VARS: TmplVar[] = [
	v('.Event.channel_id', 'snowflake', 'The new voice channel id ("" on leave)'),
	v('.Event.old_channel_id', 'snowflake', 'The previous voice channel id'),
	v('.Event.self_mute', 'bool', 'Self-muted'),
	v('.Event.self_deaf', 'bool', 'Self-deafened'),
	v('.Event.self_video', 'bool', 'Camera on'),
	v('.Event.self_stream', 'bool', 'Streaming (Go Live)')
];

const AUTOMOD_EVENT_VARS: TmplVar[] = [
	v('.Event.rule_name', 'string', 'The automod rule that fired'),
	v('.Event.rule_id', 'string', 'The rule id'),
	v('.Event.trigger_type', 'string', 'The trigger that matched (keyword, spam, ...)'),
	v('.Event.reason', 'string', 'Human description of the hit'),
	v('.Event.points', 'int', 'Points added by this hit'),
	v('.Event.total_points', 'int', "The member's active infraction total after"),
	v('.Event.escalated', 'string', 'Escalation action fired ("" if none)'),
	v('.Event.content', 'string', 'The offending message content (truncated)'),
	v('.Event.message_id', 'snowflake', 'The offending message id ("" if none)'),
	v('.Event.channel_id', 'snowflake', 'The channel it happened in ("" if none)'),
	v('.Event.actions', 'list', 'Action types the rule applied, in order')
];

const TICKET_EVENT_VARS: TmplVar[] = [
	v('.Event.ticket_id', 'string', 'The ticket id'),
	v('.Event.number', 'int', 'The per-server ticket number'),
	v('.Event.category_label', 'string', 'The ticket category (type)'),
	v('.Event.category_id', 'string', 'The ticket category key'),
	v('.Event.subject', 'string', 'The ticket subject (first form answer)'),
	v('.Event.channel_id', 'snowflake', 'The ticket channel id'),
	v('.Event.panel_id', 'string', 'The panel the ticket was opened from'),
	v('.Event.actor_id', 'snowflake', 'Who performed the action (claimer/closer/requester/reopener/rater)'),
	v('.Event.claimed_by', 'snowflake', 'Who claimed the ticket ("" if none)'),
	v('.Event.closed_by', 'snowflake', 'Who closed the ticket ("" if none)'),
	v('.Event.reason', 'string', 'The close reason ("" if none)'),
	v('.Event.rating', 'int', 'The rating 1-5 (0 if unrated)')
];

const CHANNEL_EVENT_VARS: TmplVar[] = [
	v('.Event.channel.id', 'snowflake', 'The channel id'),
	v('.Event.channel.name', 'string', 'The channel name'),
	v('.Event.channel.type', 'int', 'The channel type'),
	v('.Event.channel.parent_id', 'snowflake', 'The parent category id'),
	v('.Event.channel.topic', 'string', 'The channel topic')
];

const VERIFY_PASSED_VARS: TmplVar[] = [
	v('.Event.mode', 'string', 'How they verified ("button" or "captcha")'),
	v('.Event.channel_id', 'snowflake', 'The gate channel id')
];

const VERIFY_FAILED_VARS: TmplVar[] = [
	v('.Event.reason', 'string', 'Why it failed ("failed_captcha" or "timed_out")'),
	v('.Event.kicked', 'bool', 'Whether the member was removed')
];

const RAID_EVENT_VARS: TmplVar[] = [
	v('.Event.active', 'bool', 'True when raid mode is entered, false when lifted'),
	v('.Event.joins', 'int', 'Joins counted in the window (on trip)'),
	v('.Event.threshold', 'int', 'The configured trip threshold'),
	v('.Event.window', 'int', 'The rolling window, seconds'),
	v('.Event.action', 'string', 'Action applied to joiners (kick/ban/timeout)')
];

const MODACTION_EVENT_VARS: TmplVar[] = [
	v('.Event.action', 'string', 'The action (ban/kick/timeout/untimeout/unban/warn/note)'),
	v('.Event.reason', 'string', 'The moderator-supplied reason'),
	v('.Event.moderator_id', 'snowflake', 'The moderator who ran the command'),
	v('.Event.moderator_name', 'string', "The moderator's name"),
	v('.Event.case_number', 'int', 'The mod-log case number'),
	v('.Event.duration_seconds', 'int', 'Timeout/temp-ban duration in seconds (0 if none)')
];

const GIVEAWAY_EVENT_VARS: TmplVar[] = [
	v('.Event.giveaway_id', 'string', 'The giveaway id'),
	v('.Event.prize', 'string', 'The prize'),
	v('.Event.host_id', 'snowflake', 'The host who started it'),
	v('.Event.winner_count', 'int', 'Number of winners drawn'),
	v('.Event.winner_ids', 'list', 'Winner user ids (loop these to reward each)'),
	v('.Event.entry_count', 'int', 'Number of distinct entrants'),
	v('.Event.rerolled', 'bool', 'True when this was a reroll'),
	v('.Event.message_id', 'snowflake', 'The giveaway message id'),
	v('.Event.channel_id', 'snowflake', 'The channel the giveaway lives in')
];

const GIVEAWAY_ENTRY_EVENT_VARS: TmplVar[] = [
	v('.Event.giveaway_id', 'string', 'The giveaway id'),
	v('.Event.prize', 'string', 'The prize'),
	v('.Event.host_id', 'snowflake', 'The host who started it'),
	v('.Event.outcome', 'string', 'entered, left, denied, or blocked'),
	v('.Event.entries', 'int', 'The member’s weighted tickets (on a successful entry)'),
	v('.Event.reason', 'string', 'Why entry was denied (outcome=="denied")'),
	v('.Event.entry_count', 'int', 'Distinct entrants after this click'),
	v('.Event.message_id', 'snowflake', 'The giveaway message id'),
	v('.Event.channel_id', 'snowflake', 'The channel the giveaway lives in')
];

// Mirrors the eventMap set in runtime.go's TypeSocialUpdate case. Keep the key
// names exact: the runtime emits `account` (not account_name) and `subscription`
// (not subscription_id), and does not surface a thumbnail.
const SOCIAL_EVENT_VARS: TmplVar[] = [
	v('.Event.provider', 'string', 'The platform (twitch, youtube, kick, bluesky, rss)'),
	v('.Event.kind', 'string', 'What happened (live_start, live_end, new_video, new_post)'),
	v('.Event.account', 'string', 'The followed account name'),
	v('.Event.account_url', 'string', "Link to the account's page"),
	v('.Event.title', 'string', 'The stream, video or post title'),
	v('.Event.url', 'string', 'Link to the stream, video or post'),
	v('.Event.description', 'string', 'A short excerpt or description ("" if none)'),
	v('.Event.category', 'string', 'The stream game or category ("" if none)'),
	v('.Event.account_id', 'string', 'The upstream account id (or feed URL for RSS)'),
	v('.Event.item_id', 'string', 'The upstream item id (dedupe key)'),
	v('.Event.started_at', 'string', 'When a stream started (RFC 3339, "" if not a stream)'),
	v('.Event.subscription', 'int', 'The Dia subscription id that matched')
];

export const TRIGGERS: TriggerKindMeta[] = [
	{
		key: 'member_join',
		label: 'Member joins',
		description: 'A member joins the server.',
		category: 'members',
		event: 'GUILD_MEMBER_ADD',
		actor: 'the member who joined',
		hasChannel: false,
		filters: ['ignore_bots', 'cooldown'],
		eventVars: [v('.Event.member_count', 'int', 'Server member count'), v('.Event.pending', 'bool', 'Pending membership screening')]
	},
	{
		key: 'member_leave',
		label: 'Member leaves',
		description: 'A member leaves, is kicked, or is banned.',
		category: 'members',
		event: 'GUILD_MEMBER_REMOVE',
		actor: 'the member who left',
		hasChannel: false,
		filters: ['cooldown'],
		eventVars: [v('.Event.member_count', 'int', 'Server member count')]
	},
	{
		key: 'member_update',
		label: 'Member updated',
		description: "A member's roles, nickname or boost status changes.",
		category: 'members',
		event: 'GUILD_MEMBER_UPDATE',
		actor: 'the updated member',
		hasChannel: false,
		filters: ['cooldown'],
		eventVars: MEMBER_EVENT_VARS
	},
	{
		key: 'level_up',
		label: 'Member levels up',
		description: 'A member reaches a new level.',
		category: 'members',
		event: 'LEVEL_UP',
		actor: 'the member who leveled up',
		hasChannel: true,
		filters: ['channels', 'cooldown'],
		eventVars: LEVELUP_EVENT_VARS
	},
	{
		key: 'verification_passed',
		label: 'Member verified',
		description: 'A member passes verification (button or captcha).',
		category: 'members',
		event: 'VERIFICATION_PASSED',
		actor: 'the verified member',
		hasChannel: false,
		filters: ['cooldown'],
		eventVars: VERIFY_PASSED_VARS
	},
	{
		key: 'verification_failed',
		label: 'Verification failed',
		description: 'A member fails the captcha, or is removed for not verifying in time.',
		category: 'members',
		event: 'VERIFICATION_FAILED',
		actor: 'the member who failed',
		hasChannel: false,
		filters: ['cooldown'],
		eventVars: VERIFY_FAILED_VARS
	},
	{
		key: 'role_added',
		label: 'Role added',
		description: 'A specific role is granted (watch the Server Booster role to catch boosts).',
		category: 'roles',
		event: 'GUILD_MEMBER_UPDATE',
		actor: 'the member who got the role',
		hasChannel: false,
		filters: ['role', 'cooldown'],
		eventVars: MEMBER_EVENT_VARS
	},
	{
		key: 'role_removed',
		label: 'Role removed',
		description: 'A specific role is removed from a member.',
		category: 'roles',
		event: 'GUILD_MEMBER_UPDATE',
		actor: 'the member who lost the role',
		hasChannel: false,
		filters: ['role', 'cooldown'],
		eventVars: MEMBER_EVENT_VARS
	},
	{
		key: 'reaction_role_pick',
		label: 'Reaction role picked',
		description: 'A member picks roles from a reaction-role menu.',
		category: 'roles',
		event: 'REACTION_ROLE_PICK',
		actor: 'the member who picked',
		hasChannel: true,
		filters: ['channels', 'cooldown'],
		eventVars: RR_PICK_EVENT_VARS
	},
	{
		key: 'message_create',
		label: 'Message sent',
		description: 'A message is posted in the server.',
		category: 'messages',
		event: 'MESSAGE_CREATE',
		actor: 'the message author',
		hasChannel: true,
		filters: ['channels', 'roles', 'ignore_bots', 'keywords', 'cooldown'],
		eventVars: MESSAGE_EVENT_VARS
	},
	{
		key: 'message_edit',
		label: 'Message edited',
		description: 'A message is edited.',
		category: 'messages',
		event: 'MESSAGE_UPDATE',
		actor: 'the message author',
		hasChannel: true,
		filters: ['channels', 'ignore_bots', 'keywords'],
		eventVars: MESSAGE_EVENT_VARS
	},
	{
		key: 'message_delete',
		label: 'Message deleted',
		description: 'A message is deleted.',
		category: 'messages',
		event: 'MESSAGE_DELETE',
		actor: '(no actor)',
		hasChannel: true,
		filters: ['channels'],
		eventVars: [
			v('.Event.message.id', 'snowflake', 'The deleted message id'),
			v('.Event.message.channel_id', 'snowflake', 'The message channel id')
		]
	},
	{
		key: 'reaction_add',
		label: 'Reaction added',
		description: 'Someone reacts to a message.',
		category: 'reactions',
		event: 'MESSAGE_REACTION_ADD',
		actor: 'the member who reacted',
		hasChannel: true,
		filters: ['channels', 'emojis', 'ignore_bots', 'cooldown'],
		eventVars: REACTION_EVENT_VARS
	},
	{
		key: 'reaction_remove',
		label: 'Reaction removed',
		description: 'Someone removes a reaction.',
		category: 'reactions',
		event: 'MESSAGE_REACTION_REMOVE',
		actor: 'the member who un-reacted',
		hasChannel: true,
		filters: ['channels', 'emojis'],
		eventVars: REACTION_EVENT_VARS
	},
	{
		key: 'voice_join',
		label: 'Joins voice',
		description: 'A member joins a voice channel.',
		category: 'voice',
		event: 'VOICE_STATE_UPDATE',
		actor: 'the member',
		hasChannel: true,
		filters: ['channels', 'ignore_bots', 'cooldown'],
		eventVars: VOICE_EVENT_VARS
	},
	{
		key: 'voice_leave',
		label: 'Leaves voice',
		description: 'A member leaves a voice channel.',
		category: 'voice',
		event: 'VOICE_STATE_UPDATE',
		actor: 'the member',
		hasChannel: true,
		filters: ['channels', 'cooldown'],
		eventVars: VOICE_EVENT_VARS
	},
	{
		key: 'voice_move',
		label: 'Switches voice channel',
		description: 'A member moves between voice channels.',
		category: 'voice',
		event: 'VOICE_STATE_UPDATE',
		actor: 'the member',
		hasChannel: true,
		filters: ['cooldown'],
		eventVars: VOICE_EVENT_VARS
	},
	{
		key: 'ban_add',
		label: 'Member banned',
		description: 'A user is banned from the server.',
		category: 'moderation',
		event: 'GUILD_BAN_ADD',
		actor: 'the banned user',
		hasChannel: false,
		filters: ['cooldown'],
		eventVars: []
	},
	{
		key: 'ban_remove',
		label: 'Member unbanned',
		description: 'A user is unbanned.',
		category: 'moderation',
		event: 'GUILD_BAN_REMOVE',
		actor: 'the unbanned user',
		hasChannel: false,
		filters: ['cooldown'],
		eventVars: []
	},
	{
		key: 'automod_action',
		label: 'Automod action taken',
		description: 'An automod rule fires on a member (keyword, spam, escalation, and more).',
		category: 'moderation',
		event: 'AUTOMOD_ACTION',
		actor: 'the flagged member',
		hasChannel: true,
		filters: ['ignore_bots', 'cooldown'],
		eventVars: AUTOMOD_EVENT_VARS
	},
	{
		key: 'moderation_action',
		label: 'Moderation action taken',
		description: 'A moderator runs /ban, /kick, /timeout, /warn or /note.',
		category: 'moderation',
		event: 'MODERATION_ACTION',
		actor: 'the actioned member',
		hasChannel: false,
		filters: ['cooldown'],
		eventVars: MODACTION_EVENT_VARS
	},
	{
		key: 'raid_alert',
		label: 'Anti-raid mode changes',
		description: 'The server enters or leaves anti-raid mode (branch on .Event.active).',
		category: 'moderation',
		event: 'RAID_ALERT',
		actor: '(no actor)',
		hasChannel: false,
		filters: ['cooldown'],
		eventVars: RAID_EVENT_VARS
	},
	{
		key: 'ticket_opened',
		label: 'Ticket opened',
		description: 'A member opens a support ticket.',
		category: 'tickets',
		event: 'TICKET_OPENED',
		actor: 'the member who opened the ticket',
		hasChannel: true,
		filters: ['cooldown'],
		eventVars: TICKET_EVENT_VARS
	},
	{
		key: 'ticket_claimed',
		label: 'Ticket claimed',
		description: 'A staff member claims a ticket.',
		category: 'tickets',
		event: 'TICKET_CLAIMED',
		actor: 'the ticket opener',
		hasChannel: true,
		filters: ['cooldown'],
		eventVars: TICKET_EVENT_VARS
	},
	{
		key: 'ticket_closed',
		label: 'Ticket closed',
		description: 'A ticket is closed (by staff, the opener, or auto-close).',
		category: 'tickets',
		event: 'TICKET_CLOSED',
		actor: 'the ticket opener',
		hasChannel: true,
		filters: ['cooldown'],
		eventVars: TICKET_EVENT_VARS
	},
	{
		key: 'ticket_close_requested',
		label: 'Ticket close requested',
		description: 'Staff ask the opener to confirm closing a ticket (.Event.actor_id is the requester).',
		category: 'tickets',
		event: 'TICKET_CLOSE_REQUESTED',
		actor: 'the ticket opener',
		hasChannel: true,
		filters: ['cooldown'],
		eventVars: TICKET_EVENT_VARS
	},
	{
		key: 'ticket_reopened',
		label: 'Ticket reopened',
		description: 'Staff reopen a closed ticket (.Event.actor_id is the reopener).',
		category: 'tickets',
		event: 'TICKET_REOPENED',
		actor: 'the ticket opener',
		hasChannel: true,
		filters: ['cooldown'],
		eventVars: TICKET_EVENT_VARS
	},
	{
		key: 'ticket_rated',
		label: 'Ticket rated',
		description: 'A member rates their closed ticket (branch on .Event.rating).',
		category: 'tickets',
		event: 'TICKET_RATED',
		actor: 'the ticket opener',
		hasChannel: false,
		filters: ['cooldown'],
		eventVars: TICKET_EVENT_VARS
	},
	{
		key: 'channel_create',
		label: 'Channel created',
		description: 'A channel is created.',
		category: 'channels',
		event: 'CHANNEL_CREATE',
		actor: '(no actor)',
		hasChannel: false,
		filters: [],
		eventVars: CHANNEL_EVENT_VARS
	},
	{
		key: 'channel_delete',
		label: 'Channel deleted',
		description: 'A channel is deleted.',
		category: 'channels',
		event: 'CHANNEL_DELETE',
		actor: '(no actor)',
		hasChannel: false,
		filters: [],
		eventVars: CHANNEL_EVENT_VARS
	},
	{
		key: 'thread_create',
		label: 'Thread created',
		description: 'A thread is created.',
		category: 'channels',
		event: 'THREAD_CREATE',
		actor: '(no actor)',
		hasChannel: true,
		filters: [],
		eventVars: CHANNEL_EVENT_VARS
	},
	{
		key: 'giveaway_ended',
		label: 'Giveaway ends',
		description:
			'A giveaway is drawn (natural end, manual end, or reroll). .User is the first winner; loop .Event.winner_ids for all winners.',
		category: 'giveaways',
		event: 'GIVEAWAY_ENDED',
		actor: 'the first winner (if any)',
		hasChannel: true,
		filters: ['channels', 'cooldown'],
		eventVars: GIVEAWAY_EVENT_VARS
	},
	{
		key: 'giveaway_entry',
		label: 'Giveaway entered',
		description:
			"A member clicks a giveaway's Enter button. Branch on .Event.outcome (entered, left, denied, blocked). .User is the member who clicked.",
		category: 'giveaways',
		event: 'GIVEAWAY_ENTERED',
		actor: 'the member who clicked Enter',
		hasChannel: true,
		filters: ['channels', 'ignore_bots', 'cooldown'],
		eventVars: GIVEAWAY_ENTRY_EVENT_VARS
	},
	{
		key: 'social_update',
		label: 'Social account update',
		description:
			'A followed social account goes live or posts. Branch on .Event.kind (live_start, live_end, new_video, new_post) and .Event.provider. No .User or .Channel, send to an explicit channel.',
		category: 'social',
		event: 'SOCIAL_UPDATE',
		actor: '(no actor)',
		hasChannel: false,
		filters: ['social_accounts', 'social_kinds', 'cooldown'],
		eventVars: SOCIAL_EVENT_VARS
	}
];

export const TRIGGER_BY_KEY = new Map(TRIGGERS.map((t) => [t.key, t]));

export interface AutomationSummary {
	id: string;
	name: string;
	description: string;
	enabled: boolean;
	status: AutomationStatus;
	version?: number;
	trigger_type: string;
	trigger_config?: TriggerConfig;
	updated_at?: string;
	builtin?: boolean;
	feature_key?: string;
	feature_name?: string;
	feature_tab?: string;
	step_count?: number;
	flow_shape?: import('$lib/commands/types').ShapeNode[];
	shape_more?: number;
	runs_24h?: number;
	last_run_at?: string | null;
}

export interface AutomationFull extends AutomationSummary {
	definition: Definition;
}

// triggerEventVars returns the extra `.Event.*` variables for a trigger key
// (empty when unknown), used to seed the editor's expression scope.
export function triggerEventVars(triggerType: string): TmplVar[] {
	return TRIGGER_BY_KEY.get(triggerType)?.eventVars ?? [];
}
