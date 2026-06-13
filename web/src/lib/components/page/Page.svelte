<script lang="ts">
	import type { Snippet } from 'svelte';
	let { children, class: cls = '' }: { children: Snippet; class?: string } = $props();
</script>

<div class="flex min-h-[calc(100vh-3.5rem)] flex-col bg-bg {cls}">
	{@render children()}
</div>
