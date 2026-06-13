<script lang="ts">
	import { Select as SelectPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import ChevronDown from 'lucide-svelte/icons/chevron-down';
	import { cn } from '$lib/utils';

	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<SelectPrimitive.Trigger
	class={cn(
		'group flex h-7 w-full items-center justify-between rounded-md border border-input bg-background px-2.5 text-[12.5px] text-foreground ring-offset-background transition-colors placeholder:text-muted-foreground hover:border-ring/60 focus:outline-none focus:ring-1 focus:ring-ring disabled:cursor-not-allowed disabled:opacity-50 data-[state=open]:border-ring',
		cls
	)}
>
	{@render children()}
	<ChevronDown
		class="size-3.5 opacity-50 transition-transform duration-200 ease-out group-data-[state=open]:rotate-180"
	/>
</SelectPrimitive.Trigger>
