<script lang="ts">
	import { Select as SelectPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';

	let {
		value = $bindable(''),
		type = 'single',
		children,
		onValueChange
	}: {
		value?: string;
		type?: 'single' | 'multiple';
		children: Snippet;
		onValueChange?: (v: string) => void;
	} = $props();
</script>

{#if type === 'single'}
	<SelectPrimitive.Root type="single" bind:value {onValueChange}>
		{@render children()}
	</SelectPrimitive.Root>
{:else}
	<SelectPrimitive.Root type="multiple">
		{@render children()}
	</SelectPrimitive.Root>
{/if}
