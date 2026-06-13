<script lang="ts">
	import { Popover as PopoverPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';

	let {
		// Fallback REQUIRED: binding an undefined prop into bits-ui's Root
		// (which has its own fallback) is a Svelte runtime error the moment
		// the popover tries to open.
		open = $bindable(false),
		children
	}: { open?: boolean; children: Snippet } = $props();
</script>

<PopoverPrimitive.Root bind:open>
	{@render children()}
</PopoverPrimitive.Root>
