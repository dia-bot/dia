<script lang="ts">
	// The server Overview: a control-room status board, not a second sidebar.
	// Identity and live numbers up top, then a dense switchboard where every
	// feature cell reads its REAL state (next scheduled post, live accounts,
	// running giveaways, rule counts) with an instant toggle. Full-bleed and
	// flat like the feature pages: slab topbar, hairline cells, mono labels.
	import { onMount, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import type { ScheduledMessage } from '$lib/schedules';
	import type { SocialSubscription } from '$lib/social';
	import type { AutomationSummary } from '$lib/automations/types';
	import Toggle from '$lib/components/Toggle.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import {
		LayoutDashboard,
		ImageIcon,
		TrendingUp,
		ToggleRight,
		UserPlus,
		Gift,
		Megaphone,
		CalendarClock,
		BarChart3,
		ShieldCheck,
		ShieldAlert,
		UserCheck,
		Ticket,
		ScrollText,
		Wand2,
		Zap,
		TriangleAlert
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);

	// ── Live data behind the cells (each best-effort; a cell falls back to its
	// static line if the fetch fails) ─────────────────────────────────────────
	let autos = $state<AutomationSummary[]>([]);
	let schedules = $state<ScheduledMessage[]>([]);
	let socialSubs = $state<SocialSubscription[]>([]);
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let giveaways = $state<any[]>([]);

	onMount(() => {
		api.automations(store.id).then((r) => (autos = r.automations ?? [])).catch(() => {});
		api.schedules(store.id).then((r) => (schedules = r.schedules ?? [])).catch(() => {});
		api.social(store.id).then((r) => (socialSubs = r.subscriptions ?? [])).catch(() => {});
		api.giveaways(store.id).then((r) => (giveaways = r.giveaways ?? [])).catch(() => {});
	});

	// cfg reads a feature's stored config off the already-loaded guild snapshot.
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function cfg(key: string): any {
		return store.feature(key).config ?? {};
	}
	function channelName(id?: string): string {
		if (!id) return '';
		return store.channels.find((c) => c.id === id)?.name ?? '';
	}
	function inLabel(ts: number): string {
		const diff = ts - Date.now();
		if (diff <= 0) return 'due now';
		const m = Math.round(diff / 60000);
		if (m < 60) return `in ${m}m`;
		if (m < 1440) return `in ${Math.round(m / 60)}h`;
		return `in ${Math.round(m / 1440)}d`;
	}

	// One live subline per feature: what it is DOING right now, not what it is.
	const lines = $derived.by(() => {
		const flowsOn = autos.filter((a) => a.enabled).length;
		const drafts = autos.filter((a) => a.status === 'draft').length;
		const nextSched = schedules
			.filter((s) => s.enabled && s.next_run_at)
			.sort((a, b) => (a.next_run_at ?? 0) - (b.next_run_at ?? 0))[0];
		const live = socialSubs.filter((s) => s.live).length;
		const running = giveaways.filter((g) => g.status === 'running').length;
		const welcomeCh = channelName(cfg('welcome').channel_id);
		const autoroleN = (cfg('autorole').roles ?? []).length;
		const automodN = (cfg('automod').rules ?? []).length;
		const statsC = (cfg('stats').counters ?? []).length;
		const statsM = (cfg('stats').milestones ?? []).length;
		const logCh = channelName(cfg('logging').channel);
		return {
			welcome: welcomeCh ? `Greeting in #${welcomeCh}` : 'No channel picked yet',
			leveling: 'XP, rank cards and role rewards',
			reactionroles: 'Self-serve roles with buttons and menus',
			autorole: autoroleN ? `${autoroleN} role${autoroleN === 1 ? '' : 's'} granted on join` : 'No roles picked yet',
			giveaway: running ? `${running} running now` : 'None running',
			social: socialSubs.length
				? `${socialSubs.length} followed${live ? ` · ${live} live now` : ''}`
				: 'No accounts followed yet',
			scheduler: nextSched ? `Next: ${nextSched.name} · ${inLabel(nextSched.next_run_at!)}` : 'Nothing scheduled',
			stats: statsC || statsM ? `${statsC} counter${statsC === 1 ? '' : 's'} · ${statsM} milestone${statsM === 1 ? '' : 's'}` : 'No counters yet',
			moderation: 'Ban, kick, timeout and warn, with a case log',
			automod: automodN ? `${automodN} rule${automodN === 1 ? '' : 's'} active` : 'No rules yet',
			verification: 'Button or captcha gate on join',
			tickets: 'Panels, categories and transcripts',
			logging: logCh ? `Logging to #${logCh}` : 'No log channel yet',
			customcommands: 'Your own slash commands, built on the canvas',
			automations: `${flowsOn} flow${flowsOn === 1 ? '' : 's'} on${drafts ? ` · ${drafts} draft${drafts === 1 ? '' : 's'}` : ''}`
		} as Record<string, string>;
	});

	type Cell = {
		key: string; // guild_feature_configs key ('' = link-only, no toggle)
		path: string;
		label: string;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		icon: any;
		// live marks the subline as real state (rendered in mono).
		live?: boolean;
	};

	const cells: Cell[] = [
		{ key: 'welcome', path: 'welcome', label: 'Welcome', icon: ImageIcon, live: true },
		{ key: 'social', path: 'social', label: 'Social Alerts', icon: Megaphone, live: true },
		{ key: 'scheduler', path: 'scheduling', label: 'Scheduling', icon: CalendarClock, live: true },
		{ key: 'stats', path: 'stats', label: 'Server Stats', icon: BarChart3, live: true },
		{ key: 'giveaway', path: 'giveaways', label: 'Giveaways', icon: Gift, live: true },
		{ key: '', path: 'automations', label: 'Automations', icon: Zap, live: true },
		{ key: 'automod', path: 'automod', label: 'Automod', icon: ShieldAlert, live: true },
		{ key: 'autorole', path: 'auto-roles', label: 'Auto Roles', icon: UserPlus, live: true },
		{ key: 'logging', path: 'logging', label: 'Server Logs', icon: ScrollText, live: true },
		{ key: 'moderation', path: 'moderation', label: 'Moderation', icon: ShieldCheck },
		{ key: 'verification', path: 'verification', label: 'Verification', icon: UserCheck },
		{ key: 'tickets', path: 'tickets', label: 'Tickets', icon: Ticket },
		{ key: 'leveling', path: 'leveling', label: 'Leveling', icon: TrendingUp },
		{ key: 'reactionroles', path: 'reaction-roles', label: 'Reaction Roles', icon: ToggleRight },
		{ key: 'customcommands', path: 'commands', label: 'Custom Commands', icon: Wand2 }
	];

	const toggleable = cells.filter((c) => c.key);
	const enabledCount = $derived(toggleable.filter((c) => store.feature(c.key).enabled).length);

	let busy = $state('');
	let saveErr = $state('');

	async function toggle(key: string, v: boolean) {
		const f = store.feature(key);
		busy = key;
		saveErr = '';
		try {
			await api.saveFeature(store.id, key, v, f.config);
			if (store.detail) store.detail.features[key] = { enabled: v, config: f.config };
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Could not save';
			if (store.detail) store.detail.features[key] = { enabled: !v, config: f.config };
		} finally {
			busy = '';
		}
	}

	const iconUrl = $derived.by(() => {
		const hash = store.detail?.guild.icon ?? '';
		return hash ? `https://cdn.discordapp.com/icons/${store.id}/${hash}.png?size=128` : '';
	});
