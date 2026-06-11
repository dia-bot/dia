<script lang="ts">
	import { Tabs as TabsPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<TabsPrimitive.List
	class={cn(
		'inline-flex h-7 items-center gap-0.5 rounded-md border border-border bg-card p-0.5 text-muted-foreground',
		cls
	)}
>
	{@render children()}
</TabsPrimitive.List>
