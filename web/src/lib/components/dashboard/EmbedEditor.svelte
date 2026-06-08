<script lang="ts">
	import ColorField from '$lib/components/ColorField.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { ChevronDown, Trash2, Plus } from 'lucide-svelte';

	type Field = { name: string; value: string; inline: boolean };
	type Embed = {
		enabled: boolean;
		color: string;
		author_name: string;
		author_icon: string;
		title: string;
		url: string;
		description: string;
		fields: Field[];
		thumbnail: string;
		image_url: string;
		footer_text: string;
		footer_icon: string;
		timestamp: boolean;
	};

	let { embed, index, onRemove }: { embed: Embed; index: number; onRemove: () => void } = $props();
	let open = $state(true);

	function addField() {
		embed.fields = [...embed.fields, { name: '', value: '', inline: false }];
	}
	function removeField(i: number) {
		embed.fields = embed.fields.filter((_, idx) => idx !== i);
	}
</script>

<div class="overflow-hidden rounded-lg border border-line bg-ink-2">
	<div class="flex items-center gap-2.5 px-3 py-2.5">
		<button
			type="button"
			onclick={() => (open = !open)}
			class="text-faint transition-transform hover:text-muted {open ? '' : '-rotate-90'}"
			aria-label="Collapse embed"
		>
			<ChevronDown size={15} />
		</button>
		<span class="h-3.5 w-1 rounded-full" style="background:{embed.color || '#5865f2'}"></span>
		<span class="text-sm font-medium text-ink">Embed {index + 1}</span>
		<span class="flex-1"></span>
		<button
			type="button"
			onclick={onRemove}
			class="grid h-7 w-7 place-items-center rounded-md text-faint transition-colors hover:bg-surface hover:text-danger"
			aria-label="Remove embed"
		>
			<Trash2 size={14} />
		</button>
	</div>

	{#if open}
		<div class="space-y-4 border-t border-line p-3.5">
			<div class="flex flex-wrap items-end gap-5">
				<div><div class="label">Color</div><ColorField bind:value={embed.color} /></div>
				<label class="flex items-center gap-2 pb-1.5 text-sm text-muted">
					<Toggle bind:checked={embed.timestamp} /> Timestamp
				</label>
			</div>
			<div class="grid gap-3 sm:grid-cols-2">
				<div><div class="label">Author</div><input class="input" bind:value={embed.author_name} placeholder="Name" /></div>
				<div><div class="label">Author icon</div><input class="input" bind:value={embed.author_icon} placeholder="{'{user.avatar}'}" /></div>
			</div>
			<div class="grid gap-3 sm:grid-cols-2">
				<div><div class="label">Title</div><input class="input" bind:value={embed.title} /></div>
				<div><div class="label">Title link</div><input class="input" bind:value={embed.url} placeholder="https://…" /></div>
			</div>
			<div><div class="label">Description</div><textarea class="input" rows="3" bind:value={embed.description}></textarea></div>

			<div>
				<div class="mb-2 flex items-center justify-between">
					<span class="label !mb-0">Fields</span>
					<button type="button" class="flex items-center gap-1 text-xs font-medium text-muted hover:text-ink" onclick={addField}>
						<Plus size={13} /> Add
					</button>
				</div>
				{#if embed.fields.length}
					<div class="space-y-2">
						{#each embed.fields as f, i (i)}
							<div class="rounded-lg border border-line bg-surface/50 p-2.5">
								<div class="flex items-center gap-2">
									<input class="input h-8 flex-1" placeholder="Field name" bind:value={f.name} />
									<label class="flex shrink-0 items-center gap-1.5 whitespace-nowrap text-xs text-muted">
										<Toggle bind:checked={f.inline} /> Inline
									</label>
									<button type="button" onclick={() => removeField(i)} class="grid h-7 w-7 shrink-0 place-items-center rounded-md text-faint hover:bg-surface hover:text-danger" aria-label="Remove field">
										<Trash2 size={13} />
									</button>
								</div>
								<input class="input mt-2 h-8" placeholder="Field value" bind:value={f.value} />
							</div>
						{/each}
					</div>
				{:else}
					<p class="text-xs text-faint">No fields yet.</p>
				{/if}
			</div>

			<div class="grid gap-3 sm:grid-cols-2">
				<div><div class="label">Thumbnail</div><input class="input" bind:value={embed.thumbnail} placeholder="{'{user.avatar}'}" /></div>
				<div><div class="label">Image</div><input class="input" bind:value={embed.image_url} placeholder="URL or {'{card}'}" /></div>
			</div>
			<div class="grid gap-3 sm:grid-cols-2">
				<div><div class="label">Footer</div><input class="input" bind:value={embed.footer_text} /></div>
				<div><div class="label">Footer icon</div><input class="input" bind:value={embed.footer_icon} /></div>
			</div>
		</div>
	{/if}
</div>
