// Types for the scheduled messages feature, mirroring
// internal/features/schedmessages/{config,spec}.go and internal/api/schedules.go.
// Field names must match the Go json tags exactly.

import type { TmplVar } from '$lib/commands/expr-meta';
import type { SocialEmbed, SocialComponentRow } from '$lib/social';

// SchedMessageSpec mirrors schedmessages.MessageSpec (same composed shape the
// shared MessageEditor produces; embed/component mirrors are shared with the
// social types since both mirror cc.EmbedSpec / cc.ComponentRow).
export interface SchedMessageSpec {
	content?: string;
	embeds?: SocialEmbed[];
	components?: SocialComponentRow[];
	button_actions?: Record<string, string>;
}

// ScheduleDef mirrors schedmessages.ScheduleDef. All times are UTC.
export interface ScheduleDef {
	kind: 'once' | 'every' | 'daily' | 'weekly';
	at?: string; // RFC 3339 (once)
	every_minutes?: number; // every (min 5)
	time?: string; // "HH:MM" UTC (daily/weekly)
	weekdays?: number[]; // 0=Sunday … 6=Saturday (weekly)
}

export interface ScheduledMessage {
	id: string;
	name: string;
	channel_id: string;
	spec: SchedMessageSpec;
	schedule: ScheduleDef;
	enabled: boolean;
	next_run_at?: number;
	last_run_at?: number;
	created_at: number;
}

export interface SchedInput {
	name?: string;
	channel_id?: string;
	spec?: SchedMessageSpec;
	schedule?: ScheduleDef;
	enabled?: boolean;
}

// SCHED_TEMPLATE_VARS is the message template scope, kept in lockstep with
// schedmessages.ScopeData.
export const SCHED_TEMPLATE_VARS: TmplVar[] = [
	{ path: '.Name', label: 'Name', type: 'string', short: "The schedule's name" },
	{ path: '.Date', label: 'Date', type: 'string', short: 'Today, like January 2, 2006 (UTC)' },
	{ path: '.Guild.Name', label: 'Guild.Name', type: 'string', short: 'Server name' },
	{ path: '.Guild.MemberCount', label: 'Guild.MemberCount', type: 'int', short: 'Live member count' }
];

export const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

// cadenceLabel renders a schedule's cadence for list rows.
export function cadenceLabel(d: ScheduleDef): string {
	switch (d.kind) {
		case 'once':
			return d.at ? `Once · ${new Date(d.at).toLocaleString()}` : 'Once';
		case 'every':
			return `Every ${d.every_minutes ?? 0} min`;
		case 'daily':
			return `Daily · ${d.time ?? '00:00'} UTC`;
		case 'weekly':
			return `Weekly · ${(d.weekdays ?? []).map((w) => WEEKDAYS[w]).join(', ')} · ${d.time ?? '00:00'} UTC`;
	}
	return '';
}
