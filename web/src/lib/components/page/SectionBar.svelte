<script lang="ts">
	import type { Snippet } from 'svelte';
	let {
		label,
		count,
		children,
		class: cls = ''
	}: {
		label: string;
		count?: number | string;
		children?: Snippet;
		class?: string;
	} = $props();
</script>

<div
	class="flex min-h-9 shrink-0 flex-wrap items-center gap-x-2.5 gap-y-1.5 border-b border-line/60 bg-bg px-5 py-1.5 md:h-9 md:flex-nowrap md:py-0 {cls}"
>
	<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
		{label}
	</span>
	{#if count !== undefined}
		<span class="font-mono text-[10.5px] tabular-nums text-faint">{count}</span>
	{/if}
	{#if children}
		<div class="ml-auto flex min-w-0 flex-wrap items-center justify-end gap-1.5">
			{@render children()}
		</div>
	{/if}
</div>
