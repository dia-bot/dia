// Browser API client for the Dia backend. Uses cookie credentials and a
// double-submit CSRF token (set once from /api/me via the root layout).
import { env } from '$env/dynamic/public';
import type {
	GuildDetail,
	GuildListItem,
	FeatureState
} from './types';

export const API_URL = env.PUBLIC_API_URL ?? 'http://localhost:8080';
export const WS_URL = env.PUBLIC_WS_URL ?? 'ws://localhost:8080';

let csrfToken = '';
export function setCsrf(token: string) {
	csrfToken = token;
}

export const loginURL = `${API_URL}/auth/login`;

export class ApiError extends Error {
	constructor(
		public status: number,
		message: string
	) {
		super(message);
	}
}

// Default per-request timeout in ms. Long enough for image renders and slow
// validation, short enough to surface a real hang as an error instead of an
// infinite spinner.
const DEFAULT_TIMEOUT_MS = 10_000;

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
	const headers: Record<string, string> = {};
	if (body !== undefined) headers['Content-Type'] = 'application/json';
	if (method !== 'GET') headers['X-CSRF-Token'] = csrfToken;

	const ctrl = new AbortController();
	// Double-belt-and-suspenders timeout: the AbortController cancels the
	// underlying TCP work, AND a Promise.race rejects with our own ApiError
	// so the call doesn't hang on any browser quirk that fails to surface the
	// abort.
	const timeoutErr = new ApiError(
		0,
		`Request timed out after ${DEFAULT_TIMEOUT_MS / 1000}s — the API may be down or unreachable`
	);
	const timeoutP = new Promise<never>((_, reject) => {
		setTimeout(() => {
			ctrl.abort();
			reject(timeoutErr);
		}, DEFAULT_TIMEOUT_MS);
	});

	let res: Response;
	try {
		res = await Promise.race([
			fetch(`${API_URL}${path}`, {
				method,
				credentials: 'include',
				headers,
				body: body !== undefined ? JSON.stringify(body) : undefined,
				signal: ctrl.signal
			}),
			timeoutP
		]);
	} catch (e) {
		if (e instanceof ApiError) throw e;
		if (e instanceof DOMException && e.name === 'AbortError') throw timeoutErr;
		throw new ApiError(0, e instanceof Error ? e.message : `network error: ${String(e)}`);
	}
	if (!res.ok) {
		let msg = res.statusText;
		try {
			const j = await res.json();
			if (j?.error) msg = j.error;
		} catch {
			/* non-JSON error */
		}
		throw new ApiError(res.status, msg);
	}
	if (res.status === 204) return undefined as T;
	try {
		return (await res.json()) as T;
	} catch (e) {
		throw new ApiError(res.status, `invalid JSON response: ${e instanceof Error ? e.message : String(e)}`);
	}
}

