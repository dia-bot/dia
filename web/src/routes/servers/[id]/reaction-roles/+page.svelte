<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import Toggle from '$lib/components/Toggle.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import MenuEditorModal from '$lib/components/reactionroles/MenuEditorModal.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import Row from '$lib/components/page/Row.svelte';
	import TopbarAction from '$lib/components/page/TopbarAction.svelte';
	import EmptyBlock from '$lib/components/page/EmptyBlock.svelte';
	import ReleaseDock from '$lib/components/page/ReleaseDock.svelte';
	import { ToggleRight, Plus, Trash2, Pencil, Route } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'reactionroles';

	type Option = { role_id: string; label: string; emoji: string; description: string };
	type Menu = {
		id?: number;
		title: string;
		mode: string;
		channel_id?: string;
		message_id?: string;
		options: Option[];
	};

	const MODE_OPTS = [
		{ value: 'toggle', label: 'Toggle, add or remove freely' },
		{ value: 'unique', label: 'Unique, only one role at a time' },
		{ value: 'verify', label: 'Verify, single-use opt-in' }
	];

	// ── Feature enable flag (the only thing the ReleaseDock tracks) ──────────
	let enabled = $state(false);
	let loaded = $state(false);
	let baseline = $state('');
	let savePhase = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
	let saveErr = $state('');

	function serialize() {
		return JSON.stringify({ enabled });
	}
	const dirty = $derived(loaded && serialize() !== baseline);

	// ── Menus (immediate-save CRUD, not part of the dirty state) ─────────────
	let menus = $state<Menu[]>([]);

	// The editor lives in an animated modal now; the list is the calm default.
	let modalOpen = $state(false);
	let editingMenu = $state<Menu | null>(null);

	// Delete confirmation
	let confirmOpen = $state(false);
	let pendingDelete = $state<Menu | null>(null);

	function isPosted(m: Menu): boolean {
		return (
			!!m.message_id &&
			m.message_id !== '0' &&
			!!m.channel_id &&
			m.channel_id !== '0'
		);
	}

	async function reload() {
		const res = await api.menus(store.id);
		menus = (res.menus ?? []).map((m: any) => ({
			id: m.id,
			title: m.title ?? '',
			mode: m.mode ?? 'toggle',
			channel_id: m.channel_id,
			message_id: m.message_id,
			options: (m.options ?? []).map((o: any) => ({
				role_id: o.role_id ?? '',
				label: o.label ?? '',
				emoji: o.emoji ?? '',
				description: o.description ?? ''
			}))
		}));
	}

	onMount(async () => {
		const [f] = await Promise.all([api.feature(store.id, FEATURE), reload()]);
		enabled = f.enabled;
		loaded = true;
		baseline = serialize();
	});

	function modeLabel(mode: string): string {
		return MODE_OPTS.find((m) => m.value === mode)?.label.split(',')[0] ?? mode;
	}

	function channelName(id?: string): string {
		if (!id || id === '0') return '';
		return store.channels.find((c) => c.id === id)?.name ?? '';
	}

	function startNew() {
		editingMenu = null;
		modalOpen = true;
	}
	function startEdit(m: Menu) {
		editingMenu = m;
		modalOpen = true;
	}

	function askDelete(m: Menu) {
		pendingDelete = m;
		confirmOpen = true;
	}
	async function confirmDelete() {
		const m = pendingDelete;
		pendingDelete = null;
		if (!m || m.id == null) return;
		await api.deleteMenu(store.id, m.id);
		await reload();
	}

	// ── Feature enable save lifecycle (ReleaseDock) ──────────────────────────
	async function save() {
		if (savePhase === 'saving' || !dirty) return;
		savePhase = 'saving';
		saveErr = '';
		try {
			await api.saveFeature(store.id, FEATURE, enabled, {});
			if (store.detail) store.detail.features[FEATURE] = { enabled, config: {} };
			baseline = serialize();
			savePhase = 'saved';
			setTimeout(() => {
				if (savePhase === 'saved') savePhase = 'idle';
			}, 1800);
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Something went wrong';
			savePhase = 'error';
		}
	}
	function reset() {
		const b = JSON.parse(baseline);
		enabled = b.enabled;
		savePhase = 'idle';
		saveErr = '';
	}

	function onKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
			e.preventDefault();
			if (dirty) save();
		}
	}
</script>

<svelte:head><title>Reaction Roles · {store.name} · Dia</title></svelte:head>
<svelte:window onkeydown={onKeydown} />

