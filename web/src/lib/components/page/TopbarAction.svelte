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
		variant?: 'primary' | 'ghost' | 'danger';
		disabled?: boolean;
		title?: string;
		ariaLabel?: string;
	} = $props();

	const cls = $derived(
		variant === 'primary'
			? 'bg-accent text-ink hover:bg-accent/85'
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
