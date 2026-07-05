// Sidebar order of the dashboard's feature routes. This mirrors the nav[]
// groups in servers/[id]/+layout.svelte, flattened top-to-bottom. It drives the
// directional page transition: navigating DOWN the sidebar (a higher index)
// slides the content surface in from the right, navigating UP slides it in from
// the left, so moving between pages reads as spatial movement rather than a
// crossfade. Keep this list in lockstep with the sidebar nav order.
const ORDER = [
	'', // Overview
	'welcome',
	'leveling',
	'reaction-roles',
	'auto-roles',
	'moderation',
	'automod',
	'verification',
	'logging',
	'commands',
	'automations',
	'editor',
	'billing'
];

function dashInfo(pathname?: string | null): { guild: string; slug: string } | null {
	if (!pathname) return null;
	const m = pathname.match(/^\/servers\/([^/]+)(?:\/([^/]+))?/);
	if (!m) return null;
	return { guild: m[1], slug: m[2] ?? '' };
}

// navDirection returns 'fwd' | 'back' when both routes are dashboard pages in
// the SAME guild whose sidebar order is known, and null otherwise (the caller
// then falls back to the default crossfade). Cross-server switches and drilling
// into a sub-route of the same feature (same slug) return null on purpose:
// there is no meaningful left/right for those.
export function navDirection(
	from?: string | null,
	to?: string | null
): 'fwd' | 'back' | null {
	const a = dashInfo(from);
	const b = dashInfo(to);
	if (!a || !b) return null;
	if (a.guild !== b.guild) return null;
	const ia = ORDER.indexOf(a.slug);
	const ib = ORDER.indexOf(b.slug);
	if (ia === -1 || ib === -1 || ia === ib) return null;
	return ib > ia ? 'fwd' : 'back';
}
