<script lang="ts">
	import type { Snippet } from 'svelte';
	import Logo from '$lib/components/Logo.svelte';

	// A single Discord chat message: avatar, author + optional APP tag,
	// timestamp, and content. `brand` swaps the avatar for the Dia mark and
	// auto-adds the bot tag. `cont` renders a grouped continuation (no header).
	let {
		author = '',
		color = '#5865f2',
		initials = '',
		time = 'Today at 4:20 PM',
		bot = false,
		brand = false,
		nameColor = '',
		cont = false,
		children
	}: {
		author?: string;
		color?: string;
		initials?: string;
		time?: string;
		bot?: boolean;
		brand?: boolean;
		nameColor?: string;
		cont?: boolean;
		children?: Snippet;
	} = $props();

	const isBot = $derived(bot || brand);
	const ini = $derived(
		initials || author.replace(/[^a-zA-Z0-9]/g, '').slice(0, 2).toUpperCase() || '?'
	);
	const nColor = $derived(nameColor || (brand ? '#c79bff' : '#f2f3f5'));
</script>

<div class="flex gap-3 {cont ? 'py-0.5' : 'pt-1'}">
	<!-- avatar column -->
	<div class="w-10 shrink-0">
		{#if !cont}
			{#if brand}
				<div class="grid h-10 w-10 place-items-center rounded-full bg-[#f1dfdf]">
					<Logo size={24} />
				</div>
			{:else}
				<div
					class="grid h-10 w-10 place-items-center rounded-full text-[13px] font-semibold text-white"
					style="background: {color};"
				>
					{ini}
				</div>
			{/if}
		{/if}
	</div>

	<!-- body -->
	<div class="min-w-0 flex-1">
		{#if !cont}
			<div class="flex items-center gap-2 leading-none">
				<span class="text-[15px] font-medium" style="color: {nColor};">{author}</span>
				{#if isBot}
					<span
						class="rounded-[3px] bg-[#5865f2] px-1 py-px text-[10px] font-semibold uppercase leading-tight text-white"
						>App</span
					>
				{/if}
				<span class="text-[11px] text-[#949ba4]">{time}</span>
			</div>
		{/if}
		<div class="mt-0.5 text-[14.5px] leading-[1.4] text-[#dbdee1]">
			{@render children?.()}
		</div>
	</div>
</div>
