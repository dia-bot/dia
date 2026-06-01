// Shared types mirroring the Dia API responses.

export interface User {
	id: string;
	username: string;
	global_name?: string;
	avatar?: string;
	avatar_url?: string;
}

export interface GuildListItem {
	id: string;
	name: string;
	icon?: string;
	icon_url?: string;
	bot_present: boolean;
	invite_url?: string;
}

export interface Channel {
	id: string;
	name: string;
	type: number;
	position: number;
	parent_id?: string;
}

export interface Role {
	id: string;
	name: string;
	color: number;
	position: number;
	managed?: boolean;
}

export interface GuildMeta {
	id: string;
	name: string;
	icon: string;
	owner_id: string;
	member_count: number;
}

export interface FeatureState {
	enabled: boolean;
	config: Record<string, unknown>;
}

export interface GuildDetail {
	guild: GuildMeta;
	channels: Channel[];
	roles: Role[];
	features: Record<string, FeatureState>;
}

export interface Background {
	color?: string;
	from?: string;
	to?: string;
	angle?: number;
	image_url?: string;
	blur?: boolean;
}

// Realtime message pushed over the dashboard WebSocket.
export interface RealtimeMessage {
	type:
		| 'channel.upsert'
		| 'channel.delete'
		| 'role.upsert'
		| 'role.delete'
		| 'guild.update'
		| 'member.count';
	data: any;
}

// Discord channel type ids we care about for dropdowns.
export const CHANNEL_TEXT = 0;
export const CHANNEL_VOICE = 2;
export const CHANNEL_CATEGORY = 4;
export const CHANNEL_ANNOUNCEMENT = 5;
