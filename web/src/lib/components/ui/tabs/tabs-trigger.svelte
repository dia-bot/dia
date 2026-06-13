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

<TabsPrimitive.Trigger
	{value}
	class={cn(
		'inline-flex h-6 items-center justify-center gap-1 whitespace-nowrap rounded px-2.5 text-[11.5px] font-medium ring-offset-background transition-all focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50 data-[state=active]:bg-secondary data-[state=active]:text-foreground data-[state=inactive]:hover:text-foreground [&_svg]:size-3 [&_svg]:shrink-0',
		cls
	)}
>
	{@render children()}
</TabsPrimitive.Trigger>
