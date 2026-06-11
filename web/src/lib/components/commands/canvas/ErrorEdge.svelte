<script lang="ts">
	import { BaseEdge, EdgeLabel, getBezierPath, type EdgeProps } from '@xyflow/svelte';
	import { tweenedCoords } from './path-tween.svelte';

	let {
		id,
		sourceX,
		sourceY,
		targetX,
		targetY,
		sourcePosition,
		targetPosition,
		markerEnd,
		style
	}: EdgeProps = $props();

	const coords = tweenedCoords(
		() => id,
		() => ({ sx: sourceX, sy: sourceY, tx: targetX, ty: targetY })
	);
	const pathInfo = $derived(
		getBezierPath({
			sourceX: coords.current.sx,
			sourceY: coords.current.sy,
			targetX: coords.current.tx,
			targetY: coords.current.ty,
			sourcePosition,
			targetPosition
		})
	);
	const path = $derived(pathInfo[0]);
	const labelX = $derived(pathInfo[1]);
	const labelY = $derived(pathInfo[2]);
</script>

<BaseEdge
	{id}
	{path}
	{markerEnd}
	style={`stroke: hsl(var(--destructive)); stroke-width: 1.75; stroke-dasharray: 4 3; ${style ?? ''}`}
	class="dia-edge dia-edge-error"
/>
<!-- Clicking the label opens the same line panel as clicking the path. -->
<EdgeLabel
	x={labelX}
	y={labelY}
	class="dia-edge-label nodrag nopan inline-flex cursor-pointer items-center rounded-full border border-destructive/30 bg-background/85 px-2 py-0.5 text-[10px] font-medium text-destructive backdrop-blur-sm hover:border-destructive/60"
>
	<button
		type="button"
		class="cursor-pointer"
		onclick={(e) => {
			e.stopPropagation();
			document.dispatchEvent(new CustomEvent('dia-canvas-edge-label', { detail: { id } }));
		}}
	>
		on error
	</button>
</EdgeLabel>
