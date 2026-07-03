<script lang="ts">
	import { Handle, Position, type NodeProps } from '@xyflow/svelte';
	import Play from 'lucide-svelte/icons/play';
	import Zap from 'lucide-svelte/icons/zap';
	import type { NodeData } from './adapter';

	type Props = NodeProps & { data: NodeData };
	let { data }: Props = $props();

	// Automations enter on a trigger, not a slash command: swap the /command pill
	// for a "when …" event pill that opens the trigger config on click.
	const isTrigger = $derived(data.entryKind === 'trigger');
</script>

{#if isTrigger}
	<div
		class="entry-node flex h-9 cursor-pointer items-center gap-2 rounded-full border border-accent/25 bg-accent/[0.08] px-3.5 font-mono text-[12.5px] font-medium text-ink shadow-[0_1px_2px_rgba(0,0,0,0.25)]"
		title="Trigger. Runs when: {data.commandName ?? 'event'}. Click to configure."
	>
		<Zap class="size-3 fill-accent-ink text-accent-ink" strokeWidth={0} />
		<span class="text-[9px] uppercase tracking-[0.16em] text-accent-ink/70">when</span>
		<span>{data.commandName ?? 'event'}</span>
		<Handle
			type="source"
			position={Position.Bottom}
			id="out"
			class="!size-2.5 !border-2 !border-background !bg-foreground/70"
		/>
	</div>
{:else}
	<div
		class="entry-node flex h-9 items-center gap-2 rounded-full border border-accent/25 bg-accent/[0.08] px-3.5 font-mono text-[12.5px] font-medium text-ink shadow-[0_1px_2px_rgba(0,0,0,0.25)]"
		title="Command entry, runs when a member sends the slash command"
	>
		<Play class="size-3 fill-accent-ink text-accent-ink" strokeWidth={0} />
		<span><span class="text-accent-ink/70">/</span>{data.commandName ?? 'command'}</span>
		<Handle
			type="source"
			position={Position.Bottom}
			id="out"
			class="!size-2.5 !border-2 !border-background !bg-foreground/70"
		/>
	</div>
{/if}

<style>
	.entry-node {
		animation: step-pop-in 220ms cubic-bezier(0.22, 1, 0.36, 1) both;
	}
	@keyframes step-pop-in {
		from {
			opacity: 0;
			transform: scale(0.95);
		}
		to {
			opacity: 1;
			transform: scale(1);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.entry-node {
			animation: none;
		}
	}
</style>
