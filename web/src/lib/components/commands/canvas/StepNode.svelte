<script lang="ts">
	import { Handle, Position, type NodeProps } from '@xyflow/svelte';
	import { STEP_KIND_BY_KIND, type Step } from '$lib/commands/types';
	import { stepSummary } from '$lib/commands/summaries';
	import { iconFor } from '$lib/commands/icons';
	import type { NodeData } from './adapter';
	import AlertTriangle from 'lucide-svelte/icons/alert-triangle';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import Flag from 'lucide-svelte/icons/flag';
	import ShieldAlert from 'lucide-svelte/icons/shield-alert';
	import EmojiGlyph from '../EmojiGlyph.svelte';

	type Props = NodeProps & { data: NodeData & { hasError?: boolean; dimmed?: boolean } };
	let { data, id, selected }: Props = $props();

	const step = $derived(data.step as Step);
	const meta = $derived(STEP_KIND_BY_KIND.get(step?.kind ?? ''));
	const Icon = $derived(iconFor(meta?.icon ?? 'Square'));
	const hasError = $derived(!!data.hasError);
	const isStart = $derived(!!data.isStart);
	const endsHere = $derived(!!data.endsHere);
	const summary = $derived(step ? stepSummary(step) : '');
	const hasErrorHandler = $derived(
		step?.on_error !== undefined || (step?.on_error_cases?.length ?? 0) > 0
	);
	// wait_for nodes expose an extra "on timeout" path (the right dot): steps to
	// run if the wait window elapses without the click / submit / message.
	const isWaitFor = $derived(step?.kind === 'wait_for');
	const hasTimeout = $derived(
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		(((step?.spec as any)?.on_timeout ?? []) as unknown[]).length > 0
	);

	// Buttons & selects render on the card, ONE PER LINE — each with its own
	// dot on the right. Drag a dot to build that component's click path (a
	// wait-for-click scoped to it, then whatever you chain after).
	type CardComponent = {
		key: string;
		type: string;
		style: string;
		label: string;
		emoji: string;
		sfx: string;
		url: string;
		manual: boolean;
		noop: boolean;
	};
	const componentItems = $derived.by(() => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const rows = (((step?.spec ?? {}) as any).components ?? []) as { components: any[] }[];
		const out: CardComponent[] = [];
		rows.forEach((row, ri) =>
			(row.components ?? []).forEach((c, ci) => {
				out.push({
					key: `${ri}-${ci}`,
					type: c.type ?? 'button',
					style: c.style ?? 'primary',
					label: c.type === 'button' ? c.label || 'Button' : c.placeholder || 'Select menu',
					emoji: c.emoji ?? '',
					sfx: c.custom_id_suffix ?? '',
					url: c.url ?? '',
					manual: !!c.custom_id_manual,
					noop: c.on_click === 'none'
				});
			})
		);
		return out;
	});

	const BTN_BG: Record<string, string> = {
		primary: '#5865f2',
		secondary: '#4e5058',
		success: '#248046',
		danger: '#da373c',
		link: '#4e5058'
	};

	function emit(name: string, detail: object) {
		document.dispatchEvent(new CustomEvent(`dia-canvas-${name}`, { detail }));
	}
</script>

