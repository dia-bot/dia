<script lang="ts" module>
	import type { ShapeNode } from '$lib/commands/types';

	// ── Pure layout over the compact shape ─────────────────────────────────
	// A sequence flows down one lane; extra branch arms take the first free
	// lane to the right. Everything is clamped to the tiny stage; what falls
	// off is counted so the tile can say "+n more".

	export type PlacedNode = { cx: number; cy: number; k: string; e: boolean };
	export type PlacedEdge = { x1: number; y1: number; x2: number; y2: number };

	const RANK_H = 26;
	const LANE_W = 44;
	export const NODE_W = 30;
	export const NODE_H = 14;
	const TOP_Y = 12;
	const PILL_H = 18;
	const MAX_RANK = 5; // entry sits at rank 0; steps occupy 1..5
	const MIN_LANE = -2;
	const MAX_LANE = 2;

	function sizeOf(seq: ShapeNode[]): number {
		let n = 0;
		for (const s of seq) {
			n++;
			for (const arm of s.c ?? []) n += sizeOf(arm);
		}
		return n;
	}

	export function layoutShape(shape: ShapeNode[] | undefined): {
		nodes: PlacedNode[];
		edges: PlacedEdge[];
		dropped: number;
		dx: number;
	} {
		const nodes: PlacedNode[] = [];
		const edges: PlacedEdge[] = [];
		const occupied = new Set<string>();
		let dropped = 0;

		const laneX = (lane: number) => 140 + lane * LANE_W;
		const rankY = (rank: number) => TOP_Y + rank * RANK_H + PILL_H / 2;

		function firstFreeLane(rank: number, from: number): number | null {
			for (let l = from; l <= MAX_LANE; l++) {
				if (!occupied.has(`${rank}:${l}`)) return l;
			}
			return null;
		}

		// Returns the next free rank in this lane after the sequence.
		function placeSeq(
			seq: ShapeNode[],
			lane: number,
			rank: number,
			prev: PlacedNode | null
		): number {
			for (let i = 0; i < seq.length; i++) {
				const s = seq[i];
				if (rank > MAX_RANK || lane < MIN_LANE || lane > MAX_LANE || occupied.has(`${rank}:${lane}`)) {
					dropped += sizeOf(seq.slice(i));
					return rank;
				}
				const node: PlacedNode = { cx: laneX(lane), cy: rankY(rank), k: s.k, e: !!s.e };
				nodes.push(node);
				occupied.add(`${rank}:${lane}`);
				if (prev) {
					edges.push({ x1: prev.cx, y1: prev.cy + NODE_H / 2, x2: node.cx, y2: node.cy - NODE_H / 2 });
				}
				prev = node;
				let next = rank + 1;
				const arms = s.c ?? [];
				for (let j = 0; j < arms.length; j++) {
					const arm = arms[j];
					if (arm.length === 0) continue;
					let armLane = lane;
					if (j > 0) {
						const free = firstFreeLane(rank + 1, lane + 1);
						if (free === null) {
							dropped += sizeOf(arm);
							continue;
						}
						armLane = free;
					}
					const end = placeSeq(arm, armLane, rank + 1, node);
					if (end > next) next = end;
				}
				rank = next;
			}
			return rank;
		}

		const entry: PlacedNode = { cx: laneX(0), cy: TOP_Y + PILL_H / 2, k: '__entry__', e: false };
		occupied.add('0:0');
		placeSeq(shape ?? [], 0, 1, entry);

		// Center the drawing horizontally on the 280-wide stage.
		let minX = entry.cx;
		let maxX = entry.cx;
		for (const n of nodes) {
			if (n.cx < minX) minX = n.cx;
			if (n.cx > maxX) maxX = n.cx;
		}
		const dx = 140 - (minX + maxX) / 2;

		return { nodes, edges, dropped, dx };
	}
</script>

<script lang="ts">
	// The miniature canvas: a command's real program drawn at poster scale.
	// Monochrome on purpose, matching the editor's color truth: the only
	// color is the rose entry pill (and the dashed red error rail).
	let {
		shape,
		name,
		more = 0
	}: {
		shape?: ShapeNode[];
		name: string;
		more?: number;
	} = $props();

	const laid = $derived(layoutShape(shape));
	const overflow = $derived(more + laid.dropped);
	const label = $derived('/' + (name || 'command').slice(0, 14));
</script>

<svg viewBox="0 0 280 168" class="h-full w-full" aria-hidden="true">
	<g class="tg" transform="translate({laid.dx} 0)">
		{#each laid.edges as e, i (i)}
			<path
				class="tedge"
				d="M {e.x1} {e.y1} C {e.x1} {e.y1 + 9}, {e.x2} {e.y2 - 9}, {e.x2} {e.y2}"
			/>
		{/each}

		<!-- Entry pill: the page's single rose moment -->
		<rect class="tpill" x={140 - 32} y={12} width="64" height="18" rx="9" />
		<path
			d="M {140 - 24} {12 + 5.5} l 5 3.5 l -5 3.5 z"
			fill="var(--color-accent)"
			opacity="0.9"
		/>
		<text class="tname" x={140 - 16} y={12 + 12.5}>{label}</text>

		{#if !shape || shape.length === 0}
			<rect
				class="tghost"
				x={140 - NODE_W / 2}
				y={12 + RANK_H + 2}
				width={NODE_W}
				height={NODE_H}
				rx="3"
			/>
		{/if}

		{#each laid.nodes as n, i (i)}
			<rect
				class="tnode"
				x={n.cx - NODE_W / 2}
				y={n.cy - NODE_H / 2}
				width={NODE_W}
				height={NODE_H}
				rx="3"
			/>
			<circle class="tdot" cx={n.cx - NODE_W / 2 + 5} cy={n.cy} r="1.5" />
			{#if n.e}
				<path
					class="terr"
					d="M {n.cx - NODE_W / 2} {n.cy} h -8"
				/>
				<circle class="terrdot" cx={n.cx - NODE_W / 2 - 11} cy={n.cy} r="2.5" />
			{/if}
		{/each}
	</g>
</svg>
{#if overflow > 0}
	<span
		class="pointer-events-none absolute bottom-1.5 right-2 font-mono text-[8.5px] text-faint"
	>
		+{overflow} more
	</span>
{/if}

<style>
	.tg {
		transition: transform 280ms var(--canvas-ease, cubic-bezier(0.22, 1, 0.36, 1));
		transform-origin: 140px 84px;
	}
	.tedge {
		fill: none;
		stroke: rgba(255, 255, 255, 0.18);
		stroke-width: 1;
	}
	.tpill {
		fill: color-mix(in srgb, var(--color-accent) 8%, transparent);
		stroke: color-mix(in srgb, var(--color-accent) 25%, transparent);
		stroke-width: 1;
	}
	.tname {
		font-family: var(--font-mono, monospace);
		font-size: 8.5px;
		fill: var(--color-ink);
	}
	.tnode {
		fill: var(--color-surface);
		stroke: var(--color-line-strong);
		stroke-width: 1;
	}
	.tghost {
		fill: none;
		stroke: var(--color-line);
		stroke-width: 1;
		stroke-dasharray: 3 3;
	}
	.tdot {
		fill: var(--color-ink);
		opacity: 0.6;
	}
	.terr,
	.terrdot {
		fill: none;
		stroke: color-mix(in srgb, var(--color-danger) 50%, transparent);
		stroke-width: 1;
	}
	.terr {
		stroke-dasharray: 3 3;
	}
	@media (prefers-reduced-motion: reduce) {
		.tg {
			transition: none;
		}
	}
</style>
