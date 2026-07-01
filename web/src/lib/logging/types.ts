// Mirror of internal/features/logging/config.go. Keep json field names in lockstep.

export interface LoggingConfig {
	channel: string;
	message_delete: boolean;
	message_edit: boolean;
	member_join: boolean;
	member_leave: boolean;
	member_ban: boolean;
	member_unban: boolean;
	role_changes: boolean;
	mod_actions: boolean;
	message_channel?: string;
	member_channel?: string;
	ignored_channels?: string[];
}

// Categories grouped for the dashboard, each a (key, label, hint) the page can
// render as a toggle list.
export const LOG_CATEGORIES: { key: keyof LoggingConfig; label: string; hint: string }[] = [
	{ key: 'message_delete', label: 'Deleted messages', hint: 'Log when a message is deleted (with recovered content).' },
	{ key: 'message_edit', label: 'Edited messages', hint: 'Log message edits with before and after.' },
	{ key: 'member_join', label: 'Member joins', hint: 'Log when someone joins, with account age.' },
	{ key: 'member_leave', label: 'Member leaves', hint: 'Log when someone leaves or is removed.' },
	{ key: 'member_ban', label: 'Bans', hint: 'Log when a member is banned.' },
	{ key: 'member_unban', label: 'Unbans', hint: 'Log when a ban is lifted.' },
	{ key: 'role_changes', label: 'Role changes', hint: 'Log when a member gains or loses roles.' },
	{ key: 'mod_actions', label: 'Moderation actions', hint: 'Relay every mod case (manual and automod) here.' }
];

export function defaultLogging(): LoggingConfig {
	return {
		channel: '',
		message_delete: true,
		message_edit: true,
		member_join: true,
		member_leave: true,
		member_ban: true,
		member_unban: true,
		role_changes: false,
		mod_actions: true,
		message_channel: '',
		member_channel: '',
		ignored_channels: []
	};
}
