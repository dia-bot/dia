<script lang="ts">
	import { onMount, getContext, setContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api, ApiError } from '$lib/api';
	import {
		defaultTicketsConfig,
		defaultPanelConfig,
		defaultControlButtons,
		newCategory,
		normalizeMessageSpec,
		TICKET_TEMPLATE_VARS,
		type TicketsConfig,
		type PanelSummary,
		type PanelConfig,
		type CategoryConfig,
		type ControlButtons,
		type TicketRow,
		type TicketStats
	} from '$lib/tickets/types';
	import type { Step } from '$lib/commands/types';
	import { AUTOMATION_CTX, EXPR_SCOPE_CTX, type ExprScope } from '$lib/commands/expr-meta';
	import ModerationShell, { type ModTab } from '$lib/components/moderation/ModerationShell.svelte';
	import ModSection from '$lib/components/moderation/ModSection.svelte';
	import TabSwipe from '$lib/components/page/TabSwipe.svelte';
	import Field from '$lib/components/Field.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import NumberField from '$lib/components/ui/NumberField.svelte';
	import Select from '$lib/components/Select.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import CategoryEditor from '$lib/components/tickets/CategoryEditor.svelte';
	import {
		Ticket,
		LayoutList,
		ListChecks,
		BarChart3,
		BookOpen,
		Plus,
		Trash2,
		Send,
		ArrowLeft,
		Save,
		RefreshCw,
		ExternalLink
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'tickets';

	// Every MessageEditor on this page (panel + category surfaces) offers the
	// ticket template variables in its picker, in the non-automation form.
	setContext(AUTOMATION_CTX, false);
	setContext(EXPR_SCOPE_CTX, {
		options: [],
		variables: [],
		steps: [],
		extraVars: TICKET_TEMPLATE_VARS
	} satisfies ExprScope);

	let enabled = $state(false);
	let cfg = $state<TicketsConfig>(defaultTicketsConfig());
	let loaded = $state(false);
	let loadError = $state('');
	let saving = $state(false);
	let baseline = $state('');
	let tab = $state('setup');

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);
	const inputCls =
		'w-full rounded-md border border-line bg-bg px-3 py-2 text-sm text-ink outline-none focus:border-line-strong';

	const tabs = $derived<ModTab[]>([
		{ key: 'setup', label: 'Setup', icon: Ticket },
		{ key: 'panels', label: 'Panels', icon: LayoutList },
		{ key: 'queue', label: 'Queue', icon: ListChecks },
		{ key: 'analytics', label: 'Analytics', icon: BarChart3 },
		{ key: 'guide', label: 'How it works', icon: BookOpen }
	]);

	async function load() {
		loadError = '';
		loaded = false;
		try {
			const f = await api.feature(store.id, FEATURE);
			cfg = { ...defaultTicketsConfig(), ...((f.config as Partial<TicketsConfig>) ?? {}) };
			enabled = f.enabled;
			baseline = JSON.stringify({ enabled, cfg });
			loaded = true;
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

	// ── Panels ────────────────────────────────────────────────
	let panels = $state<PanelSummary[]>([]);
	let panelsLoaded = $state(false);
	let panelsError = $state('');
	let editing = $state<PanelSummary | null>(null);
	let panelSaving = $state(false);
	let panelStatus = $state('');
	let publishChannel = $state('');

	// mergeButtons fills in any missing system-button overrides.
	function mergeButtons(b: Partial<ControlButtons> | undefined): ControlButtons {
		const d = defaultControlButtons();
		return {
			claim: { ...d.claim, ...(b?.claim ?? {}) },
			close: { ...d.close, ...(b?.close ?? {}) },
			reopen: { ...d.reopen, ...(b?.reopen ?? {}) },
			delete: { ...d.delete, ...(b?.delete ?? {}) },
			transcript: { ...d.transcript, ...(b?.transcript ?? {}) }
		};
	}
	// normalizeCategory upgrades stored categories to the current shape (folding
	// the legacy single-embed welcome into embeds, mirroring the Go decoder).
	function normalizeCategory(c: Partial<CategoryConfig>): CategoryConfig {
		const base = newCategory();
		return {
			...base,
			...c,
			welcome: normalizeMessageSpec(c.welcome ?? base.welcome),
			closed: normalizeMessageSpec(c.closed),
			close_request: normalizeMessageSpec(c.close_request),
			buttons: mergeButtons(c.buttons),
			transcript: { ...base.transcript, ...(c.transcript ?? {}) },
			feedback: { ...base.feedback, ...(c.feedback ?? {}), message: normalizeMessageSpec(c.feedback?.message) },
			auto_close: {
				...base.auto_close,
				...(c.auto_close ?? {}),
				warn_message: normalizeMessageSpec(c.auto_close?.warn_message)
			},
			form: c.form ?? []
		};
	}
	function normalizeConfig(pc: Partial<PanelConfig> | undefined): PanelConfig {
		const d = defaultPanelConfig();
		let embeds = (pc?.embeds ?? []).map((e) => ({ ...e }));
		if (embeds.length === 0 && pc?.embed && Object.keys(pc.embed).length > 0) {
			embeds = [{ ...pc.embed }]; // legacy single-embed panel
		}
		if (!pc) embeds = d.embeds ?? [];
		return {
			content: pc?.content ?? '',
			embeds,
			select_placeholder: pc?.select_placeholder ?? d.select_placeholder,
			categories: (pc?.categories ?? []).map(normalizeCategory)
		};
	}

	// The panel message is edited with the shared WYSIWYG MessageEditor; the step
	// is (re)seeded when a panel is opened for editing and the effect below syncs
	// edits back into the panel config being edited.
	let panelEditSeq = $state(0);
	let panelStep = $state<Step>({ id: 'panel-msg', kind: 'send_message', spec: { content: '', embeds: [] } });
	function seedPanelStep(pc: PanelConfig) {
		panelStep = {
			id: 'panel-msg',
			kind: 'send_message',
			spec: { content: pc.content ?? '', embeds: JSON.parse(JSON.stringify(pc.embeds ?? [])) }
		};
		panelEditSeq++;
	}
	$effect(() => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const s = panelStep.spec as any;
		if (!editing) return;
		editing.config.content = s.content ?? '';
		editing.config.embeds = s.embeds ?? [];
	});

	async function loadPanels() {
		panelsError = '';
		try {
			const r = await api.ticketPanels(store.id);
			panels = (r.panels ?? []).map((p) => ({ ...p, config: normalizeConfig(p.config) }));
		} catch (e) {
			panelsError = e instanceof Error ? e.message : 'Could not load panels.';
		} finally {
			panelsLoaded = true;
		}
	}

	function newPanel() {
		editing = {
			id: '',
			name: 'Support',
			style: 'buttons',
			enabled: true,
			position: 0,
			channel_id: '',
			message_id: '',
			config: defaultPanelConfig()
		};
		seedPanelStep(editing.config);
		publishChannel = '';
		panelStatus = '';
	}
	function editPanel(p: PanelSummary) {
		editing = { ...p, config: normalizeConfig(p.config) };
		seedPanelStep(editing.config);
		publishChannel = '';
		panelStatus = '';
	}
	function addCategory() {
		if (!editing) return;
		if (editing.config.categories.length >= 25) return;
		editing.config.categories = [...editing.config.categories, newCategory('New category')];
	}
	function removeCategory(i: number) {
		if (!editing) return;
		editing.config.categories = editing.config.categories.filter((_, idx) => idx !== i);
	}

	async function savePanel() {
		if (!editing) return;
		panelSaving = true;
		panelStatus = '';
		try {
			const res = await api.upsertTicketPanel(store.id, {
				id: editing.id,
				name: editing.name,
				style: editing.style,
				enabled: editing.enabled,
				config: editing.config
			});
			editing.id = res.id;
			panelStatus = 'Saved.';
			await loadPanels();
		} catch (e) {
			panelStatus = e instanceof ApiError ? e.message : 'Could not save the panel.';
		} finally {
			panelSaving = false;
		}
	}

	async function publishPanel() {
		if (!editing) return;
		if (!publishChannel) {
			panelStatus = 'Pick a channel to post the panel in.';
			return;
		}
		panelSaving = true;
		try {
			// Persist the on-screen edits first: PostPanel renders from the stored
			// row, so publishing without saving would post the last-saved config.
			const res = await api.upsertTicketPanel(store.id, {
				id: editing.id,
				name: editing.name,
				style: editing.style,
				enabled: editing.enabled,
				config: editing.config
			});
			editing.id = res.id;
			await api.publishTicketPanel(store.id, editing.id, publishChannel);
			panelStatus = 'Panel posted.';
			await loadPanels();
		} catch (e) {
			panelStatus = e instanceof ApiError ? e.message : 'Could not post the panel.';
		} finally {
			panelSaving = false;
		}
	}

	async function deletePanel(p: PanelSummary) {
		if (!confirm(`Delete the "${p.name}" panel? Existing tickets stay open.`)) return;
		try {
			await api.deleteTicketPanel(store.id, p.id);
			if (editing?.id === p.id) editing = null;
			await loadPanels();
		} catch {
			/* best-effort */
		}
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

	// Lazy-load panels + analytics the first time their tab is opened. The loaders
	// only ever flip their *Loaded flag true, so this never re-fires itself.
	$effect(() => {
		if (tab === 'panels' && !panelsLoaded) loadPanels();
		if (tab === 'analytics' && !statsLoaded) loadStats();
	});
	// The queue reloads on open and whenever the status filter changes; loadQueue
	// touches neither `tab` nor `queueFilter`, so there is no loop.
	$effect(() => {
		queueFilter; // track filter changes
		if (tab === 'queue') loadQueue();
	});

	const styleOptions = [
		{ value: 'buttons', label: 'Buttons (one per category)' },
		{ value: 'select', label: 'Dropdown menu' }
	];

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

<ModerationShell
	icon={Ticket}
	title="Tickets"
	blurb="Let members open private support tickets from a panel. Fully customizable channels, forms, claiming, transcripts, ratings and auto-close."
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
		{#if tab === 'setup'}
			<ModSection label="Staff & channels" desc="Who handles tickets and where activity is logged.">
				<div class="grid gap-4 sm:grid-cols-2">
					<Field label="Support staff roles" hint="Added to every ticket and allowed to run ticket commands">
						<RolePicker multiple value={cfg.staff_role_ids} onChange={(v) => (cfg.staff_role_ids = v as string[])} />
					</Field>
					<Field label="Default ticket category" hint="Discord category new ticket channels are created under">
						<ChannelPicker kind="all" value={cfg.default_parent_id} placeholder="None" onChange={(v) => (cfg.default_parent_id = v as string)} />
					</Field>
					<Field label="Log channel" hint="Open / claim / close / delete events">
						<ChannelSelect bind:value={cfg.log_channel} placeholder="No log channel" />
					</Field>
					<Field label="Transcript channel" hint="Where closing transcripts are posted (defaults to the log channel)">
						<ChannelSelect bind:value={cfg.transcript_channel} placeholder="Use log channel" />
					</Field>
				</div>
			</ModSection>

			<ModSection label="Limits" desc="Guard against ticket spam.">
				<div class="grid gap-4 sm:grid-cols-2">
					<Field label="Max open tickets per member" hint="0 = unlimited; a category can tighten this"><NumberField bind:value={cfg.max_open_per_user} min={0} /></Field>
					<Field label="Channel name prefix" hint="Used when a category has no name template"><input class={inputCls} bind:value={cfg.name_prefix} placeholder="ticket" /></Field>
				</div>
			</ModSection>

			<ModSection label="Blacklist" desc="Roles and members that can never open a ticket.">
				<div class="grid gap-4 sm:grid-cols-2">
					<Field label="Blacklisted roles">
						<RolePicker multiple value={cfg.blacklist_role_ids} onChange={(v) => (cfg.blacklist_role_ids = v as string[])} />
					</Field>
				</div>
			</ModSection>
		{:else if tab === 'panels'}
			{#if editing}
				<ModSection label={editing.id ? 'Edit panel' : 'New panel'} desc="The message members click to open a ticket. Each category is its own ticket type.">
					{#snippet actions()}
						<button type="button" class="flex items-center gap-1.5 text-sm text-muted hover:text-ink" onclick={() => (editing = null)}>
							<ArrowLeft class="h-4 w-4" /> Back
						</button>
					{/snippet}

					<div class="space-y-5">
						<div class="grid gap-4 sm:grid-cols-2">
							<Field label="Panel name" hint="For your reference only"><input class={inputCls} bind:value={editing.name} placeholder="Support" /></Field>
							<Field label="Layout"><Select bind:value={editing.style} options={styleOptions} /></Field>
						</div>
						{#if editing.style === 'select'}
							<Field label="Dropdown placeholder"><input class={inputCls} bind:value={editing.config.select_placeholder} placeholder="Choose a ticket type" /></Field>
						{/if}
						<label class="flex items-center gap-3 text-sm text-ink">
							<Toggle bind:checked={editing.enabled} label="Enabled" /> Panel enabled (disabled panels refuse new tickets)
						</label>

						<div class="space-y-3">
							<p class="eyebrow">Panel message</p>
							<p class="text-xs text-muted">
								Compose the message members see — content and as many embeds as you like. The open
								buttons (or dropdown) below it come from the categories.
							</p>
							{#key panelEditSeq}
								<MessageEditor step={panelStep} embeds clickPaths={false} />
							{/key}
						</div>

						<div class="space-y-3 border-t border-line pt-4">
							<div class="flex items-center justify-between">
								<p class="eyebrow">Categories <span class="text-faint">({editing.config.categories.length}/25)</span></p>
								<button type="button" class="flex items-center gap-1 text-xs text-accent-ink hover:underline disabled:opacity-40" disabled={editing.config.categories.length >= 25} onclick={addCategory}>
									<Plus class="h-3.5 w-3.5" /> Add category
								</button>
							</div>
							{#each editing.config.categories as cat, i (cat.id)}
								<CategoryEditor category={cat} guildId={store.id} index={i} onRemove={() => removeCategory(i)} />
							{/each}
							{#if editing.config.categories.length === 0}
								<p class="text-sm text-muted">Add at least one category so members have something to open.</p>
							{/if}
						</div>

						<div class="flex flex-wrap items-center gap-3 border-t border-line pt-4">
							<button type="button" class="flex items-center gap-1.5 rounded-md bg-ink px-3 py-2 text-sm font-medium text-bg disabled:opacity-50" disabled={panelSaving} onclick={savePanel}>
								<Save class="h-4 w-4" /> Save panel
							</button>
							<div class="flex items-center gap-2">
								<div class="w-56"><ChannelSelect bind:value={publishChannel} placeholder="Post to channel…" /></div>
								<button type="button" class="flex items-center gap-1.5 rounded-md border border-line px-3 py-2 text-sm text-ink hover:border-line-strong disabled:opacity-50" disabled={panelSaving} onclick={publishPanel}>
									<Send class="h-4 w-4" /> Publish
								</button>
							</div>
							{#if panelStatus}<span class="text-sm text-muted">{panelStatus}</span>{/if}
						</div>
					</div>
				</ModSection>
			{:else}
				<ModSection label="Panels" desc="Design the messages members use to open tickets.">
					{#snippet actions()}
						<button type="button" class="flex items-center gap-1.5 rounded-md bg-ink px-3 py-1.5 text-sm font-medium text-bg" onclick={newPanel}>
							<Plus class="h-4 w-4" /> New panel
						</button>
					{/snippet}
					{#if !panelsLoaded}
						<p class="text-sm text-muted">Loading…</p>
					{:else if panelsError}
						<p class="text-sm text-accent-ink">{panelsError}</p>
					{:else if panels.length === 0}
						<p class="text-sm text-muted">No panels yet. Create one, add categories, then publish it to a channel.</p>
					{:else}
						<div class="space-y-2">
							{#each panels as p (p.id)}
								<div class="flex items-center gap-4 rounded-lg border border-line bg-surface px-4 py-3">
									<div class="flex-1">
										<div class="flex items-center gap-2">
											<span class="font-medium text-ink">{p.name || '(untitled)'}</span>
											{#if !p.enabled}<span class="rounded bg-line px-1.5 py-0.5 text-[10px] uppercase text-muted">disabled</span>{/if}
										</div>
										<p class="text-xs text-muted">
											{p.config.categories.length} categor{p.config.categories.length === 1 ? 'y' : 'ies'} · {p.style}
											{#if p.message_id !== '0' && p.channel_id !== '0'}
												· <a class="text-accent-ink hover:underline" href={channelLink(p.channel_id)} target="_blank" rel="noreferrer">posted</a>
											{:else}· not posted{/if}
										</p>
									</div>
									<button type="button" class="text-sm text-muted hover:text-ink" onclick={() => editPanel(p)}>Edit</button>
									<button type="button" class="text-muted hover:text-accent-ink" title="Delete" onclick={() => deletePanel(p)}><Trash2 class="h-4 w-4" /></button>
								</div>
							{/each}
						</div>
					{/if}
				</ModSection>
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
		{:else if tab === 'guide'}
			<ModSection label="How ticketing works" desc="From panel to transcript.">
				<div class="max-w-2xl space-y-4 text-sm text-muted">
					<p><span class="text-ink">1. Design a panel.</span> A panel is an embed with buttons (or a dropdown). Each button is a category — its own ticket type with its own permissions, opening message, form and rules.</p>
					<p><span class="text-ink">2. Publish it.</span> Post the panel to a channel. Members click to open a ticket. If the category has a form, they fill it first.</p>
					<p><span class="text-ink">3. A private channel opens.</span> Only the opener and your support roles can see it. Staff can claim it, add or remove members, rename it, and leave private notes.</p>
					<p><span class="text-ink">4. Close politely.</span> Staff can close outright, or send a close request with <code class="text-ink">/ticket closerequest</code> — the opener confirms with a button, and an optional delay closes the ticket automatically if they never answer.</p>
					<p><span class="text-ink">5. Follow up.</span> Closing generates an HTML transcript (posted to your transcript channel and optionally DMed to the opener) and can ask the opener to rate the help. Inactive tickets can auto-close after a warning.</p>
					<p><span class="text-ink">6. Make it yours.</span> Every message — panel, opening, closed card, close request, inactivity warning, feedback DM — is fully composable (content, embeds, buttons), and the built-in Claim/Close/Reopen buttons can be restyled per category.</p>
					<p><span class="text-ink">7. Automate it.</span> Every ticket event (opened, claimed, closed, close requested, rated) is a trigger in Automations; each category can launch a saved automation on open or close; and any button you add to a ticket message can run an automation when clicked.</p>
					<p>Staff also get <code class="text-ink">/ticket</code> commands (close, closerequest, claim, add, remove, rename, note, transcript) inside any ticket channel, and admins can post a panel with <code class="text-ink">/tickets post</code>.</p>
				</div>
			</ModSection>
		{/if}
	</TabSwipe>
</ModerationShell>
