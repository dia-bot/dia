<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<div class={cn('flex flex-col gap-1.5 px-4 pt-4 pb-3', cls)}>
	{@render children()}
</div>
