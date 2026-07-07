<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { fade } from 'svelte/transition';
	import { dur } from '$lib/motion';
	import { goto } from '$app/navigation';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import {
		FEATURE,
		defaultConfig,
		newPresetID,
		type GiveawayConfig,
		type GiveawaySummary,
		type Preset
	} from '$lib/giveaway';
	import ModerationShell, { type ModTab } from '$lib/components/moderation/ModerationShell.svelte';
	import ModSection from '$lib/components/moderation/ModSection.svelte';
	import Field from '$lib/components/Field.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import AccessPreview from '$lib/components/giveaway/AccessPreview.svelte';
	import {
		Gift,
		Settings,
		List,
		Layers,
		Plus,
		Trash2,
		Dices,
		Ban,
		CheckCircle2,
		Pencil,
		Star,
		Copy,
		Lock,
		ExternalLink
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);

	let enabled = $state(false);
	let cfg = $state<GiveawayConfig>(defaultConfig());
	let loaded = $state(false);
	let loadError = $state('');
	let saving = $state(false);
	let baseline = $state('');
	let tab = $state('list');
	let showAccessPreview = $state(false);

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	// ── Giveaway list ────────────────────────────────────────────────────────
	let list = $state<GiveawaySummary[]>([]);
	let busyId = $state('');
	const drafts = $derived(list.filter((g) => g.status === 'draft'));
	const scheduled = $derived(list.filter((g) => g.status === 'scheduled'));
	const running = $derived(list.filter((g) => g.status === 'running'));
	const ended = $derived(list.filter((g) => g.status === 'ended'));
	const liveCount = $derived(drafts.length + scheduled.length + running.length);

	// Presets + Settings edit the feature config (admin-only). A non-admin manager
	// only gets the operational Giveaways list.
	const admin = $derived(store.admin);
	const tabs = $derived<ModTab[]>(
		admin
			? [
					{ key: 'list', label: 'Giveaways', icon: List, badge: liveCount || '' },
					{ key: 'presets', label: 'Presets', icon: Layers, badge: cfg.presets.length || '' },
					{ key: 'settings', label: 'Settings', icon: Settings }
				]
			: [{ key: 'list', label: 'Giveaways', icon: List, badge: liveCount || '' }]
	);

	function mergeConfig(d: GiveawayConfig, c: Partial<GiveawayConfig>): GiveawayConfig {
		return {
			...d,
			...c,
			presets: c.presets?.length ? c.presets : d.presets
		};
	}

	async function load() {
		loadError = '';
		loaded = false;
		try {
			const f = await api.feature(store.id, FEATURE);
			cfg = mergeConfig(defaultConfig(), (f.config ?? {}) as Partial<GiveawayConfig>);
			enabled = f.enabled;
			baseline = JSON.stringify({ enabled, cfg });
			loaded = true;
			loadList();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load giveaways.';
		}
	}
	onMount(load);

	async function loadList() {
		try {
			const r = await api.giveaways(store.id);
			list = (r.giveaways ?? []) as GiveawaySummary[];
		} catch {
			/* the list is best-effort; the config tabs still work */
		}
	}

	async function save() {
		saving = true;
		try {
			await api.saveFeature(store.id, FEATURE, enabled, cfg);
			if (store.detail)
				store.detail.features[FEATURE] = {
					enabled,
					config: cfg as unknown as Record<string, unknown>
				};
			baseline = JSON.stringify({ enabled, cfg });
		} finally {
			saving = false;
		}
	}
	function reset() {
		const b = JSON.parse(baseline);
		enabled = b.enabled;
		cfg = b.cfg;
	}

	async function act(g: GiveawaySummary, fn: () => Promise<unknown>) {
		if (busyId) return;
		busyId = g.id;
		try {
			await fn();
			await loadList();
		} finally {
			busyId = '';
		}
	}

	function editorPath(gwid: string, presetId = ''): string {
		const q = presetId ? `?preset=${encodeURIComponent(presetId)}` : '';
		return `/servers/${store.id}/giveaways/${gwid}${q}`;
	}
	function presetEditorPath(id: string): string {
		return `/servers/${store.id}/giveaways/preset-${id}`;
	}
	function newGiveaway() {
		goto(editorPath('new', cfg.default_preset_id ?? ''));
	}

	// ── Preset library (saved immediately; message/settings edited in the
	// dedicated preset editor so it never feels like creating a giveaway) ──────
	function clone<T>(v: T): T {
		return JSON.parse(JSON.stringify(v));
	}
	let presetBusy = $state(false);
	async function persistPresets(presets: Preset[], defaultId = cfg.default_preset_id) {
		if (presetBusy) return;
		presetBusy = true;
		const next = { ...cfg, presets, default_preset_id: defaultId };
		try {
			await api.saveFeature(store.id, FEATURE, enabled, next);
			if (store.detail)
				store.detail.features[FEATURE] = {
					enabled,
					config: next as unknown as Record<string, unknown>
				};
			cfg = next;
			baseline = JSON.stringify({ enabled, cfg });
		} finally {
			presetBusy = false;
		}
	}
	function duplicatePreset(p: Preset) {
		persistPresets([...cfg.presets, { ...clone(p), id: newPresetID(), name: `${p.name} copy` }]);
	}
	function deletePreset(id: string) {
		const presets = cfg.presets.filter((p) => p.id !== id);
		persistPresets(presets, cfg.default_preset_id === id ? (presets[0]?.id ?? '') : cfg.default_preset_id);
	}
	function makeDefault(id: string) {
		persistPresets(cfg.presets, id);
	}

	// ── Display helpers ──────────────────────────────────────────────────────
	function relTime(iso?: string | null): string {
		if (!iso) return '';
		const diff = (new Date(iso).getTime() - Date.now()) / 1000;
		const abs = Math.abs(diff);
		const suffix = diff >= 0 ? 'from now' : 'ago';
		if (abs < 60) return `${Math.round(abs)}s ${suffix}`;
		if (abs < 3600) return `${Math.round(abs / 60)}m ${suffix}`;
		if (abs < 86400) return `${Math.round(abs / 3600)}h ${suffix}`;
		return `${Math.round(abs / 86400)}d ${suffix}`;
	}
	function jumpLink(g: GiveawaySummary): string {
		return `https://discord.com/channels/${store.id}/${g.channel_id}/${g.message_id}`;
	}
	function label(g: GiveawaySummary): string {
		return g.name?.trim() || g.prize || 'Untitled giveaway';
	}

	const statusChip: Record<string, string> = {
		draft: 'border-line text-muted',
		scheduled: 'border-line text-accent-ink',
		running: 'border-line-strong text-ink',
		ended: 'border-line text-muted',
		cancelled: 'border-line text-faint'
	};