</script>

<svelte:head><title>{store.name} · Dia</title></svelte:head>

<div class="relative flex h-full flex-col bg-bg text-ink">
	<PageTopbar eyebrow="Overview" subtitle="What Dia is doing in this server, right now.">
		{#snippet leading()}
			<div class="grid size-6 place-items-center rounded border border-line bg-surface text-muted">
				<LayoutDashboard size={13} />
			</div>
		{/snippet}
	</PageTopbar>

	<div class="relative min-h-0 flex-1 overflow-y-auto bg-bg pb-16">
		{#if !store.detail}
			<div class="p-6"><div class="skeleton h-40 w-full rounded"></div></div>
		{:else}
			<!-- ── Identity + live numbers ───────────────────────────────────── -->
			<div class="grid border-b border-line/60 sm:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)]">
				<div class="flex items-center gap-3.5 border-b border-line/60 px-5 py-4 sm:border-b-0 sm:border-r">
					{#if iconUrl}
						<img src={iconUrl} alt="" class="size-10 shrink-0 rounded-lg border border-line bg-surface" />
					{:else}
						<span class="grid size-10 shrink-0 place-items-center rounded-lg border border-line bg-surface font-mono text-[14px] font-semibold text-muted">
							{store.name.slice(0, 1).toUpperCase()}
						</span>
					{/if}
					<div class="min-w-0">
						<div class="truncate text-[14px] font-semibold leading-tight text-ink">{store.name}</div>
						<div class="mt-0.5 flex items-center gap-2">
							<span class="font-mono text-[10px] tabular-nums text-faint">{store.id}</span>
							<span class="size-0.5 rounded-full bg-faint/50"></span>
							<span class="font-mono text-[10px] text-faint">{enabledCount} / {toggleable.length} features on</span>
						</div>
					</div>
				</div>
				<div class="border-r border-line/60 px-5 py-4">
					<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Members</div>
					<div class="mt-1 font-mono text-[22px] font-semibold leading-none text-ink">
						{store.memberCount.toLocaleString()}
					</div>
				</div>
				<div class="border-r border-line/60 px-5 py-4">
					<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Channels</div>
					<div class="mt-1 font-mono text-[22px] font-semibold leading-none text-ink">{store.channels.length}</div>
				</div>
				<div class="px-5 py-4">
					<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Roles</div>
					<div class="mt-1 font-mono text-[22px] font-semibold leading-none text-ink">{store.roles.length}</div>
				</div>
			</div>

			<!-- ── The switchboard ───────────────────────────────────────────── -->
			<SectionBar label="Status" count={`${enabledCount} / ${toggleable.length} on`} />
			<div class="grid sm:grid-cols-2">
				{#each cells as c (c.path)}
					{@const state = c.key ? store.feature(c.key) : { enabled: true, config: {} }}
					<div class="group flex items-center gap-3 border-b border-line/60 px-5 py-3.5 transition-colors hover:bg-surface/60 sm:odd:border-r">
						<span
							class="relative grid size-8 shrink-0 place-items-center rounded-md border border-line bg-surface transition-colors {state.enabled
								? 'text-ink'
								: 'text-faint'}"
						>
							<c.icon size={15} />
							<span
								class="absolute -right-0.5 -top-0.5 size-2 rounded-full border-2 border-bg {state.enabled ? 'bg-success' : 'bg-faint/40'}"
							></span>
						</span>
						<a href={`/servers/${store.id}/${c.path}`} class="min-w-0 flex-1">
							<div class="truncate text-[12.5px] font-medium text-ink group-hover:underline">{c.label}</div>
							{#if c.live}
								<div class="mt-0.5 truncate font-mono text-[10.5px] {state.enabled ? 'text-muted' : 'text-faint'}">
									{lines[c.key || 'automations']}
								</div>
							{:else}
								<div class="mt-0.5 truncate text-[11.5px] text-faint">{lines[c.key]}</div>
							{/if}
						</a>
						{#if c.key}
							<Toggle
								checked={state.enabled}
								disabled={busy === c.key}
								label="{c.label} enabled"
								onchange={(v) => toggle(c.key, v)}
							/>
						{:else}
							<span class="shrink-0 font-mono text-[9.5px] uppercase tracking-[0.12em] text-faint">always on</span>
						{/if}
					</div>
				{/each}
			</div>

			{#if saveErr}
				<p class="flex items-center gap-1.5 px-5 py-3 text-[12px] text-danger">
					<TriangleAlert size={13} />
					{saveErr}
				</p>
			{/if}

			<!-- ── Bridge to automations ─────────────────────────────────────── -->
			<div class="flex flex-wrap items-center gap-x-2 gap-y-1 border-t border-line/60 px-5 py-3.5 text-[12px] text-muted">
				<Zap size={13} class="shrink-0 text-faint" />
				<span>Every feature fires triggers you can build on: joins, milestones, giveaways, social updates.</span>
				<a href={`/servers/${store.id}/automations`} class="font-medium text-accent-ink hover:underline">
					Open automations →
				</a>
			</div>
		{/if}
	</div>
</div>
