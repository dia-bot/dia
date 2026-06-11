<script lang="ts">
	import { DropdownMenu as DropdownMenuPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';

	let {
		open = $bindable(false),
		children
	}: { open?: boolean; children: Snippet } = $props();
</script>

<DropdownMenuPrimitive.Root bind:open>
	{@render children()}
</DropdownMenuPrimitive.Root>
