<script lang="ts">
	import { Tabs as TabsPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';

	let {
		value = $bindable(''),
		class: cls,
		children,
		onValueChange
	}: {
		value?: string;
		class?: string;
		children: Snippet;
		onValueChange?: (v: string) => void;
	} = $props();
</script>

<TabsPrimitive.Root bind:value class={cn('flex flex-col gap-3', cls)} {onValueChange}>
	{@render children()}
</TabsPrimitive.Root>
