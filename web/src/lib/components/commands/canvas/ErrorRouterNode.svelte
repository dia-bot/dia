<script lang="ts">
	// On-error router — a small switch-style card hanging off a step's red dot.
	// One row per typed error case (drag its dot to build that arm) plus an
	// "else" row for the default handler. Click the card to describe the cases
	// (which error kinds each arm matches).
	import { Handle, Position, type NodeProps } from '@xyflow/svelte';
	import type { Step } from '$lib/commands/types';
	import type { NodeData } from './adapter';
	import ShieldAlert from 'lucide-svelte/icons/shield-alert';
	import Trash2 from 'lucide-svelte/icons/trash-2';

	type Props = NodeProps & { data: NodeData & { dimmed?: boolean } };
	let { data, id, selected }: Props = $props();

	const step = $derived(data.step as Step);
	const cases = $derived(step?.on_error_cases ?? []);
	const hasElse = $derived(step?.on_error !== undefined);

	function patterns(when: string[] | undefined): string {
		const s = (when ?? []).join(', ');
		return s === '' || s === '*' ? 'any error' : s;
	}

	function emit(name: string, detail: object) {
		document.dispatchEvent(new CustomEvent(`dia-canvas-${name}`, { detail }));
	}
</script>

<div
	class="router group/router relative w-[200px] rounded-lg border bg-card text-foreground transition-all duration-200
		{selected
		? 'border-destructive/60 shadow-[0_0_0_3px_hsl(var(--destructive)/0.12),0_12px_32px_-12px_rgba(0,0,0,0.5)]'
		: 'border-destructive/30 shadow-[0_1px_2px_rgba(0,0,0,0.3)] hover:border-destructive/50'}
		{data.dimmed ? 'opacity-30' : ''}"
	data-router-id={id}
>
	<Handle
		type="target"
		position={Position.Top}
		id="in"
		class="!size-2 !border-2 !border-card !bg-destructive/80"
	/>

	<div
		class="flex items-center gap-1.5 rounded-t-lg border-b border-destructive/15 bg-destructive/[0.06] px-2 py-1.5"
	>
		<ShieldAlert class="size-3 shrink-0 text-destructive" />
		<span class="flex-1 text-[11px] font-semibold text-foreground">On error</span>
		<button
			type="button"
			class="nodrag grid size-4.5 shrink-0 place-items-center rounded text-muted-foreground/50 opacity-0 transition-all hover:text-destructive group-hover/router:opacity-100"
			title="Remove error handling from this step"
			aria-label="Remove error handling"
			onclick={(e) => {
				e.stopPropagation();
				emit('remove-error-router', { id: data.ownerId });
			}}
		>
			<Trash2 class="size-2.5" />
		</button>
	</div>

	<div class="py-1">
		{#each cases as ec, i (i)}
			<div class="relative flex h-6 items-center gap-1.5 px-2">
				<span class="size-1 shrink-0 rounded-full bg-destructive/60"></span>
				<span class="min-w-0 flex-1 truncate font-mono text-[10px] text-muted-foreground">
					{patterns(ec.when)}
				</span>
				<Handle
					type="source"
					id={`arm-${i}`}
					position={Position.Right}
					class="!absolute !-right-1 !top-1/2 !size-2 !-translate-y-1/2 !border-2 !border-card !bg-destructive/80 hover:!bg-destructive"
				/>
			</div>
		{/each}
		{#if hasElse}
			<div class="relative flex h-6 items-center gap-1.5 px-2">
				<span class="size-1 shrink-0 rounded-full bg-muted-foreground/40"></span>
				<span class="min-w-0 flex-1 truncate font-mono text-[10px] italic text-muted-foreground/70">
					{cases.length ? 'else' : 'any error'}
				</span>
				<Handle
					type="source"
					id="default"
					position={Position.Right}
					class="!absolute !-right-1 !top-1/2 !size-2 !-translate-y-1/2 !border-2 !border-card !bg-muted-foreground/70 hover:!bg-foreground"
				/>
			</div>
		{/if}
	</div>

	<div
		class="rounded-b-lg border-t border-border/40 px-2 py-1 text-center font-mono text-[8.5px] uppercase tracking-[0.1em] text-muted-foreground/50"
	>
		click to edit cases
	</div>
</div>

<style>
	.router {
		animation: router-pop-in 220ms cubic-bezier(0.22, 1, 0.36, 1) both;
	}
	@keyframes router-pop-in {
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
		.router {
			animation: none;
		}
	}
</style>
