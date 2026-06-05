<script lang="ts">
	import { onMount } from 'svelte';

	// A leaderboard card with bars that grow on scroll-in. Mirrors the dashboard's
	// /leaderboard view with representative data.
	// Muted, on-palette avatar tones — a restrained rose→neutral ramp, not the
	// vibrant brand hexes (those are reserved for the welcome/rank cards).
	const rows = [
		{ name: 'Luna', level: 42, xp: 184200, color: '#a472ff' },
		{ name: 'Kaito', level: 38, xp: 152940, color: '#8154d6' },
		{ name: 'Mira', level: 35, xp: 131500, color: '#5b6472' },
		{ name: 'Theo', level: 31, xp: 98300, color: '#8a93a3' },
		{ name: 'Ivy', level: 28, xp: 74120, color: '#6e7079' }
	];
	const top = rows[0].xp;
	const ini = (n: string) => n.slice(0, 2).toUpperCase();

	let el: HTMLElement;
	let play = $state(false);

	onMount(() => {
		const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
		if (reduce || typeof IntersectionObserver === 'undefined') {
			play = true;
			return;
		}
		const io = new IntersectionObserver(
			(e) => {
				if (e[0]?.isIntersecting) {
					play = true;
					io.disconnect();
				}
			},
			{ threshold: 0.4 }
		);
		io.observe(el);
		return () => io.disconnect();
	});
</script>

<div bind:this={el} class="card p-5">
	<div class="mb-4 flex items-center justify-between">
		<h3 class="text-[15px] font-semibold">Leaderboard</h3>
		<span class="rounded-full bg-blush px-2 py-0.5 text-[11px] font-semibold text-accent-ink"
			>All time</span
		>
	</div>

	<div class="space-y-3.5">
		{#each rows as r, i (r.name)}
			<div class="flex items-center gap-3" style="transition: opacity .5s {i * 80}ms; opacity: {play ? 1 : 0};">
				<span
					class="w-4 shrink-0 text-right text-sm font-bold tabular-nums {i < 3
						? 'text-accent-ink'
						: 'text-faint'}">{i + 1}</span
				>
				<div
					class="grid h-9 w-9 shrink-0 place-items-center rounded-full text-xs font-semibold text-white"
					style="background: {r.color};"
				>
					{ini(r.name)}
				</div>
				<div class="min-w-0 flex-1">
					<div class="flex items-baseline justify-between gap-2">
						<span class="truncate text-sm font-semibold">{r.name}</span>
						<span class="shrink-0 font-mono text-xs text-muted"
							>{r.xp.toLocaleString('en-US')} XP</span
						>
					</div>
					<div class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-line-strong">
						<div
							class="h-full rounded-full"
							style="width: {play ? (r.xp / top) * 100 : 0}%; background: var(--color-accent); transition: width 1s cubic-bezier(0.16,1,0.3,1) {i *
								90}ms;"
						></div>
					</div>
				</div>
				<span
					class="shrink-0 rounded-full bg-blush px-2 py-0.5 text-[11px] font-semibold text-accent-ink"
					>Lv {r.level}</span
				>
			</div>
		{/each}
	</div>
</div>