export const api = {
	logout: () => req<void>('POST', '/api/auth/logout'),

	guilds: () => req<{ guilds: GuildListItem[] }>('GET', '/api/guilds'),
	guild: (id: string) => req<GuildDetail>('GET', `/api/guilds/${id}`),

	feature: (id: string, key: string) =>
		req<FeatureState>('GET', `/api/guilds/${id}/features/${key}`),
	saveFeature: (id: string, key: string, enabled: boolean, config: unknown) =>
		req<{ ok: boolean }>('PUT', `/api/guilds/${id}/features/${key}`, { enabled, config }),

	leaderboard: (id: string) =>
		req<{ entries: any[] }>('GET', `/api/guilds/${id}/leaderboard`),
	rewards: (id: string) => req<{ rewards: any[] }>('GET', `/api/guilds/${id}/level-rewards`),
	setReward: (id: string, level: number, role_id: string, remove_previous: boolean) =>
		req('PUT', `/api/guilds/${id}/level-rewards`, { level, role_id, remove_previous }),
	deleteReward: (id: string, level: number) =>
		req('DELETE', `/api/guilds/${id}/level-rewards/${level}`),

	commands: (id: string) => req<{ commands: any[] }>('GET', `/api/guilds/${id}/commands`),
	command: (id: string, cid: string) => req<any>('GET', `/api/guilds/${id}/commands/${cid}`),
	emojis: (id: string) =>
		req<{ emojis: { id: string; name: string; animated: boolean }[] }>(
			'GET',
			`/api/guilds/${id}/emojis`
		),
	upsertCommand: (id: string, cmd: unknown) =>
		req<{ id: string; validation: any }>('PUT', `/api/guilds/${id}/commands`, cmd),
	validateCommand: (id: string, cmd: unknown) =>
		req<{ validation: any }>('POST', `/api/guilds/${id}/commands/validate`, cmd),
	deleteCommand: (id: string, cid: string) =>
		req('DELETE', `/api/guilds/${id}/commands/${cid}`),
	setCommandGroup: (id: string, cid: string, groupId: string | null) =>
		req('PATCH', `/api/guilds/${id}/commands/${cid}/group`, { group_id: groupId }),
	commandGroups: (id: string) =>
		req<{ groups: { id: string; name: string; position: number; created_at: string }[] }>(
			'GET',
			`/api/guilds/${id}/command-groups`
		),
	createCommandGroup: (id: string, name: string) =>
		req<{ id: string; name: string; position: number }>('POST', `/api/guilds/${id}/command-groups`, {
			name
		}),
	renameCommandGroup: (id: string, gid: string, name: string) =>
		req('PATCH', `/api/guilds/${id}/command-groups/${gid}`, { name }),
	deleteCommandGroup: (id: string, gid: string) =>
		req('DELETE', `/api/guilds/${id}/command-groups/${gid}`),
	reorderCommandGroups: (id: string, ids: string[]) =>
		req('PATCH', `/api/guilds/${id}/command-group-order`, { ids }),
	commandRuns: (id: string, commandId?: string, limit = 25) =>
		req<{ runs: any[] }>(
			'GET',
			`/api/guilds/${id}/command-runs?limit=${limit}` +
				(commandId ? `&command_id=${commandId}` : '')
		),
	commandRun: (id: string, runId: string) =>
		req<{ run: any; logs: any[] }>('GET', `/api/guilds/${id}/command-runs/${runId}`),
	commandTemplates: (id: string) =>
		req<{ templates: any[] }>('GET', `/api/guilds/${id}/command-templates`),
	upsertCommandTemplate: (id: string, tpl: unknown) =>
		req<{ id: number }>('PUT', `/api/guilds/${id}/command-templates`, tpl),
	deleteCommandTemplate: (id: string, tid: number) =>
		req('DELETE', `/api/guilds/${id}/command-templates/${tid}`),

	// ── Automations (server-event step flows) ──
	automations: (id: string) =>
		req<{ automations: any[]; builtins: any[] }>('GET', `/api/guilds/${id}/automations`),
	automation: (id: string, aid: string) => req<any>('GET', `/api/guilds/${id}/automations/${aid}`),
	upsertAutomation: (id: string, auto: unknown) =>
		req<{ id: string; validation: any }>('PUT', `/api/guilds/${id}/automations`, auto),
	validateAutomation: (id: string, auto: unknown) =>
		req<{ validation: any }>('POST', `/api/guilds/${id}/automations/validate`, auto),
	deleteAutomation: (id: string, aid: string) =>
		req('DELETE', `/api/guilds/${id}/automations/${aid}`),
	automationTriggers: (id: string) =>
		req<{ triggers: any[] }>('GET', `/api/guilds/${id}/automation-triggers`),
	automationRuns: (id: string, automationId?: string, limit = 25) =>
		req<{ runs: any[] }>(
			'GET',
			`/api/guilds/${id}/automation-runs?limit=${limit}` +
				(automationId ? `&automation_id=${automationId}` : '')
		),
	automationRun: (id: string, runId: string) =>
		req<{ run: any; logs: any[] }>('GET', `/api/guilds/${id}/automation-runs/${runId}`),

	// ── Giveaways ──
	giveaways: (id: string, status = '') =>
		req<{ giveaways: any[] }>(
			'GET',
			`/api/guilds/${id}/giveaways` + (status ? `?status=${status}` : '')
		),
	giveaway: (id: string, gwid: string) =>
		req<any>('GET', `/api/guilds/${id}/giveaways/${gwid}`),
	createGiveaway: (id: string, body: unknown) =>
		req<any>('POST', `/api/guilds/${id}/giveaways`, body),
	updateGiveaway: (id: string, gwid: string, body: unknown) =>
		req<any>('PATCH', `/api/guilds/${id}/giveaways/${gwid}`, body),
	startGiveaway: (id: string, gwid: string, durationSeconds: number, startsInSeconds = 0) =>
		req<any>('POST', `/api/guilds/${id}/giveaways/${gwid}/start`, {
			duration_seconds: durationSeconds,
			starts_in_seconds: startsInSeconds
		}),
	deleteGiveaway: (id: string, gwid: string) =>
		req<{ ok: boolean }>('DELETE', `/api/guilds/${id}/giveaways/${gwid}`),
	endGiveaway: (id: string, gwid: string) =>
		req<{ ok: boolean }>('POST', `/api/guilds/${id}/giveaways/${gwid}/end`),
	rerollGiveaway: (id: string, gwid: string, winners = 0) =>
		req<{ ok: boolean; winners: string[] }>('POST', `/api/guilds/${id}/giveaways/${gwid}/reroll`, {
			winners
		}),
	cancelGiveaway: (id: string, gwid: string) =>
		req<{ ok: boolean }>('POST', `/api/guilds/${id}/giveaways/${gwid}/cancel`),

	menus: (id: string) => req<{ menus: any[] }>('GET', `/api/guilds/${id}/reaction-roles`),
	upsertMenu: (id: string, menu: unknown) =>
		req<{ id?: number; ok?: boolean }>('PUT', `/api/guilds/${id}/reaction-roles`, menu),
	deleteMenu: (id: string, mid: number) =>
		req('DELETE', `/api/guilds/${id}/reaction-roles/${mid}`),
	postMenu: (id: string, mid: number, channel_id: string) =>
		req<{ ok: boolean; message_id: string }>(
			'POST',
			`/api/guilds/${id}/reaction-roles/${mid}/post`,
			{ channel_id }
		),

	cases: (id: string) => req<{ cases: any[] }>('GET', `/api/guilds/${id}/cases`),
	// Automod infractions (the escalation heat ledger); optional user filter.
	infractions: (id: string, userId?: string) =>
		req<{ infractions: any[] }>(
			'GET',
			`/api/guilds/${id}/infractions` + (userId ? `?user=${userId}` : '')
		),
	// Automod overview stats: hit counts and the top-offender leaderboard.
	automodStats: (id: string) =>
		req<{ hits_24h: number; hits_7d: number; rules: number; offenders: any[] }>(
			'GET',
			`/api/guilds/${id}/automod-stats`
		),
	// Native Discord AutoMod rules (managed via Discord's own AutoMod API).
	automodRules: (id: string) =>
		req<{ rules: any[] }>('GET', `/api/guilds/${id}/automod-rules`),
	saveAutomodRule: (id: string, rule: unknown) =>
		req<{ rule: any }>('PUT', `/api/guilds/${id}/automod-rules`, rule),
	deleteAutomodRule: (id: string, ruleId: string) =>
		req('DELETE', `/api/guilds/${id}/automod-rules/${ruleId}`),
	welcomePresets: () => req<{ presets: any[] }>('GET', '/api/welcome/presets'),
	welcomeVariables: (id: string) =>
		req<{ variables: { token: string; desc: string }[] }>('GET', `/api/guilds/${id}/welcome/variables`),
	welcomeTest: (id: string, kind: 'welcome' | 'goodbye') =>
		req<{ ok: boolean }>('POST', `/api/guilds/${id}/welcome/test`, { kind }),
	// saveWelcomeActions persists the canvas-authored programs back into the
	// welcome config: the per-button click actions and the post-message tail
	// (the follow-up flow wired after the message).
	saveWelcomeActions: (
		id: string,
		kind: 'welcome' | 'goodbye',
		actions: unknown,
		dmActions: unknown,
		tail: unknown
	) =>
		req<{ ok: boolean }>('POST', `/api/guilds/${id}/welcome/actions`, {
			kind,
			actions,
			dm_actions: dmActions,
			tail
		}),
	// saveLevelingActions persists the canvas-authored programs back into the
	// leveling config: the level-up announcement's per-button click actions and
	// the post-message tail. Mirrors saveWelcomeActions, minus the welcome/goodbye
	// kind and DM tab (the announcement is a single channel message).
	saveLevelingActions: (id: string, actions: unknown, tail: unknown) =>
		req<{ ok: boolean }>('POST', `/api/guilds/${id}/leveling/actions`, {
			actions,
			tail
		}),
	// saveAutoroleActions persists the canvas-authored post-grant tail back into the
	// auto-roles config. Auto-roles sends no message and has no buttons, so unlike
	// welcome/leveling there are no click actions: only the follow-up flow wired
	// after the read-only grant-roles step.
	saveAutoroleActions: (id: string, tail: unknown) =>
		req<{ ok: boolean }>('POST', `/api/guilds/${id}/autorole/actions`, { tail }),
	// saveMenuTail persists the canvas-authored follow-up flow for one
	// reaction-role menu. Like auto-roles the spine (the role-apply step) is
	// read-only and there are no click actions, so only the post-pick tail saves.
	saveMenuTail: (id: string, menuId: number, tail: unknown[]) =>
		req<{ ok: boolean }>('POST', `/api/guilds/${id}/reaction-roles/${menuId}/actions`, { tail }),
	// saveAutomodRuleTail persists the canvas-authored follow-up flow for one
	// automod rule. The spine (the rule's built-in actions) is read-only, so
	// like reaction-roles only the post-fire tail saves.
	saveAutomodRuleTail: (id: string, ruleId: string, tail: unknown[]) =>
		req<{ ok: boolean }>('POST', `/api/guilds/${id}/automod/rules/${ruleId}/actions`, { tail }),
	levelingVariables: (id: string) =>
		req<{ variables: { token: string; desc: string }[] }>('GET', `/api/guilds/${id}/leveling/variables`),
	// templatingPreview renders one template string and returns the text + any error.
	templatingPreview: (id: string, template: string, extraVars?: Record<string, string>) =>
		req<{ rendered: string; error: string }>('POST', `/api/guilds/${id}/templating/preview`, {
			template,
			extra_vars: extraVars
		})
};

