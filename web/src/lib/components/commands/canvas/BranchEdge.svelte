<script lang="ts">
	import { BaseEdge, EdgeLabel, getBezierPath, type EdgeProps } from '@xyflow/svelte';

	let {
		id,
		sourceX,
		sourceY,
		targetX,
		targetY,
		sourcePosition,
		targetPosition,
		markerEnd,
		style,
		label
	}: EdgeProps = $props();

	const pathInfo = $derived(
		getBezierPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition })
	);
	const path = $derived(pathInfo[0]);
	const labelX = $derived(pathInfo[1]);
	const labelY = $derived(pathInfo[2]);
</script>

<BaseEdge
	{id}
	{path}
	{markerEnd}
	style={`stroke: hsl(var(--muted-foreground) / 0.55); stroke-width: 1.5; ${style ?? ''}`}
	class="dia-edge dia-edge-branch"
/>
{#if label}
	<!-- The label is the easiest thing to hit — clicking it opens the same
	     line panel as clicking the path. -->
	<EdgeLabel
		x={labelX}
		y={labelY}
		class="dia-edge-label nodrag nopan inline-flex cursor-pointer items-center rounded-full border border-border/40 bg-background/85 px-2 py-0.5 text-[10px] font-medium text-muted-foreground/90 backdrop-blur-sm transition-colors hover:border-border hover:text-foreground"
	>
		<button
			type="button"
			class="cursor-pointer"
			onclick={(e) => {
				e.stopPropagation();
				document.dispatchEvent(new CustomEvent('dia-canvas-edge-label', { detail: { id } }));
			}}
		>
			{label}
		</button>
	</EdgeLabel>
{/if}
