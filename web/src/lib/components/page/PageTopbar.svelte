<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		eyebrow,
		subtitle,
		leading,
		actions,
		class: cls = ''
	}: {
		eyebrow: string;
		subtitle?: string;
		leading?: Snippet;
		actions?: Snippet;
		class?: string;
	} = $props();
</script>

<div
	class="sticky top-0 z-10 flex h-12 shrink-0 items-center gap-3 border-b border-line bg-bg/95 px-5 backdrop-blur supports-[backdrop-filter]:bg-bg/80 {cls}"
>
	{#if leading}
		{@render leading()}
	{/if}
	<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
		{eyebrow}
	</span>
	{#if subtitle}
		<div class="h-4 w-px bg-line"></div>
		<span class="truncate text-[12.5px] text-muted">{subtitle}</span>
	{/if}
	{#if actions}
		<div class="ml-auto flex items-center gap-1.5">
			{@render actions()}
		</div>
	{/if}
</div>
