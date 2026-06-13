<script lang="ts">
	// Clickable image slot used across the message composer (embed image,
	// thumbnail, author / footer icons, attachments). Opens a popover where you
	// can UPLOAD a picture (goes through the guild's asset endpoint and stores
	// the public URL), paste a URL, or reference a template variable — the
	// value is a Go template like every other string in a custom command.
	import { page } from '$app/stores';
	import { uploadImage } from '$lib/api';
	import { Popover } from '$lib/components/ui';

	import ImagePlus from 'lucide-svelte/icons/image-plus';
	import Upload from 'lucide-svelte/icons/upload';
	import Braces from 'lucide-svelte/icons/braces';
	import Trash2 from 'lucide-svelte/icons/trash-2';

	let {
		value = '',
		onChange,
		shape = 'thumb',
		label = 'Image'
	}: {
		value?: string;
		onChange: (v: string) => void;
		shape?: 'icon' | 'thumb' | 'banner';
		label?: string;
	} = $props();

	let open = $state(false);
	let uploading = $state(false);
	let uploadError = $state('');
	let fileEl: HTMLInputElement | null = $state(null);

	const isUrl = $derived(/^https?:\/\//.test(value));

	async function onFile(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		uploading = true;
		uploadError = '';
		try {
			const url = await uploadImage($page.params.id ?? '', file);
			onChange(url);
			open = false;
		} catch (err) {
			uploadError = err instanceof Error ? err.message : 'Upload failed';
		} finally {
			uploading = false;
			input.value = '';
		}
	}

	const frame = $derived(
		shape === 'icon'
			? 'size-6 rounded-full'
			: shape === 'banner'
				? 'h-28 w-full rounded-md'
				: 'size-[72px] rounded-md'
	);
</script>

<Popover.Root bind:open>
	<Popover.Trigger
		class="group/img relative block shrink-0 overflow-hidden text-left transition-shadow {frame} {value
			? ''
			: 'border border-dashed border-white/15 hover:border-white/30'}"
	>
		{#if isUrl}
			<img src={value} alt={label} class="absolute inset-0 size-full object-cover" />
			<span
				class="absolute inset-0 grid place-items-center bg-black/55 opacity-0 transition-opacity group-hover/img:opacity-100"
			>
				<ImagePlus size={shape === 'icon' ? 10 : 14} class="text-white" />
			</span>
		{:else if value}
			<!-- A template — resolved at runtime, shown as a braces chip here. -->
			<span
				class="absolute inset-0 flex items-center justify-center gap-1 bg-white/[0.04] px-1 text-[#949ba4]"
			>
				<Braces size={shape === 'icon' ? 9 : 12} class="shrink-0" />
				{#if shape !== 'icon'}
					<span class="min-w-0 truncate font-mono text-[9px]">{value}</span>
				{/if}
			</span>
		{:else}
			<span
				class="absolute inset-0 grid place-items-center text-[#6d6f78] transition-colors group-hover/img:text-[#b5bac1]"
			>
				{#if shape === 'banner'}
					<span class="flex flex-col items-center gap-1">
						<ImagePlus size={16} />
						<span class="text-[10.5px] font-medium">{label}</span>
					</span>
				{:else}
					<ImagePlus size={shape === 'icon' ? 10 : 14} />
				{/if}
			</span>
		{/if}
	</Popover.Trigger>

	<Popover.Content class="w-72 p-2.5" align="start">
		<div class="mb-2 font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
			{label}
		</div>

		<input
			bind:this={fileEl}
			type="file"
			accept="image/png,image/jpeg,image/webp,image/gif"
			class="hidden"
			onchange={onFile}
		/>
		<button
			type="button"
			class="flex h-8 w-full items-center justify-center gap-2 rounded-md bg-foreground text-[12px] font-medium text-background transition-opacity hover:opacity-90 disabled:opacity-50"
			disabled={uploading}
			onclick={() => fileEl?.click()}
		>
			<Upload size={12} />
			{uploading ? 'Uploading…' : 'Upload a picture'}
		</button>
		{#if uploadError}
			<p class="mt-1.5 text-[10.5px] leading-snug text-destructive">{uploadError}</p>
		{/if}

		<div class="my-2.5 flex items-center gap-2">
			<span class="h-px flex-1 bg-border"></span>
			<span class="font-mono text-[9px] uppercase tracking-[0.12em] text-muted-foreground">or</span>
			<span class="h-px flex-1 bg-border"></span>
		</div>

		<input
			class="h-7 w-full rounded-md border border-input bg-background px-2 font-mono text-[11px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
			placeholder="https://… or {'{{ .Vars.image }}'}"
			value={value ?? ''}
			oninput={(e) => onChange((e.currentTarget as HTMLInputElement).value)}
		/>
		<p class="mt-1.5 text-[10px] leading-snug text-muted-foreground">
			Templates resolve at runtime — e.g. a property value or an Image-load
			variable.
		</p>

		{#if value}
			<button
				type="button"
				class="mt-2 inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-destructive transition-colors hover:bg-destructive/10"
				onclick={() => {
					onChange('');
					open = false;
				}}
			>
				<Trash2 size={11} />
				Remove
			</button>
		{/if}
	</Popover.Content>
</Popover.Root>
