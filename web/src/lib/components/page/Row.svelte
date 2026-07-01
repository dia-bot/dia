<script lang="ts">
	import type { Snippet } from 'svelte';
	let {
		children,
		href,
		onclick,
		class: cls = ''
	}: {
		children: Snippet;
		href?: string;
		onclick?: () => void;
		class?: string;
	} = $props();
	const interactive = $derived(!!href || !!onclick);
	const klass = $derived(
		[
			'group flex h-10 items-center gap-3 border-b border-line/60 px-5 transition-colors',
			interactive ? 'hover:bg-ink-2/30 cursor-pointer' : '',
			cls
		].join(' ')
	);
</script>

{#if href}
	<a {href} class={klass}>
		{@render children()}
	</a>
{:else if onclick}
	<button type="button" {onclick} class="{klass} w-full text-left">
		{@render children()}
	</button>
{:else}
	<div class={klass}>
		{@render children()}
	</div>
{/if}
