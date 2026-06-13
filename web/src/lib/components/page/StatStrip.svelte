<script lang="ts">
	import type { Snippet } from 'svelte';
	let { children, cols = 4 }: { children: Snippet; cols?: 2 | 3 | 4 | 5 } = $props();
	const gridCls = $derived(
		(
			{
				2: 'grid-cols-2',
				3: 'grid-cols-2 md:grid-cols-3',
				4: 'grid-cols-2 md:grid-cols-4',
				5: 'grid-cols-2 md:grid-cols-5'
			} as const
		)[cols]
	);
</script>

<div class="grid shrink-0 border-b border-line bg-bg {gridCls}">
	{@render children()}
</div>
