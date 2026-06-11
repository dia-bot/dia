<script lang="ts">
	import { Dialog as DialogPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<DialogPrimitive.Description class={cn('text-[12px] text-muted-foreground', cls)}>
	{@render children()}
</DialogPrimitive.Description>