// layoutPreview posts a layout document and returns an object URL for the
// server-rendered PNG (the exact image the bot would post).
export async function layoutPreview(
	id: string,
	layout: unknown,
	extraVars?: Record<string, string>
): Promise<string> {
	const res = await fetch(`${API_URL}/api/guilds/${id}/layout/preview`, {
		method: 'POST',
		credentials: 'include',
		headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken },
		body: JSON.stringify({ layout, extra_vars: extraVars })
	});
	if (!res.ok) throw new ApiError(res.status, 'layout preview failed');
	return URL.createObjectURL(await res.blob());
}

// uploadImage sends a file to the guild's upload endpoint (multipart) and
// returns the stored public URL. Throws ApiError (e.g. 503 when uploads aren't
// configured, 415 for a non-image, 413 when too large).
export async function uploadImage(id: string, file: File): Promise<string> {
	const form = new FormData();
	form.append('file', file);
	const res = await fetch(`${API_URL}/api/guilds/${id}/uploads`, {
		method: 'POST',
		credentials: 'include',
		headers: { 'X-CSRF-Token': csrfToken }, // no Content-Type: the browser sets the multipart boundary
		body: form
	});
	if (!res.ok) {
		let msg = 'upload failed';
		try {
			const j = await res.json();
			if (j?.error) msg = j.error;
		} catch {
			/* non-JSON */
		}
		throw new ApiError(res.status, msg);
	}
	const j = (await res.json()) as { url: string };
	return j.url;
}

