// Reactive per-guild store shared across dashboard pages via context. It loads
// the guild snapshot once and then mutates channels/roles/meta live from the
// realtime WebSocket, so dropdowns stay current without a refresh.
import { api } from './api';
import { connectRealtime } from './realtime';
import {
	CHANNEL_TEXT,
	CHANNEL_ANNOUNCEMENT,
	type Channel,
	type GuildDetail,
	type RealtimeMessage,
	type Role
} from './types';

export const GUILD_CTX = Symbol('dia-guild');

export class GuildStore {
	id = $state('');
	detail = $state<GuildDetail | null>(null);
	loading = $state(true);
	error = $state('');
	private disconnect: (() => void) | null = null;

	constructor(id: string) {
		this.id = id;
	}

	async load() {
		this.loading = true;
		this.error = '';
		try {
			this.detail = await api.guild(this.id);
		} catch (e) {
			this.error = e instanceof Error ? e.message : 'Failed to load server';
		} finally {
			this.loading = false;
		}
	}

	connect() {
		this.disconnect?.();
		this.disconnect = connectRealtime(this.id, (m) => this.apply(m));
	}

	destroy() {
		this.disconnect?.();
		this.disconnect = null;
	}

	get channels(): Channel[] {
		return this.detail?.channels ?? [];
	}
	get roles(): Role[] {
		return this.detail?.roles ?? [];
	}
	get name(): string {
		return this.detail?.guild.name ?? '';
	}
	get memberCount(): number {
		return this.detail?.guild.member_count ?? 0;
	}

	feature(key: string) {
		return this.detail?.features[key] ?? { enabled: false, config: {} };
	}

	textChannelOptions() {
		return this.channels
			.filter((c) => c.type === CHANNEL_TEXT || c.type === CHANNEL_ANNOUNCEMENT)
			.sort((a, b) => a.position - b.position)
			.map((c) => ({ value: c.id, label: '# ' + c.name }));
	}

	roleOptions(includeManaged = false) {
		return this.roles
			.filter((r) => r.id !== this.id && (includeManaged || !r.managed))
			.sort((a, b) => b.position - a.position)
			.map((r) => ({ value: r.id, label: r.name }));
	}

	private apply(m: RealtimeMessage) {
		if (!this.detail) return;
		switch (m.type) {
			case 'channel.upsert': {
				const ch = m.data as Channel;
				const i = this.detail.channels.findIndex((c) => c.id === ch.id);
				if (i >= 0) this.detail.channels[i] = ch;
				else this.detail.channels.push(ch);
				break;
			}
			case 'channel.delete':
				this.detail.channels = this.detail.channels.filter((c) => c.id !== m.data.id);
				break;
			case 'role.upsert': {
				const r = m.data as Role;
				const i = this.detail.roles.findIndex((x) => x.id === r.id);
				if (i >= 0) this.detail.roles[i] = r;
				else this.detail.roles.push(r);
				break;
			}
			case 'role.delete':
				this.detail.roles = this.detail.roles.filter((r) => r.id !== m.data.id);
				break;
			case 'guild.update':
				if (m.data.name) this.detail.guild.name = m.data.name;
				if (typeof m.data.member_count === 'number')
					this.detail.guild.member_count = m.data.member_count;
				break;
			case 'member.count':
				if (typeof m.data.count === 'number') this.detail.guild.member_count = m.data.count;
				break;
		}
	}
}
