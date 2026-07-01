<script lang="ts">
	// The floating save dock shared by the redesigned feature pages: a
	// bottom-centered pill that only appears when there is something to save or a
	// save is resolving. It mirrors the welcome page's release dock so every
	// builder surface saves the same way. The parent owns the save lifecycle and
	// the ⌘S handler; this component only renders the current phase.
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import Loader2 from 'lucide-svelte/icons/loader-2';
	import Check from 'lucide-svelte/icons/check';
	import CircleAlert from 'lucide-svelte/icons/circle-alert';

	let {
		dirty = false,
		phase = 'idle',
		error = '',
		onsave,
		onreset
	}: {
		dirty?: boolean;
		phase?: 'idle' | 'saving' | 'saved' | 'error';
		error?: string;
		onsave?: () => void;
		onreset?: () => void;
	} = $props();

	const visible = $derived(dirty || phase !== 'idle');
</script>

{#if visible}
	<div
		class="pointer-events-none absolute inset-x-0 bottom-4 z-40 flex justify-center px-4"
		transition:fly={{ y: 14, duration: 180, easing: cubicOut }}
	>
		<div
			class="pointer-events-auto relative flex h-11 items-center gap-2.5 overflow-hidden rounded-[14px] border bg-surface/95 px-3.5 shadow-[0_16px_40px_-12px_rgba(0,0,0,0.7)] backdrop-blur-md {phase ===
			'error'
				? 'dock-shake border-danger/40'
				: 'border-line'}"
		>
			{#if phase === 'saving'}
				<span
					class="dock-beam-sweep pointer-events-none absolute inset-y-0 left-0 w-1/3 bg-gradient-to-r from-transparent via-accent/30 to-transparent"
				></span>
				<Loader2 size={15} class="animate-spin text-muted" />
				<span class="text-[12.5px] text-muted">Saving…</span>
			{:else if phase === 'saved'}
				<span class="grid size-4 place-items-center rounded-full bg-success/15 text-success">
					<Check size={11} />
				</span>
				<span class="text-[12.5px] text-ink">Saved</span>
			{:else if phase === 'error'}
				<CircleAlert size={15} class="text-danger" />
				<span class="max-w-[16rem] truncate text-[12.5px] text-ink" title={error}
					>{error || "Couldn't save"}</span
				>
				<button
					type="button"
					onclick={onsave}
					class="ml-1 inline-flex h-7 items-center rounded-md bg-ink px-2.5 text-[12px] font-medium text-bg transition-opacity hover:opacity-90"
					>Retry</button
				>
			{:else}
				<span class="size-1.5 animate-pulse rounded-full bg-accent"></span>
				<span class="text-[12.5px] text-muted">Unsaved changes</span>
				<div class="ml-1 flex items-center gap-1.5">
					<button
						type="button"
						onclick={onreset}
						class="inline-flex h-7 items-center rounded-md border border-line-strong px-2.5 text-[12px] font-medium text-muted transition-colors hover:text-ink"
						>Discard</button
					>
					<button
						type="button"
						onclick={onsave}
						class="inline-flex h-7 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-medium text-bg transition-opacity hover:opacity-90"
					>
						Save <kbd class="hidden font-mono text-[10px] text-bg/60 sm:inline">⌘S</kbd>
					</button>
				</div>
			{/if}
		</div>
	</div>
{/if}
