<script lang="ts">
	import { Dialog as DialogPrimitive } from 'bits-ui';
	import type { Snippet } from 'svelte';
	import X from 'lucide-svelte/icons/x';
	import { cn } from '$lib/utils';

	let {
		class: cls,
		showClose = true,
		children
	}: { class?: string; showClose?: boolean; children: Snippet } = $props();
</script>

<DialogPrimitive.Portal>
	<DialogPrimitive.Overlay
		class="dialog-overlay fixed inset-0 z-50 bg-black/70 backdrop-blur-sm"
	/>
	<DialogPrimitive.Content
		class={cn(
			'dialog-content fixed left-[50%] top-[50%] z-50 grid w-full max-w-lg gap-4 border border-border bg-card p-5 shadow-2xl outline-none sm:rounded-lg',
			cls
		)}
	>
		{@render children()}
		{#if showClose}
			<DialogPrimitive.Close
				class="absolute right-3 top-3 rounded-sm text-muted-foreground opacity-70 ring-offset-background transition-opacity hover:opacity-100 focus:outline-none focus:ring-1 focus:ring-ring disabled:pointer-events-none"
			>
				<X class="size-3.5" />
				<span class="sr-only">Close</span>
			</DialogPrimitive.Close>
		{/if}
	</DialogPrimitive.Content>
</DialogPrimitive.Portal>
