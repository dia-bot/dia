<script lang="ts">
	import { SvelteFlowProvider } from '@xyflow/svelte';
	import type { Step } from '$lib/commands/types';
	import FlowInner from './FlowInner.svelte';

	let {
		steps,
		scratch = [],
		commandName,
		commandId = '',
		selectedId = $bindable<string>(),
		errorPaths = new Set<string>(),
		onAddAtRoot,
		onAddFromHandle,
		onDeleteStep,
		onAddErrorRouter,
		onRemoveErrorRouter,
		onTruncateChain,
		onAbsorbAfter,
		onDetach,
		onAttachScratch,
		onAddCase,
		onAddParallelBranch,
		showLegend = true
	}: {
		steps: Step[];
		scratch?: Step[][];
		commandName: string;
		commandId?: string;
		selectedId: string;
		errorPaths?: Set<string>;
		showLegend?: boolean;
		onAddAtRoot?: (kind: string, position?: { x: number; y: number }) => void;
		onAddFromHandle?: (
			sourceNodeId: string,
			sourceHandle: string | null,
			kind: string,
			position: { x: number; y: number }
		) => void;
		onDeleteStep?: (id: string) => void;
		onAddErrorRouter?: (id: string) => void;
		onRemoveErrorRouter?: (id: string) => void;
		onTruncateChain?: (id: string) => void;
		onAbsorbAfter?: (id: string, which: 'then' | 'else' | 'default') => void;
		onDetach?: (id: string) => void;
		onAttachScratch?: (sourceId: string, handle: string | null, headId: string) => void;
		onAddCase?: (id: string) => void;
		onAddParallelBranch?: (id: string) => void;
	} = $props();
</script>

<SvelteFlowProvider>
	<FlowInner
		{steps}
		{scratch}
		{commandName}
		{commandId}
		bind:selectedId
		{errorPaths}
		{onAddAtRoot}
		{onAddFromHandle}
		{onDeleteStep}
		{onAddErrorRouter}
		{onRemoveErrorRouter}
		{onTruncateChain}
		{onAbsorbAfter}
		{onDetach}
		{onAttachScratch}
		{onAddCase}
		{onAddParallelBranch}
		{showLegend}
	/>
</SvelteFlowProvider>
