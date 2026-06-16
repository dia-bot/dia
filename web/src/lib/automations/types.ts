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
	{ id: 'channels', label: 'Channels & threads' }
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

const CHANNEL_EVENT_VARS: TmplVar[] = [
	v('.Event.channel.id', 'snowflake', 'The channel id'),
	v('.Event.channel.name', 'string', 'The channel name'),
	v('.Event.channel.type', 'int', 'The channel type'),
	v('.Event.channel.parent_id', 'snowflake', 'The parent category id'),
	v('.Event.channel.topic', 'string', 'The channel topic')
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
