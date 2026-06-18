<script lang="ts">
	// Full editor for a single rule: name + enabled, the trigger's dynamic fields,
	// per-rule exemptions, and an actions builder. The rule is a $state object owned
	// by the page, so every edit here flows straight back into the config.
	import {
		TRIGGERS_BY_KEY,
		ACTIONS_BY_KEY,
		actionsForSurface,
		type AutomodRule,
		type RuleAction,
		type ActionKey,
		type FieldSpec
	} from '$lib/moderation/automod';
	import { iconFor } from '$lib/commands/icons';
	import { DropdownMenu } from 'bits-ui';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import FieldControl from './FieldControl.svelte';
	import { TONE_CHIP } from './tone';
	import { ChevronUp, ChevronDown, Trash2, Plus, X } from 'lucide-svelte';

	let { rule, onclose }: { rule: AutomodRule; onclose?: () => void } = $props();

	const spec = $derived(TRIGGERS_BY_KEY[rule.trigger.type]);
	const TIcon = $derived(iconFor(spec.icon));

	// Only the trigger fields whose showWhen condition (if any) is satisfied.
	function visible(fields: FieldSpec[], obj: Record<string, unknown>): FieldSpec[] {
		return fields.filter((f) => {
			if (!f.showWhen) return true;
			return f.showWhen.in.includes(obj[f.showWhen.key] as string | number | boolean);
		});
	}
	const triggerFields = $derived(
		visible(spec.fields, rule.trigger as unknown as Record<string, unknown>)
	);

	const available = $derived(actionsForSurface(spec.surface));

	function addAction(key: ActionKey) {
		const a = ACTIONS_BY_KEY[key];
		rule.actions = [...rule.actions, { type: key, ...a.defaults } as RuleAction];
	}
	function removeAction(i: number) {
		rule.actions = rule.actions.filter((_, idx) => idx !== i);
	}
	function moveAction(i: number, dir: -1 | 1) {
		const j = i + dir;
		if (j < 0 || j >= rule.actions.length) return;
		const next = [...rule.actions];
		[next[i], next[j]] = [next[j], next[i]];
		rule.actions = next;
	}

	function ensureExempt() {
		if (!rule.exempt) rule.exempt = {};
	}
</script>

