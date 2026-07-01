<script lang="ts">
	// Create/edit a reaction-role menu inside an animated modal. The list page
	// stays the calm default; all editing happens here. Mirrors the shape of
	// NewCommandWizard (seed a local draft on open, one atomic save) and the
	// unsaved-guard pattern from CardStudioModal (a ConfirmDialog intercepts a
	// dirty close). The modal owns every api call, then calls onSaved() so the
	// page reloads its list.
	import { api } from '$lib/api';
	import { Dialog } from '$lib/components/ui';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import Field from '$lib/components/Field.svelte';
	import Select from '$lib/components/Select.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import TopbarAction from '$lib/components/page/TopbarAction.svelte';
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';
	import { ToggleRight, Plus, X, Send, Check } from 'lucide-svelte';

	type Option = { role_id: string; label: string; emoji: string; description: string };
	type Menu = {
		id?: number;
		title: string;
		mode: string;
		channel_id?: string;
		message_id?: string;
		options: Option[];
	};

	let {
		open = $bindable(false),
		guildId,
		menu,
		onSaved
	}: {
		open?: boolean;
		guildId: string;
		menu: Menu | null;
		onSaved: () => void;
	} = $props();

	const MODE_OPTS = [
		{ value: 'toggle', label: 'Toggle, add or remove freely' },
		{ value: 'unique', label: 'Unique, only one role at a time' },
		{ value: 'verify', label: 'Verify, single-use opt-in' }
	];

	function blankOption(): Option {
		return { role_id: '', label: '', emoji: '', description: '' };
	}
	function emptyMenu(): Menu {
		return { title: '', mode: 'toggle', options: [blankOption()] };
	}
	function clone(m: Menu): Menu {
		return {
			...(m.id != null ? { id: m.id } : {}),
			title: m.title ?? '',
			mode: m.mode ?? 'toggle',
			channel_id: m.channel_id,
			message_id: m.message_id,
			options: (m.options?.length ? m.options : [blankOption()]).map((o) => ({
				role_id: o.role_id ?? '',
				label: o.label ?? '',
				emoji: o.emoji ?? '',
				description: o.description ?? ''
			}))
		};
	}

	function isPosted(m: Menu | null): boolean {
		return (
			!!m &&
			!!m.message_id &&
			m.message_id !== '0' &&
			!!m.channel_id &&
			m.channel_id !== '0'
		);
	}
	function modeChip(mode: string): string {
		return MODE_OPTS.find((o) => o.value === mode)?.label.split(',')[0] ?? mode;
	}

	// ── Draft + phase ────────────────────────────────────────────────────────
	type Phase = 'edit' | 'post';
	let draft = $state<Menu>(emptyMenu());
	let baseline = $state('');
	let phase = $state<Phase>('edit');
	let savingMenu = $state(false);
	let saveErr = $state('');

	// Post step (compact, in-modal)
	let postChannelId = $state('');
	let posting = $state(false);
	let postMsg = $state('');
	let postOk = $state(false);

	const isNew = $derived(menu == null || menu.id == null);

	// Seed a fresh draft copy on a genuine open (closed -> open). A guard-driven
	// re-open (see guardClose) toggles `open` within a session, so track whether
	// we've already seeded to avoid wiping in-progress edits. Mirrors
	// NewCommandWizard's reset-on-open effect.
	let seeded = false;
	$effect(() => {
		if (!open) {
			seeded = false;
			return;
		}
		if (seeded) return;
		seeded = true;
		draft = clone(menu ?? emptyMenu());
		baseline = JSON.stringify(draft);
		phase = 'edit';
		savingMenu = false;
		saveErr = '';
		postChannelId = draft.channel_id && draft.channel_id !== '0' ? draft.channel_id : '';
		posting = false;
		postMsg = '';
		postOk = false;
	});

	const canSaveMenu = $derived(
		draft.title.trim().length > 0 &&
			draft.options.length > 0 &&
			draft.options.every((o) => o.role_id)
	);

	function addOption() {
		draft.options = [...draft.options, blankOption()];
	}
	function removeOption(i: number) {
		draft.options = draft.options.filter((_, idx) => idx !== i);
	}

	// ── Unsaved guard ────────────────────────────────────────────────────────
	function isDirty(): boolean {
		return JSON.stringify(draft) !== baseline;
	}
	let confirmOpen = $state(false);
	// While a save->post transition is in flight the draft baseline no longer
	// matches the list, but that is not an "unsaved" edit — suppress the guard.
	let transitioning = false;

	// Controlled close: intercept the dialog's own close attempts (Esc, outside
	// click, X) so a dirty edit-phase draft asks before discarding. bits-ui sets
	// our bound `open=false`; we re-open and raise the confirm instead.
	function guardClose() {
		if (transitioning || phase === 'post' || !isDirty()) {
			doClose();
			return;
		}
		open = true;
		confirmOpen = true;
	}
	function doClose() {
		confirmOpen = false;
		open = false;
	}
	function keepEditing() {
		confirmOpen = false;
		open = true;
	}

	// ── Save → Post ──────────────────────────────────────────────────────────
	async function saveMenu() {
		if (!canSaveMenu || savingMenu) return;
		savingMenu = true;
		saveErr = '';
		try {
			const payload: Menu = {
				...(draft.id != null ? { id: draft.id } : {}),
				title: draft.title.trim(),
				mode: draft.mode,
				options: draft.options.map((o) => ({
					role_id: o.role_id,
					label: o.label.trim(),
					emoji: o.emoji.trim(),
					description: o.description.trim()
				}))
			};
			const res = await api.upsertMenu(guildId, payload);
			if (draft.id == null && res.id != null) draft.id = res.id;
			// The save succeeded; the edit-phase draft now matches what's stored,
			// so the guard must not fire as we swap to the post step.
			transitioning = true;
			baseline = JSON.stringify(draft);
			phase = 'post';
			onSaved();
		} catch (e) {
			saveErr = e instanceof Error ? e.message : "Couldn't save the menu.";
		} finally {
			savingMenu = false;
			transitioning = false;
		}
	}

	async function postMenu() {
		if (draft.id == null || !postChannelId || posting) return;
		posting = true;
		postMsg = '';
		postOk = false;
		try {
			const res = await api.postMenu(guildId, draft.id, postChannelId);
			postOk = !!res.ok;
			postMsg = res.ok ? 'Posted to the channel.' : "Couldn't post the menu.";
			if (res.ok) {
				draft.channel_id = postChannelId;
				draft.message_id = res.message_id;
			}
			onSaved();
		} catch (e) {
			postOk = false;
			postMsg = e instanceof Error ? e.message : "Couldn't post the menu.";
		} finally {
			posting = false;
		}
	}
