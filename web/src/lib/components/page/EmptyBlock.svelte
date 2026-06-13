<script lang="ts">
	import type { Snippet } from 'svelte';
	let {
		title,
		body = '',
		cta
	}: {
		title: string;
		body?: string;
		cta?: Snippet;
	} = $props();
</script>

<div class="px-5 py-16 text-center">
	<p class="mb-1 text-[12.5px] font-medium text-ink">{title}</p>
	{#if body}
		<p class="mx-auto mb-4 max-w-[34ch] text-[11.5px] leading-relaxed text-muted">{body}</p>
	{/if}
	{#if cta}
		<div class="mt-4 flex justify-center gap-1.5">
			{@render cta()}
		</div>
	{/if}
</div>
