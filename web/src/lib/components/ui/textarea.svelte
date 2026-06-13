<script lang="ts">
	import type { HTMLTextareaAttributes } from 'svelte/elements';
	import { cn } from '$lib/utils';

	let {
		class: cls,
		value = $bindable(),
		...rest
	}: HTMLTextareaAttributes & { class?: string } = $props();
</script>

<textarea
	class={cn(
		'flex min-h-16 w-full rounded-md border border-input bg-background px-2.5 py-1.5 text-[12.5px] text-foreground placeholder:text-muted-foreground transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50 resize-y',
		cls
	)}
	bind:value
	{...rest}
></textarea>
