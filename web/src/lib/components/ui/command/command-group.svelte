<script lang="ts">
	import { Command as CommandPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let {
		class: cls,
		heading,
		children
	}: { class?: string; heading?: string; children: Snippet } = $props();
</script>

<CommandPrimitive.Group
	class={cn(
		'overflow-hidden p-1 text-foreground [&_[data-cmdk-group-heading]]:px-2 [&_[data-cmdk-group-heading]]:py-1 [&_[data-cmdk-group-heading]]:font-mono [&_[data-cmdk-group-heading]]:text-[10px] [&_[data-cmdk-group-heading]]:font-medium [&_[data-cmdk-group-heading]]:uppercase [&_[data-cmdk-group-heading]]:tracking-wider [&_[data-cmdk-group-heading]]:text-muted-foreground',
		cls
	)}
>
	{#if heading}
		<CommandPrimitive.GroupHeading>{heading}</CommandPrimitive.GroupHeading>
	{/if}
	{@render children()}
</CommandPrimitive.Group>
