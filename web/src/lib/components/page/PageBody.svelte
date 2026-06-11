<script lang="ts">
	import type { Snippet } from 'svelte';
	let { children, class: cls = '' }: { children: Snippet; class?: string } = $props();
</script>

<div class="min-h-0 flex-1 overflow-auto {cls}">
	{@render children()}
</div>
