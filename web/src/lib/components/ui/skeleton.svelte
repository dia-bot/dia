<script lang="ts">
	import { cn } from '$lib/utils';
	let { class: cls }: { class?: string } = $props();
</script>

<div class={cn('animate-pulse rounded-md bg-secondary', cls)}></div>
