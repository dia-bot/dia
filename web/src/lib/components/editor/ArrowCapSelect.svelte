<script lang="ts">
	// Figma's "Start point / End point" control: a dropdown of arrowhead decorations whose
	// trigger and options PREVIEW the marker on a line. `flip` mirrors the glyph so the
	// Start-point selector points the other way. Inline popover (no Portal) so it never
	// dismisses the parent "Stroke settings" popover.
	import { Popover } from 'bits-ui';
	import { ChevronDown, Check } from 'lucide-svelte';
	import type { ArrowCap } from '$lib/layout/schema';

	let {
		value,
		set,
		flip = false
	}: { value: ArrowCap | ''; set: (v: ArrowCap) => void; flip?: boolean } = $props();

	const opts: { v: ArrowCap; label: string }[] = [
		{ v: 'none', label: 'None' },
		{ v: 'line', label: 'Line' },
		{ v: 'arrow', label: 'Arrow' },
		{ v: 'triangle', label: 'Triangle' },
		{ v: 'circle', label: 'Circle' },
		{ v: 'diamond', label: 'Diamond' }
	];
	let open = $state(false);
	const current = $derived(opts.find((o) => o.v === value));
</script>

<!-- marker drawn at the RIGHT end of a baseline; `flip` mirrors the whole glyph. -->
{#snippet glyph(kind: ArrowCap)}
	<svg width="26" height="11" viewBox="0 0 26 11" fill="none" stroke="currentColor" aria-hidden="true">
		<g transform={flip ? 'translate(26,0) scale(-1,1)' : ''}>
			{#if kind === 'none'}
				<line x1="2" y1="5.5" x2="24" y2="5.5" stroke-width="1.6" stroke-linecap="round" />
			{:else if kind === 'line'}
				<line x1="2" y1="5.5" x2="21" y2="5.5" stroke-width="1.6" stroke-linecap="round" />
				<line x1="22" y1="1.5" x2="22" y2="9.5" stroke-width="1.6" stroke-linecap="round" />
			{:else if kind === 'arrow'}
				<line x1="2" y1="5.5" x2="21" y2="5.5" stroke-width="1.6" stroke-linecap="round" />
				<path d="M16 1.5 L23 5.5 L16 9.5" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" />
			{:else if kind === 'triangle'}
				<line x1="2" y1="5.5" x2="15" y2="5.5" stroke-width="1.6" stroke-linecap="round" />
				<path d="M15 1.2 L24 5.5 L15 9.8 Z" fill="currentColor" />
			{:else if kind === 'circle'}
				<line x1="2" y1="5.5" x2="17" y2="5.5" stroke-width="1.6" stroke-linecap="round" />
				<circle cx="20" cy="5.5" r="3.4" fill="currentColor" />
			{:else}
				<line x1="2" y1="5.5" x2="15" y2="5.5" stroke-width="1.6" stroke-linecap="round" />
				<path d="M20 1.7 L23.6 5.5 L20 9.3 L16.4 5.5 Z" fill="currentColor" />
			{/if}
		</g>
	</svg>
{/snippet}

<Popover.Root bind:open>
	<Popover.Trigger
		class="flex h-7 w-full items-center gap-2 rounded-md border border-line bg-ink-2 pl-2 pr-1.5 text-xs text-ink transition-colors hover:border-line-strong data-[state=open]:border-faint"
	>
		<span class="text-muted">{@render glyph(current?.v ?? 'none')}</span>
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
				<span class="text-muted">{@render glyph(o.v)}</span>
				<span class="flex-1 text-left">{o.label}</span>
				{#if value === o.v}<Check size={13} class="shrink-0 text-accent-ink" />{/if}
			</button>
		{/each}
	</Popover.Content>
</Popover.Root>
