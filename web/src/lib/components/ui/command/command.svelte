<script lang="ts">
	import { Command as CommandPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	let {
		class: cls,
		children,
		loop = true
	}: { class?: string; children: Snippet; loop?: boolean } = $props();
</script>

<CommandPrimitive.Root
	{loop}
	class={cn(
		'flex h-full w-full flex-col overflow-hidden rounded-md bg-popover text-popover-foreground',
		cls
	)}
>
	{@render children()}
</CommandPrimitive.Root>
