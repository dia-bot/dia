<script lang="ts">
	// Figma's "Width profile" control: a dropdown whose trigger and options PREVIEW the
	// taper as a filled ribbon. Inline Bits UI popover (no Portal) so opening it never
	// dismisses the parent "Stroke settings" popover it lives in.
	import { Popover } from 'bits-ui';
	import { ChevronDown, Check } from 'lucide-svelte';
	import type { WidthProfile } from '$lib/layout/schema';

	let { value, set }: { value: WidthProfile | ''; set: (v: WidthProfile) => void } = $props();

	// Each profile is a filled symmetric ribbon: `d` is the SVG path of the upper edge
	// half-thickness at x = 1 / 12 / 23 (left, mid, right); the shape mirrors below y=5.
	const opts: { v: WidthProfile; label: string }[] = [
		{ v: 'uniform', label: 'Uniform' },
		{ v: 'taper_start', label: 'Taper start' },
		{ v: 'taper_end', label: 'Taper end' },
		{ v: 'taper', label: 'Taper' },
		{ v: 'lens', label: 'Lens' }
	];
	let open = $state(false);
	const current = $derived(opts.find((o) => o.v === value));
</script>

{#snippet ribbon(v: WidthProfile)}
	<svg width="24" height="11" viewBox="0 0 24 11" fill="currentColor" aria-hidden="true">
		{#if v === 'uniform'}
			<rect x="1" y="3.2" width="22" height="4.6" rx="0.6" />
		{:else if v === 'taper_start'}
			<path d="M1 5.5 L23 1.4 L23 9.6 Z" />
		{:else if v === 'taper_end'}
			<path d="M1 1.4 L23 5.5 L1 9.6 Z" />
		{:else if v === 'taper'}
			<path d="M1 5.5 L12 1 L23 5.5 L12 10 Z" />
		{:else}
			<ellipse cx="12" cy="5.5" rx="11" ry="4.3" />
		{/if}
	</svg>
{/snippet}

<Popover.Root bind:open>
	<Popover.Trigger
		class="flex h-7 w-full items-center gap-2 rounded-md border border-line bg-ink-2 pl-2 pr-1.5 text-xs text-ink transition-colors hover:border-line-strong data-[state=open]:border-faint"
	>
		<span class="text-muted">{@render ribbon(current?.v ?? 'uniform')}</span>
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
				<span class="text-muted">{@render ribbon(o.v)}</span>
				<span class="flex-1 text-left">{o.label}</span>
				{#if value === o.v}<Check size={13} class="shrink-0 text-accent-ink" />{/if}
			</button>
		{/each}
	</Popover.Content>
</Popover.Root>
