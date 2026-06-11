<script lang="ts">
	import { Popover as PopoverPrimitive, type WithoutChildrenOrChild } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	let {
		class: cls,
		sideOffset = 4,
		align = 'center',
		side = 'bottom',
		children,
		...rest
	}: WithoutChildrenOrChild<PopoverPrimitive.ContentProps> & { children: Snippet } = $props();
</script>

<PopoverPrimitive.Portal>
	<PopoverPrimitive.Content
		{sideOffset}
		{align}
		{side}
		class={cn(
			'menu-pop z-50 w-72 rounded-md border border-border bg-popover p-1 text-popover-foreground shadow-md outline-none',
			cls
		)}
		{...rest}
	>
		{@render children()}
	</PopoverPrimitive.Content>
</PopoverPrimitive.Portal>
