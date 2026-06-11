<script lang="ts">
	import type { Snippet } from 'svelte';
	import { cn } from '$lib/utils';
	let { class: cls, children }: { class?: string; children: Snippet } = $props();
</script>

<h3 class={cn('text-[13px] font-semibold leading-none tracking-tight text-foreground', cls)}>
	{@render children()}
</h3>
