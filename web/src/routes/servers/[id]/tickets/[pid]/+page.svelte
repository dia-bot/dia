<script lang="ts">
	import { onMount, getContext, setContext } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api, ApiError } from '$lib/api';
	import {
		defaultPanelConfig,
		normalizePanelConfig,
		newCategory,
		TICKET_TEMPLATE_VARS,
		type PanelConfig,
		type CategoryConfig,
		type TicketComponent
	} from '$lib/tickets/types';
	import type { Step } from '$lib/commands/types';
	import { AUTOMATION_CTX, EXPR_SCOPE_CTX, type ExprScope } from '$lib/commands/expr-meta';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import AutomationPicker from '$lib/components/commands/AutomationPicker.svelte';
	import TicketTypeModal from '$lib/components/tickets/TicketTypeModal.svelte';
	import ModSection from '$lib/components/moderation/ModSection.svelte';
	import Field from '$lib/components/Field.svelte';
	import Select from '$lib/components/Select.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import TemplateGuide from '$lib/components/commands/TemplateGuide.svelte';
	import ReleaseDock from '$lib/components/page/ReleaseDock.svelte';
	import { ChevronLeft, Ticket, Plus, Trash2, Send, Save, BookOpen, ExternalLink, Pencil } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);

	// Every MessageEditor on this page offers the ticket variables in its picker,
	// in the non-automation form (same wiring as the giveaway editor).
	setContext(AUTOMATION_CTX, false);
	setContext(EXPR_SCOPE_CTX, {
		options: [],
		variables: [],
		steps: [],
		extraVars: TICKET_TEMPLATE_VARS
	} satisfies ExprScope);

	const pid = $derived($page.params.pid ?? '');
	const isNew = $derived(pid === 'new');

	let loaded = $state(false);
	let loadError = $state('');
	let busy = $state('');
	let showGuide = $state(false);

	// ── Setup state ───────────────────────────────────────────────────────────
	let name = $state('Support');
	let style = $state('buttons');
	let enabled = $state(true);
	let config = $state<PanelConfig>(defaultPanelConfig());
	let channelId = $state(''); // where the panel is (or will be) posted
	let messageId = $state('');

	// The panel message (content + embeds + its own buttons) edited with the
	// shared WYSIWYG editor. Composed buttons are wired inline in the preview:
	// open a ticket type, run an automation, or nothing.
	let msgStep = $state<Step>({ id: 'panel-msg', kind: 'send_message', spec: { content: '', embeds: [], components: [] } });
	function seedStep() {
		msgStep = {
			id: 'panel-msg',
			kind: 'send_message',
			spec: {
				content: config.content ?? '',
				embeds: JSON.parse(JSON.stringify(config.embeds ?? [])),
				components: JSON.parse(JSON.stringify(config.components ?? []))
			}
		};
	}
	$effect(() => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const s = msgStep.spec as any;
		config.content = s.content ?? '';
		config.embeds = s.embeds ?? [];
		config.components = s.components ?? [];
	});

	// A panel button's action mode: open a ticket type, run an automation, or
	// nothing (mirrors the giveaway editor's inline picker).
	function panelButtonMode(suffix: string): 'open' | 'auto' | 'none' {
		if (config.button_bindings[suffix]) return 'open';
		if (suffix in config.button_actions) return 'auto';
		return 'none';
	}
	function setPanelButtonMode(suffix: string, mode: 'open' | 'auto' | 'none') {
		if (mode === 'open') {
			if (!config.button_bindings[suffix]) config.button_bindings[suffix] = config.categories[0]?.id ?? '';
			delete config.button_actions[suffix];
			config.button_actions = { ...config.button_actions };
		} else {
			delete config.button_bindings[suffix];
			config.button_bindings = { ...config.button_bindings };
			if (mode === 'auto') {
				if (!(suffix in config.button_actions)) config.button_actions[suffix] = '';
			} else {
				delete config.button_actions[suffix];
				config.button_actions = { ...config.button_actions };
			}
		}
	}
	const typeOptions = $derived(
		config.categories.map((c) => ({ value: c.id, label: `${c.emoji ?? ''} ${c.label || 'Untitled'}`.trim() }))
	);
	// The composed actionable buttons on the panel (link buttons excluded).
	const panelButtons = $derived(
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		((((msgStep.spec as any)?.components ?? []) as { components?: TicketComponent[] }[])
			.flatMap((r) => r.components ?? [])
			.filter((c) => (c.type ?? 'button') === 'button' && c.style !== 'link' && !c.url && !!c.custom_id_suffix)
			.map((c) => c.custom_id_suffix as string))
	);
	// With the buttons layout, composed buttons REPLACE the generated ones — warn
	// when none of them opens an existing ticket type.
	const panelMissingOpen = $derived(
		style === 'buttons' &&
			panelButtons.length > 0 &&
			!panelButtons.some((s) => {
				const cid = config.button_bindings[s];
				return !!cid && config.categories.some((c) => c.id === cid);
			})
	);

	// ── Ticket-type modal ──────────────────────────────────────────────────────
	let typeModalOpen = $state(false);
	let editingType = $state<CategoryConfig | null>(null);
	function openType(cat: CategoryConfig) {
		editingType = cat;
		typeModalOpen = true;
	}
	function removeEditingType() {
		if (!editingType) return;
		if (!confirm(`Delete the "${editingType.label || 'untitled'}" ticket type?`)) return;
		const id = editingType.id;
		typeModalOpen = false;
		editingType = null;
		const i = config.categories.findIndex((c) => c.id === id);
		if (i >= 0) removeCategory(i);
	}

	async function load() {
		loadError = '';
		loaded = false;
		try {
			if (!isNew) {
				const r = await api.ticketPanels(store.id);
				const p = (r.panels ?? []).find((x) => x.id === pid);
				if (!p) {
					loadError = 'This ticket setup no longer exists.';
					return;
				}
				name = p.name ?? '';
				style = p.style ?? 'buttons';
				enabled = !!p.enabled;
				config = normalizePanelConfig(p.config);
				channelId = p.channel_id && p.channel_id !== '0' ? p.channel_id : '';
				messageId = p.message_id && p.message_id !== '0' ? p.message_id : '';
			}
			seedStep();
			baseline = snapshot();
			loaded = true;
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load the ticket setup.';
		}
	}
	onMount(load);

	function backToList() {
		goto(`/servers/${store.id}/tickets`);
	}

	function body() {
		return { id: isNew ? '' : pid, name, style, enabled, config };
	}

	async function run(key: string, fn: () => Promise<unknown>) {
		if (busy) return;
		busy = key;
		loadError = '';
		try {
			await fn();
		} catch (e) {
			loadError = e instanceof ApiError ? e.message : e instanceof Error ? e.message : 'Action failed.';
		} finally {
			busy = '';
		}
	}

	// Create a brand-new setup, then continue editing it under its real id.
	async function createSetup() {
		await run('create', async () => {
			const res = await api.upsertTicketPanel(store.id, body());
			await goto(`/servers/${store.id}/tickets/${res.id}`, { replaceState: true });
			await load();
		});
	}

	// Publish saves the on-screen edits first (the bot renders from the stored
	// row), then posts the panel message to the chosen channel.
	async function publish() {
		await run('publish', async () => {
			const res = await api.upsertTicketPanel(store.id, body());
			const pub = await api.publishTicketPanel(store.id, res.id, channelId);
			messageId = pub.message_id ?? '';
			baseline = snapshot();
			if (isNew) await goto(`/servers/${store.id}/tickets/${res.id}`, { replaceState: true });
		});
	}

	async function deleteSetup() {
		if (!confirm(`Delete the "${name || 'untitled'}" setup? Existing tickets stay open.`)) return;
		await run('delete', async () => {
			await api.deleteTicketPanel(store.id, pid);
			backToList();
		});
	}

	// appendTypeButton drops a ready-bound open button for the type into the
	// panel composition (into the last row with room, else a new row).
	function appendTypeButton(cat: CategoryConfig) {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const s = msgStep.spec as any;
		const rows = ((s.components ?? []) as { components: TicketComponent[] }[]).map((r) => ({
			components: [...(r.components ?? [])]
		}));
		const btn: TicketComponent = {
			type: 'button',
			style: 'primary',
			label: cat.label || 'Open ticket',
			emoji: cat.emoji || '🎫',
			custom_id_suffix: cat.id
		};
		const last = rows[rows.length - 1];
		if (last && last.components.length < 5) last.components.push(btn);
		else rows.push({ components: [btn] });
		msgStep.spec = { ...s, components: rows };
		config.button_bindings[cat.id] = cat.id;
	}

	function addCategory() {
		if (config.categories.length >= 25) return;
		config.categories = [...config.categories, newCategory('New ticket type')];
		const cat = config.categories[config.categories.length - 1];
		appendTypeButton(cat);
		openType(cat);
	}
	function removeCategory(i: number) {
		const removed = config.categories[i];
		config.categories = config.categories.filter((_, idx) => idx !== i);
		if (!removed) return;
		// Also drop the panel buttons that opened this type, and their bindings.
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const s = msgStep.spec as any;
		const rows = ((s.components ?? []) as { components: TicketComponent[] }[])
			.map((r) => ({
				components: (r.components ?? []).filter(
					(c) => config.button_bindings[c.custom_id_suffix ?? ''] !== removed.id
				)
			}))
			.filter((r) => r.components.length > 0);
		msgStep.spec = { ...s, components: rows };
		for (const [suffix, cid] of Object.entries(config.button_bindings)) {
			if (cid === removed.id) delete config.button_bindings[suffix];
		}
		config.button_bindings = { ...config.button_bindings };
	}

	// With the buttons layout, a type is only reachable if some composed button
	// opens it (unless nothing is composed and the generated fallback applies).
	function typeHasButton(cat: CategoryConfig): boolean {
		if (style !== 'buttons' || panelButtons.length === 0) return true;
		return panelButtons.some((s) => config.button_bindings[s] === cat.id);
	}

	const canPublish = $derived(!!channelId && config.categories.length > 0);
	const posted = $derived(!!messageId && !!channelId);
	function panelLink(): string {
		return `https://discord.com/channels/${store.id}/${channelId}/${messageId}`;
	}

	const styleOptions = [
		{ value: 'buttons', label: 'Buttons (one per ticket type)' },
		{ value: 'select', label: 'Dropdown menu' }
	];
	const inputCls =
		'h-8 w-full rounded-md border border-line bg-bg px-2.5 text-[13px] text-ink placeholder:text-faint focus-visible:border-line-strong focus-visible:outline-none disabled:opacity-60';
	const btnGhost =
		'inline-flex h-8 items-center gap-1.5 rounded-md border border-line px-3 text-[12px] font-medium text-ink hover:border-line-strong disabled:opacity-50';

	const title = $derived(isNew ? 'New ticket setup' : name || 'Ticket setup');

	// ── Unsaved-changes dock (existing setups; a new one uses Create/Publish) ──
	let baseline = $state('');
	let savePhase = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
	function snapshot(): string {
		return JSON.stringify({ name, style, enabled, config, spec: msgStep.spec });
	}
	const dirty = $derived(loaded && !isNew && snapshot() !== baseline);
	async function saveChanges() {
		if (savePhase === 'saving' || !dirty) return;
		savePhase = 'saving';
		loadError = '';
		try {
			await api.upsertTicketPanel(store.id, body());
			baseline = snapshot();
			savePhase = 'saved';
			setTimeout(() => (savePhase = 'idle'), 1400);
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not save.';
			savePhase = 'error';
		}
	}
	function discardChanges() {
		savePhase = 'idle';
		load();
	}
	function onKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's' && dirty) {
			e.preventDefault();
			saveChanges();
		}
	}
