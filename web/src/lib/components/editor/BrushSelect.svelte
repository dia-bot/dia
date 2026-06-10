<script lang="ts">
	// Figma's brush picker: a dropdown of the named Stretch/Scatter brushes, each previewed with
	// a tiny sample stroke. Rendered INLINE (no Portal) so opening it doesn't dismiss the parent
	// "Stroke settings" popover it lives in.
	import { Popover } from 'bits-ui';
	import { ChevronDown, Check } from 'lucide-svelte';
	import { BRUSHES, brushDef, brushPreviewSvg, type BrushDef } from '$lib/layout/brushes';

	let { value, set }: { value: string; set: (v: string) => void } = $props();
	let open = $state(false);
	const current = $derived(brushDef(value));
	const stretch = BRUSHES.filter((b) => b.kind === 'stretch');
	const scatter = BRUSHES.filter((b) => b.kind === 'scatter');
</script>

{#snippet preview(def: BrushDef, w: number, h: number)}
	<svg
		width={w}
		height={h}
		viewBox="0 0 {w} {h}"
		fill="currentColor"
		stroke="none"
		aria-hidden="true"
		class="shrink-0"
	>
		{@html brushPreviewSvg(def, w, h)}
	</svg>
{/snippet}

{#snippet group(label: string, list: BrushDef[])}
	<div class="px-2 pb-1 pt-1.5 text-[10px] font-semibold uppercase tracking-wide text-faint">
		{label}
	</div>
	{#each list as b (b.id)}
		<button
			type="button"
			onclick={() => {
				set(b.id);
				open = false;
			}}
			class="flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-[13px] text-muted transition-colors hover:bg-ink-2 hover:text-ink"
		>
			<span class="text-ink">{@render preview(b, 40, 16)}</span>
			<span class="flex-1 text-left">{b.name}</span>
			{#if value === b.id}<Check size={13} class="shrink-0 text-accent-ink" />{/if}
		</button>
	{/each}
{/snippet}

<Popover.Root bind:open>
	<Popover.Trigger
		class="flex h-7 w-full items-center gap-2 rounded-md border border-line bg-ink-2 pl-1.5 pr-1.5 text-xs text-ink transition-colors hover:border-line-strong data-[state=open]:border-faint"
	>
		<span class="text-ink">{@render preview(current, 34, 16)}</span>
		<span class="flex-1 truncate text-left">{current.name}</span>
		<ChevronDown size={13} class="shrink-0 text-faint" />
	</Popover.Trigger>
	<Popover.Content
		align="end"
		sideOffset={4}
		class="menu-pop z-[60] max-h-72 w-56 overflow-y-auto rounded-lg border border-line-strong bg-surface p-1.5 shadow-2xl shadow-black/60 ring-1 ring-black/40 outline-none"
	>
		{@render group('Stretch', stretch)}
		{@render group('Scatter', scatter)}
	</Popover.Content>
</Popover.Root>
