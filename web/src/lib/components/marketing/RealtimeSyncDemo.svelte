<script lang="ts">
	import { onMount } from 'svelte';
	import Hash from 'lucide-svelte/icons/hash';

	// Dia's realtime guild state: create a channel in Discord, it appears in the
	// dashboard with no refresh. Animated with GPU-friendly opacity/transform over
	// reserved space (no layout reflow) so it stays smooth. Auto-loops on screen.
	const base = ['welcome', 'general', 'level-ups', 'roles'];

	let el: HTMLElement;
	let inDiscord = $state(false);
	let inDash = $state(false);

	let timers: ReturnType<typeof setTimeout>[] = [];
	const clear = () => {
		timers.forEach(clearTimeout);
		timers = [];
	};

	function cycle() {
		clear();
		inDiscord = false;
		inDash = false;
		timers.push(setTimeout(() => (inDiscord = true), 1000));
		timers.push(setTimeout(() => (inDash = true), 1650));
		timers.push(setTimeout(() => (inDiscord = false), 4600));
		timers.push(setTimeout(() => (inDash = false), 4800));
		timers.push(setTimeout(cycle, 6000));
	}

	onMount(() => {
		const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
		if (reduce || typeof IntersectionObserver === 'undefined') {
			inDiscord = true;
			inDash = true;
			return;
		}
		const io = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) cycle();
				else {
					clear();
					inDiscord = false;
					inDash = false;
				}
			},
			{ threshold: 0.3 }
		);
		io.observe(el);
		return () => {
			io.disconnect();
			clear();
		};
	});

	const ease = 'transition-timing-function: cubic-bezier(0.22, 1, 0.36, 1);';
</script>

<div bind:this={el} class="grid items-stretch gap-4 md:grid-cols-[1fr_auto_1fr]">
	<!-- Discord -->
	<div class="overflow-hidden rounded-2xl bg-[#2b2d31] p-4 ring-1 ring-white/10">
		<div class="mb-2.5 flex h-5 items-center justify-between px-1">
			<span class="text-[12px] font-semibold uppercase tracking-wide text-[#8a8e95]">Discord</span>
			<span
				class="rounded-full bg-[#23a55a]/20 px-2 py-0.5 text-[11px] font-semibold text-[#3ba55c] transition-opacity duration-500"
				style="opacity: {inDiscord ? 1 : 0};"
			>
				+ channel created
			</span>
		</div>
		<div class="space-y-0.5">
			{#each base as ch (ch)}
				<div class="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-[14px] text-[#8a8e95]">
					<span class="text-[17px] leading-none text-[#80848e]">#</span>{ch}
				</div>
			{/each}
			<div
				class="flex items-center gap-1.5 rounded-md bg-[#404249] px-2 py-1.5 text-[14px] font-medium text-[#f2f3f5] ring-1 ring-[#3ba55c]/50 transition-all duration-[600ms]"
				style="{ease} opacity: {inDiscord ? 1 : 0}; transform: translateY({inDiscord ? 0 : 6}px) scale({inDiscord ? 1 : 0.98});"
			>
				<span class="text-[17px] leading-none text-[#80848e]">#</span>summer-event
			</div>
		</div>
	</div>

	<!-- connector -->
	<div class="flex items-center justify-center md:flex-col">
		<div
			class="flex items-center gap-1.5 rounded-full border border-line bg-surface px-2.5 py-1 text-[11px] font-semibold text-accent-ink shadow-sm transition-shadow duration-500"
		>
			<span
				class="h-1.5 w-1.5 rounded-full bg-accent transition-transform duration-500"
				style="transform: scale({inDiscord && !inDash ? 1.6 : 1});"
			></span>
			LIVE
		</div>
	</div>

	<!-- Dashboard -->
	<div class="card p-4">
		<div class="mb-2.5 flex h-5 items-center justify-between px-1">
			<span class="text-[12px] font-semibold uppercase tracking-wide text-muted">Dia dashboard</span>
			<span
				class="flex items-center gap-1 rounded-full bg-blush px-2 py-0.5 text-[11px] font-semibold text-accent-ink transition-opacity duration-500"
				style="opacity: {inDash ? 1 : 0};"
			>
				<span class="h-1.5 w-1.5 rounded-full bg-success"></span> synced just now
			</span>
		</div>
		<span class="label px-1">Announcement channel</span>
		<div class="space-y-0.5">
			{#each base as ch (ch)}
				<div class="flex items-center gap-1.5 rounded-md px-2 py-1.5 text-sm text-ink">
					<Hash size={14} class="text-faint" />{ch}
				</div>
			{/each}
			<div
				class="flex items-center gap-1.5 rounded-md bg-blush px-2 py-1.5 text-sm font-medium text-accent-ink ring-1 ring-accent/30 transition-all duration-[600ms]"
				style="{ease} opacity: {inDash ? 1 : 0}; transform: translateY({inDash ? 0 : 6}px) scale({inDash ? 1 : 0.98});"
			>
				<Hash size={14} class="text-accent" />summer-event
			</div>
		</div>
	</div>
</div>
