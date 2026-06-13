<script lang="ts">
	import { Command as CommandPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let {
		class: cls,
		value,
		keywords,
		onSelect,
		children
	}: {
		class?: string;
		value?: string;
		keywords?: string[];
		onSelect?: () => void;
		children: Snippet;
	} = $props();
</script>

<CommandPrimitive.Item
	{value}
	{keywords}
	{onSelect}
	class={cn(
		'relative flex cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-[12.5px] outline-none data-[selected=true]:bg-secondary data-[selected=true]:text-foreground data-[disabled=true]:pointer-events-none data-[disabled=true]:opacity-50 [&_svg]:size-3.5 [&_svg]:shrink-0',
		cls
	)}
>
	{@render children()}
</CommandPrimitive.Item>
