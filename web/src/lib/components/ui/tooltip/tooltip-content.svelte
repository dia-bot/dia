<script lang="ts">
	import { Tooltip as TooltipPrimitive, type WithoutChildrenOrChild } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	let {
		class: cls,
		sideOffset = 4,
		children,
		...rest
	}: WithoutChildrenOrChild<TooltipPrimitive.ContentProps> & { children: Snippet } = $props();
</script>

<TooltipPrimitive.Portal>
	<TooltipPrimitive.Content
		{sideOffset}
		class={cn(
			'menu-pop z-[80] overflow-hidden rounded-md border border-border bg-popover px-2.5 py-1 text-[11.5px] text-popover-foreground shadow-md',
			cls
		)}
		{...rest}
	>
		{@render children()}
	</TooltipPrimitive.Content>
</TooltipPrimitive.Portal>
