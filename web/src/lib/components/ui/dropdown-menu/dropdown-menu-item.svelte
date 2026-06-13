<script lang="ts">
	import { DropdownMenu as DropdownMenuPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	let {
		class: cls,
		onSelect,
		disabled = false,
		children
	}: {
		class?: string;
		onSelect?: () => void;
		disabled?: boolean;
		children: Snippet;
	} = $props();
</script>

<DropdownMenuPrimitive.Item
	{disabled}
	{onSelect}
	class={cn(
		'relative flex cursor-default select-none items-center gap-2 rounded-sm px-2 py-1.5 text-[12.5px] outline-none transition-colors data-[highlighted]:bg-secondary data-[highlighted]:text-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50 [&_svg]:size-3.5',
		cls
	)}
>
	{@render children()}
</DropdownMenuPrimitive.Item>
