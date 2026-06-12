<script lang="ts">
	import { Popover as PopoverPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';

	let {
		child,
		children,
		class: cls,
		...rest
	}: {
		child?: Snippet<[{ props: Record<string, unknown> }]>;
		children?: Snippet;
		class?: string;
		title?: string;
		'aria-label'?: string;
	} = $props();
</script>

{#if child}
	<PopoverPrimitive.Trigger {child} {...rest} />
{:else}
	<PopoverPrimitive.Trigger class={cls} {...rest}>
		{#if children}{@render children()}{/if}
	</PopoverPrimitive.Trigger>
{/if}
