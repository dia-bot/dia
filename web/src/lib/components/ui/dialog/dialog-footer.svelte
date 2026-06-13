<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<div class={cn('flex flex-col-reverse sm:flex-row sm:justify-end sm:gap-2', cls)}>
	{@render children()}
</div>
