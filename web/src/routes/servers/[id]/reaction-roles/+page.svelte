<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import Select from '$lib/components/Select.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import Row from '$lib/components/page/Row.svelte';
	import TopbarAction from '$lib/components/page/TopbarAction.svelte';
	import EmptyBlock from '$lib/components/page/EmptyBlock.svelte';
	import ReleaseDock from '$lib/components/page/ReleaseDock.svelte';
	import { ToggleRight, Plus, Trash2, Pencil, X, Send, Check } from 'lucide-svelte';

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
	let editing = $state<Menu | null>(null);
	let savingMenu = $state(false);

	// Delete confirmation
	let confirmOpen = $state(false);
	let pendingDelete = $state<Menu | null>(null);

	// Post-to-channel state (per open editor)
	let postChannelId = $state('');
	let posting = $state(false);
	let postMsg = $state('');
	let postOk = $state(false);

	function emptyMenu(): Menu {
		return { title: '', mode: 'toggle', options: [blankOption()] };
	}
	function blankOption(): Option {
		return { role_id: '', label: '', emoji: '', description: '' };
	}

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

	function startNew() {
		editing = emptyMenu();
		resetPostState();
	}
	function startEdit(m: Menu) {
		editing = {
			id: m.id,
			title: m.title,
			mode: m.mode,
			channel_id: m.channel_id,
			message_id: m.message_id,
			options: m.options.length ? m.options.map((o) => ({ ...o })) : [blankOption()]
		};
		postChannelId = m.channel_id && m.channel_id !== '0' ? m.channel_id : '';
		postMsg = '';
		postOk = false;
	}
	function cancelEdit() {
		editing = null;
		resetPostState();
	}
	function resetPostState() {
		postChannelId = '';
		postMsg = '';
		postOk = false;
	}
	function addOption() {
		if (editing) editing.options = [...editing.options, blankOption()];
	}
	function removeOption(i: number) {
		if (editing) editing.options = editing.options.filter((_, idx) => idx !== i);
	}

	const canSaveMenu = $derived(
		!!editing &&
			editing.title.trim().length > 0 &&
			editing.options.length > 0 &&
			editing.options.every((o) => o.role_id)
	);

	async function saveMenu() {
		if (!editing || !canSaveMenu) return;
		savingMenu = true;
		try {
			const payload: Menu = {
				...(editing.id != null ? { id: editing.id } : {}),
				title: editing.title.trim(),
				mode: editing.mode,
				options: editing.options.map((o) => ({
					role_id: o.role_id,
					label: o.label.trim(),
					emoji: o.emoji.trim(),
					description: o.description.trim()
				}))
			};
			const res = await api.upsertMenu(store.id, payload);
			await reload();
			// Keep the editor open on the freshly saved menu so posting is the next
			// step without a re-open. Resolve its id from the response or the reload.
			const savedId = editing.id ?? res.id;
			const fresh = menus.find((m) => m.id === savedId);
			if (fresh) startEdit(fresh);
		} finally {
			savingMenu = false;
		}
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
		if (editing && editing.id === m.id) editing = null;
		await reload();
	}

	async function postMenu() {
		if (!editing || editing.id == null || !postChannelId) return;
		posting = true;
		postMsg = '';
		postOk = false;
		try {
			const res = await api.postMenu(store.id, editing.id, postChannelId);
			postOk = !!res.ok;
			postMsg = res.ok ? 'Posted to the channel.' : "Couldn't post the menu.";
			await reload();
			const fresh = menus.find((m) => m.id === editing?.id);
			if (fresh && editing) {
				editing.channel_id = fresh.channel_id;
				editing.message_id = fresh.message_id;
			}
		} catch (e) {
			postOk = false;
			postMsg = e instanceof Error ? e.message : "Couldn't post the menu.";
		} finally {
			posting = false;
		}
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

<div class="flex h-full flex-col bg-bg text-ink">
	<!-- ── Slab topbar ──────────────────────────────────────────────────── -->
	<PageTopbar
		eyebrow="Reaction Roles"
		subtitle="Let members self-assign roles from buttons or a menu you post."
	>
		{#snippet leading()}
			<div class="grid size-6 place-items-center rounded border border-line bg-surface text-accent-ink">
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
	<div class="relative min-h-0 flex-1 overflow-y-auto bg-bg">
		{#if !loaded}
			<div class="mx-auto w-full max-w-2xl p-6">
				<div class="skeleton mb-3 h-6 w-40 rounded"></div>
				<div class="skeleton h-40 w-full rounded-xl"></div>
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
							<span class="shrink-0 rounded border border-line px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-[0.1em] text-muted">
								{modeLabel(m.mode)}
							</span>
							{#if isPosted(m)}
								<span class="shrink-0 rounded-full bg-blush px-2 py-0.5 text-[11px] font-medium text-accent-ink">
									Posted
								</span>
							{:else}
								<span class="shrink-0 rounded-full border border-line px-2 py-0.5 text-[11px] font-medium text-faint">
									Not posted yet
								</span>
							{/if}
							<div class="ml-auto flex shrink-0 items-center gap-1.5">
								<TopbarAction variant="ghost" onclick={() => startEdit(m)}>
									{#snippet icon()}<Pencil size={12} />{/snippet}
									Edit
								</TopbarAction>
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

			<!-- ── Editor ───────────────────────────────────────────────────── -->
			{#if editing}
				<div class="px-5 py-6" in:fly={{ y: 8, duration: 160, easing: cubicOut }}>
					<div class="rounded-lg border border-line bg-surface p-5">
						<div class="mb-4 flex items-center justify-between">
							<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
								{editing.id != null ? 'Edit menu' : 'New menu'}
							</span>
							<button
								type="button"
								class="text-muted transition-colors hover:text-ink"
								onclick={cancelEdit}
								aria-label="Close editor"
							>
								<X size={16} />
							</button>
						</div>

						<div class="grid gap-x-4 sm:grid-cols-2">
							<Field label="Title">
								<input class="input" placeholder="Pick your roles" bind:value={editing.title} />
							</Field>
							<Field label="Mode" hint="How members interact with the options.">
								<Select bind:value={editing.mode} options={MODE_OPTS} />
							</Field>
						</div>

						<Field label="Roles">
							<div class="space-y-3">
								{#each editing.options as opt, i (i)}
									<div class="rounded-lg border border-line-strong bg-bg p-3">
										<div class="mb-2 flex items-center justify-between">
											<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
												Role {i + 1}
											</span>
											<button
												type="button"
												class="text-muted transition-colors hover:text-danger"
												onclick={() => removeOption(i)}
												aria-label="Remove role"
											>
												<X size={15} />
											</button>
										</div>
										<div class="grid gap-2 sm:grid-cols-2">
											<RolePicker
												value={opt.role_id}
												onChange={(v) => (opt.role_id = v as string)}
												placeholder="Select a role…"
											/>
											<input class="input" placeholder="Label (optional)" bind:value={opt.label} />
											<input class="input" placeholder="Emoji (optional, e.g. 🎮)" bind:value={opt.emoji} />
											<input class="input" placeholder="Description (optional)" bind:value={opt.description} />
										</div>
									</div>
								{/each}
								<TopbarAction variant="ghost" onclick={addOption}>
									{#snippet icon()}<Plus size={13} />{/snippet}
									Add role
								</TopbarAction>
							</div>
						</Field>

						<div class="flex items-center justify-end gap-2">
							<button class="btn btn-ghost" onclick={cancelEdit} disabled={savingMenu}>Cancel</button>
							<button
								class="btn btn-accent"
								onclick={saveMenu}
								disabled={savingMenu || !canSaveMenu}
							>
								{savingMenu ? 'Saving…' : 'Save menu'}
							</button>
						</div>

						<!-- ── Post to a channel (saved menus only) ─────────────────── -->
						{#if editing.id != null}
							<div class="mt-5 border-t border-line/60 pt-5">
								<div class="mb-2 flex items-center gap-2">
									<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
										Post to channel
									</span>
									{#if isPosted(editing)}
										<span class="rounded-full bg-blush px-2 py-0.5 text-[11px] font-medium text-accent-ink">
											Posted
										</span>
									{/if}
								</div>
								<p class="mb-3 text-[12px] text-muted">
									Post this menu to a channel. Re-posting sends a fresh message.
								</p>
								<div class="flex flex-wrap items-end gap-2">
									<div class="min-w-[220px] flex-1">
										<ChannelPicker
											value={postChannelId}
											onChange={(v) => (postChannelId = v as string)}
											placeholder="Channel to post the menu in"
										/>
									</div>
									<TopbarAction
										variant="ink"
										onclick={postMenu}
										disabled={posting || !postChannelId}
									>
										{#snippet icon()}<Send size={12} />{/snippet}
										{posting ? 'Posting…' : 'Post menu'}
									</TopbarAction>
								</div>
								{#if postMsg}
									<p class="mt-2 flex items-center gap-1.5 text-[12px] {postOk ? 'text-success' : 'text-danger'}">
										{#if postOk}<Check size={13} />{/if}
										{postMsg}
									</p>
								{/if}
							</div>
						{:else}
							<p class="mt-4 border-t border-line/60 pt-4 text-[12px] text-faint">
								Save the menu to post it to a channel.
							</p>
						{/if}
					</div>
				</div>
			{/if}
		{/if}

		<!-- Release dock — the enable-flag saving experience -->
		{#if loaded}
			<ReleaseDock {dirty} phase={savePhase} error={saveErr} onsave={save} onreset={reset} />
		{/if}
	</div>
</div>

<ConfirmDialog
	bind:open={confirmOpen}
	title="Delete this menu?"
	description="This removes the menu and its options. If it was posted, the message stays but stops handing out roles."
	confirmLabel="Delete menu"
	cancelLabel="Keep it"
	onconfirm={confirmDelete}
	oncancel={() => (pendingDelete = null)}
/>
