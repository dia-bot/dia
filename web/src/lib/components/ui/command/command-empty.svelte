<script lang="ts">
	import { Command as CommandPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<CommandPrimitive.Empty class={cn('py-6 text-center text-[12px] text-muted-foreground', cls)}>
	{@render children()}
</CommandPrimitive.Empty>
