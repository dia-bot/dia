<script lang="ts">
	// Thin wrapper over Bits UI's Avatar (the shadcn-svelte primitive): shows the
	// image when it loads, otherwise a tinted monogram fallback.
	import { Avatar } from 'bits-ui';
	import { cn } from '$lib/utils';

	let {
		src,
		alt = '',
		fallback,
		class: className = '',
		fallbackClass = ''
	}: {
		src?: string;
		alt?: string;
		fallback: string;
		class?: string;
		fallbackClass?: string;
	} = $props();
</script>

<Avatar.Root delayMs={0} class={cn('relative flex shrink-0 overflow-hidden', className)}>
	{#if src}
		<Avatar.Image {src} {alt} class="h-full w-full object-cover" />
	{/if}
	<Avatar.Fallback
		class={cn(
			'grid h-full w-full place-items-center bg-blush font-bold leading-none text-accent',
			fallbackClass
		)}
	>
		{fallback}
	</Avatar.Fallback>
</Avatar.Root>
