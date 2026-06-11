<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<p class={cn('text-[11.5px] text-muted-foreground', cls)}>
	{@render children()}
</p>
