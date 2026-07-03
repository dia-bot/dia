<script lang="ts">
	// Right-side edit drawer over the canvas — click a step to open, edit in
	// place, close to get back to the flow. The canvas stays visible (and
	// keeps the subtree of the edited step highlighted) behind it.
	import type { Step } from '$lib/commands/types';
	import { STEP_KIND_BY_KIND } from '$lib/commands/types';
	import { iconFor } from '$lib/commands/icons';
	import { stepSummary } from '$lib/commands/summaries';
	import StepInspector from './StepInspector.svelte';
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';

	import X from 'lucide-svelte/icons/x';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import Copy from 'lucide-svelte/icons/copy';

	let {
		step,
		onClose,
		onDelete,
		onDuplicate
	}: {
		step: Step;
		onClose: () => void;
		onDelete: (id: string) => void;
		// Optional: when provided, a Copy button duplicates this step (the
		// automations editor wires it; the command editor leaves it off).
		onDuplicate?: (id: string) => void;
	} = $props();

	const meta = $derived(STEP_KIND_BY_KIND.get(step.kind));
	const Icon = $derived(iconFor(meta?.icon ?? 'Square'));
	const summary = $derived(stepSummary(step));
	// Message composers get a wider drawer — room for the live preview.
	const isMessageKind = $derived(
		['reply', 'edit_reply', 'send_message', 'send_dm', 'embed_send'].includes(step.kind)
	);
</script>

<div
	in:fly={{ x: 28, duration: dur(240), easing: cubicOut }}
	out:fly={{ x: 28, duration: dur(160), easing: cubicOut }}
	class="absolute inset-y-0 right-0 z-20 flex w-full flex-col border-l border-line bg-bg shadow-[0_0_48px_-12px_rgba(0,0,0,0.7)] {isMessageKind
		? 'max-w-md md:max-w-xl xl:max-w-2xl'
		: 'max-w-sm md:max-w-md'}"
>
	<!-- Header -->
	<div class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-3.5">
		<span class="grid size-6 shrink-0 place-items-center rounded-md border border-line bg-ink-2 text-muted">
			<Icon size={12} />
		</span>
		<div class="min-w-0 flex-1">
			<div class="flex items-baseline gap-2">
				<span class="text-[13px] font-medium text-ink">{meta?.label ?? step.kind}</span>
				<code class="font-mono text-[9.5px] text-faint">{step.kind}</code>
			</div>
			{#if summary}
				<p class="truncate font-mono text-[10px] text-faint">{summary}</p>
			{/if}
		</div>
		{#if onDuplicate}
			<button
				type="button"
				class="grid size-7 shrink-0 place-items-center rounded-md text-muted transition-colors hover:bg-surface hover:text-ink"
				onclick={() => onDuplicate?.(step.id)}
				title="Duplicate step (⌘D)"
				aria-label="Duplicate step"
			>
				<Copy size={12} />
			</button>
		{/if}
		<button
			type="button"
			class="inline-flex h-7 shrink-0 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-muted transition-colors hover:bg-surface hover:text-danger"
			onclick={() => onDelete(step.id)}
		>
			<Trash2 size={12} />
			Delete
		</button>
		<button
			type="button"
			class="grid size-7 shrink-0 place-items-center rounded-md text-faint transition-colors hover:bg-surface hover:text-ink"
			onclick={onClose}
			title="Close (Esc)"
			aria-label="Close"
		>
			<X size={13} />
		</button>
	</div>

	<!-- Body: the per-kind spec form -->
	<div class="min-h-0 flex-1 overflow-y-auto">
		<StepInspector {step} embedded />
	</div>
</div>

