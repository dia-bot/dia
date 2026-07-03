<script lang="ts">
	// Collapsed view of a rule in the list: icon, name, trigger label + summary,
	// action chips, an enabled toggle and edit/duplicate/flow/delete controls.
	import {
		TRIGGERS_BY_KEY,
		ACTIONS_BY_KEY,
		triggerSummary,
		type AutomodRule
	} from '$lib/moderation/automod';
	import { iconFor } from '$lib/commands/icons';
	import Toggle from '$lib/components/Toggle.svelte';
	import { TONE_CHIP } from './tone';
	import { Pencil, Copy, Route, Trash2 } from 'lucide-svelte';

	let {
		rule,
		editing = false,
		onedit,
		onduplicate,
		ondelete,
		onflow
	}: {
		rule: AutomodRule;
		editing?: boolean;
		onedit?: () => void;
		onduplicate?: () => void;
		ondelete?: () => void;
		onflow?: () => void;
	} = $props();

	const spec = $derived(TRIGGERS_BY_KEY[rule.trigger.type]);
	const TIcon = $derived(iconFor(spec?.icon ?? 'Square'));
	// Steps wired after the rule's actions on the automations canvas.
	const tailCount = $derived(rule.tail?.length ?? 0);
</script>

<div
	class="flex items-center gap-3.5 rounded-xl border bg-surface px-4 py-3.5 transition-colors {editing
		? 'border-line-strong'
		: 'border-line hover:border-line-strong'} {rule.enabled ? '' : 'opacity-60'}"
>
	<span class="grid size-10 shrink-0 place-items-center rounded-lg border border-line bg-ink-2 text-muted">
		<TIcon size={18} />
	</span>

	<div class="min-w-0 flex-1">
		<div class="flex items-center gap-2">
			<span class="truncate text-sm font-semibold text-ink">{rule.name}</span>
			<span class="shrink-0 rounded-full border border-line px-1.5 py-px font-mono text-[10px] text-muted">
				{spec?.label ?? rule.trigger.type}
			</span>
		</div>
		<p class="mt-0.5 truncate text-xs text-muted">{triggerSummary(rule.trigger)}</p>
		{#if rule.actions.length || tailCount}
			<div class="mt-2 flex flex-wrap items-center gap-1.5">
				{#each rule.actions as a, i (i)}
					{@const aspec = ACTIONS_BY_KEY[a.type]}
					{@const AIcon = iconFor(aspec?.icon ?? 'Square')}
					<span
						class="inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] font-medium {TONE_CHIP[
							aspec?.tone ?? 'neutral'
						]}"
					>
						<AIcon size={11} />
						{aspec?.label ?? a.type}
					</span>
				{/each}
				{#if tailCount}
					<span class="font-mono text-[10px] text-muted">
						+{tailCount} flow step{tailCount === 1 ? '' : 's'}
					</span>
				{/if}
			</div>
		{:else}
			<p class="mt-2 text-[11px] text-faint">No actions configured.</p>
		{/if}
	</div>

	<div class="flex shrink-0 items-center gap-1">
		<Toggle bind:checked={rule.enabled} label="Rule enabled" />
		<button
			type="button"
			onclick={onedit}
			class="grid size-8 place-items-center rounded-lg text-muted transition-colors hover:bg-ink-2 hover:text-ink"
			aria-label="Edit rule"
		>
			<Pencil size={15} />
		</button>
		<button
			type="button"
			onclick={onduplicate}
			class="grid size-8 place-items-center rounded-lg text-muted transition-colors hover:bg-ink-2 hover:text-ink"
			aria-label="Duplicate rule"
		>
			<Copy size={15} />
		</button>
		{#if onflow}
			<button
				type="button"
				onclick={onflow}
				class="grid size-8 place-items-center rounded-lg text-muted transition-colors hover:bg-ink-2 hover:text-ink"
				title="Customize follow-up flow"
				aria-label="Customize follow-up flow"
			>
				<Route size={15} />
			</button>
		{/if}
		<button
			type="button"
			onclick={ondelete}
			class="grid size-8 place-items-center rounded-lg text-muted transition-colors hover:bg-ink-2 hover:text-danger"
			aria-label="Delete rule"
		>
			<Trash2 size={15} />
		</button>
	</div>
</div>
