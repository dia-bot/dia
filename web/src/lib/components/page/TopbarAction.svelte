<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		children,
		onclick,
		href,
		icon,
		variant = 'primary',
		disabled = false,
		title,
		ariaLabel
	}: {
		children?: Snippet;
		onclick?: () => void;
		href?: string;
		icon?: Snippet;
		variant?: 'primary' | 'ghost' | 'danger' | 'ink';
		disabled?: boolean;
		title?: string;
		ariaLabel?: string;
	} = $props();

	// 'primary' is deliberately ink-styled: filled CTAs are near-white pills, never accent fills.
	const cls = $derived(
		variant === 'primary'
			? 'bg-ink text-bg hover:bg-ink/90'
			: variant === 'ink'
				? 'bg-ink text-bg hover:bg-ink/90'
			: variant === 'danger'
				? 'border border-line text-muted hover:border-danger/40 hover:text-danger bg-bg'
				: 'border border-line text-muted hover:border-line-strong hover:text-ink bg-bg'
	);
</script>

{#if href}
	<a
		{href}
		title={title}
		aria-label={ariaLabel}
		class="inline-flex h-7 items-center gap-1.5 rounded-md px-2.5 text-[12px] font-medium transition-colors {cls}"
	>
		{#if icon}{@render icon()}{/if}
		{#if children}{@render children()}{/if}
	</a>
{:else}
	<button
		type="button"
		{onclick}
		{disabled}
		{title}
		aria-label={ariaLabel}
		class="inline-flex h-7 items-center gap-1.5 rounded-md px-2.5 text-[12px] font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50 {cls}"
	>
		{#if icon}{@render icon()}{/if}
		{#if children}{@render children()}{/if}
	</button>
{/if}
