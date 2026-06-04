<script lang="ts">
	import type { Snippet } from 'svelte';

	// A faithful Discord rich-embed: coloured left bar, title, description,
	// optional fields, image, and footer. Rendered on the dark chat surface.
	let {
		color = '#B244FC',
		authorName = '',
		title = '',
		description = '',
		image = '',
		footer = '',
		footerIcon = false,
		fields = [],
		children
	}: {
		color?: string;
		authorName?: string;
		title?: string;
		description?: string;
		image?: string;
		footer?: string;
		footerIcon?: boolean;
		fields?: { name: string; value: string }[];
		children?: Snippet;
	} = $props();
</script>

<div
	class="max-w-[440px] overflow-hidden rounded-[4px] bg-[#2b2d31]"
	style="border-left: 4px solid {color};"
>
	<div class="px-3.5 py-3">
		{#if authorName}
			<div class="mb-1 text-[12px] font-semibold text-[#f2f3f5]">{authorName}</div>
		{/if}
		{#if title}
			<div class="mb-1 text-[15px] font-semibold leading-snug text-[#f2f3f5]">{title}</div>
		{/if}
		{#if description}
			<p class="whitespace-pre-line text-[13.5px] leading-relaxed text-[#d4d7dc]">{description}</p>
		{/if}

		{#if children}
			<div class="text-[13.5px] leading-relaxed text-[#d4d7dc]">{@render children()}</div>
		{/if}

		{#if fields.length}
			<div class="mt-2.5 grid grid-cols-2 gap-x-4 gap-y-2.5">
				{#each fields as f (f.name)}
					<div>
						<div class="text-[12px] font-semibold text-[#f2f3f5]">{f.name}</div>
						<div class="mt-0.5 text-[13px] text-[#b5bac1]">{f.value}</div>
					</div>
				{/each}
			</div>
		{/if}

		{#if image}
			<img src={image} alt="" class="mt-3 w-full rounded-[4px]" />
		{/if}

		{#if footer}
			<div class="mt-2.5 flex items-center gap-1.5 text-[11.5px] font-medium text-[#a3a6ac]">
				{#if footerIcon}
					<span
						class="grid h-4 w-4 place-items-center rounded-full text-[8px] font-bold text-white"
						style="background: linear-gradient(135deg,#ff6363,#b244fc);">D</span
					>
				{/if}
				{footer}
			</div>
		{/if}
	</div>
</div>
