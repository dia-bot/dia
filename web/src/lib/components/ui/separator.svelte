<script lang="ts">
	import { cn } from '$lib/utils';

	let {
		orientation = 'horizontal',
		class: cls
	}: { orientation?: 'horizontal' | 'vertical'; class?: string } = $props();
</script>

<div
	role="none"
	class={cn(
		'shrink-0 bg-border',
		orientation === 'horizontal' ? 'h-px w-full' : 'h-full w-px',
		cls
	)}
></div>
