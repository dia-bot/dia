<script lang="ts">
	// The server Overview: identity and live numbers up top, then every feature
	// as a row grouped the way the sidebar groups them. Full-bleed and flat like
	// the feature pages (slab topbar, hairline rows, mono eyebrows); toggles
	// apply instantly and each row links into its tab.
	import { getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
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
		ChevronRight,
		TriangleAlert
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);

	type Feature = {
		key: string; // guild_feature_configs key ('' = not toggleable, link-only)
		path: string;
		label: string;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		icon: any;
		desc: string;
	};

	// Grouped exactly like the sidebar so the overview reads as a map of it.
	const groups: { section: string; features: Feature[] }[] = [
		{
			section: 'Engagement',
			features: [
				{ key: 'welcome', path: 'welcome', label: 'Welcome', icon: ImageIcon, desc: 'Greet new members with composed messages and cards.' },
				{ key: 'leveling', path: 'leveling', label: 'Leveling', icon: TrendingUp, desc: 'XP, levels, rank cards and role rewards.' },
				{ key: 'reactionroles', path: 'reaction-roles', label: 'Reaction Roles', icon: ToggleRight, desc: 'Self-serve roles with buttons and menus.' },
				{ key: 'autorole', path: 'auto-roles', label: 'Auto Roles', icon: UserPlus, desc: 'Roles assigned automatically on join.' },
				{ key: 'giveaway', path: 'giveaways', label: 'Giveaways', icon: Gift, desc: 'Button-entry giveaways with presets and winners.' }
			]
		},
		{
			section: 'Social',
			features: [
				{ key: 'social', path: 'social', label: 'Social Alerts', icon: Megaphone, desc: 'Announce streams, uploads and posts from followed accounts.' }
			]
		},
		{
			section: 'Utility',
			features: [
				{ key: 'scheduler', path: 'scheduling', label: 'Scheduling', icon: CalendarClock, desc: 'Composed messages posted on a schedule.' },
				{ key: 'stats', path: 'stats', label: 'Server Stats', icon: BarChart3, desc: 'Live counter channels and member milestones.' }
			]
		},
		{
			section: 'Moderation',
			features: [
				{ key: 'moderation', path: 'moderation', label: 'Moderation', icon: ShieldCheck, desc: 'Ban, kick, timeout and warn, with a case log.' },
				{ key: 'automod', path: 'automod', label: 'Automod', icon: ShieldAlert, desc: 'Rule-based filters for spam, links and banned words.' },
				{ key: 'verification', path: 'verification', label: 'Verification', icon: UserCheck, desc: 'Gate entry behind a button or captcha.' },
				{ key: 'tickets', path: 'tickets', label: 'Tickets', icon: Ticket, desc: 'Panels, categories and private support tickets.' },
				{ key: 'logging', path: 'logging', label: 'Server Logs', icon: ScrollText, desc: 'An audit trail of what happens in the server.' }
			]
		},
		{
			section: 'Advanced',
			features: [
				{ key: 'customcommands', path: 'commands', label: 'Custom Commands', icon: Wand2, desc: 'Design your own slash commands on the canvas.' },
				{ key: '', path: 'automations', label: 'Automations', icon: Zap, desc: 'When something happens on the server, run a flow.' }
			]
		}
	];

	const toggleable = groups.flatMap((g) => g.features).filter((f) => f.key);
	const enabledCount = $derived(toggleable.filter((f) => store.feature(f.key).enabled).length);

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
	<PageTopbar eyebrow="Overview" subtitle="What Dia runs in this server.">
		{#snippet leading()}
			<div class="grid size-6 place-items-center rounded border border-line bg-surface text-muted">
				<LayoutDashboard size={13} />
			</div>
		{/snippet}
		{#snippet actions()}
			<a
				href={`/servers/${store.id}/automations`}
				class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line bg-bg px-2.5 text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink"
				title="Open the automations canvas"
			>
				<Zap size={13} /> Automations
			</a>
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

			<!-- ── Features, grouped like the sidebar ────────────────────────── -->
			{#each groups as g (g.section)}
				{@const on = g.features.filter((f) => f.key && store.feature(f.key).enabled).length}
				{@const total = g.features.filter((f) => f.key).length}
				<SectionBar label={g.section} count={total ? `${on} / ${total}` : undefined} />
				{#each g.features as f (f.path)}
					{@const state = f.key ? store.feature(f.key) : { enabled: true, config: {} }}
					<div class="group flex items-center gap-3 border-b border-line/60 px-5 py-3.5 transition-colors hover:bg-surface/60">
						<span
							class="grid size-8 shrink-0 place-items-center rounded-md border border-line bg-surface transition-colors {state.enabled
								? 'text-ink'
								: 'text-faint'}"
						>
							<f.icon size={15} />
						</span>
						<a href={`/servers/${store.id}/${f.path}`} class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<span class="size-1.5 shrink-0 rounded-full {state.enabled ? 'bg-success' : 'bg-faint/40'}"></span>
								<span class="truncate text-[12.5px] font-medium text-ink group-hover:underline">{f.label}</span>
							</div>
							<div class="mt-0.5 truncate text-[11.5px] text-muted">{f.desc}</div>
						</a>
						{#if f.key}
							<Toggle
								checked={state.enabled}
								disabled={busy === f.key}
								label="{f.label} enabled"
								onchange={(v) => toggle(f.key, v)}
							/>
						{:else}
							<span class="font-mono text-[9.5px] uppercase tracking-[0.12em] text-faint">always on</span>
						{/if}
						<a
							href={`/servers/${store.id}/${f.path}`}
							class="grid h-7 w-7 shrink-0 place-items-center rounded-md border border-line bg-bg text-muted transition-colors hover:border-line-strong hover:text-ink"
							aria-label="Configure {f.label}"
						>
							<ChevronRight size={13} />
						</a>
					</div>
				{/each}
			{/each}

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