export interface CustomFont {
	family: string;
	url: string;
}

// guildFonts lists a guild's uploaded custom fonts + whether it's premium.
export function guildFonts(id: string) {
	return req<{ fonts: CustomFont[]; premium: boolean }>('GET', `/api/guilds/${id}/fonts`);
}

// uploadFont sends a TTF/OTF to the guild's font store (premium only) and returns
// the parsed family name + public URL.
export async function uploadFont(id: string, file: File): Promise<CustomFont> {
	const form = new FormData();
	form.append('file', file);
	const res = await fetch(`${API_URL}/api/guilds/${id}/fonts`, {
		method: 'POST',
		credentials: 'include',
		headers: { 'X-CSRF-Token': csrfToken },
		body: form
	});
	if (!res.ok) {
		let msg = 'font upload failed';
		try {
			const j = await res.json();
			if (j?.error) msg = j.error;
		} catch {
			/* non-JSON */
		}
		throw new ApiError(res.status, msg);
	}
	return (await res.json()) as CustomFont;
}

// deleteFont unlinks a guild custom font by family.
export function deleteFont(id: string, family: string) {
	return req<void>('DELETE', `/api/guilds/${id}/fonts/${encodeURIComponent(family)}`);
}

// ── storage assets + billing ────────────────────────────────────────────────
export interface AssetItem {
	id: number;
	kind: 'image' | 'font';
	family: string;
	url: string;
	bytes: number;
	created_at: number;
}
export function guildAssets(id: string) {
	return req<{ assets: AssetItem[]; used: number; quota: number; premium: boolean }>(
		'GET',
		`/api/guilds/${id}/assets`
	);
}
export function deleteAsset(id: string, assetId: number) {
	return req<void>('DELETE', `/api/guilds/${id}/assets/${assetId}`);
}

