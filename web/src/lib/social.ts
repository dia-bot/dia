// Types for the social alerts feature, mirroring internal/api/social.go.

// SocialCapability is one provider tile: whether this deployment has the
// credentials to offer it (available) or it's locked (coming_soon).
export interface SocialCapability {
	provider: string;
	name: string;
	status: 'available' | 'coming_soon';
	input?: string;
	kinds?: string[];
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

// The editable fields sent on create / update.
export interface SocialSubInput {
	provider?: string;
	account?: string;
	channel_id?: string;
	ping_role_id?: string;
	template?: string;
	embed?: boolean;
	enabled?: boolean;
}
