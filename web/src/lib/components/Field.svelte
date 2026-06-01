<script lang="ts">
	import type { Snippet } from 'svelte';
	let {
		label = '',
		hint = '',
		children
	}: { label?: string; hint?: string; children?: Snippet } = $props();
</script>

<div class="mb-5">
	{#if label}<span class="label">{label}</span>{/if}
	{@render children?.()}
	{#if hint}<p class="hint">{hint}</p>{/if}
</div>
