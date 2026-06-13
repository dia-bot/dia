<script lang="ts">
	import { Select as SelectPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import Check from 'lucide-svelte/icons/check';
	import { cn } from '$lib/utils';

	let {
		value,
		class: cls,
		children,
		label
	}: { value: string; class?: string; children?: Snippet; label?: string } = $props();
</script>

<SelectPrimitive.Item
	{value}
	{label}
	class={cn(
		'relative flex w-full cursor-default select-none items-center rounded-sm py-1.5 pl-7 pr-2 text-[12.5px] outline-none transition-colors duration-100 data-[highlighted]:bg-secondary data-[highlighted]:text-foreground data-[disabled]:pointer-events-none data-[disabled]:opacity-50',
		cls
	)}
>
	{#snippet children({ selected })}
		<span class="absolute left-2 flex h-3.5 w-3.5 items-center justify-center">
			{#if selected}
				<span class="check-pop inline-flex"><Check class="size-3" /></span>
			{/if}
		</span>
		{label}
	{/snippet}
</SelectPrimitive.Item>

<style>
	.check-pop {
		animation: check-pop-in 180ms cubic-bezier(0.22, 1.4, 0.36, 1) both;
	}
	@keyframes check-pop-in {
		from {
			opacity: 0;
			transform: scale(0.4);
		}
		to {
			opacity: 1;
			transform: scale(1);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.check-pop {
			animation: none;
		}
	}
</style>