</script>

<svelte:head>
	<title>Giveaways · {store.name} · Dia</title>
</svelte:head>

<ModerationShell
	icon={Gift}
	title="Giveaways"
	blurb="Compose, schedule and draw prize giveaways. Start from a preset, tweak the message like any other embed, and manage them all here."
	toggleLabel="Giveaways"
	bind:enabled
	bind:active={tab}
	{tabs}
	configReadOnly={!admin}
	ready={loaded}
	error={loadError}
	onretry={load}
	{dirty}
	{saving}
	onsave={save}
	onreset={reset}
>
	{#key tab}
	<div in:fade={{ duration: dur(140) }}>
	{#if tab === 'list'}
		<div class="flex items-center justify-between px-4 py-3 sm:px-5">
			<div class="text-[12px] text-muted">
				{running.length} live · {scheduled.length} scheduled · {drafts.length} draft
			</div>
			<button
				type="button"
				onclick={newGiveaway}
				class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90"
			>
				<Plus size={14} /> New giveaway
			</button>
		</div>

		{#if list.length === 0}
			<div class="px-5 py-16 text-center">
				<p class="text-[13px] font-medium text-ink">No giveaways yet</p>
				<p class="mt-1 text-[12px] text-muted">
					Create one with <span class="font-medium">New giveaway</span>, or start one from a Custom
					Command / Automation with the “Start giveaway” step.
				</p>
			</div>
		{:else}
			<div class="divide-y divide-line">
				{#each [...drafts, ...scheduled, ...running, ...ended, ...list.filter((g) => g.status === 'cancelled')] as g (g.id)}
					<div class="flex items-center gap-3 px-4 py-3 sm:px-5">
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<span class="truncate text-[13px] font-semibold text-ink">{label(g)}</span>
								<span
									class="shrink-0 rounded-full border px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide {statusChip[
										g.status
									] ?? 'border-line text-muted'}"
								>
									{g.status}
								</span>
							</div>
							<div class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] text-muted">
								{#if g.status === 'scheduled'}
									<span>starts {relTime(g.starts_at)}</span>
								{:else if g.status === 'running'}
									<span>ends {relTime(g.ends_at)}</span>
								{:else if g.status === 'ended'}
									<span>ended {relTime(g.ended_at)}</span>
								{:else if g.status === 'draft'}
									<span>draft</span>
								{/if}
								<span>{g.entry_count} entries</span>
								<span>{g.winner_count} winner(s)</span>
								{#if g.message_id}
									<a
										class="inline-flex items-center gap-0.5 text-accent-ink hover:underline"
										href={jumpLink(g)}
										target="_blank"
										rel="noreferrer">jump <ExternalLink size={11} /></a
									>
								{/if}
							</div>
						</div>
						<div class="flex shrink-0 items-center gap-1.5">
							{#if g.status !== 'ended' && g.status !== 'cancelled'}
								<a
									href={editorPath(g.id)}
									class="inline-flex h-7 items-center gap-1 rounded-md border border-line px-2.5 text-[12px] font-medium text-ink hover:border-line-strong"
								>
									<Pencil size={12} /> Edit
								</a>
							{/if}
							{#if g.status === 'running'}
								<button
									type="button"
									disabled={busyId === g.id}
									onclick={() => act(g, () => api.endGiveaway(store.id, g.id))}
									class="inline-flex h-7 items-center gap-1 rounded-md border border-line-strong px-2.5 text-[12px] font-medium text-ink hover:bg-ink-2 disabled:opacity-50"
								>
									<CheckCircle2 size={13} /> End
								</button>
							{/if}
							{#if g.status === 'running' || g.status === 'scheduled'}
								<button
									type="button"
									disabled={busyId === g.id}
									onclick={() => act(g, () => api.cancelGiveaway(store.id, g.id))}
									class="inline-flex h-7 items-center gap-1 rounded-md border border-line px-2.5 text-[12px] font-medium text-muted hover:border-line-strong hover:text-danger disabled:opacity-50"
								>
									<Ban size={13} /> Cancel
								</button>
							{/if}
							{#if g.status === 'ended'}
								<button
									type="button"
									disabled={busyId === g.id}
									onclick={() => act(g, () => api.rerollGiveaway(store.id, g.id, 0))}
									class="inline-flex h-7 items-center gap-1 rounded-md border border-line-strong px-2.5 text-[12px] font-medium text-ink hover:bg-ink-2 disabled:opacity-50"
								>
									<Dices size={13} /> Reroll
								</button>
							{/if}
							{#if g.status === 'draft'}
								<button
									type="button"
									disabled={busyId === g.id}
									onclick={() => act(g, () => api.deleteGiveaway(store.id, g.id))}
									class="grid size-7 shrink-0 place-items-center rounded-md border border-line text-muted hover:border-line-strong hover:text-danger disabled:opacity-50"
									aria-label="Delete draft"
								>
									<Trash2 size={13} />
								</button>
							{/if}
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{:else if tab === 'presets'}
		<ModSection label="Preset library" desc="Reusable templates new giveaways start from. Click a preset to compose its message and settings.">
			{#snippet actions()}
				<a
					href={presetEditorPath('new')}
					class="inline-flex h-7 items-center gap-1.5 rounded-md bg-ink px-2.5 text-[12px] font-semibold text-bg hover:bg-ink/90"
				>
					<Plus size={13} /> New preset
				</a>
			{/snippet}
			<div class="divide-y divide-line">
				{#each cfg.presets as p (p.id)}
					<a
						href={presetEditorPath(p.id)}
						class="group flex items-center gap-3 px-1 py-2.5 hover:bg-ink-2/40"
					>
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<span class="truncate text-[13px] font-semibold text-ink">{p.name}</span>
								{#if cfg.default_preset_id === p.id}
									<span class="shrink-0 rounded-full border border-line px-1.5 font-mono text-[10px] uppercase tracking-wide text-accent-ink">default</span>
								{/if}
							</div>
							<div class="mt-0.5 flex items-center gap-2 text-[11px] text-muted">
								<span>{p.default_duration || '24h'}</span>
								<span>· {p.default_winner_count || 1} winner(s)</span>
							</div>
						</div>
						<div class="flex shrink-0 items-center gap-1.5 opacity-70 group-hover:opacity-100">
							<span class="hidden items-center gap-1 text-[12px] font-medium text-muted sm:inline-flex"><Pencil size={12} /> Edit</span>
							<button
								type="button"
								onclick={(e) => { e.preventDefault(); makeDefault(p.id); }}
								disabled={presetBusy || cfg.default_preset_id === p.id}
								class="grid size-7 place-items-center rounded-md border border-line text-muted hover:border-line-strong hover:text-ink disabled:opacity-30"
								aria-label="Make default"
							>
								<Star size={13} />
							</button>
							<button
								type="button"
								onclick={(e) => { e.preventDefault(); duplicatePreset(p); }}
								disabled={presetBusy}
								class="grid size-7 place-items-center rounded-md border border-line text-muted hover:border-line-strong hover:text-ink disabled:opacity-30"
								aria-label="Duplicate"
							>
								<Copy size={13} />
							</button>
							<button
								type="button"
								onclick={(e) => { e.preventDefault(); deletePreset(p.id); }}
								disabled={presetBusy || cfg.presets.length <= 1}
								class="grid size-7 place-items-center rounded-md border border-line text-muted hover:border-line-strong hover:text-danger disabled:opacity-30"
								aria-label="Delete preset"
							>
								<Trash2 size={13} />
							</button>
						</div>
					</a>
				{/each}
			</div>
		</ModSection>
	{:else if tab === 'settings'}
		<ModSection label="Access" desc="Roles allowed to create and manage giveaways (server admins always can).">
			<Field label="Manager roles" hint="These roles can open and use the Giveaways tab.">
				<RolePicker
					value={cfg.manager_roles ?? []}
					multiple
					onChange={(v) => (cfg.manager_roles = v as string[])}
				/>
			</Field>
			<div class="mt-3 flex items-center gap-3">
				<button
					type="button"
					onclick={() => (showAccessPreview = true)}
					class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line px-3 text-[12px] font-medium text-ink hover:border-line-strong"
				>
					<Lock size={13} /> Preview restricted view
				</button>
				<span class="text-[11px] text-muted">See what members without a manager role get.</span>
			</div>
		</ModSection>
	{/if}
	</div>
	{/key}
</ModerationShell>

<AccessPreview bind:open={showAccessPreview} featureName="Giveaways" />
