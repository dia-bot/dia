<script lang="ts">
	// A flat, full-bleed section: a slim mono header (label, optional count / desc /
	// actions) over its content, closed by a single hairline. Stacking these is how
	// the moderation pages avoid the "box in box" look. Pass pad={false} for content
	// that draws its own edges (a full-bleed table or row list).
	import type { Snippet } from 'svelte';

	let {
		label,
		desc,
		count,
		actions,
		children,
		pad = true
	}: {
		label: string;
		desc?: string;
		count?: number | string;
		actions?: Snippet;
		children: Snippet;
		pad?: boolean;
	} = $props();
</script>

<section class="border-b border-line">
	<div class="flex min-h-8 flex-wrap items-center gap-x-3 gap-y-1 px-4 pt-3.5 sm:px-5">
		<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
			{label}
		</span>
		{#if count !== undefined}
			<span class="font-mono text-[10.5px] tabular-nums text-faint">{count}</span>
		{/if}
		{#if desc}
			<span class="hidden truncate text-[12px] text-muted sm:inline">{desc}</span>
		{/if}
		{#if actions}
			<div class="ml-auto flex flex-wrap items-center justify-end gap-1.5">
				{@render actions()}
			</div>
		{/if}
	</div>
	<div class={pad ? 'px-4 pb-5 pt-2.5 sm:px-5' : 'pb-1'}>
		{@render children()}
	</div>
</section>
