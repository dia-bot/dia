import type { Handle } from '@sveltejs/kit';
import { env } from '$env/dynamic/public';
import { env as privateEnv } from '$env/dynamic/private';

// Server→API calls use the internal URL (container hostname) when set, falling
// back to the public URL for single-origin / local dev setups.
const API = privateEnv.API_INTERNAL_URL || env.PUBLIC_API_URL || 'http://localhost:8080';

// Resolve the current user from the API using the session cookie. On localhost
// the cookie is shared across ports; in production put the API and web behind
// one origin (proxy /api → api) so the session cookie stays first-party.
export const handle: Handle = async ({ event, resolve }) => {
	event.locals.user = null;
	event.locals.csrf = null;

	const token = event.cookies.get('dia_session');
	if (token) {
		try {
			const res = await fetch(`${API}/api/me`, {
				headers: { cookie: `dia_session=${token}` }
			});
			if (res.ok) {
				const data = await res.json();
				event.locals.user = data.user;
				event.locals.csrf = data.csrf_token;
			}
		} catch {
			// API unreachable — treat as logged out.
		}
	}
	return resolve(event);
};
