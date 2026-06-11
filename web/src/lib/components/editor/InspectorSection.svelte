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
	<div class="flex items-center gap-1.5 px-4 pb-2 pt-3">
		<!-- Figma's UI3 section header: a quiet semibold sentence-case title; the
		     collapse chevron only reveals on hover so the header stays clean. -->
		<button
			type="button"
			onclick={() => (open = !open)}
			aria-expanded={open}
			class="group -ml-1 flex flex-1 items-center gap-1 rounded px-1 py-0.5 text-[11px] font-semibold text-ink transition-colors hover:text-ink"
		>
			{title}
			<ChevronRight
				size={11}
				class="text-faint opacity-0 transition-all duration-150 group-hover:opacity-100 {open
					? 'rotate-90'
					: ''}"
			/>
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
