<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<div class={cn('flex items-center px-4 py-3 border-t border-border', cls)}>
	{@render children()}
</div>