</script>

<svelte:head>
	<title>{title} · Tickets · Dia</title>
</svelte:head>

<!-- Per-button action picker, rendered inline inside each panel button in the
     message preview (MessageEditor buttonExtras). Edit the button's LOOK in
     the preview; set what it DOES here: open a ticket type, run one of your
     automations, or nothing. -->
{#snippet panelButtonAction({ component }: { component: TicketComponent; ri: number; ci: number })}
	{@const suffix = component.custom_id_suffix}
	{#if suffix && component.style !== 'link' && !component.url}
		{@const mode = panelButtonMode(suffix)}
		<div class="mt-2 space-y-1.5">
			<span class="font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground">Action</span>
			<div class="flex rounded-md border border-input p-0.5" role="radiogroup" aria-label="Button action">
				<button
					type="button"
					role="radio"
					aria-checked={mode === 'open'}
					class="flex-1 rounded px-2 py-0.5 text-[10px] font-medium transition-colors {mode === 'open' ? 'bg-secondary text-foreground' : 'text-muted-foreground hover:text-foreground'}"
					onclick={() => setPanelButtonMode(suffix, 'open')}
				>
					Open ticket
				</button>
				<button
					type="button"
					role="radio"
					aria-checked={mode === 'auto'}
					class="flex-1 rounded px-2 py-0.5 text-[10px] font-medium transition-colors {mode === 'auto' ? 'bg-secondary text-foreground' : 'text-muted-foreground hover:text-foreground'}"
					onclick={() => setPanelButtonMode(suffix, 'auto')}
				>
					Run automation
				</button>
				<button
					type="button"
					role="radio"
					aria-checked={mode === 'none'}
					class="flex-1 rounded px-2 py-0.5 text-[10px] font-medium transition-colors {mode === 'none' ? 'bg-secondary text-foreground' : 'text-muted-foreground hover:text-foreground'}"
					onclick={() => setPanelButtonMode(suffix, 'none')}
				>
					Nothing
				</button>
			</div>
			{#if mode === 'open'}
				{#if typeOptions.length === 0}
					<p class="text-[10px] leading-snug text-muted-foreground">No ticket types yet — add one below first.</p>
				{:else}
					<Select bind:value={config.button_bindings[suffix]} options={typeOptions} />
				{/if}
			{:else if mode === 'auto'}
				<AutomationPicker
					value={config.button_actions[suffix] ?? ''}
					onChange={(v) => (config.button_actions[suffix] = v)}
				/>
			{/if}
		</div>
	{/if}
{/snippet}

<div class="flex h-full flex-col bg-bg text-ink">
	<!-- ── Header (matches the giveaway editor chrome) ─────────────────────────── -->
	<header class="flex min-h-12 shrink-0 flex-wrap items-center gap-2.5 border-b border-line bg-bg px-4 py-2 sm:px-5">
		<button
			type="button"
			onclick={backToList}
			class="grid size-6 shrink-0 place-items-center rounded border border-line bg-surface text-muted hover:text-ink"
			aria-label="Back to tickets"
		>
			<ChevronLeft size={14} />
		</button>
		<Ticket size={14} class="shrink-0 text-accent-ink" />
		<span class="truncate text-[13px] font-semibold tracking-tight text-ink">{title}</span>
		{#if !isNew}
			<span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.14em] {posted ? 'text-accent-ink' : 'text-muted'}">
				{posted ? 'posted' : 'not posted'}
			</span>
			{#if !enabled}
				<span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.14em] text-faint">disabled</span>
			{/if}
		{/if}

		<div class="ml-auto flex flex-wrap items-center justify-end gap-1.5">
			{#if !isNew}
				<button
					type="button"
					disabled={!!busy}
					onclick={deleteSetup}
					class="grid size-8 place-items-center rounded-md border border-line text-muted hover:border-line-strong hover:text-danger disabled:opacity-50"
					aria-label="Delete setup"
				>
					<Trash2 size={14} />
				</button>
			{/if}
			{#if isNew}
				<button type="button" disabled={!!busy} onclick={createSetup} class={btnGhost}>
					<Save size={13} /> Save setup
				</button>
			{/if}
			<button
				type="button"
				disabled={!!busy || !canPublish}
				onclick={publish}
				class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90 disabled:opacity-40"
				title={canPublish ? '' : 'Pick a channel and add at least one ticket type first.'}
			>
				<Send size={13} /> {posted ? 'Repost panel' : 'Publish panel'}
			</button>
		</div>
	</header>

	<!-- ── Body ──────────────────────────────────────────────────────────────── -->
	<div class="min-h-0 flex-1 overflow-y-auto">
		{#if loadError}
			<div class="border-b border-danger/40 bg-danger/5 px-4 py-2 text-[12px] text-danger sm:px-5">{loadError}</div>
		{/if}

		{#if !loaded && !loadError}
			<div class="grid place-items-center py-24 text-[13px] text-muted">Loading…</div>
		{:else if loaded}
			<div class="pb-24">
				<ModSection
					label="Panel message"
					desc="The message members see in your channel. Compose it right here — content, embeds and buttons. Click a button to set what it does: open a ticket type, run one of your automations, or open a link. Compose no buttons and the classic per-type buttons (or dropdown) are generated for you."
				>
					<div class="max-w-2xl">
						<div class="mb-2 flex flex-wrap items-center gap-2">
							<p class="text-[12px] text-muted">
								Use variables like
								<code class="rounded bg-surface px-1 font-mono text-[11px]">{'{{ .Guild.Name }}'}</code>,
								<code class="rounded bg-surface px-1 font-mono text-[11px]">{'{{ .Channel.Mention }}'}</code>.
							</p>
							<button
								type="button"
								class="inline-flex h-6 items-center gap-1 rounded border border-line px-1.5 font-mono text-[10px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink"
								onclick={() => (showGuide = true)}
							>
								<BookOpen size={10} /> Template guide
							</button>
						</div>
						{#if panelMissingOpen}
							<div class="mb-2 rounded-md border border-accent/40 bg-accent/5 px-2.5 py-1.5 text-[12px] text-accent-ink">
								Your composed buttons replace the generated ones, but none of them opens a ticket.
								Set a button's action to <span class="font-medium">Open ticket</span>, or members won't
								be able to open one from this panel.
							</div>
						{/if}
						<MessageEditor step={msgStep} embeds components clickPaths={false} buttonExtras={panelButtonAction} />
					</div>
				</ModSection>

				<ModSection label="Setup" desc="Where the panel lives and how the ticket types are offered.">
					<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
						<Field label="Setup name" hint="For your reference only.">
							<input class={inputCls} bind:value={name} placeholder="Support" />
						</Field>
						<Field label="Layout"><Select bind:value={style} options={styleOptions} /></Field>
						{#if style === 'select'}
							<Field label="Dropdown placeholder">
								<input class={inputCls} bind:value={config.select_placeholder} placeholder="Choose a ticket type" />
							</Field>
						{/if}
						<Field label="Panel channel" hint="Where the panel is posted.">
							<ChannelSelect bind:value={channelId} placeholder="Pick a channel…" />
						</Field>
					</div>
					<div class="mt-3 flex flex-wrap items-center gap-4">
						<label class="flex items-center gap-2 text-[13px] text-ink">
							<Toggle bind:checked={enabled} label="Enabled" /> Accept new tickets from this panel
						</label>
						{#if posted}
							<a
								class="inline-flex items-center gap-1 text-[12px] text-accent-ink hover:underline"
								href={panelLink()}
								target="_blank"
								rel="noreferrer"
							>
								View posted panel <ExternalLink size={11} />
							</a>
						{/if}
					</div>
				</ModSection>

				<ModSection
					label="Ticket types"
					desc="Each type is its own button (or dropdown option) with its own private-channel permissions, opening message, pre-open form, transcript, feedback, auto-close and automations. Click one to edit everything about it."
				>
					{#snippet actions()}
						<button
							type="button"
							class="inline-flex h-7 items-center gap-1.5 rounded-md bg-ink px-2.5 text-[12px] font-semibold text-bg hover:bg-ink/90 disabled:opacity-40"
							disabled={config.categories.length >= 25}
							onclick={addCategory}
						>
							<Plus size={13} /> Add ticket type
						</button>
					{/snippet}
					<div class="divide-y divide-line">
						{#each config.categories as cat, i (cat.id)}
							<div class="flex items-center gap-3 px-1 py-2.5">
								<button type="button" class="flex min-w-0 flex-1 items-center gap-3 text-left" onclick={() => openType(cat)}>
									<span class="text-lg leading-none">{cat.emoji || '🎫'}</span>
									<span class="min-w-0">
										<span class="flex items-center gap-2">
											<span class="truncate text-[13px] font-semibold text-ink">{cat.label || 'Untitled ticket type'}</span>
											{#if !typeHasButton(cat)}
												<span class="shrink-0 rounded-full border border-accent/40 px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide text-accent-ink">no panel button</span>
											{/if}
										</span>
										<span class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] text-muted">
											<span>{cat.open_mode === 'thread' ? 'private thread' : 'private channel'}</span>
											{#if (cat.form?.length ?? 0) > 0}<span>{cat.form?.length} form question{(cat.form?.length ?? 0) === 1 ? '' : 's'}</span>{/if}
											{#if cat.claim_enabled}<span>claiming</span>{/if}
											{#if cat.transcript.enabled}<span>transcript</span>{/if}
											{#if cat.feedback.enabled}<span>feedback</span>{/if}
											{#if cat.auto_close.enabled}<span>auto-close</span>{/if}
										</span>
									</span>
								</button>
								{#if !typeHasButton(cat)}
									<button
										type="button"
										class="inline-flex h-7 items-center gap-1 rounded-md border border-line px-2.5 text-[12px] font-medium text-ink hover:border-line-strong"
										title="Add an open button for this type to the panel message"
										onclick={() => appendTypeButton(cat)}
									>
										<Plus size={12} /> Add button
									</button>
								{/if}
								<button
									type="button"
									class="inline-flex h-7 items-center gap-1 rounded-md border border-line px-2.5 text-[12px] font-medium text-ink hover:border-line-strong"
									onclick={() => openType(cat)}
								>
									<Pencil size={12} /> Edit
								</button>
								<button
									type="button"
									class="grid size-7 shrink-0 place-items-center rounded-md border border-line text-muted hover:border-line-strong hover:text-danger"
									aria-label="Delete ticket type"
									onclick={() => {
										if (confirm(`Delete the "${cat.label || 'untitled'}" ticket type?`)) removeCategory(i);
									}}
								>
									<Trash2 size={13} />
								</button>
							</div>
						{/each}
						{#if config.categories.length === 0}
							<p class="py-3 text-sm text-muted">Add at least one ticket type so members have something to open.</p>
						{/if}
					</div>
				</ModSection>
			</div>
		{/if}
	</div>
</div>

<svelte:window onkeydown={onKeydown} />
<ReleaseDock {dirty} phase={savePhase} error={loadError} onsave={saveChanges} onreset={discardChanges} />
<TemplateGuide bind:open={showGuide} variables={TICKET_TEMPLATE_VARS} variablesLabel="Ticket variables" lookups={false} />
<TicketTypeModal bind:open={typeModalOpen} category={editingType} guildId={store.id} panelStyle={style} onRemove={removeEditingType} />
