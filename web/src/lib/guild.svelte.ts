// Reactive per-guild store shared across dashboard pages via context. It loads
// the guild snapshot once and then mutates channels/roles/meta live from the
// realtime WebSocket, so dropdowns stay current without a refresh.
import { api } from './api';
import { connectRealtime } from './realtime';
import {
	CHANNEL_TEXT,
	CHANNEL_ANNOUNCEMENT,
	CHANNEL_CATEGORY,
	type Channel,
	type GuildAccess,
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

	// ── Access (feature delegation) ──────────────────────────────────────────
	get access(): GuildAccess {
		return this.detail?.access ?? { admin: false, features: {} };
	}
	// admin: the user is a server admin/owner (full dashboard access).
	get admin(): boolean {
		return this.access.admin;
	}
	// canAccess reports whether the user may use a given feature key (admins can
	// use everything; a manager only the features their roles grant).
	canAccess(featureKey: string): boolean {
		const a = this.access;
		return a.admin || !!a.features[featureKey];
	}

	textChannelOptions() {
		return this.channels
			.filter((c) => c.type === CHANNEL_TEXT || c.type === CHANNEL_ANNOUNCEMENT)
			.sort((a, b) => a.position - b.position)
			.map((c) => ({ value: c.id, label: '# ' + c.name }));
	}

	// channelGroups returns postable (text/announcement) channels organised the
	// way Discord shows them: top-level channels first, then each category with
	// its own channels nested under it — both ordered by Discord position.
	channelGroups(): { id: string; name: string; channels: { value: string; label: string }[] }[] {
		const text = this.channels
			.filter((c) => c.type === CHANNEL_TEXT || c.type === CHANNEL_ANNOUNCEMENT)
			.sort((a, b) => a.position - b.position);
		const inCat = (catId: string) =>
			text
				.filter((c) => (c.parent_id ?? '') === catId)
				.map((c) => ({ value: c.id, label: c.name }));

		const groups: { id: string; name: string; channels: { value: string; label: string }[] }[] = [];
		const uncategorised = inCat('');
		if (uncategorised.length) groups.push({ id: '', name: '', channels: uncategorised });
		for (const cat of this.channels
			.filter((c) => c.type === CHANNEL_CATEGORY)
			.sort((a, b) => a.position - b.position)) {
			const channels = inCat(cat.id);
			if (channels.length) groups.push({ id: cat.id, name: cat.name, channels });
		}
		return groups;
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
