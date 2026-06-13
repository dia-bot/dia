<script lang="ts">
	import { BaseEdge, getBezierPath, type EdgeProps } from '@xyflow/svelte';
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
</script>

<BaseEdge
	{id}
	{path}
	{markerEnd}
	style={`stroke: hsl(var(--muted-foreground) / 0.7); stroke-width: 1.6; ${style ?? ''}`}
	class="dia-edge dia-edge-plain"
/>
