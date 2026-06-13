<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<div class={cn('flex flex-col gap-1 text-left', cls)}>
	{@render children()}
</div>