<div class="rounded-2xl border border-line-strong bg-surface">
	<!-- header: name + enabled + close -->
	<div class="flex items-center gap-3 border-b border-line px-4 py-3">
		<span class="grid size-9 shrink-0 place-items-center rounded-lg border border-line bg-ink-2 text-accent-ink">
			<TIcon size={17} />
		</span>
		<div class="min-w-0 flex-1">
			<input
				class="input h-9"
				placeholder="Rule name"
				bind:value={rule.name}
				aria-label="Rule name"
			/>
		</div>
		<label class="flex shrink-0 items-center gap-2 text-xs text-muted">
			{rule.enabled ? 'On' : 'Off'}
			<Toggle bind:checked={rule.enabled} label="Rule enabled" />
		</label>
		<button
			type="button"
			onclick={onclose}
			class="grid size-8 shrink-0 place-items-center rounded-lg text-muted transition-colors hover:bg-ink-2 hover:text-ink"
			aria-label="Close editor"
		>
			<X size={16} />
		</button>
	</div>

	<div class="divide-y divide-line">
		<!-- trigger fields -->
		<section class="px-4 py-4">
			<div class="eyebrow mb-1">Trigger</div>
			<p class="mb-4 text-xs text-muted">{spec.description}</p>
			{#if triggerFields.length}
				<div class="space-y-4">
					{#each triggerFields as f (f.key)}
						<FieldControl spec={f} obj={rule.trigger as unknown as Record<string, unknown>} />
					{/each}
				</div>
			{:else}
				<p class="text-xs text-faint">This trigger has no options to configure.</p>
			{/if}
		</section>

		<!-- actions -->
		<section class="px-4 py-4">
			<div class="mb-3 flex items-center justify-between gap-3">
				<div>
					<div class="eyebrow">Actions</div>
					<p class="mt-0.5 text-xs text-muted">Run top to bottom when this rule trips.</p>
				</div>
				<DropdownMenu.Root>
					<DropdownMenu.Trigger
						class="inline-flex items-center gap-1.5 rounded-lg border border-line-strong px-2.5 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-ink-2 data-[state=open]:bg-ink-2"
					>
						<Plus size={13} /> Add action
					</DropdownMenu.Trigger>
					<DropdownMenu.Portal>
						<DropdownMenu.Content
							align="end"
							sideOffset={6}
							class="menu-pop z-50 max-h-80 w-64 overflow-y-auto rounded-xl border border-line-strong bg-surface p-1.5 shadow-2xl outline-none"
						>
							{#each available as a (a.key)}
								{@const AIcon = iconFor(a.icon)}
								<DropdownMenu.Item
									onSelect={() => addAction(a.key)}
									class="flex cursor-pointer items-start gap-2.5 rounded-lg px-2 py-1.5 text-left outline-none transition-colors data-[highlighted]:bg-ink-2"
								>
									<AIcon size={15} class="mt-0.5 shrink-0 text-muted" />
									<span class="min-w-0">
										<span class="block text-[13px] font-medium text-ink">{a.label}</span>
										<span class="block text-[11px] leading-snug text-muted">{a.short}</span>
									</span>
								</DropdownMenu.Item>
							{/each}
						</DropdownMenu.Content>
					</DropdownMenu.Portal>
				</DropdownMenu.Root>
			</div>

			{#if rule.actions.length}
				<div class="space-y-2.5">
					{#each rule.actions as action, i (i)}
						{@const aspec = ACTIONS_BY_KEY[action.type]}
						{@const AIcon = iconFor(aspec?.icon ?? 'Square')}
						<div class="rounded-xl border border-line bg-ink-2/30 p-3">
							<div class="flex items-center gap-2.5">
								<span
									class="inline-flex items-center gap-1.5 rounded-md border px-2 py-1 text-xs font-medium {TONE_CHIP[
										aspec?.tone ?? 'neutral'
									]}"
								>
									<AIcon size={13} />
									{aspec?.label ?? action.type}
								</span>
								<div class="ml-auto flex items-center gap-0.5">
									<button
										type="button"
										onclick={() => moveAction(i, -1)}
										disabled={i === 0}
										class="grid size-7 place-items-center rounded-md text-faint transition-colors hover:bg-ink-2 hover:text-ink disabled:opacity-30"
										aria-label="Move up"
									>
										<ChevronUp size={14} />
									</button>
									<button
										type="button"
										onclick={() => moveAction(i, 1)}
										disabled={i === rule.actions.length - 1}
										class="grid size-7 place-items-center rounded-md text-faint transition-colors hover:bg-ink-2 hover:text-ink disabled:opacity-30"
										aria-label="Move down"
									>
										<ChevronDown size={14} />
									</button>
									<button
										type="button"
										onclick={() => removeAction(i)}
										class="grid size-7 place-items-center rounded-md text-faint transition-colors hover:bg-blush hover:text-accent-ink"
										aria-label="Remove action"
									>
										<Trash2 size={14} />
									</button>
								</div>
							</div>
							{#if aspec?.fields.length}
								<div class="mt-3 space-y-3">
									{#each aspec.fields as f (f.key)}
										<FieldControl spec={f} obj={action as unknown as Record<string, unknown>} />
									{/each}
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{:else}
				<div class="rounded-xl border border-dashed border-line px-4 py-6 text-center">
					<p class="text-xs text-muted">No actions yet. Add at least one so the rule does something.</p>
				</div>
			{/if}
		</section>

		<!-- exemptions -->
		<section class="px-4 py-4">
			<div class="eyebrow mb-1">Exemptions</div>
			<p class="mb-4 text-xs text-muted">
				Members with these roles, or messages in these channels, skip this rule.
			</p>
			<div class="grid gap-4 sm:grid-cols-2">
				<div>
					<span class="label">Exempt roles</span>
					<RolePicker
						multiple
						value={rule.exempt?.roles ?? []}
						onChange={(v) => {
							ensureExempt();
							rule.exempt.roles = v as string[];
						}}
						placeholder="Add a role…"
					/>
				</div>
				<div>
					<span class="label">Exempt channels</span>
					<ChannelPicker
						multiple
						value={rule.exempt?.channels ?? []}
						onChange={(v) => {
							ensureExempt();
							rule.exempt.channels = v as string[];
						}}
						placeholder="Add a channel…"
					/>
				</div>
			</div>
		</section>
	</div>
</div>
