<script lang="ts">
	import { Command as CommandPrimitive } from 'bits-ui';
	import Search from 'lucide-svelte/icons/search';
	import { cn } from '$lib/utils';

	let {
		class: cls,
		value = $bindable(''),
		placeholder = 'Search…'
	}: { class?: string; value?: string; placeholder?: string } = $props();
</script>

<div class="flex items-center border-b border-border px-3" data-cmdk-input-wrapper="">
	<Search class="mr-2 size-3.5 shrink-0 text-muted-foreground" />
	<CommandPrimitive.Input
		bind:value
		{placeholder}
		class={cn(
			'flex h-9 w-full rounded-md bg-transparent py-2 text-[12.5px] text-foreground outline-none placeholder:text-muted-foreground disabled:cursor-not-allowed disabled:opacity-50',
			cls
		)}
	/>
</div>
