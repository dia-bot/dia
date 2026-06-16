<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import Select from '$lib/components/Select.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import SaveBar from '$lib/components/SaveBar.svelte';
	import { Plus, Trash2, Pencil, X } from 'lucide-svelte';

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
		{ value: 'toggle', label: 'Toggle — add/remove freely' },
		{ value: 'unique', label: 'Unique — only one role at a time' },
		{ value: 'verify', label: 'Verify — single-use opt-in' }
	];

	let enabled = $state(false);
	let loaded = $state(false);
	let saving = $state(false);
	let baseline = $state('');

	let menus = $state<Menu[]>([]);
	let editing = $state<Menu | null>(null);
	let savingMenu = $state(false);

	const dirty = $derived(loaded && JSON.stringify({ enabled }) !== baseline);

	function emptyMenu(): Menu {
		return { title: '', mode: 'toggle', options: [blankOption()] };
	}
	function blankOption(): Option {
		return { role_id: '', label: '', emoji: '', description: '' };
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
		baseline = JSON.stringify({ enabled });
	});

	function modeLabel(mode: string) {
		return MODE_OPTS.find((m) => m.value === mode)?.label.split(' — ')[0] ?? mode;
	}

	function startNew() {
		editing = emptyMenu();
	}
	function startEdit(m: Menu) {
		editing = {
			id: m.id,
			title: m.title,
			mode: m.mode,
			options: m.options.length ? m.options.map((o) => ({ ...o })) : [blankOption()]
		};
	}
	function cancelEdit() {
		editing = null;
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
			await api.upsertMenu(store.id, payload);
			await reload();
			editing = null;
		} finally {
			savingMenu = false;
		}
	}

	async function removeMenu(m: Menu) {
		if (m.id == null) return;
		await api.deleteMenu(store.id, m.id);
		if (editing && editing.id === m.id) editing = null;
		await reload();
	}

	async function save() {
		saving = true;
		try {
			await api.saveFeature(store.id, FEATURE, enabled, {});
			if (store.detail) store.detail.features[FEATURE] = { enabled, config: {} };
			baseline = JSON.stringify({ enabled });
		} finally {
			saving = false;
		}
	}

	function reset() {
		const b = JSON.parse(baseline);
		enabled = b.enabled;
	}
</script>

<svelte:head><title>Reaction Roles · {store.name} · Dia</title></svelte:head>

<header class="mb-6 flex items-start justify-between gap-4">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Reaction Roles</h1>
		<p class="mt-1 text-muted">Let members self-assign roles from a posted menu.</p>
	</div>
	<Toggle bind:checked={enabled} />
</header>

{#if !loaded}
	<div class="text-muted">Loading…</div>
{:else}
	<div class="space-y-5">
		<!-- Existing menus -->
		<section class="card p-6">
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-base font-semibold">Menus</h2>
				<button class="btn btn-ghost h-9" onclick={startNew}>
					<Plus size={15} /> New menu
				</button>
			</div>

			{#if menus.length === 0}
				<p class="text-sm text-muted">No menus yet. Create one to let members pick their roles.</p>
			{:else}
				<ul class="divide-y divide-line">
					{#each menus as m (m.id)}
						<li class="flex items-center justify-between gap-4 py-3 first:pt-0 last:pb-0">
							<div class="min-w-0">
								<div class="flex items-center gap-2">
									<span class="truncate font-medium">{m.title || 'Untitled menu'}</span>
									{#if m.message_id && m.message_id !== '0'}
										<span class="rounded-full bg-blush px-2 py-0.5 text-[11px] font-medium text-accent-ink">Posted</span>
									{/if}
								</div>
								<div class="mt-0.5 text-xs text-muted">
									{modeLabel(m.mode)} · {m.options.length}
									{m.options.length === 1 ? 'option' : 'options'}
								</div>
							</div>
							<div class="flex shrink-0 items-center gap-1">
								<button class="btn btn-ghost h-9 px-3" onclick={() => startEdit(m)}>
									<Pencil size={14} /> Edit
								</button>
								<button
									class="btn btn-ghost h-9 px-3 text-danger"
									onclick={() => removeMenu(m)}
									aria-label="Delete menu"
								>
									<Trash2 size={14} />
								</button>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<!-- Editor -->
		{#if editing}
			<section class="card p-6">
				<div class="mb-4 flex items-center justify-between">
					<h2 class="text-base font-semibold">{editing.id != null ? 'Edit menu' : 'New menu'}</h2>
					<button class="text-muted hover:text-ink" onclick={cancelEdit} aria-label="Close editor">
						<X size={18} />
					</button>
				</div>

				<div class="grid gap-4 sm:grid-cols-2">
					<Field label="Title">
						<input class="input" placeholder="Pick your roles" bind:value={editing.title} />
					</Field>
					<Field label="Mode" hint="How members interact with the options.">
						<Select bind:value={editing.mode} options={MODE_OPTS} />
					</Field>
				</div>

				<Field label="Options">
					<div class="space-y-3">
						{#each editing.options as opt, i (i)}
							<div class="rounded-xl border border-line-strong p-3">
								<div class="mb-2 flex items-center justify-between">
									<span class="eyebrow">Option {i + 1}</span>
									<button
										type="button"
										class="text-muted hover:text-danger"
										onclick={() => removeOption(i)}
										aria-label="Remove option"
									>
										<X size={15} />
									</button>
								</div>
								<div class="grid gap-2 sm:grid-cols-2">
									<RolePicker value={opt.role_id} onChange={(v) => (opt.role_id = v as string)} placeholder="Select a role…" />
									<input class="input" placeholder="Label (optional)" bind:value={opt.label} />
									<input class="input" placeholder="Emoji (optional, e.g. 🎮)" bind:value={opt.emoji} />
									<input class="input" placeholder="Description (optional)" bind:value={opt.description} />
								</div>
							</div>
						{/each}
						<button type="button" class="btn btn-ghost h-9" onclick={addOption}>
							<Plus size={15} /> Add option
						</button>
					</div>
				</Field>

				<div class="mt-2 flex items-center justify-end gap-2">
					<button class="btn btn-ghost" onclick={cancelEdit} disabled={savingMenu}>Cancel</button>
					<button class="btn btn-accent" onclick={saveMenu} disabled={savingMenu || !canSaveMenu}>
						{savingMenu ? 'Saving…' : 'Save menu'}
					</button>
				</div>
			</section>
		{/if}

		<!-- Hint -->
		<div class="rounded-xl border border-line bg-blush/40 p-4 text-sm text-muted">
			After saving a menu, run
			<code class="break-words rounded bg-ink-2 px-1.5 py-0.5 font-mono text-[13px] text-accent-ink">/reactionroles post &lt;id&gt; &lt;channel&gt;</code>
			in your server to post it.
		</div>
	</div>

	<SaveBar {dirty} {saving} onsave={save} onreset={reset} />
{/if}
