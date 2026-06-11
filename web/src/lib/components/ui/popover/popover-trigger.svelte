<script lang="ts">
	import { Popover as PopoverPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';

	let {
		child,
		children,
		class: cls
	}: {
		child?: Snippet<[{ props: Record<string, unknown> }]>;
		children?: Snippet;
		class?: string;
	} = $props();
</script>

{#if child}
	<PopoverPrimitive.Trigger {child} />
{:else}
	<PopoverPrimitive.Trigger class={cls}>
		{#if children}{@render children()}{/if}
	</PopoverPrimitive.Trigger>
{/if}
