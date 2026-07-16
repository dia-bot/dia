<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import { STATS_TEMPLATE_VARS, newCounterId, type StatsConfig, type StatsCounter } from '$lib/stats';
	import Toggle from '$lib/components/Toggle.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import EmptyBlock from '$lib/components/page/EmptyBlock.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import { BarChart3, Plus, Trash2, Zap, Loader2, TriangleAlert } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'stats';

	let enabled = $state(false);
	let counters = $state<StatsCounter[]>([]);
	let milestoneStep = $state(100);
	let loaded = $state(false);
	let loadErr = $state('');
	let baseline = $state('');
	let saving = $state(false);
	let saveErr = $state('');
	let savedTick = $state(false);
	let confirmDelete = $state<StatsCounter | null>(null);
	let creatingChannel = $state('');

	function serialize(): string {
		return JSON.stringify({ counters, milestone_step: milestoneStep });
	}
	const dirty = $derived(loaded && serialize() !== baseline);

	onMount(async () => {
		try {
			const f = await api.feature(store.id, FEATURE);
			enabled = f.enabled;
			const cfg = (f.config ?? {}) as StatsConfig;
			counters = cfg.counters ?? [];
			milestoneStep = cfg.milestone_step ?? 100;
			loaded = true;
			baseline = serialize();
		} catch (e) {
			loadErr = e instanceof Error ? e.message : 'Failed to load server stats';
		}
	});

	async function save() {
		if (saving || !dirty) return;
		saving = true;
		saveErr = '';
		try {
			// tail is canvas-owned; the API merges the stored copy back.
			await api.saveFeature(store.id, FEATURE, enabled, {
				counters: $state.snapshot(counters),
				milestone_step: milestoneStep
			});
			baseline = serialize();
			savedTick = true;
			setTimeout(() => (savedTick = false), 1500);
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Could not save';
		} finally {
			saving = false;
		}
	}

	async function toggleFeature(v: boolean) {
		try {
			await api.saveFeature(store.id, FEATURE, v, {
				counters: $state.snapshot(counters),
				milestone_step: milestoneStep
			});
		} catch {
			enabled = !v;
		}
	}

	function addCounter() {
		counters = [
			...counters,
			{ id: newCounterId(), channel_id: '', template: '📊 Members: {{ .Members }}', enabled: true }
		];
	}

	async function createChannel(c: StatsCounter) {
		if (creatingChannel) return;
		creatingChannel = c.id;
		saveErr = '';
		try {
			const r = await api.createStatsChannel(store.id, previewName(c.template));
			counters = counters.map((x) => (x.id === c.id ? { ...x, channel_id: r.channel_id } : x));
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Could not create the channel';
		} finally {
			creatingChannel = '';
		}
	}

	function doDelete() {
		const c = confirmDelete;
		if (!c) return;
		counters = counters.filter((x) => x.id !== c.id);
	}

	// previewName approximates the rendered channel name with sample values so
	// edits read live; the worker renders the real template.
	function previewName(tpl: string): string {
		return tpl
			.replace(/\{\{\s*\.Members\s*\}\}/g, '1,024')
			.replace(/\{\{\s*\.Channels\s*\}\}/g, '42')
			.replace(/\{\{\s*\.Roles\s*\}\}/g, '18')
			.replace(/\{\{\s*\.Milestone\s*\}\}/g, '1,000')
			.replace(/\{\{\s*\.Guild\.Name\s*\}\}/g, store.name)
			.slice(0, 100);
	}

	function insertVar(c: StatsCounter, path: string) {
		counters = counters.map((x) => (x.id === c.id ? { ...x, template: x.template + `{{ ${path} }}` } : x));
	}
</script>

<svelte:head><title>Server Stats · {store.name} · Dia</title></svelte:head>

