<script lang="ts">
	// Figma's "Stroke style" control: a dropdown whose trigger and options PREVIEW the
	// line (solid / dashed). Built as a nested Bits UI popover rendered INLINE (no Portal)
	// so opening it never dismisses the parent "Stroke settings" popover it lives in.
	import { Popover } from 'bits-ui';
	import { ChevronDown, Check } from 'lucide-svelte';
	import type { StrokeStyle } from '$lib/layout/schema';

	let { value, set }: { value: StrokeStyle | ''; set: (v: StrokeStyle) => void } = $props();

	const opts: { v: StrokeStyle; label: string; dash: string }[] = [
		{ v: 'solid', label: 'Solid', dash: 'none' },
		{ v: 'dashed', label: 'Dashed', dash: '4 2.5' }
	];
	let open = $state(false);
	const current = $derived(opts.find((o) => o.v === value));
</script>

{#snippet line(dash: string)}
	<svg width="24" height="10" viewBox="0 0 24 10" fill="none" stroke="currentColor" aria-hidden="true">
		<line x1="1.5" y1="5" x2="22.5" y2="5" stroke-width="1.6" stroke-linecap="round" stroke-dasharray={dash} />
	</svg>
{/snippet}

<Popover.Root bind:open>
	<Popover.Trigger
		class="flex h-7 w-full items-center gap-2 rounded-md border border-line bg-ink-2 pl-2 pr-1.5 text-xs text-ink transition-colors hover:border-line-strong data-[state=open]:border-faint"
	>
		<span class="text-muted">{@render line(current?.dash ?? 'none')}</span>
		<span class="flex-1 text-left">{current?.label ?? 'Mixed'}</span>
		<ChevronDown size={13} class="shrink-0 text-faint" />
	</Popover.Trigger>
	<Popover.Content
		align="end"
		sideOffset={4}
		class="menu-pop z-[60] w-52 rounded-lg border border-line-strong bg-surface p-1.5 shadow-2xl shadow-black/60 ring-1 ring-black/40 outline-none"
	>
		{#each opts as o (o.v)}
			<button
				type="button"
				onclick={() => {
					set(o.v);
					open = false;
				}}
				class="flex w-full items-center gap-2.5 rounded-md px-2.5 py-2 text-[13px] text-muted transition-colors hover:bg-ink-2 hover:text-ink"
			>
				<span class="text-muted">{@render line(o.dash)}</span>
				<span class="flex-1 text-left">{o.label}</span>
				{#if value === o.v}<Check size={13} class="shrink-0 text-accent-ink" />{/if}
			</button>
		{/each}
	</Popover.Content>
</Popover.Root>
