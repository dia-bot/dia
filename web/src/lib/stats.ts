// Types for the server stats feature, mirroring
// internal/features/statschannels/config.go. Field names must match the Go
// json tags exactly.

import type { TmplVar } from '$lib/commands/expr-meta';

export interface StatsCounter {
	id: string;
	channel_id: string;
	template: string;
	enabled: boolean;
}

export interface StatsConfig {
	counters?: StatsCounter[];
	milestone_step?: number;
	// tail is canvas-owned (the built-in automation's follow-up flow); the
	// settings page never writes it (the API merges the stored copy back).
	tail?: unknown[];
}

// STATS_TEMPLATE_VARS is the counter-name template scope, kept in lockstep
// with the data map in statschannels.updateGuild.
export const STATS_TEMPLATE_VARS: TmplVar[] = [
	{ path: '.Members', label: 'Members', type: 'int', short: 'Member count' },
	{ path: '.Channels', label: 'Channels', type: 'int', short: 'Channel count' },
	{ path: '.Roles', label: 'Roles', type: 'int', short: 'Role count' },
	{ path: '.Milestone', label: 'Milestone', type: 'int', short: 'Last milestone crossed' },
	{ path: '.Guild.Name', label: 'Guild.Name', type: 'string', short: 'Server name' }
];

export function newCounterId(): string {
	return 'ctr-' + Math.random().toString(36).slice(2, 10);
}
