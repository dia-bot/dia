<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ColorField from '$lib/components/ColorField.svelte';
	import Plus from 'lucide-svelte/icons/plus';
	import Pencil from 'lucide-svelte/icons/pencil';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import Slash from 'lucide-svelte/icons/slash';

	const store = getContext<GuildStore>(GUILD_CTX);

	type Embed = { title: string; description: string; color: string; image_url: string };
	type Response = { content: string; ephemeral: boolean; embed?: Embed };
	type Command = {
		id?: number;
		name: string;
		description: string;
		enabled: boolean;
		response: Response;
	};

	const NAME_RULE = /^[a-z0-9_-]{1,32}$/;

	function blank(): Command {
		return {
			name: '',
			description: '',
			enabled: true,
			response: { content: '', ephemeral: false }
		};
	}

	function emptyEmbed(): Embed {
		return { title: '', description: '', color: '#B244FC', image_url: '' };
	}

	let commands = $state<Command[]>([]);
	let loaded = $state(false);
	let saving = $state(false);

	// Editor state. `editing` is null when the form is closed.
	let editing = $state<Command | null>(null);
	let useEmbed = $state(false);

	const nameValid = $derived(!!editing && NAME_RULE.test(editing.name));
	const canSave = $derived(
		!!editing && nameValid && editing.description.trim().length > 0 && !saving
	);

	onMount(async () => {
		await reload();
		loaded = true;
	});

	async function reload() {
		const res = await api.commands(store.id);
		commands = (res.commands ?? []).map((c: any) => ({
			id: c.id,
			name: c.name ?? '',
			description: c.description ?? '',
			enabled: c.enabled ?? true,
			response: {
				content: c.response?.content ?? '',
				ephemeral: c.response?.ephemeral ?? false,
				embed: c.response?.embed
					? { ...emptyEmbed(), ...c.response.embed }
					: undefined
			}
		}));
	}

	function startCreate() {
		editing = blank();
		useEmbed = false;
	}

	function startEdit(cmd: Command) {
		// Deep clone so edits stay local until saved.
		editing = {
			...cmd,
			response: {
				...cmd.response,
				embed: cmd.response.embed ? { ...cmd.response.embed } : undefined
			}
		};
		useEmbed = !!cmd.response.embed;
	}

	function cancel() {
		editing = null;
	}

	function toggleEmbed(on: boolean) {
		if (!editing) return;
		useEmbed = on;
		if (on && !editing.response.embed) editing.response.embed = emptyEmbed();
	}

	async function save() {
		if (!editing || !canSave) return;
		saving = true;
		try {
			const e = editing;
			const response: Response = {
				content: e.response.content,
				ephemeral: e.response.ephemeral
			};
			if (useEmbed && e.response.embed) response.embed = e.response.embed;
			const payload: Command = {
				name: e.name.trim(),
				description: e.description.trim(),
				enabled: e.enabled,
				response
			};
			if (e.id != null) payload.id = e.id;
			await api.upsertCommand(store.id, payload);
			await reload();
			editing = null;
		} finally {
			saving = false;
		}
	}

	async function remove(cmd: Command) {
		if (cmd.id == null) return;
		if (!confirm(`Delete the /${cmd.name} command?`)) return;
		await api.deleteCommand(store.id, cmd.id);
		if (editing?.id === cmd.id) editing = null;
		await reload();
	}
</script>

<svelte:head><title>Commands · {store.name} · Dia</title></svelte:head>