export interface BillingStatus {
	premium: boolean;
	price: string;
	billing_enabled: boolean;
	status?: string;
	manage?: boolean;
	current_period_end?: number;
}
export function billingStatus(id: string) {
	return req<BillingStatus>('GET', `/api/guilds/${id}/billing`);
}
export function billingCheckout(id: string) {
	return req<{ url: string }>('POST', `/api/guilds/${id}/billing/checkout`);
}
export function billingPortal(id: string) {
	return req<{ url: string }>('POST', `/api/guilds/${id}/billing/portal`);
}

// resolveCard renders card template strings ({{.User.Username}} etc.) on the
// server with sample data, so the live studio canvas matches the bot's output.
export async function resolveCard(
	id: string,
	strings: string[],
	extraVars?: Record<string, string>
): Promise<string[]> {
	const res = await req<{ resolved: string[] }>('POST', `/api/guilds/${id}/layout/resolve`, {
		strings,
		extra_vars: extraVars
	});
	return res.resolved;
}

// previewImage posts a config and returns an object URL for an <img src>.
export async function previewImage(
	id: string,
	kind: 'welcome' | 'rank',
	payload: unknown
): Promise<string> {
	const res = await fetch(`${API_URL}/api/guilds/${id}/${kind}/preview`, {
		method: 'POST',
		credentials: 'include',
		headers: { 'Content-Type': 'application/json', 'X-CSRF-Token': csrfToken },
		body: JSON.stringify(payload)
	});
	if (!res.ok) throw new ApiError(res.status, 'preview failed');
	const blob = await res.blob();
	return URL.createObjectURL(blob);
}
