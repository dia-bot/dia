<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLLabelAttributes } from 'svelte/elements';
	import { cn } from '$lib/utils';

	let {
		class: cls,
		children,
		...rest
	}: HTMLLabelAttributes & { class?: string; children: Snippet } = $props();
</script>

<label
	class={cn(
		'text-[11px] font-medium leading-none text-muted-foreground peer-disabled:cursor-not-allowed peer-disabled:opacity-70',
		cls
	)}
	{...rest}
>
	{@render children()}
</label>
