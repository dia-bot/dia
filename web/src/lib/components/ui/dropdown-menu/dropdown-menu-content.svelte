<script lang="ts">
	import { DropdownMenu as DropdownMenuPrimitive, type WithoutChildrenOrChild } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	let {
		class: cls,
		sideOffset = 4,
		align = 'end',
		children,
		...rest
	}: WithoutChildrenOrChild<DropdownMenuPrimitive.ContentProps> & { children: Snippet } = $props();
</script>

<DropdownMenuPrimitive.Portal>
	<DropdownMenuPrimitive.Content
		{sideOffset}
		{align}
		class={cn(
			'menu-pop z-[80] min-w-[10rem] overflow-hidden rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md',
			cls
		)}
		{...rest}
	>
		{@render children()}
	</DropdownMenuPrimitive.Content>
</DropdownMenuPrimitive.Portal>
