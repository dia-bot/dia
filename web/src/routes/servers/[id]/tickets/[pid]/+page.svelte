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
		type PanelConfig
	} from '$lib/tickets/types';
	import type { Step } from '$lib/commands/types';
	import { AUTOMATION_CTX, EXPR_SCOPE_CTX, type ExprScope } from '$lib/commands/expr-meta';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import CategoryEditor from '$lib/components/tickets/CategoryEditor.svelte';
	import ModSection from '$lib/components/moderation/ModSection.svelte';
	import Field from '$lib/components/Field.svelte';
	import Select from '$lib/components/Select.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import TemplateGuide from '$lib/components/commands/TemplateGuide.svelte';
	import ReleaseDock from '$lib/components/page/ReleaseDock.svelte';
	import { ChevronLeft, Ticket, Plus, Trash2, Send, Save, BookOpen, ExternalLink } from 'lucide-svelte';

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

	// The panel message (content + embeds) edited with the shared WYSIWYG editor.
	let msgStep = $state<Step>({ id: 'panel-msg', kind: 'send_message', spec: { content: '', embeds: [] } });
	function seedStep() {
		msgStep = {
			id: 'panel-msg',
			kind: 'send_message',
			spec: {
				content: config.content ?? '',
				embeds: JSON.parse(JSON.stringify(config.embeds ?? []))
			}
		};
	}
	$effect(() => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const s = msgStep.spec as any;
		config.content = s.content ?? '';
		config.embeds = s.embeds ?? [];
	});

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

	function addCategory() {
		if (config.categories.length >= 25) return;
		config.categories = [...config.categories, newCategory('New ticket type')];
	}
	function removeCategory(i: number) {
		config.categories = config.categories.filter((_, idx) => idx !== i);
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
					desc="The message members see in your channel. Compose it right here — content and as many embeds as you like. The open buttons (or dropdown) underneath come from your ticket types."
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
						<MessageEditor step={msgStep} embeds clickPaths={false} />
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
					desc="Each type is its own button (or dropdown option) with its own private-channel permissions, opening message, pre-open form, transcript, feedback, auto-close and automations."
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
					<div class="space-y-3">
						{#each config.categories as cat, i (cat.id)}
							<CategoryEditor category={cat} guildId={store.id} index={i} onRemove={() => removeCategory(i)} />
						{/each}
						{#if config.categories.length === 0}
							<p class="text-sm text-muted">Add at least one ticket type so members have something to open.</p>
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
