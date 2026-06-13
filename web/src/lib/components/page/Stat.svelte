<script lang="ts">
	let {
		label,
		value,
		sub = '',
		accent = false,
		href,
		onclick,
		last = false,
		active = false
	}: {
		label: string;
		value: string | number;
		sub?: string;
		accent?: boolean;
		href?: string;
		onclick?: () => void;
		last?: boolean;
		active?: boolean;
	} = $props();

	const displayValue = $derived(typeof value === 'number' ? value.toLocaleString() : value);
	const interactive = $derived(!!href || !!onclick);
	const cls = $derived(
		[
			'group px-5 py-4 transition-colors',
			last ? '' : 'border-r border-line',
			interactive ? 'hover:bg-ink-2/30 cursor-pointer' : '',
			active ? 'bg-ink-2/40' : ''
		].join(' ')
	);
</script>

{#snippet body()}
	<div class="flex items-center gap-2">
		<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
			{label}
		</span>
		{#if accent}
			<span class="h-1.5 w-1.5 animate-pulse rounded-full bg-accent"></span>
		{/if}
		{#if interactive}
			<span class="ml-auto text-[10px] text-faint transition-colors group-hover:text-muted">→</span>
		{/if}
	</div>
	<div class="mt-2 font-light leading-none text-ink tabular-nums" style="font-size: 26px;">
		{displayValue}
	</div>
	{#if sub}
		<div class="mt-1.5 truncate font-mono text-[10px] text-faint">{sub}</div>
	{/if}
{/snippet}

{#if href}
	<a {href} class={cls}>
		{@render body()}
	</a>
{:else if onclick}
	<button type="button" {onclick} class="{cls} w-full text-left">
		{@render body()}
	</button>
{:else}
	<div class={cls}>
		{@render body()}
	</div>
{/if}
