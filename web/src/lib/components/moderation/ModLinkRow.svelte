<script lang="ts">
	// A flat cross-link row: a monochrome icon, a title + sub, and a trailing arrow.
	// Replaces the boxed "callout" cards — it reads as a quiet, hairline-bounded
	// link inside a ModSection (pad={false}), never a coloured box. The icon stays
	// neutral; rose is reserved for true status, not decoration.
	import { ArrowRight } from 'lucide-svelte';
	import type { LucideIcon } from '$lib/commands/icons';

	let {
		href,
		icon,
		title,
		desc,
		external = false
	}: {
		href: string;
		icon: LucideIcon;
		title: string;
		desc?: string;
		external?: boolean;
	} = $props();
	const Icon = $derived(icon);
</script>

<a
	{href}
	target={external ? '_blank' : undefined}
	rel={external ? 'noreferrer' : undefined}
	class="group flex items-center gap-3 px-4 py-3.5 transition-colors hover:bg-ink-2/40 sm:px-5"
>
	<span
		class="grid size-8 shrink-0 place-items-center rounded-lg border border-line bg-ink-2 text-muted transition-colors group-hover:text-ink"
	>
		<Icon size={15} />
	</span>
	<span class="min-w-0 flex-1">
		<span class="block text-[13px] font-medium text-ink">{title}</span>
		{#if desc}<span class="block text-[11.5px] text-muted">{desc}</span>{/if}
	</span>
	<ArrowRight
		size={15}
		class="shrink-0 text-faint transition-transform group-hover:translate-x-0.5 group-hover:text-ink"
	/>
</a>
