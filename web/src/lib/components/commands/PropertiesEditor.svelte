<script lang="ts">
	// Builder for a command's slash properties (Discord "options"). Mirrors the
	// platform rules so the registration call can't fail: lowercase names ≤32,
	// description ≤100 (required by Discord), max 25 properties, required ones
	// listed first, choices (≤25) mutually exclusive with autocomplete, numeric
	// bounds on int/number, length bounds on text, channel-type filters on
	// channel pickers.
	import type { CommandOption } from '$lib/commands/types';
	import {
		SLASH_OPTION_KINDS,
		SLASH_OPTION_KIND_BY_ID,
		SLASH_CHANNEL_TYPES
	} from '$lib/commands/types';
	import { iconFor } from '$lib/commands/icons';
	import Toggle from '$lib/components/Toggle.svelte';
	import FieldSelect from './FieldSelect.svelte';
	import NumberField from './NumberField.svelte';
	import { slide } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';

	import Plus from 'lucide-svelte/icons/plus';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import ChevronDown from 'lucide-svelte/icons/chevron-down';
	import ArrowUp from 'lucide-svelte/icons/arrow-up';
	import ArrowDown from 'lucide-svelte/icons/arrow-down';
	import Copy from 'lucide-svelte/icons/copy';
	import Check from 'lucide-svelte/icons/check';
	import CircleAlert from 'lucide-svelte/icons/circle-alert';
	import ArrowUpDown from 'lucide-svelte/icons/arrow-up-down';

	let {
		options,
		onChange
	}: {
		options: CommandOption[];
		onChange: (next: CommandOption[]) => void;
	} = $props();

	let expanded = $state<number | null>(null);

	// "Use it" chip — copy the Go-template token that reads this property.
	let copiedIdx = $state<number | null>(null);
	let copiedTimer: ReturnType<typeof setTimeout> | null = null;
	function copyToken(i: number) {
		const name = options[i]?.name || 'name';
		void navigator.clipboard?.writeText(`{{ .Input.${name} }}`);
		copiedIdx = i;
		if (copiedTimer) clearTimeout(copiedTimer);
		copiedTimer = setTimeout(() => (copiedIdx = null), 1400);
	}

	const NAME_RE = /^[a-z0-9_-]{1,32}$/;
	const MAX_OPTIONS = 25;
	const MAX_CHOICES = 25;

	function commit(next: CommandOption[]) {
		onChange(next);
	}

	function patch(i: number, p: Partial<CommandOption>) {
		commit(options.map((o, idx) => (idx === i ? { ...o, ...p } : o)));
	}

	function add() {
		if (options.length >= MAX_OPTIONS) return;
		commit([
			...options,
			{ kind: 'string', name: '', description: '', required: options.length === 0 }
		]);
		expanded = options.length;
	}

	function remove(i: number) {
		commit(options.filter((_, idx) => idx !== i));
		if (expanded === i) expanded = null;
		else if (expanded !== null && expanded > i) expanded -= 1;
	}

	function move(i: number, dir: -1 | 1) {
		const j = i + dir;
		if (j < 0 || j >= options.length) return;
		const next = options.slice();
		[next[i], next[j]] = [next[j], next[i]];
		commit(next);
		if (expanded === i) expanded = j;
		else if (expanded === j) expanded = i;
	}

	// Switching the type drops config the new type can't carry (the backend
	// validator rejects e.g. channel_types on a text property).
	function setKind(i: number, kind: string) {
		const meta = SLASH_OPTION_KIND_BY_ID.get(kind);
		const o = options[i];
		const next: CommandOption = {
			kind,
			name: o.name,
			description: o.description,
			required: o.required
		};
		if (meta?.supportsValueBounds) {
			next.min_value = o.min_value;
			next.max_value = o.max_value;
		}
		if (meta?.supportsLength) {
			next.min_length = o.min_length;
			next.max_length = o.max_length;
		}
		if (meta?.supportsChannelTypes) next.channel_types = o.channel_types;
		if (meta?.supportsChoices) next.choices = o.choices;
		if (meta?.supportsAutocomplete && !(next.choices?.length ?? 0)) {
			next.autocomplete = o.autocomplete;
		}
		commit(options.map((opt, idx) => (idx === i ? next : opt)));
	}

	// ── choices ──────────────────────────────────────────────────────────────
	function choiceValue(kind: string, raw: string): unknown {
		if (kind === 'int') {
			const n = parseInt(raw, 10);
			return Number.isNaN(n) ? 0 : n;
		}
		if (kind === 'number') {
			const n = parseFloat(raw);
			return Number.isNaN(n) ? 0 : n;
		}
		return raw;
	}

	function addChoice(i: number) {
		const o = options[i];
		if ((o.choices?.length ?? 0) >= MAX_CHOICES) return;
		patch(i, {
			choices: [...(o.choices ?? []), { name: '', value: choiceValue(o.kind, '') }],
			// Choices and autocomplete are mutually exclusive on Discord.
			autocomplete: undefined
		});
	}

	function patchChoice(i: number, ci: number, p: { name?: string; value?: unknown }) {
		const o = options[i];
		patch(i, {
			choices: (o.choices ?? []).map((c, idx) => (idx === ci ? { ...c, ...p } : c))
		});
	}

	function removeChoice(i: number, ci: number) {
		const o = options[i];
		const next = (o.choices ?? []).filter((_, idx) => idx !== ci);
		patch(i, { choices: next.length ? next : undefined });
	}

	function toggleChannelType(i: number, id: number) {
		const o = options[i];
		const cur = o.channel_types ?? [];
		const next = cur.includes(id) ? cur.filter((t) => t !== id) : [...cur, id];
		patch(i, { channel_types: next.length ? next : undefined });
	}

	function numOrUndef(raw: string): number | undefined {
		if (raw.trim() === '') return undefined;
		const n = Number(raw);
		return Number.isNaN(n) ? undefined : n;
	}

	// ── validation ───────────────────────────────────────────────────────────
	function problemsFor(o: CommandOption, i: number): string[] {
		const out: string[] = [];
		if (!NAME_RE.test(o.name)) out.push('name: a–z 0–9 _ - · 1–32 chars');
		else if (options.some((p, pi) => pi !== i && p.name === o.name))
			out.push('name already used');
		if (!o.description.trim()) out.push('description is required by Discord');
		else if (o.description.length > 100) out.push('description over 100 chars');
		if (
			o.min_value !== undefined &&
			o.max_value !== undefined &&
			o.min_value > o.max_value
		)
			out.push('min value above max');
		if (
			o.min_length !== undefined &&
			o.max_length !== undefined &&
			o.min_length > o.max_length
		)
			out.push('min length above max');
		if ((o.choices ?? []).some((c) => !c.name.trim())) out.push('every choice needs a name');
		return out;
	}

	const problems = $derived(options.map((o, i) => problemsFor(o, i)));

	// Discord requires every required property to come before the optionals.
	const orderBroken = $derived(
		options.some((o, i) => o.required && options.slice(0, i).some((p) => !p.required))
	);

	function fixOrder() {
		commit([...options].sort((a, b) => Number(!!b.required) - Number(!!a.required)));
		expanded = null;
	}

	export function isValid(): boolean {
		return problems.every((p) => p.length === 0);
	}
