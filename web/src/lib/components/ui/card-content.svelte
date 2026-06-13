<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<div class={cn('px-4 pb-4', cls)}>
	{@render children()}
</div>
