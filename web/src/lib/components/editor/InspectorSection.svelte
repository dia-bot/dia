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
	<div class="flex items-center gap-1.5 px-4 pb-1.5 pt-3">
		<button
			type="button"
			onclick={() => (open = !open)}
			aria-expanded={open}
			class="flex flex-1 items-center gap-1 text-[10px] font-medium uppercase tracking-[0.09em] text-faint transition-colors hover:text-muted"
		>
			<ChevronRight size={11} class="transition-transform duration-150 {open ? 'rotate-90' : ''}" />
			{title}
		</button>
		{#if action}
			{@render action()}
		{/if}
	</div>
	{#if open}
		<div class="px-4 pb-3.5" transition:slide={{ duration: 160, easing: cubicOut }}>
			{@render children()}
		</div>
	{/if}
</section>
