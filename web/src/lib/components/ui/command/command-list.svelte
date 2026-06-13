<script lang="ts">
	import { Command as CommandPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<CommandPrimitive.List class={cn('max-h-80 overflow-y-auto overflow-x-hidden p-1', cls)}>
	{@render children()}
</CommandPrimitive.List>
