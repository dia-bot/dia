<script lang="ts">
	import { Select as SelectPrimitive, type WithoutChildrenOrChild } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	let {
		class: cls,
		sideOffset = 4,
		children,
		...rest
	}: WithoutChildrenOrChild<SelectPrimitive.ContentProps> & { children: Snippet } = $props();
</script>

<SelectPrimitive.Portal>
	<SelectPrimitive.Content
		{sideOffset}
		class={cn(
			'menu-pop relative z-50 max-h-80 min-w-[8rem] overflow-hidden rounded-md border border-border bg-popover text-popover-foreground shadow-md',
			cls
		)}
		{...rest}
	>
		<SelectPrimitive.Viewport class="p-1">
			{@render children()}
		</SelectPrimitive.Viewport>
	</SelectPrimitive.Content>
</SelectPrimitive.Portal>
