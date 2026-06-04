<script lang="ts">
	import type { Snippet } from 'svelte';

	// Clean, smooth interior-page hero — mono eyebrow, large grotesk title, lede,
	// and an optional actions slot, over a faint accent wash (no hard grid).
	let {
		eyebrow,
		title,
		lede = '',
		children
	}: { eyebrow: string; title: string; lede?: string; children?: Snippet } = $props();
</script>

<section>
	<div class="mx-auto max-w-page px-6 pb-12 pt-16 sm:pb-16 sm:pt-24">
		<span class="eyebrow">{eyebrow}</span>
		<h1 class="mt-4 max-w-3xl text-[2.7rem] font-extrabold leading-[1.03] tracking-[-0.035em] sm:text-[3.6rem]">
			{title}
		</h1>
		{#if lede}
			<p class="mt-6 max-w-2xl text-lg leading-relaxed text-muted sm:text-xl">{lede}</p>
		{/if}
		{#if children}
			<div class="mt-9">{@render children()}</div>
		{/if}
	</div>
</section>
