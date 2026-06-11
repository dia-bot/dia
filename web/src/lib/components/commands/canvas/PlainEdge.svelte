<script lang="ts">
	import { BaseEdge, getBezierPath, type EdgeProps } from '@xyflow/svelte';

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

	const pathInfo = $derived(
		getBezierPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition })
	);
</script>

<BaseEdge
	{id}
	path={pathInfo[0]}
	{markerEnd}
	style={`stroke: hsl(var(--muted-foreground) / 0.7); stroke-width: 1.6; ${style ?? ''}`}
	class="dia-edge dia-edge-plain"
/>