<header class="mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-4">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Custom commands</h1>
		<p class="mt-1 text-muted">Create your own slash commands with custom replies and embeds.</p>
	</div>
	{#if loaded && !editing}
		<button class="btn btn-accent shrink-0" onclick={startCreate}>
			<Plus size={16} /> New command
		</button>
	{/if}
</header>

{#if !loaded}
	<div class="text-muted">Loading…</div>
{:else}
	<div class="space-y-5">
		<!-- Existing commands -->
		<section class="card p-6">
			<h2 class="mb-4 text-base font-semibold">Your commands</h2>
			{#if commands.length === 0}
				<div class="flex flex-col items-center justify-center gap-2 py-8 text-center">
					<Slash size={22} class="text-faint" />
					<p class="text-sm text-muted">No custom commands yet.</p>
				</div>
			{:else}
				<ul class="divide-y divide-line">
					{#each commands as cmd (cmd.id)}
						<li class="flex items-start justify-between gap-4 py-3 first:pt-0 last:pb-0">
							<div class="min-w-0">
								<div class="flex items-center gap-2">
									<code class="font-mono text-sm font-semibold text-accent-ink">/{cmd.name}</code>
									<span
										class="rounded-full px-2 py-0.5 text-[11px] font-medium {cmd.enabled
											? 'bg-blush text-accent-ink'
											: 'bg-ink-2 text-muted'}"
									>
										{cmd.enabled ? 'Enabled' : 'Disabled'}
									</span>
								</div>
								<p class="mt-0.5 truncate text-sm text-muted">{cmd.description}</p>
							</div>
							<div class="flex shrink-0 items-center gap-1">
								<button
									class="btn btn-ghost h-9 px-3"
									onclick={() => startEdit(cmd)}
									aria-label="Edit command"
								>
									<Pencil size={15} /> Edit
								</button>
								<button
									class="btn btn-ghost h-9 px-3"
									onclick={() => remove(cmd)}
									aria-label="Delete command"
								>
									<Trash2 size={15} />
								</button>
							</div>
						</li>
					{/each}
				</ul>
			{/if}
			<p class="hint mt-4">
				Saved commands are registered to your server instantly as slash commands.
			</p>
		</section>

		<!-- Create / edit form -->
		{#if editing}
			<section class="card p-6">
				<div class="mb-4 flex items-center justify-between">
					<h2 class="text-base font-semibold">
						{editing.id != null ? `Edit /${editing.name}` : 'New command'}
					</h2>
					<Toggle bind:checked={editing.enabled} />
				</div>

				<div class="grid gap-4 sm:grid-cols-2">
					<Field label="Name" hint="Lowercase letters, numbers, - and _ only. Max 32 characters.">
						<div class="flex items-center gap-2">
							<span class="text-muted">/</span>
							<input
								class="input"
								maxlength="32"
								placeholder="poll"
								bind:value={editing.name}
							/>
						</div>
						{#if editing.name && !nameValid}
							<p class="hint" style="color: var(--color-danger)">
								Use only a–z, 0–9, hyphen and underscore (1–32 chars).
							</p>
						{/if}
					</Field>
					<Field label="Description" hint="Shown in Discord's command picker.">
						<input class="input" placeholder="Start a quick poll" bind:value={editing.description} />
					</Field>
				</div>

				<Field
					label="Response"
					hint="Placeholders: {'{user}'} {'{user.mention}'} {'{server}'}"
				>
					<textarea
						class="input"
						rows="3"
						placeholder="Hey {'{user.mention}'}, here you go!"
						bind:value={editing.response.content}
					></textarea>
				</Field>

				<label class="mb-5 flex items-center gap-3">
					<Toggle bind:checked={editing.response.ephemeral} />
					<span class="text-sm">Ephemeral — only the invoking user sees the reply</span>
				</label>

				<!-- Embed -->
				<div class="rounded-xl border border-line p-4">
					<label class="flex items-center justify-between gap-3">
						<span class="text-sm font-medium">Attach an embed</span>
						<Toggle checked={useEmbed} onchange={toggleEmbed} />
					</label>

					{#if useEmbed && editing.response.embed}
						<div class="mt-4 space-y-1">
							<div class="grid gap-4 sm:grid-cols-2">
								<Field label="Title">
									<input class="input" bind:value={editing.response.embed.title} />
								</Field>
								<ColorField label="Color" bind:value={editing.response.embed.color} />
							</div>
							<Field label="Description">
								<textarea class="input" rows="2" bind:value={editing.response.embed.description}
								></textarea>
							</Field>
							<Field label="Image URL">
								<input
									class="input"
									placeholder="https://image.png"
									bind:value={editing.response.embed.image_url}
								/>
							</Field>
						</div>
					{/if}
				</div>

				<div class="mt-5 flex items-center justify-end gap-2">
					<button class="btn btn-ghost" onclick={cancel} disabled={saving}>Cancel</button>
					<button class="btn btn-accent" onclick={save} disabled={!canSave}>
						{saving ? 'Saving…' : editing.id != null ? 'Save command' : 'Create command'}
					</button>
				</div>
			</section>
		{/if}
	</div>
{/if}