<div class="relative flex h-full flex-col bg-bg text-ink">
	<PageTopbar eyebrow="Server Stats" subtitle="Live counters as channel names, refreshed automatically.">
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
			{#if dirty || saving || savedTick}
				<button
					type="button"
					onclick={save}
					disabled={saving}
					class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90 disabled:opacity-60"
				>
					{#if saving}<Loader2 size={13} class="animate-spin" />{/if}
					{savedTick ? 'Saved' : 'Save'}
				</button>
			{/if}
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
			<SectionBar label="Counters" count={`${counters.length}`}>
				<button
					type="button"
					onclick={addCounter}
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
							onclick={addCounter}
							class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90"
						>
							<Plus size={13} /> Add your first counter
						</button>
					{/snippet}
				</EmptyBlock>
			{:else}
				{#each counters as c (c.id)}
					<div class="border-b border-line/60 px-5 py-4 {c.enabled ? '' : 'opacity-55'}">
						<div class="grid gap-x-8 gap-y-3 lg:grid-cols-2">
							<div class="min-w-0">
								<div class="mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
									Channel name template
								</div>
								<input
									type="text"
									value={c.template}
									oninput={(e) =>
										(counters = counters.map((x) =>
											x.id === c.id ? { ...x, template: (e.currentTarget as HTMLInputElement).value } : x
										))}
									class="input w-full font-mono text-[12px]"
									placeholder={'📊 Members: {{ .Members }}'}
								/>
								<div class="mt-1.5 flex flex-wrap items-center gap-1">
									{#each STATS_TEMPLATE_VARS as tv (tv.path)}
										<button
											type="button"
											onclick={() => insertVar(c, tv.path)}
											class="inline-flex h-5 items-center rounded border border-line bg-surface px-1.5 font-mono text-[10px] text-muted hover:border-line-strong hover:text-ink"
											title={tv.short}
										>
											{`{{ ${tv.path} }}`}
										</button>
									{/each}
								</div>
								<p class="mt-1.5 font-mono text-[10.5px] text-faint">Preview: {previewName(c.template)}</p>
							</div>
							<div class="min-w-0">
								<div class="mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
									Channel
								</div>
								<div class="flex items-center gap-2">
									<div class="min-w-0 flex-1">
										<ChannelPicker
											kind="all"
											value={c.channel_id}
											onChange={(v) =>
												(counters = counters.map((x) => (x.id === c.id ? { ...x, channel_id: v as string } : x)))}
											placeholder="Pick or create a channel"
										/>
									</div>
									<button
										type="button"
										onclick={() => createChannel(c)}
										disabled={!!creatingChannel}
										class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border border-line bg-bg px-2.5 text-[11.5px] font-medium text-muted hover:border-line-strong hover:text-ink disabled:opacity-50"
										title="Create a locked voice channel for this counter"
									>
										{#if creatingChannel === c.id}<Loader2 size={12} class="animate-spin" />{:else}<Plus size={12} />{/if}
										Create for me
									</button>
								</div>
								<div class="mt-3 flex items-center justify-between gap-3">
									<span class="text-[12px] text-muted">Updates at most twice per 10 minutes (Discord limit).</span>
									<span class="flex items-center gap-2">
										<button
											type="button"
											onclick={() => (confirmDelete = c)}
											class="grid h-7 w-7 place-items-center rounded-md border border-line bg-bg text-muted hover:border-line-strong hover:text-danger"
											aria-label="Remove counter"
										>
											<Trash2 size={12} />
										</button>
										<Toggle
											checked={c.enabled}
											label="Counter enabled"
											onchange={(v) =>
												(counters = counters.map((x) => (x.id === c.id ? { ...x, enabled: v } : x)))}
										/>
									</span>
								</div>
							</div>
						</div>
					</div>
				{/each}
			{/if}

			<SectionBar label="Milestones" />
			<div class="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-line/60 px-5 py-4">
				<span class="text-[12.5px] text-ink">Fire an automation event every</span>
				<input
					type="number"
					min="0"
					step="50"
					bind:value={milestoneStep}
					class="input w-24 text-center font-mono text-[12px]"
				/>
				<span class="text-[12.5px] text-ink">members.</span>
				<span class="text-[12px] text-muted">
					0 turns it off. Build the celebration on the member milestone trigger.
				</span>
				<a href={`/servers/${store.id}/automations/stats.milestone`} class="text-[12px] font-medium text-accent-ink hover:underline">
					Open the flow →
				</a>
			</div>

			{#if saveErr}
				<p class="flex items-center gap-1.5 px-5 py-3 text-[12px] text-danger">
					<TriangleAlert size={13} />
					{saveErr}
				</p>
			{/if}
		{/if}
	</div>
</div>

<ConfirmDialog
	open={!!confirmDelete}
	title="Remove this counter?"
	description="The channel stays; Dia just stops renaming it. Save to apply."
	confirmLabel="Remove"
	onconfirm={doDelete}
	oncancel={() => (confirmDelete = null)}
/>
