<script lang="ts">
	// A collapsible section for the layout inspector — Figma's right-panel pattern:
	// a hairline-ruled header with a chevron + uppercase label, an optional
	// right-aligned action (e.g. a "+" add button), and a body that slides open.
	import type { Snippet } from 'svelte';
	import { ChevronRight } from 'lucide-svelte';
	import { slide } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';

	let {
		title,
		open = $bindable(true),
		action,
		children
	}: {
		title: string;
		open?: boolean;
		action?: Snippet;
		children: Snippet;
	} = $props();
</script>

<section class="border-t border-line first:border-t-0">
	<div class="flex items-center gap-1.5 px-4 pb-2 pt-3.5">
		<button
			type="button"
			onclick={() => (open = !open)}
			aria-expanded={open}
			class="-ml-1 flex flex-1 items-center gap-1 rounded px-1 py-0.5 text-[10px] font-semibold uppercase tracking-[0.09em] text-faint transition-colors hover:text-muted"
		>
			<ChevronRight
				size={11}
				class="text-faint/70 transition-transform duration-150 {open ? 'rotate-90' : ''}"
			/>
			{title}
		</button>
		{#if action}
			{@render action()}
		{/if}
	</div>
	{#if open}
		<div class="px-4 pb-4" transition:slide={{ duration: 160, easing: cubicOut }}>
			{@render children()}
		</div>
	{/if}
</section>
