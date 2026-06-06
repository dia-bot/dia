// OAuth callback (server-side). Discord redirects the browser here on the web
// origin. We exchange the code against the API server-to-server (the client
// secret never reaches the browser), set the session as a first-party HttpOnly
// cookie, and hand off to /auth/done — which closes the popup or lands the
// full-page flow on the dashboard. The token is never exposed to client JS.
import { redirect } from '@sveltejs/kit';
import { env } from '$env/dynamic/public';
import { env as privateEnv } from '$env/dynamic/private';
import type { RequestHandler } from './$types';

const API = privateEnv.API_INTERNAL_URL || env.PUBLIC_API_URL || 'http://localhost:8080';
const COOKIE = 'dia_session';
const WEEK = 60 * 60 * 24 * 7;

export const GET: RequestHandler = async ({ url, cookies, fetch }) => {
	const oauthError = url.searchParams.get('error');
	if (oauthError) {
		// User denied consent (or Discord errored) — no code to exchange.
		throw redirect(303, '/auth/done?error=cancelled');
	}

	const code = url.searchParams.get('code');
	const state = url.searchParams.get('state');
	if (!code || !state) {
		throw redirect(303, '/auth/done?error=missing_code');
	}

	let token: string | null = null;
	let maxAge = WEEK;
	try {
		const res = await fetch(`${API}/auth/exchange`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ code, state })
		});
		if (res.ok) {
			const data = (await res.json()) as { token?: string; expires_in?: number };
			token = data.token ?? null;
			if (typeof data.expires_in === 'number' && data.expires_in > 0) maxAge = data.expires_in;
		}
	} catch {
		token = null;
	}

	if (!token) {
		throw redirect(303, '/auth/done?error=login_failed');
	}

	cookies.set(COOKIE, token, {
		path: '/',
		httpOnly: true,
		sameSite: 'lax',
		// Secure only over HTTPS, so the cookie still sets over plain http in dev
		// (localhost / a Tailscale IP) where SvelteKit would otherwise force it on.
		secure: url.protocol === 'https:',
		maxAge
	});

	throw redirect(303, '/auth/done');
};