</script>

<div class="space-y-2">
	{#if orderBroken}
		<button
			type="button"
			onclick={fixOrder}
			class="flex w-full items-center gap-2 rounded-lg border border-line bg-surface/40 px-3 py-2 text-left transition-colors hover:border-line-strong"
		>
			<ArrowUpDown size={12} class="shrink-0 text-muted" />
			<span class="min-w-0 flex-1 text-[11.5px] text-muted">
				Discord lists required properties before optional ones — yours are out of order.
			</span>
			<span class="shrink-0 font-mono text-[10.5px] font-medium text-ink">Fix order</span>
		</button>
	{/if}

	{#each options as o, i (i)}
		{@const meta = SLASH_OPTION_KIND_BY_ID.get(o.kind)}
		{@const Icon = iconFor(meta?.icon ?? 'Square')}
		{@const probs = problems[i]}
		{@const open = expanded === i}
		<div
			class="group/prop overflow-hidden rounded-lg border bg-surface/40 transition-colors {open
				? 'border-line-strong'
				: probs.length > 0
					? 'border-danger/30'
					: 'border-line hover:border-line-strong/70'}"
		>
			<!-- Header row: chip · name · type · required · actions -->
			<div class="flex h-10 items-center gap-2 px-2.5">
				<button
					type="button"
					class="grid size-6 shrink-0 place-items-center rounded-md border border-line bg-ink-2 text-muted"
					onclick={() => (expanded = open ? null : i)}
					title={meta?.label ?? o.kind}
					aria-label="Toggle property"
				>
					<Icon size={12} />
				</button>
				<input
					class="h-6 min-w-0 flex-1 rounded border border-transparent bg-transparent px-1 font-mono text-[12px] text-ink placeholder:text-faint focus:border-line focus:bg-bg focus:outline-none"
					placeholder="property-name"
					maxlength="32"
					value={o.name}
					oninput={(e) => patch(i, { name: (e.currentTarget as HTMLInputElement).value })}
				/>
				<FieldSelect
					class="h-6 w-28 shrink-0 text-[11px]"
					value={o.kind}
					onChange={(v) => setKind(i, v)}
					options={SLASH_OPTION_KINDS.map((k) => ({ value: k.id, label: k.label }))}
				/>
				<button
					type="button"
					class="hidden shrink-0 items-center gap-1.5 rounded border px-1.5 py-0.5 font-mono text-[9.5px] font-medium uppercase tracking-[0.12em] transition-colors sm:inline-flex {o.required
						? 'border-line-strong bg-bg text-ink'
						: 'border-line text-faint hover:text-muted'}"
					onclick={() => patch(i, { required: !o.required })}
					title={o.required ? 'Members must fill this in' : 'Offered in the optional picker'}
				>
					<span
						class="size-1 rounded-full {o.required ? 'bg-success' : 'bg-faint/50'}"
					></span>
					{o.required ? 'required' : 'optional'}
				</button>

				{#if probs.length > 0}
					<span class="shrink-0 text-danger" title={probs.join(' · ')}>
						<CircleAlert size={12} />
					</span>
				{/if}

				<div class="ml-auto flex shrink-0 items-center gap-0.5">
					<button
						type="button"
						class="grid size-6 place-items-center rounded text-faint opacity-0 transition-[color,background,opacity] hover:bg-bg hover:text-ink disabled:opacity-30 group-hover/prop:opacity-100"
						disabled={i === 0}
						onclick={() => move(i, -1)}
						aria-label="Move up"
					>
						<ArrowUp size={11} />
					</button>
					<button
						type="button"
						class="grid size-6 place-items-center rounded text-faint opacity-0 transition-[color,background,opacity] hover:bg-bg hover:text-ink disabled:opacity-30 group-hover/prop:opacity-100"
						disabled={i === options.length - 1}
						onclick={() => move(i, 1)}
						aria-label="Move down"
					>
						<ArrowDown size={11} />
					</button>
					<button
						type="button"
						class="grid size-6 place-items-center rounded text-faint opacity-0 transition-[color,background,opacity] hover:bg-bg hover:text-danger group-hover/prop:opacity-100"
						onclick={() => remove(i)}
						aria-label="Remove property"
					>
						<Trash2 size={11} />
					</button>
					<button
						type="button"
						class="grid size-6 place-items-center rounded text-faint transition-colors hover:bg-bg hover:text-ink"
						onclick={() => (expanded = open ? null : i)}
						aria-label={open ? 'Collapse' : 'Expand'}
					>
						<ChevronDown
							size={12}
							class="transition-transform {open ? 'rotate-180' : ''}"
						/>
					</button>
				</div>
			</div>

			{#if !open}
				<!-- Collapsed: one quiet description line -->
				{#if o.description}
					<div class="border-t border-line/40 px-3 py-1.5">
						<p class="truncate text-[11.5px] text-muted">{o.description}</p>
					</div>
				{/if}
			{:else}
				<div
					transition:slide={{ duration: dur(200), easing: cubicOut }}
					class="space-y-3 border-t border-line/60 px-3 py-3"
				>
					<!-- Description -->
					<div>
						<div class="mb-1 flex items-baseline justify-between">
							<span
								class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
							>
								Description
							</span>
							<span class="font-mono text-[10px] tabular-nums text-faint">
								{o.description.length}/100
							</span>
						</div>
						<input
							class="h-7 w-full rounded-md border border-line bg-bg px-2 text-[12px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
							placeholder="Shown under the property while typing in Discord"
							maxlength="100"
							value={o.description}
							oninput={(e) =>
								patch(i, { description: (e.currentTarget as HTMLInputElement).value })}
						/>
					</div>

					<!-- How to read what the member typed — copy-paste into any step -->
					<div
						class="flex items-center gap-2 overflow-hidden rounded-md border border-line bg-ink-2/60 px-2.5 py-1.5"
					>
						<span
							class="shrink-0 font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-faint"
						>
							Use it
						</span>
						<code class="min-w-0 flex-1 truncate font-mono text-[11px] text-ink">
							{`{{ .Input.${o.name || 'name'} }}`}
						</code>
						<button
							type="button"
							class="inline-flex h-5 shrink-0 items-center gap-1 rounded border border-line px-1.5 font-mono text-[9.5px] uppercase tracking-[0.08em] transition-colors {copiedIdx === i
								? 'border-success/40 text-success'
								: 'text-muted hover:border-line-strong hover:text-ink'}"
							onclick={() => copyToken(i)}
						>
							{#if copiedIdx === i}
								<Check size={9} />
								copied
							{:else}
								<Copy size={9} />
								copy
							{/if}
						</button>
					</div>

					<!-- Required (visible on mobile where the header pill is hidden) -->
					<label class="flex items-center justify-between gap-3 sm:hidden">
						<span class="text-[12px] text-muted">Required</span>
						<Toggle checked={!!o.required} onchange={(v) => patch(i, { required: v })} />
					</label>

					<!-- Numeric bounds -->
					{#if meta?.supportsValueBounds}
						<div class="grid grid-cols-2 gap-2">
							<div>
								<span
									class="mb-1 block font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
								>
									Min value
								</span>
								<NumberField
									placeholder="none"
									value={o.min_value}
									onChange={(n) => patch(i, { min_value: n })}
								/>
							</div>
							<div>
								<span
									class="mb-1 block font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
								>
									Max value
								</span>
								<NumberField
									placeholder="none"
									value={o.max_value}
									onChange={(n) => patch(i, { max_value: n })}
								/>
							</div>
						</div>
					{/if}

					<!-- Text length bounds -->
					{#if meta?.supportsLength}
						<div class="grid grid-cols-2 gap-2">
							<div>
								<span
									class="mb-1 block font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
								>
									Min length
								</span>
								<NumberField
									min={0}
									max={6000}
									placeholder="0"
									value={o.min_length}
									onChange={(n) => patch(i, { min_length: n })}
								/>
							</div>
							<div>
								<span
									class="mb-1 block font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
								>
									Max length
								</span>
								<NumberField
									min={1}
									max={6000}
									placeholder="6000"
									value={o.max_length}
									onChange={(n) => patch(i, { max_length: n })}
								/>
							</div>
						</div>
					{/if}

					<!-- Channel type filter -->
					{#if meta?.supportsChannelTypes}
						<div>
							<span
								class="mb-1 block font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
							>
								Allowed channel types
							</span>
							<div class="flex flex-wrap gap-1">
								{#each SLASH_CHANNEL_TYPES as ct (ct.id)}
									{@const on = (o.channel_types ?? []).includes(ct.id)}
									<button
										type="button"
										class="inline-flex h-6 items-center rounded border px-1.5 font-mono text-[10.5px] transition-colors {on
											? 'border-line-strong bg-bg text-ink'
											: 'border-line text-faint hover:text-muted'}"
										onclick={() => toggleChannelType(i, ct.id)}
									>
										{ct.label}
									</button>
								{/each}
							</div>
							<p class="mt-1 font-mono text-[10px] text-faint">
								none selected = every channel type allowed
							</p>
						</div>
					{/if}

					<!-- Fixed choices -->
					{#if meta?.supportsChoices}
						<div>
							<div class="mb-1 flex items-center justify-between">
								<span
									class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
								>
									Choices
								</span>
								<span class="font-mono text-[10px] tabular-nums text-faint">
									{o.choices?.length ?? 0}/25
								</span>
							</div>
							{#if (o.choices?.length ?? 0) > 0}
								<div class="mb-1.5 space-y-1">
									{#each o.choices ?? [] as c, ci (ci)}
										<div class="flex items-center gap-1.5">
											<input
												class="h-7 min-w-0 flex-1 rounded-md border border-line bg-bg px-2 text-[12px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
												placeholder="Label members see"
												maxlength="100"
												value={c.name}
												oninput={(e) =>
													patchChoice(i, ci, {
														name: (e.currentTarget as HTMLInputElement).value
													})}
											/>
											<input
												class="h-7 w-28 rounded-md border border-line bg-bg px-2 font-mono text-[11px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none sm:w-36"
												placeholder="value"
												value={String(c.value ?? '')}
												oninput={(e) =>
													patchChoice(i, ci, {
														value: choiceValue(
															o.kind,
															(e.currentTarget as HTMLInputElement).value
														)
													})}
											/>
											<button
												type="button"
												class="grid size-6 shrink-0 place-items-center rounded text-faint transition-colors hover:bg-bg hover:text-danger"
												onclick={() => removeChoice(i, ci)}
												aria-label="Remove choice"
											>
												<Trash2 size={11} />
											</button>
										</div>
									{/each}
								</div>
							{/if}
							<button
								type="button"
								class="inline-flex h-6 items-center gap-1 rounded border border-dashed border-line bg-bg px-1.5 text-[11px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink disabled:opacity-40"
								onclick={() => addChoice(i)}
								disabled={(o.choices?.length ?? 0) >= MAX_CHOICES}
							>
								<Plus size={10} />
								Add choice
							</button>
							<p class="mt-1 font-mono text-[10px] text-faint">
								with choices, members must pick one — free input is off
							</p>
						</div>
					{/if}

					<!-- Autocomplete -->
					{#if meta?.supportsAutocomplete}
						{@const hasChoices = (o.choices?.length ?? 0) > 0}
						<label class="flex items-center justify-between gap-3">
							<span class="min-w-0">
								<span class="block text-[12px] text-ink">Autocomplete</span>
								<span class="block text-[10.5px] text-faint">
									{hasChoices
										? 'unavailable while choices are set'
										: 'the bot suggests values live as members type'}
								</span>
							</span>
							<Toggle
								checked={!!o.autocomplete}
								disabled={hasChoices}
								onchange={(v) => patch(i, { autocomplete: v || undefined })}
							/>
						</label>
					{/if}

					{#if probs.length > 0}
						<ul class="space-y-0.5">
							{#each probs as p (p)}
								<li class="font-mono text-[10.5px] text-danger">· {p}</li>
							{/each}
						</ul>
					{/if}
				</div>
			{/if}
		</div>
	{/each}

	<!-- Add affordance (dashed, full width) + budget -->
	<div class="flex items-center gap-2">
		<button
			type="button"
			class="inline-flex h-8 flex-1 items-center justify-center gap-1.5 rounded-lg border border-dashed border-line bg-transparent text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:bg-surface/40 hover:text-ink disabled:opacity-40"
			onclick={add}
			disabled={options.length >= MAX_OPTIONS}
		>
			<Plus size={12} />
			Add property
		</button>
		<span class="shrink-0 font-mono text-[10px] tabular-nums text-faint">
			{options.length}/{MAX_OPTIONS}
		</span>
	</div>
</div>
