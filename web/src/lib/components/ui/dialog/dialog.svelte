<script lang="ts">
	import { Dialog as DialogPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';

	let {
		open = $bindable(false),
		onOpenChange,
		children
	}: {
		open?: boolean;
		onOpenChange?: (open: boolean) => void;
		children: Snippet;
	} = $props();
</script>

<DialogPrimitive.Root bind:open {onOpenChange}>
	{@render children()}
</DialogPrimitive.Root>
