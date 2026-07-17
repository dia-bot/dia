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

export type MilestoneKind = 'every' | 'at';

export interface StatsMilestone {
	id: string;
	kind: MilestoneKind;
	value: number;
	enabled: boolean;
}

export interface StatsConfig {
	counters?: StatsCounter[];
	milestones?: StatsMilestone[];
	// milestone_step is the legacy single recurring milestone; normalizeStats
	// folds it into milestones (mirrors Config.Normalize in Go).
	milestone_step?: number;
	// tail is canvas-owned (the built-in automation's follow-up flow); the
	// settings page never writes it (the API merges the stored copy back).
	tail?: unknown[];
}

// normalizeStats mirrors statschannels.Config.Normalize: a config saved before
// milestones existed keeps its recurring step as a milestone row.
export function normalizeStats(cfg: StatsConfig): { counters: StatsCounter[]; milestones: StatsMilestone[] } {
	let milestones = cfg.milestones ?? [];
	if (!milestones.length && (cfg.milestone_step ?? 0) > 0) {
		milestones = [{ id: 'legacy-step', kind: 'every', value: cfg.milestone_step!, enabled: true }];
	}
	return { counters: cfg.counters ?? [], milestones };
}

// milestoneWindow mirrors statschannels.milestoneWindow: the last milestone
// value reached and the next one coming up across every enabled milestone.
export function milestoneWindow(milestones: StatsMilestone[], members: number): { last: number; next: number } {
	let last = 0;
	let next = 0;
	for (const m of milestones) {
		if (!m.enabled || m.value <= 0) continue;
		if (m.kind === 'at') {
			if (members >= m.value) last = Math.max(last, m.value);
			else if (next === 0 || m.value < next) next = m.value;
		} else {
			const r = Math.floor(members / m.value) * m.value;
			if (r > last) last = r;
			const up = (Math.floor(members / m.value) + 1) * m.value;
			if (next === 0 || up < next) next = up;
		}
	}
	return { last, next };
}

export function milestoneLabel(m: StatsMilestone): string {
	const v = m.value.toLocaleString();
	return m.kind === 'at' ? `At ${v} members` : `Every ${v} members`;
}

// STATS_TEMPLATE_VARS is the counter-name template scope, kept in lockstep
// with the data map in statschannels.updateGuild.
export const STATS_TEMPLATE_VARS: TmplVar[] = [
	{ path: '.Members', label: 'Members', type: 'int', short: 'Member count' },
	{ path: '.Channels', label: 'Channels', type: 'int', short: 'Channel count' },
	{ path: '.Roles', label: 'Roles', type: 'int', short: 'Role count' },
	{ path: '.Milestone', label: 'Milestone', type: 'int', short: 'Last milestone reached' },
	{ path: '.NextMilestone', label: 'NextMilestone', type: 'int', short: 'Next milestone coming up' },
	{ path: '.Guild.Name', label: 'Guild.Name', type: 'string', short: 'Server name' }
];

// COUNTER_PRESETS seed the counter editor so a new counter starts from a
// working template instead of a blank input.
export const COUNTER_PRESETS: { label: string; template: string }[] = [
	{ label: 'Members', template: '👥 Members: {{ .Members }}' },
	{ label: 'Next milestone', template: '🎯 Road to {{ .NextMilestone }}' },
	{ label: 'Channels', template: '📁 Channels: {{ .Channels }}' },
	{ label: 'Roles', template: '🏷️ Roles: {{ .Roles }}' }
];

export function newCounterId(): string {
	return 'ctr-' + Math.random().toString(36).slice(2, 10);
}

export function newMilestoneId(): string {
	return 'ms-' + Math.random().toString(36).slice(2, 10);
}
