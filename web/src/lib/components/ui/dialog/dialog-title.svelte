<script lang="ts">
	import { Dialog as DialogPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<DialogPrimitive.Title class={cn('text-[14px] font-semibold leading-none text-foreground', cls)}>
	{@render children()}
</DialogPrimitive.Title>