<div
	class="step-node group/node relative w-[248px] rounded-xl border bg-card text-foreground transition-all duration-200
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
		style="top: 18px; --dia-dot-dx: 128px"
		class="dia-left-dot !size-2 {data.clickTarget ? 'dia-dot-in' : '!opacity-0'}"
	/>

	<!-- Header band: icon chip · label · badges · delete -->
	<div
		class="flex items-center gap-2 rounded-t-xl border-b border-border/50 bg-gradient-to-r from-foreground/[0.05] to-transparent px-2.5 py-1.5"
	>
		<span
			class="grid size-5 shrink-0 place-items-center rounded-md bg-foreground/[0.07] text-foreground/80 ring-1 ring-border/70"
		>
			<Icon size={11} strokeWidth={1.75} />
		</span>
		<span class="min-w-0 flex-1 truncate text-[12.5px] font-semibold leading-tight text-foreground">
			{meta?.label ?? step?.kind}
		</span>
		{#if isStart}
			<span
				class="shrink-0 rounded bg-foreground px-1.5 py-px text-[9px] font-semibold uppercase tracking-[0.12em] text-background"
			>
				Start
			</span>
		{/if}
		{#if hasError}
			<AlertTriangle class="size-3 shrink-0 text-destructive" />
		{/if}
		<button
			type="button"
			class="nodrag grid size-5 shrink-0 place-items-center rounded text-muted-foreground/50 opacity-0 transition-[color,background,opacity] hover:bg-destructive/15 hover:text-destructive group-hover/node:opacity-100"
			title="Delete step"
			aria-label="Delete step"
			onclick={(e) => {
				e.stopPropagation();
				emit('delete', { id });
			}}
		>
			<Trash2 class="size-3" />
		</button>
	</div>

	<!-- Body: category eyebrow + one-line summary -->
	<div class="px-2.5 py-2">
		<div class="text-[9.5px] font-semibold uppercase tracking-[0.12em] text-muted-foreground/50">
			{data.category || 'Step'}
		</div>
		{#if summary}
			<div class="mt-0.5 truncate font-mono text-[11px] leading-relaxed text-muted-foreground">
				{summary}
			</div>
		{:else}
			<div class="mt-0.5 truncate text-[11px] italic leading-relaxed text-muted-foreground/50">
				{meta?.short ?? 'Click to configure'}
			</div>
		{/if}
	</div>

	<!-- Buttons & selects — one line each; drag the dot for its click path. -->
	{#if componentItems.length > 0}
		<div class="space-y-1 border-t border-border/40 px-2 pb-1.5 pt-1">
			<div class="font-mono text-[8.5px] font-medium uppercase tracking-[0.12em] text-muted-foreground/50">
				buttons · drag a dot = on click
			</div>
			{#each componentItems as it (it.key)}
				<div
					class="relative flex h-6 items-center gap-1.5 rounded-[3px] pl-1.5 pr-3 text-[10px] font-medium text-white"
					style="background: {it.type === 'button'
						? (BTN_BG[it.style] ?? BTN_BG.primary)
						: '#1e1f22'}"
					title={it.noop
						? 'Does nothing: clicks are acknowledged silently, no path runs'
						: it.manual
							? 'Manual custom id — routed by hand / automations, no canvas click path'
							: it.sfx
								? `Drag the dot → what happens when "${it.label}" is clicked (id: ${it.sfx})`
								: it.url
									? 'Link button — opens a URL, no click path'
									: 'Set a click id in the composer to wire this up'}
				>
					{#if it.emoji}<EmojiGlyph emoji={it.emoji} size={12} />{/if}
					<span class="min-w-0 flex-1 truncate">{it.label}</span>
					{#if it.noop}
						<span class="shrink-0 font-mono text-[8.5px] opacity-60">no action</span>
					{:else if it.sfx && !it.manual}
						<span class="shrink-0 font-mono text-[8.5px] opacity-60">{it.sfx}</span>
						<Handle
							type="source"
							id={`component-${it.sfx}`}
							position={Position.Right}
							class="!absolute !-right-1 !top-1/2 !size-2 !-translate-y-1/2 !border-2 !border-card !bg-foreground/80 hover:!bg-foreground"
						/>
					{:else if it.manual}
						<span
							class="shrink-0 font-mono text-[8.5px] opacity-60"
							title="Manual custom id — no canvas click path"
						>
							id ⚙
						</span>
					{:else if it.url}
						<span class="shrink-0 font-mono text-[8.5px] opacity-60">link ↗</span>
					{/if}
				</div>
			{/each}
		</div>
	{/if}

	<!-- Footer: terminal flag, or the on-error quick action -->
	{#if endsHere}
		<div
			class="flex items-center justify-center gap-1.5 rounded-b-xl border-t border-border/40 py-1 text-[10px] text-muted-foreground/70"
		>
			<Flag class="size-2.5" />
			Ends here
		</div>
	{:else if !hasErrorHandler}
		<button
			type="button"
			class="nodrag flex w-full items-center gap-1.5 rounded-b-xl border-t border-border/40 px-2.5 py-1.5 text-[10.5px] font-medium text-muted-foreground/60 transition-colors hover:bg-destructive/10 hover:text-destructive"
			title="Run steps when this one fails"
			onclick={(e) => {
				e.stopPropagation();
				emit('add-error-handler', { id });
			}}
		>
			<ShieldAlert class="size-3" />
			On error
		</button>
	{/if}

	<!-- Left red dot = the on-error rail (revealed on hover until used). -->
	<Handle
		type="source"
		position={Position.Left}
		id="on_error"
		class="!size-2 !border-2 !border-card !bg-destructive/80 {hasErrorHandler
			? ''
			: '!opacity-0 transition-opacity group-hover/node:!opacity-100'}"
	/>
	<Handle
		type="source"
		position={Position.Bottom}
		id="out"
		class="!size-2.5 !border-2 !border-card !bg-muted-foreground/70 hover:!bg-foreground"
	/>
	{#if isWaitFor}
		<!-- Right dot = the on-timeout path (revealed on hover until used). -->
		<Handle
			type="source"
			position={Position.Right}
			id="on_timeout"
			title="Run steps if the wait times out"
			class="!size-2 !border-2 !border-card !bg-foreground/50 {hasTimeout
				? ''
				: '!opacity-0 transition-opacity group-hover/node:!opacity-100'}"
		/>
	{/if}
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
