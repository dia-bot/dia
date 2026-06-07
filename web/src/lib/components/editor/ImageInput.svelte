<script lang="ts">
	// A URL text field paired with an upload button + drag-and-drop, used for any
	// image source in the editor (layer src, avatar src, background image). Typing
	// a URL still works; uploading sends the file to the guild's object store and
	// writes back the returned public URL. Upload is unavailable (button hidden)
	// when no guild is known; a clear error shows if the server has no storage.
	import { uploadImage } from '$lib/api';
	import { Upload, Loader2 } from 'lucide-svelte';

	let {
		value = '',
		onChange,
		guildId,
		placeholder = 'https://…'
	}: {
		value?: string;
		onChange: (v: string) => void;
		guildId: string;
		placeholder?: string;
	} = $props();

	let busy = $state(false);
	let error = $state('');
	let dragover = $state(false);
	let fileEl = $state<HTMLInputElement>();

	async function upload(file: File | null | undefined) {
		if (!file || !guildId) return;
		busy = true;
		error = '';
		try {
			onChange(await uploadImage(guildId, file));
		} catch (e) {
			error = e instanceof Error ? e.message : 'Upload failed';
		} finally {
			busy = false;
		}
	}
	function onDrop(e: DragEvent) {
		e.preventDefault();
		dragover = false;
		upload(e.dataTransfer?.files?.[0]);
	}
</script>

<div class="space-y-1.5">
	<div
		class="flex gap-1.5 rounded-md {dragover ? 'ring-1 ring-accent' : ''}"
		role="group"
		ondragover={(e) => {
			e.preventDefault();
			dragover = true;
		}}
		ondragleave={() => (dragover = false)}
		ondrop={onDrop}
	>
		<input
			type="text"
			{placeholder}
			{value}
			oninput={(e) => onChange(e.currentTarget.value)}
			class="h-8 w-full min-w-0 rounded-md border border-line-strong bg-ink-2 px-2 text-sm text-ink outline-none transition-colors hover:border-faint focus:border-accent"
		/>
		{#if guildId}
			<button
				type="button"
				onclick={() => fileEl?.click()}
				disabled={busy}
				title="Upload an image"
				aria-label="Upload an image"
				class="grid h-8 w-8 shrink-0 place-items-center rounded-md border border-line-strong text-muted transition-colors hover:border-faint hover:text-ink disabled:opacity-50"
			>
				{#if busy}<Loader2 size={14} class="animate-spin" />{:else}<Upload size={14} />{/if}
			</button>
			<input
				bind:this={fileEl}
				type="file"
				accept="image/png,image/jpeg,image/webp,image/gif"
				class="hidden"
				onchange={(e) => {
					upload(e.currentTarget.files?.[0]);
					e.currentTarget.value = '';
				}}
			/>
		{/if}
	</div>
	{#if error}
		<p class="text-[11px] text-danger">{error}</p>
	{/if}
</div>
