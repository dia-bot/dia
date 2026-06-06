// Popup-based Discord login. The main tab never navigates: a popup runs the
// OAuth redirect dance and the finisher page (/auth/done) messages us back, so
// the marketing page stays mounted and we can animate into the dashboard.
import { API_URL } from '$lib/api';
import { goto, invalidateAll } from '$app/navigation';

export type LoginResult = 'ok' | 'cancelled' | 'blocked';

const CHANNEL = 'dia-auth';

function openPopup(): Window | null {
	const w = 520;
	const h = 720;
	const left = Math.max(0, window.screenX + (window.outerWidth - w) / 2);
	const top = Math.max(0, window.screenY + (window.outerHeight - h) / 2);
	// The window name ('dia-login') survives the cross-origin Discord hop and is
	// how /auth/done detects it's running inside the login popup.
	return window.open(
		`${API_URL}/auth/login`,
		'dia-login',
		`popup=1,width=${w},height=${h},left=${left},top=${top}`
	);
}

function isAuthMessage(data: unknown): data is { type: string; ok?: boolean } {
	return typeof data === 'object' && data !== null && (data as { type?: string }).type === CHANNEL;
}

// openLoginPopup opens the popup and resolves when the closer page reports back,
// the user closes the popup, or the popup is blocked. It listens on a
// BroadcastChannel (which survives COOP severing window.opener across the
// cross-origin Discord hop) with a window message as a belt-and-braces fallback.
export function openLoginPopup(): Promise<LoginResult> {
	return new Promise((resolve) => {
		const popup = openPopup();
		if (!popup) {
			resolve('blocked');
			return;
		}

		let settled = false;
		const channel = 'BroadcastChannel' in window ? new BroadcastChannel(CHANNEL) : null;

		const finish = (result: LoginResult) => {
			if (settled) return;
			settled = true;
			channel?.close();
			window.removeEventListener('message', onMessage);
			clearInterval(poll);
			try {
				popup.close();
			} catch {
				/* already closed */
			}
			resolve(result);
		};

		const handle = (data: unknown) => {
			if (isAuthMessage(data)) finish(data.ok ? 'ok' : 'cancelled');
		};

		function onMessage(e: MessageEvent) {
			if (e.origin !== window.location.origin) return;
			handle(e.data);
		}

		if (channel) channel.onmessage = (e) => handle(e.data);
		window.addEventListener('message', onMessage);

		// Detect a manually-closed popup (no message arrives). Give a short grace
		// for an in-flight success message before declaring it cancelled.
		const poll = setInterval(() => {
			if (!popup.closed) return;
			clearInterval(poll);
			setTimeout(() => finish('cancelled'), 300);
		}, 400);
	});
}

// loginWithPopup runs the popup flow and, on success, refreshes auth state and
// navigates to the dashboard. Falls back to a full-page redirect if the popup is
// blocked. Returns the result so callers can react (e.g. hide an overlay).
export async function loginWithPopup(): Promise<LoginResult> {
	const result = await openLoginPopup();
	if (result === 'ok') {
		await invalidateAll();
		await goto('/servers');
	} else if (result === 'blocked') {
		window.location.href = `${API_URL}/auth/login`;
	}
	return result;
}
