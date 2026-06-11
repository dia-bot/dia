<script lang="ts">
	import type { HTMLInputAttributes } from 'svelte/elements';
	import { cn } from '$lib/utils';

	let {
		class: cls,
		value = $bindable(),
		...rest
	}: HTMLInputAttributes & { class?: string } = $props();
</script>

<input
	class={cn(
		'flex h-7 w-full rounded-md border border-input bg-background px-2.5 text-[12.5px] text-foreground placeholder:text-muted-foreground transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50',
		cls
	)}
	bind:value
	{...rest}
/>