</script>

<Dialog.Root
	bind:open
	onOpenChange={(next) => {
		if (!next) guardClose();
	}}
>
	<Dialog.Content class="max-w-[640px] gap-0 overflow-hidden p-0" showClose={false}>
		<Dialog.Title class="sr-only">{isNew ? 'New menu' : 'Edit menu'}</Dialog.Title>

		<!-- Header -->
		<div class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-4">
			<div class="grid size-5 place-items-center rounded border border-line bg-surface text-muted">
				<ToggleRight size={11} />
			</div>
			<span class="text-[12.5px] font-medium text-ink">{isNew ? 'New menu' : 'Edit menu'}</span>
			<span
				class="rounded border border-line px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-[0.12em] text-faint"
			>
				{modeChip(draft.mode)}
			</span>
			<button
				type="button"
				class="ml-auto grid size-6 place-items-center rounded-md text-muted transition-colors hover:bg-surface hover:text-ink"
				onclick={guardClose}
				aria-label="Close"
			>
				<X size={14} />
			</button>
		</div>

		<!-- Body — edit / post slide across on the phase change -->
		<div class="max-h-[70vh] overflow-y-auto px-5 py-5">
			{#key phase}
				<div in:fly={{ x: 14, duration: dur(200), easing: cubicOut }}>
					{#if phase === 'edit'}
						<div class="grid gap-x-3 sm:grid-cols-2">
							<Field label="Title">
								<input class="input" placeholder="Pick your roles" bind:value={draft.title} />
							</Field>
							<Field label="Mode" hint="How members interact with the options.">
								<Select bind:value={draft.mode} options={MODE_OPTS} />
							</Field>
						</div>

						<Field label="Roles">
							<div class="space-y-4">
								{#each draft.options as opt, i (i)}
									<div class="border-b border-line/60 pb-4">
										<div class="mb-2 flex items-center justify-between">
											<span
												class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
											>
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
											<input
												class="input"
												placeholder="Description (optional)"
												bind:value={opt.description}
											/>
										</div>
									</div>
								{/each}
								<TopbarAction variant="ghost" onclick={addOption}>
									{#snippet icon()}<Plus size={13} />{/snippet}
									Add role
								</TopbarAction>
							</div>
						</Field>

						{#if saveErr}
							<p class="mt-1 text-[12.5px] text-danger">{saveErr}</p>
						{/if}
					{:else}
						<!-- Post step -->
						<div>
							<span
								class="mb-1.5 block font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
							>
								Post to a channel
							</span>
							<p class="mb-3 text-[12.5px] text-muted">
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
								<TopbarAction variant="ink" onclick={postMenu} disabled={posting || !postChannelId}>
									{#snippet icon()}<Send size={12} />{/snippet}
									{posting ? 'Posting…' : 'Post menu'}
								</TopbarAction>
							</div>
							{#if isPosted(draft) && !postMsg}
								<p class="mt-2 text-[12.5px] text-faint">Already posted. Re-post to refresh it.</p>
							{/if}
							{#if postMsg}
								<p
									class="mt-2 flex items-center gap-1.5 text-[12.5px] {postOk
										? 'text-success'
										: 'text-danger'}"
								>
									{#if postOk}<Check size={13} />{/if}
									{postMsg}
								</p>
							{/if}
						</div>
					{/if}
				</div>
			{/key}
		</div>

		<!-- Footer -->
		<div class="flex h-12 shrink-0 items-center justify-end gap-2 border-t border-line px-4">
			{#if phase === 'edit'}
				<button class="btn btn-ghost" onclick={guardClose} disabled={savingMenu}>Cancel</button>
				<button class="btn btn-accent" onclick={saveMenu} disabled={savingMenu || !canSaveMenu}>
					{savingMenu ? 'Saving…' : 'Save menu'}
				</button>
			{:else}
				<button class="btn btn-accent" onclick={doClose}>Done</button>
			{/if}
		</div>
	</Dialog.Content>
</Dialog.Root>

<ConfirmDialog
	bind:open={confirmOpen}
	title="Discard changes?"
	description="You have unsaved changes to this menu. Discard them, or keep editing?"
	confirmLabel="Discard"
	cancelLabel="Keep editing"
	onconfirm={doClose}
	oncancel={keepEditing}
/>
