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

class ApiError extends Error {
	constructor(
		public status: number,
		message: string
	) {
		super(message);
	}
}

async function req<T>(method: string, path: string, body?: unknown): Promise<T> {
	const headers: Record<string, string> = {};
	if (body !== undefined) headers['Content-Type'] = 'application/json';
	if (method !== 'GET') headers['X-CSRF-Token'] = csrfToken;

	const res = await fetch(`${API_URL}${path}`, {
		method,
		credentials: 'include',
		headers,
		body: body !== undefined ? JSON.stringify(body) : undefined
	});
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
	return (await res.json()) as T;
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
	upsertCommand: (id: string, cmd: unknown) =>
		req('PUT', `/api/guilds/${id}/commands`, cmd),
	deleteCommand: (id: string, cid: number) =>
		req('DELETE', `/api/guilds/${id}/commands/${cid}`),

	menus: (id: string) => req<{ menus: any[] }>('GET', `/api/guilds/${id}/reaction-roles`),
	upsertMenu: (id: string, menu: unknown) =>
		req<{ id?: number; ok?: boolean }>('PUT', `/api/guilds/${id}/reaction-roles`, menu),
	deleteMenu: (id: string, mid: number) =>
		req('DELETE', `/api/guilds/${id}/reaction-roles/${mid}`),

	cases: (id: string) => req<{ cases: any[] }>('GET', `/api/guilds/${id}/cases`),
	welcomePresets: () => req<{ presets: any[] }>('GET', '/api/welcome/presets'),
	welcomeVariables: (id: string) =>
		req<{ variables: { token: string; desc: string }[] }>('GET', `/api/guilds/${id}/welcome/variables`),
	welcomeTest: (id: string, kind: 'welcome' | 'goodbye') =>
		req<{ ok: boolean }>('POST', `/api/guilds/${id}/welcome/test`, { kind }),
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