<div class="relative flex h-full flex-col bg-bg text-ink">
	<!-- ── Slab topbar ──────────────────────────────────────────────────── -->
	<PageTopbar
		eyebrow="Reaction Roles"
		subtitle="Let members self-assign roles from buttons or a menu you post."
	>
		{#snippet leading()}
			<div class="grid size-6 place-items-center rounded border border-line bg-surface text-muted">
				<ToggleRight size={13} />
			</div>
		{/snippet}
		{#snippet actions()}
			<label class="ml-1 flex items-center gap-2 text-[12px]">
				<span class="hidden text-muted sm:inline">{enabled ? 'On' : 'Off'}</span>
				<Toggle bind:checked={enabled} label="Reaction roles" />
			</label>
		{/snippet}
	</PageTopbar>

	<!-- ── Body ─────────────────────────────────────────────────────────── -->
	<div class="relative min-h-0 flex-1 overflow-y-auto bg-bg pb-20">
		{#if !loaded}
			<div class="p-6">
				<div class="skeleton mb-3 h-6 w-40 rounded"></div>
				<div class="skeleton h-72 w-full rounded"></div>
			</div>
		{:else}
			<!-- ── Menu list ────────────────────────────────────────────────── -->
			<SectionBar label="Menus" count={menus.length}>
				{#snippet children()}
					<TopbarAction variant="primary" onclick={startNew}>
						{#snippet icon()}<Plus size={13} />{/snippet}
						New menu
					</TopbarAction>
				{/snippet}
			</SectionBar>

			{#if menus.length === 0}
				<EmptyBlock
					title="No menus yet"
					body="Create a menu to let members pick their own roles from buttons or a dropdown you post to a channel."
				>
					{#snippet cta()}
						<TopbarAction variant="primary" onclick={startNew}>
							{#snippet icon()}<Plus size={13} />{/snippet}
							New menu
						</TopbarAction>
					{/snippet}
				</EmptyBlock>
			{:else}
				{#each menus as m (m.id)}
					<Row>
						{#snippet children()}
							<span class="min-w-0 flex-1 truncate text-[13px] font-medium text-ink">
								{m.title || 'Untitled menu'}
							</span>
							<span class="shrink-0 text-[12px] text-muted">
								{m.options.length === 1 ? '1 role' : `${m.options.length} roles`}
							</span>
							<span class="shrink-0 rounded border border-line px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-[0.1em] text-muted">
								{modeLabel(m.mode)}
							</span>
							{#if isPosted(m) && channelName(m.channel_id)}
								<span class="shrink-0 truncate font-mono text-[11px] text-muted">
									#{channelName(m.channel_id)}
								</span>
							{/if}
							{#if isPosted(m)}
								<span class="flex shrink-0 items-center gap-1.5 rounded-full border border-line px-2 py-0.5 text-[11px] font-medium text-muted">
									<span class="size-1.5 rounded-full bg-success"></span>
									Posted
								</span>
							{:else}
								<span class="shrink-0 rounded-full border border-line px-2 py-0.5 text-[11px] font-medium text-faint">
									Not posted yet
								</span>
							{/if}
							<div class="ml-auto flex shrink-0 items-center gap-1.5 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100">
								<TopbarAction variant="ghost" onclick={() => startEdit(m)}>
									{#snippet icon()}<Pencil size={12} />{/snippet}
									Edit
								</TopbarAction>
								{#if m.id != null}
									<TopbarAction
										variant="ghost"
										onclick={() => goto('/servers/' + store.id + '/automations/reactionroles.menu.' + m.id)}
										ariaLabel="Customize follow-up flow"
										title="Customize follow-up flow"
									>
										{#snippet icon()}<Route size={12} />{/snippet}
										Flow
									</TopbarAction>
								{/if}
								<TopbarAction
									variant="danger"
									onclick={() => askDelete(m)}
									ariaLabel="Delete menu"
									title="Delete menu"
								>
									{#snippet icon()}<Trash2 size={12} />{/snippet}
								</TopbarAction>
							</div>
						{/snippet}
					</Row>
				{/each}
			{/if}
		{/if}
	</div>

	<!-- Release dock — the enable-flag saving experience -->
	{#if loaded}
		<ReleaseDock {dirty} phase={savePhase} error={saveErr} onsave={save} onreset={reset} />
	{/if}
</div>

<MenuEditorModal bind:open={modalOpen} guildId={store.id} menu={editingMenu} onSaved={reload} />

<ConfirmDialog
	bind:open={confirmOpen}
	title="Delete this menu?"
	description="This removes the menu and its options. If it was posted, the message stays but stops handing out roles."
	confirmLabel="Delete menu"
	cancelLabel="Keep it"
	onconfirm={confirmDelete}
	oncancel={() => (pendingDelete = null)}
/>
