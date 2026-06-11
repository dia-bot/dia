<script lang="ts">
	import { Handle, Position, type NodeProps } from '@xyflow/svelte';
	import Columns from 'lucide-svelte/icons/columns';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import Plus from 'lucide-svelte/icons/plus';
	import AlertTriangle from 'lucide-svelte/icons/alert-triangle';
	import type { NodeData } from './adapter';
	import type { Step } from '$lib/commands/types';

	type Props = NodeProps & { data: NodeData & { hasError?: boolean; dimmed?: boolean } };
	let { data, id, selected }: Props = $props();

	const step = $derived(data.step as Step);
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const branches = $derived(((step?.spec ?? {}) as any).branches ?? []);
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const join = $derived(((step?.spec ?? {}) as any).join ?? 'all');
	const hasError = $derived(!!data.hasError);
	const hasErrorRail = $derived(
		step?.on_error !== undefined || (step?.on_error_cases?.length ?? 0) > 0
	);

	function pctFor(i: number, total: number): number {
		return Math.round(((i + 1) * 100) / (total + 1));
	}

	function emit(name: string, detail: object) {
		document.dispatchEvent(new CustomEvent(`dia-canvas-${name}`, { detail }));
	}
</script>

<div
	class="step-node group/node relative w-[240px] rounded-xl border bg-card text-foreground transition-all duration-200
		{selected
		? 'border-foreground/40 shadow-[0_0_0_3px_hsl(var(--foreground)/0.08),0_12px_32px_-12px_rgba(0,0,0,0.5)]'
		: hasError
			? 'border-destructive/40 shadow-[0_1px_2px_rgba(0,0,0,0.4)]'
			: 'border-border/50 shadow-[0_1px_2px_rgba(0,0,0,0.3)] hover:border-foreground/25 hover:shadow-[0_4px_16px_-4px_rgba(0,0,0,0.45)]'}
		{data.dimmed ? 'opacity-30' : ''}"
	data-selected={selected ? 'true' : null}
	data-step-id={id}
>
	<Handle
		type="target"
		position={Position.Top}
		id="in"
		class="!size-2.5 !border-2 !border-card !bg-muted-foreground/70 hover:!bg-foreground {data.clickTarget
			? '!opacity-0'
			: ''}"
	/>
	<Handle
		type="target"
		position={Position.Left}
		id="in-left"
		style="top: 18px; --dia-dot-dx: 124px"
		class="!size-2 !border-2 !border-card !bg-muted-foreground/70 {data.clickTarget
			? 'dia-dot-in'
			: '!opacity-0'}"
	/>

	<div
		class="flex items-center gap-2 rounded-t-xl border-b border-border/50 bg-gradient-to-r from-foreground/[0.05] to-transparent px-2.5 py-1.5"
	>
		<span
			class="grid size-5 shrink-0 place-items-center rounded-md bg-foreground/[0.07] text-foreground/80 ring-1 ring-border/70"
		>
			<Columns size={11} strokeWidth={1.75} />
		</span>
		<span class="min-w-0 flex-1 truncate text-[12.5px] font-semibold leading-tight text-foreground">
			Parallel
		</span>
		<span class="shrink-0 font-mono text-[10px] text-muted-foreground/70">join {join}</span>
		{#if hasError}
			<AlertTriangle class="size-3 shrink-0 text-destructive" />
		{/if}
		<button
			type="button"
			class="nodrag grid size-5 shrink-0 place-items-center rounded text-muted-foreground/50 opacity-0 transition-[color,background,opacity] hover:bg-foreground/10 hover:text-foreground group-hover/node:opacity-100"
			title="Add branch"
			aria-label="Add branch"
			onclick={(e) => {
				e.stopPropagation();
				emit('add-branch', { id });
			}}
		>
			<Plus class="size-3" />
		</button>
		<button
			type="button"
			class="nodrag grid size-5 shrink-0 place-items-center rounded text-muted-foreground/50 opacity-0 transition-[color,background,opacity] hover:bg-destructive/15 hover:text-destructive group-hover/node:opacity-100"
			title="Delete parallel"
			aria-label="Delete parallel"
			onclick={(e) => {
				e.stopPropagation();
				emit('delete', { id });
			}}
		>
			<Trash2 class="size-3" />
		</button>
	</div>

	<div class="px-2.5 py-2">
		<div class="text-[9.5px] font-semibold uppercase tracking-[0.12em] text-muted-foreground/50">
			Fork
		</div>
		<div class="mt-0.5 truncate font-mono text-[11px] leading-relaxed text-muted-foreground">
			{branches.length} branch{branches.length === 1 ? '' : 'es'} run concurrently
		</div>
	</div>

	<div
		class="flex items-center justify-center rounded-b-xl border-t border-border/40 px-3 py-1 text-[9.5px] font-semibold uppercase tracking-[0.12em] text-muted-foreground/50"
	>
		branches ↓
	</div>

	{#each branches as _, i (i)}
		<Handle
			type="source"
			position={Position.Bottom}
			id={`branch-${i}`}
			style="left: {pctFor(i, branches.length + 1)}%"
			class="!size-2.5 !border-2 !border-card !bg-foreground/65 hover:!bg-foreground"
		/>
	{/each}
	<!-- Continuation after the join — last bottom slot, so every line leaves
	     downward and nothing hangs off the side of the box. -->
	<Handle
		type="source"
		position={Position.Bottom}
		id="out"
		style="left: {pctFor(branches.length, branches.length + 1)}%"
		class="!size-2 !border-2 !border-card !bg-muted-foreground/40 hover:!bg-foreground"
	/>
	<Handle
		type="source"
		position={Position.Left}
		id="on_error"
		class="!size-2 !border-2 !border-card !bg-destructive/80 {hasErrorRail
			? ''
			: '!opacity-0 transition-opacity group-hover/node:!opacity-100'}"
	/>
</div>

<style>
	.step-node {
		animation: step-pop-in 220ms cubic-bezier(0.22, 1, 0.36, 1) both;
	}
	@keyframes step-pop-in {
		from {
			opacity: 0;
			transform: scale(0.97);
		}
		to {
			opacity: 1;
			transform: scale(1);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.step-node {
			animation: none;
		}
	}
</style>
