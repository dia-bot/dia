// Realtime client: connects to the API's per-guild WebSocket and invokes a
// callback for each guild-state change (channels/roles/member count), so the
// dashboard updates instantly. Reconnects with backoff.
import { WS_URL } from './api';
import type { RealtimeMessage } from './types';

export function connectRealtime(
	guildId: string,
	onMessage: (m: RealtimeMessage) => void
): () => void {
	let ws: WebSocket | null = null;
	let closed = false;
	let attempt = 0;
	let timer: ReturnType<typeof setTimeout> | undefined;

	const open = () => {
		if (closed) return;
		ws = new WebSocket(`${WS_URL}/realtime/${guildId}`);
		ws.onopen = () => {
			attempt = 0;
		};
		ws.onmessage = (ev) => {
			try {
				onMessage(JSON.parse(ev.data) as RealtimeMessage);
			} catch {
				/* ignore malformed */
			}
		};
		ws.onclose = () => {
			if (closed) return;
			const delay = Math.min(1000 * 2 ** attempt, 15000);
			attempt++;
			timer = setTimeout(open, delay);
		};
		ws.onerror = () => ws?.close();
	};

	open();

	return () => {
		closed = true;
		if (timer) clearTimeout(timer);
		ws?.close();
	};
}
