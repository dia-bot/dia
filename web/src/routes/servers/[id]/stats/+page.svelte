<script lang="ts">
	// Server Stats overview: a live numbers strip up top, then counters and
	// milestones as rows that open their own editor popups (mirrors the Social
	// tab's structure). Every change persists instantly, no page-level save
	// button; the popups guard their own unsaved state.
	import { onMount, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import {
		milestoneLabel,
		milestoneWindow,
		normalizeStats,
		type StatsConfig,
		type StatsCounter,
		type StatsMilestone
	} from '$lib/stats';
	import type { AutomationSummary } from '$lib/automations/types';
	import Toggle from '$lib/components/Toggle.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import EmptyBlock from '$lib/components/page/EmptyBlock.svelte';
	import CounterEditor from '$lib/components/stats/CounterEditor.svelte';
	import MilestoneEditor from '$lib/components/stats/MilestoneEditor.svelte';
	import {
		BarChart3,
		Flag,
		Pencil,
		Plus,
		Trash2,
		TriangleAlert,
		Users,
		Volume2,
		Zap
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'stats';

	let enabled = $state(false);
	let counters = $state<StatsCounter[]>([]);
	let milestones = $state<StatsMilestone[]>([]);
	let loaded = $state(false);
	let loadErr = $state('');
	let saveErr = $state('');

	// Connected-flow counts for the milestone rows.
	let autos = $state<AutomationSummary[]>([]);

	onMount(async () => {
		try {
			const f = await api.feature(store.id, FEATURE);
			enabled = f.enabled;
			const norm = normalizeStats((f.config ?? {}) as StatsConfig);
			counters = norm.counters;
			milestones = norm.milestones;
			loaded = true;
		} catch (e) {
			loadErr = e instanceof Error ? e.message : 'Failed to load server stats';
		}
		api
			.automations(store.id)
			.then((r) => (autos = (r.automations ?? []).filter((a) => a.trigger_type === 'member_milestone')))
			.catch(() => {});
	});

	// persist writes the whole config; the API merges the canvas-owned tail
	// back. Everything on this page saves through here, instantly.
	async function persist() {
		saveErr = '';
		try {
			await api.saveFeature(store.id, FEATURE, enabled, {
				counters: $state.snapshot(counters),
				milestones: $state.snapshot(milestones)
			});
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Could not save';
			throw e;
		}
	}

	async function toggleFeature(v: boolean) {
		try {
			await persist();
		} catch {
			enabled = !v;
		}
	}

	// ── Counter editor ──────────────────────────────────────────────────────
	let counterOpen = $state(false);
	let editCounter = $state<StatsCounter | null>(null);
	function openCounter(c: StatsCounter | null) {
		editCounter = c;
		counterOpen = true;
	}
	async function saveCounter(c: StatsCounter) {
		const prev = counters;
		counters = counters.some((x) => x.id === c.id) ? counters.map((x) => (x.id === c.id ? c : x)) : [...counters, c];
		try {
			await persist();
		} catch (e) {
			counters = prev;
			throw e;
		}
	}
	async function toggleCounter(c: StatsCounter, v: boolean) {
		const prev = counters;
		counters = counters.map((x) => (x.id === c.id ? { ...x, enabled: v } : x));
		try {
			await persist();
		} catch {
			counters = prev;
		}
	}

	// ── Milestone editor ────────────────────────────────────────────────────
	let milestoneOpen = $state(false);
	let editMilestone = $state<StatsMilestone | null>(null);
	function openMilestone(m: StatsMilestone | null) {
		editMilestone = m;
		milestoneOpen = true;
	}
	async function saveMilestone(m: StatsMilestone) {
		const prev = milestones;
		milestones = milestones.some((x) => x.id === m.id)
			? milestones.map((x) => (x.id === m.id ? m : x))
			: [...milestones, m];
		try {
			await persist();
		} catch (e) {
			milestones = prev;
			throw e;
		}
	}
	async function toggleMilestone(m: StatsMilestone, v: boolean) {
		const prev = milestones;
		milestones = milestones.map((x) => (x.id === m.id ? { ...x, enabled: v } : x));
		try {
			await persist();
		} catch {
			milestones = prev;
		}
	}

	// ── Delete (shared confirm) ─────────────────────────────────────────────
	let confirmCounter = $state<StatsCounter | null>(null);
	let confirmMilestone = $state<StatsMilestone | null>(null);
	async function doDeleteCounter() {
		const c = confirmCounter;
		confirmCounter = null;
		if (!c) return;
		const prev = counters;
		counters = counters.filter((x) => x.id !== c.id);
		try {
			await persist();
		} catch {
			counters = prev;
		}
	}
	async function doDeleteMilestone() {
		const m = confirmMilestone;
		confirmMilestone = null;
		if (!m) return;
		const prev = milestones;
		milestones = milestones.filter((x) => x.id !== m.id);
		try {
			await persist();
		} catch {
			milestones = prev;
		}
	}

	// ── Presentation helpers ────────────────────────────────────────────────
	const win = $derived(milestoneWindow(milestones, store.memberCount));
	// Progress from the last milestone to the next one, for the overview bar.
	const progress = $derived.by(() => {
		if (!win.next) return 0;
		const span = win.next - win.last;
		if (span <= 0) return 0;
		return Math.min(1, Math.max(0, (store.memberCount - win.last) / span));
	});

	function channelName(id: string): string {
		return store.channels.find((c) => c.id === id)?.name ?? id;
	}
	function counterPreview(tpl: string): string {
		return tpl
			.replace(/\{\{\s*\.Members\s*\}\}/g, store.memberCount.toLocaleString())
			.replace(/\{\{\s*\.Channels\s*\}\}/g, String(store.channels.length))
			.replace(/\{\{\s*\.Roles\s*\}\}/g, String(store.roles.length))
			.replace(/\{\{\s*\.Milestone\s*\}\}/g, win.last.toLocaleString())
			.replace(/\{\{\s*\.NextMilestone\s*\}\}/g, win.next.toLocaleString())
			.replace(/\{\{\s*\.Guild\.Name\s*\}\}/g, store.name)
			.slice(0, 100);
	}
	function flowCount(id: string): number {
		return autos.filter((a) => (a.trigger_config?.milestones ?? []).includes(id)).length;
	}
	function milestoneStatus(m: StatsMilestone): string {
		if (!m.enabled) return 'off';
		const members = store.memberCount;
		if (m.kind === 'at') {
			return members >= m.value ? 'reached' : `${(m.value - members).toLocaleString()} to go`;
		}
		const next = (Math.floor(members / m.value) + 1) * m.value;
		return `next at ${next.toLocaleString()}`;
	}
</script>

<svelte:head><title>Server Stats · {store.name} · Dia</title></svelte:head>

<div class="relative flex h-full flex-col bg-bg text-ink">
	<PageTopbar eyebrow="Server Stats" subtitle="Live counters as channel names, plus member milestones that fire automations.">
		{#snippet leading()}
			<div class="grid size-6 place-items-center rounded border border-line bg-surface text-muted">
				<BarChart3 size={13} />
			</div>
		{/snippet}
		{#snippet actions()}
			<a
				href={`/servers/${store.id}/automations/stats.milestone`}
				class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line bg-bg px-2.5 text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink"
				title="Open the built-in milestone flow on the automations canvas"
			>
				<Zap size={13} /> Advanced
			</a>
			<label class="ml-1 flex items-center gap-2 text-[12px]">
				<span class="hidden text-muted sm:inline">{enabled ? 'On' : 'Off'}</span>
				<Toggle bind:checked={enabled} label="Server stats" onchange={toggleFeature} />
			</label>
		{/snippet}
	</PageTopbar>

	<div class="relative min-h-0 flex-1 overflow-y-auto bg-bg pb-16">
		{#if loadErr}
			<div class="px-5 py-8 text-[12.5px] text-danger">{loadErr}</div>
		{:else if !loaded}
			<div class="p-6"><div class="skeleton h-40 w-full rounded"></div></div>
		{:else}
			<!-- ── Overview ──────────────────────────────────────────────────── -->
			<div class="grid grid-cols-2 border-b border-line/60 sm:grid-cols-4">
				<div class="border-r border-line/60 px-5 py-4">
					<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Members</div>
					<div class="mt-1 font-mono text-[22px] font-semibold leading-none text-ink">
						{store.memberCount.toLocaleString()}
					</div>
				</div>
				<div class="px-5 py-4 sm:border-r sm:border-line/60">
					<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Channels</div>
					<div class="mt-1 font-mono text-[22px] font-semibold leading-none text-ink">{store.channels.length}</div>
				</div>
				<div class="border-r border-line/60 border-t border-line/60 px-5 py-4 sm:border-t-0">
					<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Roles</div>
					<div class="mt-1 font-mono text-[22px] font-semibold leading-none text-ink">{store.roles.length}</div>
				</div>
				<div class="border-t border-line/60 px-5 py-4 sm:border-t-0">
					<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Next milestone</div>
					{#if win.next}
						<div class="mt-1 font-mono text-[22px] font-semibold leading-none text-ink">{win.next.toLocaleString()}</div>
						<div class="mt-2 h-1 w-full overflow-hidden rounded-full bg-line/60">
							<div class="h-full rounded-full bg-accent transition-[width] duration-500" style="width: {Math.round(progress * 100)}%"></div>
						</div>
					{:else}
						<div class="mt-1 text-[12px] text-muted">No milestones set</div>
					{/if}
				</div>
			</div>

			<!-- ── Counters ──────────────────────────────────────────────────── -->
			<SectionBar label="Counters" count={`${counters.length}`}>
				<button
					type="button"
					onclick={() => openCounter(null)}
					class="inline-flex h-7 items-center gap-1.5 rounded-md border border-line bg-bg px-2 text-[11.5px] font-medium text-muted hover:border-line-strong hover:text-ink"
				>
					<Plus size={12} /> New counter
				</button>
			</SectionBar>
			{#if counters.length === 0}
				<EmptyBlock
					title="No counters yet"
					body="A counter renames a locked voice channel with a live value, like the member count. Add one and Dia keeps it current."
				>
					{#snippet cta()}
						<button
							type="button"
							onclick={() => openCounter(null)}
							class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90"
						>
							<Plus size={13} /> Add your first counter
						</button>
					{/snippet}
				</EmptyBlock>
			{:else}
				{#each counters as c (c.id)}
					<div class="flex items-center gap-3 border-b border-line/60 px-5 py-3.5 {c.enabled ? '' : 'opacity-55'}">
						<span class="grid size-8 shrink-0 place-items-center rounded-md border border-line bg-surface text-muted">
							<Volume2 size={15} />
						</span>
						<button type="button" onclick={() => openCounter(c)} class="group min-w-0 flex-1 text-left">
							<div class="truncate text-[12.5px] font-medium text-ink group-hover:underline">
								{counterPreview(c.template)}
							</div>
							<div class="mt-0.5 truncate font-mono text-[10.5px] text-faint">
								#{channelName(c.channel_id)} · {c.template}
							</div>
						</button>
						<button
							type="button"
							onclick={() => openCounter(c)}
							class="grid h-7 w-7 place-items-center rounded-md border border-line bg-bg text-muted hover:border-line-strong hover:text-ink"
							aria-label="Edit counter"
						>
							<Pencil size={12} />
						</button>
						<button
							type="button"
							onclick={() => (confirmCounter = c)}
							class="grid h-7 w-7 place-items-center rounded-md border border-line bg-bg text-muted hover:border-line-strong hover:text-danger"
							aria-label="Remove counter"
						>
							<Trash2 size={12} />
						</button>
						<Toggle checked={c.enabled} label="Counter enabled" onchange={(v) => toggleCounter(c, v)} />
					</div>
				{/each}
			{/if}

			<!-- ── Milestones ────────────────────────────────────────────────── -->
			<SectionBar label="Milestones" count={`${milestones.length}`}>
				<button
					type="button"
					onclick={() => openMilestone(null)}
					class="inline-flex h-7 items-center gap-1.5 rounded-md border border-line bg-bg px-2 text-[11.5px] font-medium text-muted hover:border-line-strong hover:text-ink"
				>
					<Plus size={12} /> New milestone
				</button>
			</SectionBar>
			{#if milestones.length === 0}
				<EmptyBlock
					title="No milestones yet"
					body="A milestone fires an automation event as the member count grows: every 100 members, or a one-time target like 10,000. Connect flows to celebrate."
				>
					{#snippet cta()}
						<button
							type="button"
							onclick={() => openMilestone(null)}
							class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90"
						>
							<Plus size={13} /> Add your first milestone
						</button>
					{/snippet}
				</EmptyBlock>
			{:else}
				{#each milestones as m (m.id)}
					{@const flows = flowCount(m.id)}
					<div class="flex items-center gap-3 border-b border-line/60 px-5 py-3.5 {m.enabled ? '' : 'opacity-55'}">
						<span class="grid size-8 shrink-0 place-items-center rounded-md border border-line bg-surface text-accent-ink">
							<Flag size={15} />
						</span>
						<button type="button" onclick={() => openMilestone(m)} class="group min-w-0 flex-1 text-left">
							<div class="flex items-center gap-2">
								<span class="truncate text-[12.5px] font-medium text-ink group-hover:underline">{milestoneLabel(m)}</span>
								<span class="shrink-0 font-mono text-[10px] text-faint">{milestoneStatus(m)}</span>
							</div>
							<div class="mt-0.5 flex items-center gap-1.5 text-[11.5px] text-muted">
								<Zap size={11} class="shrink-0 text-faint" />
								{flows === 0 ? 'No connected flows' : flows === 1 ? '1 connected flow' : `${flows} connected flows`}
							</div>
						</button>
						<button
							type="button"
							onclick={() => openMilestone(m)}
							class="grid h-7 w-7 place-items-center rounded-md border border-line bg-bg text-muted hover:border-line-strong hover:text-ink"
							aria-label="Edit milestone"
						>
							<Pencil size={12} />
						</button>
						<button
							type="button"
							onclick={() => (confirmMilestone = m)}
							class="grid h-7 w-7 place-items-center rounded-md border border-line bg-bg text-muted hover:border-line-strong hover:text-danger"
							aria-label="Remove milestone"
						>
							<Trash2 size={12} />
						</button>
						<Toggle checked={m.enabled} label="Milestone enabled" onchange={(v) => toggleMilestone(m, v)} />
					</div>
				{/each}
			{/if}

			{#if saveErr}
				<p class="flex items-center gap-1.5 px-5 py-3 text-[12px] text-danger">
					<TriangleAlert size={13} />
					{saveErr}
				</p>
			{/if}

			<!-- ── Bridge to automations ─────────────────────────────────────── -->
			<div class="flex flex-wrap items-center gap-x-2 gap-y-1 border-t border-line/60 px-5 py-3.5 text-[12px] text-muted">
				<Users size={13} class="shrink-0 text-faint" />
				<span>Milestones fire the member milestone trigger; scope a flow to one milestone from its popup.</span>
				<a href={`/servers/${store.id}/automations`} class="font-medium text-accent-ink hover:underline">
					Open automations →
				</a>
			</div>
		{/if}
	</div>
</div>

<CounterEditor bind:open={counterOpen} guildId={store.id} counter={editCounter} milestones={win} onsave={saveCounter} />
<MilestoneEditor bind:open={milestoneOpen} guildId={store.id} milestone={editMilestone} onsave={saveMilestone} />

<ConfirmDialog
	open={!!confirmCounter}
	title="Remove this counter?"
	description="The channel stays; Dia just stops renaming it."
	confirmLabel="Remove"
	onconfirm={doDeleteCounter}
	oncancel={() => (confirmCounter = null)}
/>
<ConfirmDialog
	open={!!confirmMilestone}
	title="Remove this milestone?"
	description="It stops firing immediately. Automations scoped to it stay saved but never match."
	confirmLabel="Remove"
	onconfirm={doDeleteMilestone}
	oncancel={() => (confirmMilestone = null)}
/>
