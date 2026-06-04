<script lang="ts">
	import type { Snippet } from 'svelte';
	import Logo from '$lib/components/Logo.svelte';

	// A dark Discord-client window used to stage live feature demos. Server rail
	// and channel sidebar progressively appear on wider screens; the channel
	// header + message area are always shown. `title` labels the window chrome.
	let {
		channel = 'general',
		topic = '',
		title = 'Dia',
		channels = ['welcome', 'general', 'level-ups'],
		members = '',
		children,
		class: klass = ''
	}: {
		channel?: string;
		topic?: string;
		title?: string;
		channels?: string[];
		members?: string;
		children: Snippet;
		class?: string;
	} = $props();
</script>

<div
	class="overflow-hidden rounded-2xl bg-[#313338] shadow-[0_30px_70px_-25px_rgba(0,0,0,0.55)] ring-1 ring-white/[0.08] {klass}"
>
	<!-- window chrome -->
	<div class="flex h-9 items-center gap-2 bg-[#1e1f22] px-4">
		<span class="h-3 w-3 rounded-full bg-[#ff5f57]/90"></span>
		<span class="h-3 w-3 rounded-full bg-[#febc2e]/90"></span>
		<span class="h-3 w-3 rounded-full bg-[#28c840]/90"></span>
		<span class="flex-1 text-center text-[12px] font-medium text-[#b5bac1]">
			{title} <span class="text-[#80848e]">— #{channel}</span>
		</span>
		<span class="w-[42px]"></span>
	</div>

	<div class="flex h-full">
		<!-- server rail -->
		<div class="hidden w-[68px] shrink-0 flex-col items-center gap-2 bg-[#1e1f22] py-3 sm:flex">
			<div class="relative">
				<span
					class="absolute -left-3 top-1/2 h-9 w-1 -translate-y-1/2 rounded-r-full bg-white"
				></span>
				<div
					class="grid h-12 w-12 place-items-center rounded-2xl bg-[#f1dfdf] ring-2 ring-[#b244fc]/40"
				>
					<Logo size={26} />
				</div>
			</div>
			{#each ['#3a2e5c', '#41434a', '#41434a'] as c, i (i)}
				<div class="h-12 w-12 rounded-[26px] transition-all" style="background: {c};"></div>
			{/each}
			<div class="mt-1 grid h-12 w-12 place-items-center rounded-[26px] bg-[#313338] text-xl font-light text-[#23a55a]">
				+
			</div>
		</div>

		<!-- channel sidebar -->
		<div class="hidden w-[188px] shrink-0 flex-col bg-[#2b2d31] md:flex">
			<div
				class="flex h-12 items-center border-b border-[#1f2023] px-4 text-[15px] font-semibold text-[#f2f3f5] shadow-sm"
			>
				{title}
			</div>
			<div class="space-y-0.5 px-2 py-3">
				<div class="px-2 pb-1 text-[11px] font-semibold uppercase tracking-wide text-[#8a8e95]">
					Text channels
				</div>
				{#each channels as ch (ch)}
					<div
						class="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-[15px] {ch === channel
							? 'bg-[#404249] font-medium text-[#f2f3f5]'
							: 'text-[#8a8e95]'}"
					>
						<span class="text-[18px] leading-none text-[#80848e]">#</span>
						{ch}
					</div>
				{/each}
			</div>
		</div>

		<!-- main column -->
		<div class="flex min-w-0 flex-1 flex-col">
			<div
				class="flex h-12 shrink-0 items-center gap-2 border-b border-[#26282c] px-4 text-[#f2f3f5]"
			>
				<span class="text-[20px] leading-none text-[#80848e]">#</span>
				<span class="text-[15px] font-semibold">{channel}</span>
				{#if topic}
					<span class="mx-1 hidden h-4 w-px bg-[#3f4147] sm:block"></span>
					<span class="hidden truncate text-[13px] text-[#949ba4] sm:block">{topic}</span>
				{/if}
				{#if members}
					<span class="ml-auto hidden items-center gap-1.5 text-[13px] text-[#949ba4] sm:flex">
						<span class="h-2 w-2 rounded-full bg-[#23a55a]"></span>
						{members}
					</span>
				{/if}
			</div>
			<div class="min-w-0 flex-1 space-y-3.5 px-4 py-4">
				{@render children()}
			</div>
		</div>
	</div>
</div>
