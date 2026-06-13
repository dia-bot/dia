<script lang="ts">
	import { Tabs as TabsPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	let {
		value,
		class: cls,
		children
	}: { value: string; class?: string; children: Snippet } = $props();
</script>

<TabsPrimitive.Content
	{value}
	class={cn(
		'ring-offset-background focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
		cls
	)}
>
	{@render children()}
</TabsPrimitive.Content>
