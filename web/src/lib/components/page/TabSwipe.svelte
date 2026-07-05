<script lang="ts">
	// Direction-aware "shared axis" push for in-page tabs. Switching a tab is a
	// coordinated swipe, not a fade: the outgoing panel slides out toward the way
	// you're travelling while the incoming panel slides in from the opposite side,
	// both overlapping in one grid cell so nothing stacks vertically. Translation
	// is the dominant cue (opacity only softens the crossover), so the eye reads
	// real movement and knows which direction it went.
	//
	//   <TabSwipe key={tab} index={TABS.findIndex((t) => t.k === tab)}>
	//     ...panel...
	//   </TabSwipe>
	import type { Snippet } from 'svelte';
	import { quintOut, cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';

	let {
		key,
		index,
		children,
		distance = 56
	}: { key: string | number; index: number; children: Snippet; distance?: number } = $props();

	// Compare the incoming tab index against the one rendered last. `prev` is
	// deliberately non-reactive: the derived reads `index` (its only reactive
	// dependency) and advances `prev` as it recomputes, so the travel direction
	// is ready within the same render that re-keys the panel. `|| 1` keeps a
	// forward default for the very first mount (no previous tab to compare).
	// svelte-ignore state_referenced_locally
	let prev = index;
	const dir = $derived.by(() => {
		const d = index === prev ? 0 : index > prev ? 1 : -1;
		prev = index;
		return d || 1;
	});

	// Overflow is clipped ONLY while a swipe is in flight, so the sliding panels
	// never spawn a page scrollbar, yet inline popovers can still overflow the
	// panel at rest. Count live transitions (an intro + outro run together) and
	// clip whenever any is active.
	let live = $state(0);
	const start = () => (live += 1);
	const end = () => (live = Math.max(0, live - 1));

	// Incoming panel: travels from `dir * distance` to 0 with a fast-settling
	// curve. Opacity rushes in over the first third so the slide, not the fade,
	// carries the transition; a hair of scale adds depth as it lands.
	function enter(_node: Element, { d }: { d: number }) {
		return {
			duration: dur(360),
			easing: quintOut,
			css: (t: number, u: number) =>
				`transform: translate3d(${u * d * distance}px, 0, 0) scale(${0.99 + t * 0.01}); opacity: ${Math.min(1, 0.2 + t * 1.8)};`
		};
	}
	// Outgoing panel: continues in the same travel direction, pushed off toward
	// the opposite edge and fading a step ahead of the incoming panel so the two
	// cross cleanly instead of muddying into a double image. It is pulled out of
	// flow (position:absolute) for the whole outro so it stops contributing to
	// the grid row height: the container tracks the INCOMING panel's height
	// immediately, so a tall -> short switch collapses in step with the swipe
	// instead of snapping down ~300ms later. `top/left/right:0` keeps it pinned
	// where it already sat; the taller content simply overflows and is clipped by
	// the .is-animating clip while it fades away.
	function leave(_node: Element, { d }: { d: number }) {
		return {
			duration: dur(300),
			easing: cubicOut,
			css: (t: number, u: number) =>
				`position: absolute; top: 0; left: 0; right: 0; transform: translate3d(${-u * d * distance * 0.6}px, 0, 0) scale(${0.99 + t * 0.01}); opacity: ${Math.max(0, t * 1.5 - 0.15)};`
		};
	}
</script>

<div class="swipe" class:is-animating={live > 0}>
	{#key key}
		<div
			class="swipe__panel"
			in:enter={{ d: dir }}
			out:leave={{ d: dir }}
			onintrostart={start}
			onintroend={end}
			onoutrostart={start}
			onoutroend={end}
		>
			{@render children()}
		</div>
	{/key}
</div>

<style>
	.swipe {
		display: grid;
		/* Anchor for the outgoing panel, which goes position:absolute during its
		   outro (see leave()) so it no longer sizes the grid row. */
		position: relative;
	}
	/* Both panels share one cell during a swipe so the outgoing and incoming
	   slides overlap instead of stacking; min-width:0 keeps wide grids from
	   blowing the cell out. */
	.swipe__panel {
		grid-area: 1 / 1;
		min-width: 0;
	}
	/* Clip + promote to its own layer only while the swipe is running. */
	.swipe.is-animating {
		overflow: clip;
	}
	.swipe.is-animating .swipe__panel {
		will-change: transform, opacity;
	}
</style>
