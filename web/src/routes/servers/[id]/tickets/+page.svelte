<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import {
		defaultTicketsConfig,
		defaultSystemMessages,
		SYSTEM_MESSAGE_META,
		type TicketsConfig,
		type PanelSummary,
		type TicketRow,
		type TicketStats
	} from '$lib/tickets/types';
	import ModerationShell, { type ModTab } from '$lib/components/moderation/ModerationShell.svelte';
	import ModSection from '$lib/components/moderation/ModSection.svelte';
	import TabSwipe from '$lib/components/page/TabSwipe.svelte';
	import Field from '$lib/components/Field.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import NumberField from '$lib/components/ui/NumberField.svelte';
	import Select from '$lib/components/Select.svelte';
	import {
		Ticket,
		LayoutList,
		ListChecks,
		BarChart3,
		Settings,
		Plus,
		Trash2,
		Pencil,
		RefreshCw,
		ExternalLink
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'tickets';

	let enabled = $state(false);
	let cfg = $state<TicketsConfig>(defaultTicketsConfig());
	let loaded = $state(false);
	let loadError = $state('');
	let saving = $state(false);
	let baseline = $state('');
	let tab = $state('setups');

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);
	const inputCls =
		'w-full rounded-md border border-line bg-bg px-3 py-2 text-sm text-ink outline-none focus:border-line-strong';

	// Ticket setups (panels) — listed here, edited on their own full page.
	let panels = $state<PanelSummary[]>([]);
	let panelsLoaded = $state(false);
	let panelsError = $state('');
	let panelBusy = $state('');

	const tabs = $derived<ModTab[]>([
		{ key: 'setups', label: 'Setups', icon: LayoutList, badge: panels.length || '' },
		{ key: 'queue', label: 'Queue', icon: ListChecks },
		{ key: 'analytics', label: 'Analytics', icon: BarChart3 },
		{ key: 'settings', label: 'Settings', icon: Settings }
	]);

	async function load() {
		loadError = '';
		loaded = false;
		try {
			const f = await api.feature(store.id, FEATURE);
			const raw = (f.config as Partial<TicketsConfig>) ?? {};
			cfg = {
				...defaultTicketsConfig(),
				...raw,
				messages: { ...defaultSystemMessages(), ...(raw.messages ?? {}) }
			};
			enabled = f.enabled;
			baseline = JSON.stringify({ enabled, cfg });
			loaded = true;
			loadPanels();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load tickets.';
		}
	}
	onMount(load);

	async function save() {
		saving = true;
		try {
			await api.saveFeature(store.id, FEATURE, enabled, cfg);
			if (store.detail)
				store.detail.features[FEATURE] = { enabled, config: cfg as unknown as Record<string, unknown> };
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

	async function loadPanels() {
		panelsError = '';
		try {
			const r = await api.ticketPanels(store.id);
			panels = (r.panels ?? []) as PanelSummary[];
		} catch (e) {
			panelsError = e instanceof Error ? e.message : 'Could not load ticket setups.';
		} finally {
			panelsLoaded = true;
		}
	}

	function editorPath(pid: string): string {
		return `/servers/${store.id}/tickets/${pid}`;
	}
	function newSetup() {
		goto(editorPath('new'));
	}
	async function deletePanel(p: PanelSummary) {
		if (!confirm(`Delete the "${p.name || 'untitled'}" setup? Existing tickets stay open.`)) return;
		if (panelBusy) return;
		panelBusy = p.id;
		try {
			await api.deleteTicketPanel(store.id, p.id);
			await loadPanels();
		} finally {
			panelBusy = '';
		}
	}
	function isPosted(p: PanelSummary): boolean {
		return p.message_id !== '0' && p.message_id !== '' && p.channel_id !== '0' && p.channel_id !== '';
	}
	function panelLink(p: PanelSummary): string {
		return `https://discord.com/channels/${store.id}/${p.channel_id}/${p.message_id}`;
	}

	// ── Queue ─────────────────────────────────────────────────
	let tickets = $state<TicketRow[]>([]);
	let queueLoaded = $state(false);
	let queueError = $state('');
	let queueFilter = $state('open');

	async function loadQueue() {
		queueError = '';
		try {
			const r = await api.tickets(store.id, queueFilter);
			tickets = (r.tickets ?? []) as TicketRow[];
		} catch (e) {
			queueError = e instanceof Error ? e.message : 'Could not load tickets.';
		} finally {
			queueLoaded = true;
		}
	}
	async function closeTicket(t: TicketRow) {
		if (!confirm(`Close ticket #${t.number}?`)) return;
		try {
			await api.closeTicket(store.id, t.id);
			await loadQueue();
		} catch {
			/* best-effort */
		}
	}

	// ── Analytics ─────────────────────────────────────────────
	let stats = $state<TicketStats | null>(null);
	let statsLoaded = $state(false);

	async function loadStats() {
		try {
			const r = await api.ticketStats(store.id);
			stats = r.stats as TicketStats;
		} catch {
			stats = null;
		} finally {
			statsLoaded = true;
		}
	}

	// Lazy-load analytics the first time its tab is opened. The loader only ever
	// flips its *Loaded flag true, so this never re-fires itself.
	$effect(() => {
		if (tab === 'analytics' && !statsLoaded) loadStats();
	});
	// The queue reloads on open and whenever the status filter changes; loadQueue
	// touches neither `tab` nor `queueFilter`, so there is no loop.
	$effect(() => {
		queueFilter; // track filter changes
		if (tab === 'queue') loadQueue();
	});

	function channelLink(channelId: string) {
		return `https://discord.com/channels/${store.id}/${channelId}`;
	}
	function fmtDuration(seconds: number): string {
		if (!seconds || seconds <= 0) return '—';
		const s = Math.round(seconds);
		if (s < 60) return `${s}s`;
		const m = Math.round(s / 60);
		if (m < 60) return `${m}m`;
		const h = Math.floor(m / 60);
		const rem = m % 60;
		if (h < 24) return rem ? `${h}h ${rem}m` : `${h}h`;
		return `${Math.floor(h / 24)}d ${h % 24}h`;
	}
	function fmtDate(s: string | null): string {
		if (!s) return '—';
		try {
			return new Date(s).toLocaleString();
		} catch {
			return s;
		}
	}
</script>

<svelte:head>
	<title>Tickets · {store.name} · Dia</title>
</svelte:head>

<ModerationShell
	icon={Ticket}
	title="Tickets"
	blurb="Let members open private support tickets from a panel. Fully customizable channels, messages, forms, claiming, close requests, transcripts, ratings and auto-close."
	bind:enabled
	ready={loaded}
	error={loadError}
	onretry={load}
	toggleLabel="Tickets"
	{tabs}
	bind:active={tab}
	{dirty}
	{saving}
	onsave={save}
	onreset={reset}
>
	<TabSwipe key={tab} index={tabs.findIndex((t) => t.key === tab)}>
		{#if tab === 'setups'}
			<div class="flex items-center justify-between px-4 py-3 sm:px-5">
				<div class="text-[12px] text-muted">
					{panels.length} setup{panels.length === 1 ? '' : 's'} · each one is a panel message with its own ticket types
				</div>
				<button
					type="button"
					onclick={newSetup}
					class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90"
				>
					<Plus size={14} /> New setup
				</button>
			</div>

			{#if !panelsLoaded}
				<div class="px-5 py-16 text-center text-[13px] text-muted">Loading…</div>
			{:else if panelsError}
				<div class="px-5 py-16 text-center text-[13px] text-accent-ink">{panelsError}</div>
			{:else if panels.length === 0}
				<div class="px-5 py-16 text-center">
					<p class="text-[13px] font-medium text-ink">No ticket setups yet</p>
					<p class="mt-1 text-[12px] text-muted">
						Create one with <span class="font-medium">New setup</span>: compose the panel message,
						add ticket types, and publish it to a channel. Members click a button and get their own
						private channel with your team.
					</p>
				</div>
			{:else}
				<div class="divide-y divide-line">
					{#each panels as p (p.id)}
						<div class="flex items-center gap-3 px-4 py-3 sm:px-5">
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-2">
									<span class="truncate text-[13px] font-semibold text-ink">{p.name || '(untitled)'}</span>
									<span
										class="shrink-0 rounded-full border border-line px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide {isPosted(p) ? 'text-accent-ink' : 'text-muted'}"
									>
										{isPosted(p) ? 'posted' : 'not posted'}
									</span>
									{#if !p.enabled}
										<span class="shrink-0 rounded-full border border-line px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide text-faint">disabled</span>
									{/if}
								</div>
								<div class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] text-muted">
									<span>{p.config?.categories?.length ?? 0} ticket type{(p.config?.categories?.length ?? 0) === 1 ? '' : 's'}</span>
									<span>{p.style === 'select' ? 'dropdown' : 'buttons'}</span>
									{#if isPosted(p)}
										<a
											class="inline-flex items-center gap-0.5 text-accent-ink hover:underline"
											href={panelLink(p)}
											target="_blank"
											rel="noreferrer">jump <ExternalLink size={11} /></a
										>
									{/if}
								</div>
							</div>
							<div class="flex shrink-0 items-center gap-1.5">
								<a
									href={editorPath(p.id)}
									class="inline-flex h-7 items-center gap-1 rounded-md border border-line px-2.5 text-[12px] font-medium text-ink hover:border-line-strong"
								>
									<Pencil size={12} /> Edit
								</a>
								<button
									type="button"
									disabled={panelBusy === p.id}
									onclick={() => deletePanel(p)}
									class="grid size-7 shrink-0 place-items-center rounded-md border border-line text-muted hover:border-line-strong hover:text-danger disabled:opacity-50"
									aria-label="Delete setup"
								>
									<Trash2 size={13} />
								</button>
							</div>
						</div>
					{/each}
				</div>
			{/if}
		{:else if tab === 'queue'}
			<ModSection label="Ticket queue" desc="Live and closed tickets across the server.">
				{#snippet actions()}
					<div class="flex items-center gap-2">
						<div class="w-32">
							<Select bind:value={queueFilter} dense options={[{ value: 'open', label: 'Open' }, { value: 'closed', label: 'Closed' }, { value: '', label: 'All' }]} />
						</div>
						<button type="button" class="text-muted hover:text-ink" title="Refresh" onclick={loadQueue}><RefreshCw class="h-4 w-4" /></button>
					</div>
				{/snippet}
				{#key queueFilter}
					{#if !queueLoaded}
						<p class="text-sm text-muted">Loading…</p>
					{:else if queueError}
						<p class="text-sm text-accent-ink">{queueError}</p>
					{:else if tickets.length === 0}
						<p class="text-sm text-muted">No tickets to show.</p>
					{:else}
						<div class="space-y-2">
							{#each tickets as t (t.id)}
								<div class="flex items-center gap-4 rounded-lg border border-line bg-surface px-4 py-3">
									<div class="flex-1">
										<div class="flex items-center gap-2">
											<span class="font-mono text-sm text-ink">#{t.number}</span>
											<span class="text-sm text-ink">{t.category_label || 'Ticket'}</span>
											<span class="rounded bg-line px-1.5 py-0.5 text-[10px] uppercase text-muted">{t.status}</span>
											{#if t.rating > 0}<span class="text-xs text-accent-ink">{'★'.repeat(t.rating)}</span>{/if}
										</div>
										<p class="truncate text-xs text-muted">
											{t.subject || 'No subject'} · opened {fmtDate(t.opened_at)}
										</p>
									</div>
									{#if t.channel_id !== '0'}
										<a class="text-muted hover:text-ink" href={channelLink(t.channel_id)} target="_blank" rel="noreferrer" title="Open in Discord"><ExternalLink class="h-4 w-4" /></a>
									{/if}
									{#if t.status === 'open'}
										<button type="button" class="rounded-md border border-line px-2.5 py-1 text-xs text-ink hover:border-line-strong" onclick={() => closeTicket(t)}>Close</button>
									{/if}
								</div>
							{/each}
						</div>
					{/if}
				{/key}
			</ModSection>
		{:else if tab === 'analytics'}
			<ModSection label="Analytics" desc="Volume and response quality.">
				{#if !statsLoaded}
					<p class="text-sm text-muted">Loading…</p>
				{:else if !stats}
					<p class="text-sm text-muted">No analytics available yet.</p>
				{:else}
					<div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
						{#each [
							{ label: 'Open now', value: stats.open },
							{ label: 'Closed', value: stats.closed },
							{ label: 'Total', value: stats.total },
							{ label: 'Opened (7d)', value: stats.opened_7d },
							{ label: 'Closed (7d)', value: stats.closed_7d },
							{ label: 'Avg rating', value: stats.rated > 0 ? stats.avg_rating.toFixed(1) + ' ★' : '—' },
							{ label: 'Avg first response', value: fmtDuration(stats.avg_first_response_seconds) },
							{ label: 'Avg resolution', value: fmtDuration(stats.avg_resolution_seconds) }
						] as tile (tile.label)}
							<div class="rounded-lg border border-line bg-surface p-4">
								<p class="eyebrow">{tile.label}</p>
								<p class="mt-1 text-2xl font-semibold text-ink">{tile.value}</p>
							</div>
						{/each}
					</div>
				{/if}
			</ModSection>
		{:else if tab === 'settings'}
			<ModSection label="Staff & channels" desc="Who handles tickets and where activity is logged. Where each ticket's channel goes and how it is named is set per ticket type.">
				<div class="grid gap-4 sm:grid-cols-2">
					<Field label="Support staff roles" hint="Added to every ticket and allowed to run ticket commands">
						<RolePicker multiple value={cfg.staff_role_ids} onChange={(v) => (cfg.staff_role_ids = v as string[])} />
					</Field>
					<Field label="Max open tickets per member" hint="0 = unlimited; a ticket type can tighten this">
						<NumberField bind:value={cfg.max_open_per_user} min={0} />
					</Field>
					<Field label="Log channel" hint="Open / claim / close / delete events">
						<ChannelSelect bind:value={cfg.log_channel} placeholder="No log channel" />
					</Field>
					<Field label="Transcript channel" hint="Where closing transcripts are posted (defaults to the log channel)">
						<ChannelSelect bind:value={cfg.transcript_channel} placeholder="Use log channel" />
					</Field>
				</div>
			</ModSection>

			<ModSection label="Blacklist" desc="Roles and members that can never open a ticket.">
				<div class="grid gap-4 sm:grid-cols-2">
					<Field label="Blacklisted roles">
						<RolePicker multiple value={cfg.blacklist_role_ids} onChange={(v) => (cfg.blacklist_role_ids = v as string[])} />
					</Field>
				</div>
			</ModSection>

			<ModSection
				label="System replies"
				desc="Every short reply the bot sends around tickets. Templates work; leave a field empty to keep the built-in text."
			>
				<div class="grid gap-4 sm:grid-cols-2">
					{#each SYSTEM_MESSAGE_META as meta (meta.key)}
						<Field label={meta.label} hint={meta.hint}>
							<input class={inputCls} bind:value={cfg.messages[meta.key]} placeholder={meta.placeholder} />
						</Field>
					{/each}
				</div>
			</ModSection>
		{/if}
	</TabSwipe>
</ModerationShell>
