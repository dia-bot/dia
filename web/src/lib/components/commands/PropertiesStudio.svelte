<script lang="ts">
	// The properties studio: every slash property visible at once. Three zones:
	// a rail listing all properties (click to edit, hover to reorder), a faithful
	// Discord preview of the member filling the command in (the selected property
	// is highlighted exactly where Discord highlights it), and a full editor pane
	// with nothing folded away. New properties start from a type picker so the
	// shape is right before anything else is typed. Mirrors the platform rules:
	// lowercase names ≤32, description ≤100, max 25 properties, required first,
	// choices (≤25) mutually exclusive with autocomplete, numeric bounds on
	// int/number, length bounds on text, channel-type filters on channel pickers.
	import type { CommandOption } from '$lib/commands/types';
	import {
		SLASH_OPTION_KINDS,
		SLASH_OPTION_KIND_BY_ID,
		SLASH_CHANNEL_TYPES
	} from '$lib/commands/types';
	import { iconFor } from '$lib/commands/icons';
	import Toggle from '$lib/components/Toggle.svelte';
	import NumberField from './NumberField.svelte';
	import Logo from '$lib/components/Logo.svelte';

	import Plus from 'lucide-svelte/icons/plus';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import ArrowUp from 'lucide-svelte/icons/arrow-up';
	import ArrowDown from 'lucide-svelte/icons/arrow-down';
	import ArrowUpDown from 'lucide-svelte/icons/arrow-up-down';
	import ArrowLeft from 'lucide-svelte/icons/arrow-left';
	import Copy from 'lucide-svelte/icons/copy';
	import Check from 'lucide-svelte/icons/check';
	import CircleAlert from 'lucide-svelte/icons/circle-alert';

	let {
		name = '',
		description = '',
		options,
		onChange
	}: {
		name?: string;
		description?: string;
		options: CommandOption[];
		onChange: (next: CommandOption[]) => void;
	} = $props();

	const NAME_RE = /^[a-z0-9_-]{1,32}$/;
	const MAX_OPTIONS = 25;
	const MAX_CHOICES = 25;

	let selected = $state(0);
	// Type picker pane (always shown while there are no properties).
	let adding = $state(false);
	let nameEl = $state<HTMLInputElement | null>(null);
	let pendingFocus = $state(false);

	const sel = $derived(selected >= 0 && selected < options.length ? options[selected] : null);
	const selMeta = $derived(sel ? SLASH_OPTION_KIND_BY_ID.get(sel.kind) : undefined);
	const shownName = $derived(name || 'command');

	// Keep the selection in range when properties are removed underneath it.
	$effect(() => {
		if (selected >= options.length) selected = options.length - 1;
		if (selected < 0 && options.length > 0) selected = 0;
	});
	$effect(() => {
		if (pendingFocus && nameEl) {
			nameEl.focus();
			pendingFocus = false;
		}
	});

	function select(i: number) {
		selected = i;
		adding = false;
	}

	// Keeps the active row visible in the rail and the preview panel, whichever
	// of the two the selection came from.
	function keepInView(el: HTMLElement, active: boolean) {
		const apply = (a: boolean) => {
			if (a) el.scrollIntoView({ block: 'nearest', inline: 'nearest' });
		};
		apply(active);
		return { update: apply };
	}

	function commit(next: CommandOption[]) {
		onChange(next);
	}

	function patch(i: number, p: Partial<CommandOption>) {
		commit(options.map((o, idx) => (idx === i ? { ...o, ...p } : o)));
	}

	function addKind(kind: string) {
		if (options.length >= MAX_OPTIONS) return;
		commit([...options, { kind, name: '', description: '', required: options.length === 0 }]);
		selected = options.length;
		adding = false;
		pendingFocus = true;
	}

	function remove(i: number) {
		commit(options.filter((_, idx) => idx !== i));
		if (selected > i || selected >= options.length - 1) selected = Math.max(0, selected - 1);
	}

	function move(i: number, dir: -1 | 1) {
		const j = i + dir;
		if (j < 0 || j >= options.length) return;
		const next = options.slice();
		[next[i], next[j]] = [next[j], next[i]];
		commit(next);
		if (selected === i) selected = j;
		else if (selected === j) selected = i;
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

	// Discord wants lowercase names; smooth the obvious slips while typing.
	function normaliseName(raw: string): string {
		return raw.toLowerCase().replace(/\s+/g, '-');
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

	// ── validation ───────────────────────────────────────────────────────────
	function problemsFor(o: CommandOption, i: number): string[] {
		const out: string[] = [];
		if (!NAME_RE.test(o.name)) out.push('name: a–z 0–9 _ - · 1–32 chars');
		else if (options.some((p, pi) => pi !== i && p.name === o.name)) out.push('name already used');
		if (!o.description.trim()) out.push('description is required by Discord');
		else if (o.description.length > 100) out.push('description over 100 chars');
		if (o.min_value !== undefined && o.max_value !== undefined && o.min_value > o.max_value)
			out.push('min value above max');
		if (o.min_length !== undefined && o.max_length !== undefined && o.min_length > o.max_length)
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
		const id = sel;
		const next = [...options].sort((a, b) => Number(!!b.required) - Number(!!a.required));
		commit(next);
		if (id) selected = next.indexOf(id);
	}

	export function isValid(): boolean {
		return problems.every((p) => p.length === 0);
	}

	// ── preview ──────────────────────────────────────────────────────────────
	// Discord lists required properties before optional ones; keep the original
	// index along so clicks land on the right rail entry.
	const ordered = $derived(
		options.map((o, i) => ({ o, i })).sort((a, b) => Number(!!b.o.required) - Number(!!a.o.required))
	);

	// What Discord shows under the focused property (bounds, pickers, hints).
	function kindHint(o: CommandOption): string {
		switch (o.kind) {
			case 'string': {
				if (o.autocomplete) return 'Suggestions appear here as the member types';
				if (o.min_length !== undefined || o.max_length !== undefined)
					return `Free text, ${o.min_length ?? 0}–${o.max_length ?? 6000} characters`;
				return 'Free text, the member types anything';
			}
			case 'int':
			case 'number': {
				if (o.autocomplete) return 'Suggestions appear here as the member types';
				const noun = o.kind === 'int' ? 'A whole number' : 'A decimal number';
				if (o.min_value !== undefined && o.max_value !== undefined)
					return `${noun} between ${o.min_value} and ${o.max_value}`;
				if (o.min_value !== undefined) return `${noun} of at least ${o.min_value}`;
				if (o.max_value !== undefined) return `${noun} up to ${o.max_value}`;
				return noun;
			}
			case 'user':
				return 'Discord opens the member picker';
			case 'role':
				return 'Discord opens the role picker';
			case 'mentionable':
				return 'Discord offers members and roles';
			case 'channel': {
				const types = (o.channel_types ?? [])
					.map((id) => SLASH_CHANNEL_TYPES.find((t) => t.id === id)?.label)
					.filter(Boolean);
				return types.length
					? `Channel picker, limited to: ${types.join(', ')}`
					: 'Discord opens the channel picker';
			}
			case 'attachment':
				return 'The member uploads a file';
			default:
				return '';
		}
	}

	// ── "Use it" chip: copy the token that reads the selected property ───────
	let copied = $state(false);
	let copiedTimer: ReturnType<typeof setTimeout> | null = null;
	function copyToken() {
		void navigator.clipboard?.writeText(`{{ .Input.${sel?.name || 'name'} }}`);
		copied = true;
		if (copiedTimer) clearTimeout(copiedTimer);
		copiedTimer = setTimeout(() => (copied = false), 1400);
	}
</script>

<div class="@container flex h-full min-h-0 flex-col">
	<div class="flex min-h-0 flex-1 flex-col @3xl:flex-row">
		<!-- ── Rail: every property, always visible ── -->
		<div
			class="flex shrink-0 flex-row gap-1.5 overflow-x-auto border-b border-line p-2.5 @3xl:w-[252px] @3xl:flex-col @3xl:overflow-y-auto @3xl:overflow-x-hidden @3xl:border-b-0 @3xl:border-r"
		>
			<div class="hidden items-baseline justify-between px-1 pb-1 @3xl:flex">
				<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
					Properties
				</span>
				<span class="font-mono text-[10px] tabular-nums text-faint">
					{options.length}/{MAX_OPTIONS}
				</span>
			</div>

			{#each options as o, i (i)}
				{@const meta = SLASH_OPTION_KIND_BY_ID.get(o.kind)}
				{@const Icon = iconFor(meta?.icon ?? 'Square')}
				{@const probs = problems[i]}
				{@const active = selected === i && !adding}
				<div class="group/row relative w-48 shrink-0 @3xl:w-full" use:keepInView={active}>
					<button
						type="button"
						class="flex w-full items-center gap-2 rounded-lg border px-2 py-1.5 text-left transition-colors {active
							? 'border-line-strong bg-surface'
							: probs.length > 0
								? 'border-danger/30 hover:border-danger/50'
								: 'border-line hover:border-line-strong/70'}"
						onclick={() => select(i)}
					>
						<span class="w-3.5 shrink-0 text-right font-mono text-[10px] tabular-nums text-faint">
							{i + 1}
						</span>
						<span
							class="grid size-6 shrink-0 place-items-center rounded-md border border-line bg-ink-2 {active
								? 'text-ink'
								: 'text-muted'}"
						>
							<Icon size={12} />
						</span>
						<span class="min-w-0 flex-1">
							<span class="flex items-center gap-1.5">
								<span
									class="truncate font-mono text-[11.5px] {o.name ? 'text-ink' : 'italic text-faint'}"
								>
									{o.name || 'unnamed'}
								</span>
								{#if probs.length > 0}
									<CircleAlert size={10} class="shrink-0 text-danger" />
								{/if}
							</span>
							<span class="block truncate text-[10px] text-faint">
								{meta?.label ?? o.kind}{o.required ? ' · required' : ''}{(o.choices?.length ?? 0) > 0
									? ` · ${o.choices?.length} choices`
									: ''}
							</span>
						</span>
					</button>
					<!-- Reorder, revealed on hover (wide layout only) -->
					<div
						class="absolute right-1.5 top-1/2 hidden -translate-y-1/2 items-center gap-0.5 rounded-md border border-line bg-surface px-0.5 py-0.5 @3xl:group-hover/row:flex"
					>
						<button
							type="button"
							class="grid size-5 place-items-center rounded text-faint transition-colors hover:bg-bg hover:text-ink disabled:opacity-30"
							disabled={i === 0}
							onclick={(e) => {
								e.stopPropagation();
								move(i, -1);
							}}
							aria-label="Move up"
						>
							<ArrowUp size={10} />
						</button>
						<button
							type="button"
							class="grid size-5 place-items-center rounded text-faint transition-colors hover:bg-bg hover:text-ink disabled:opacity-30"
							disabled={i === options.length - 1}
							onclick={(e) => {
								e.stopPropagation();
								move(i, 1);
							}}
							aria-label="Move down"
						>
							<ArrowDown size={10} />
						</button>
					</div>
				</div>
			{/each}

			{#if orderBroken}
				<button
					type="button"
					onclick={fixOrder}
					class="flex w-48 shrink-0 items-center gap-2 rounded-lg border border-line bg-ink-2/60 px-2.5 py-1.5 text-left transition-colors hover:border-line-strong @3xl:w-full"
					title="Discord lists required properties before optional ones, yours are out of order."
				>
					<ArrowUpDown size={11} class="shrink-0 text-muted" />
					<span class="min-w-0 flex-1 truncate text-[10.5px] text-muted">required first</span>
					<span class="shrink-0 font-mono text-[9.5px] font-medium text-ink">Fix</span>
				</button>
			{/if}

			<button
				type="button"
				class="inline-flex h-9 w-40 shrink-0 items-center justify-center gap-1.5 rounded-lg border border-dashed text-[11.5px] font-medium transition-colors disabled:opacity-40 @3xl:w-full {adding
					? 'border-line-strong bg-surface/60 text-ink'
					: 'border-line text-muted hover:border-line-strong hover:bg-surface/40 hover:text-ink'}"
				onclick={() => (adding = true)}
				disabled={options.length >= MAX_OPTIONS}
			>
				<Plus size={11} />
				Add property
				<span class="font-mono text-[10px] tabular-nums text-faint @3xl:hidden">
					{options.length}/{MAX_OPTIONS}
				</span>
			</button>
		</div>

		<!-- ── Right column: Discord preview pinned, editor scrolls ── -->
		<div class="flex min-h-0 min-w-0 flex-1 flex-col">
			<!-- The command, exactly as a member fills it in -->
			<div class="shrink-0 border-b border-line bg-[#313338] px-4 pb-3.5 pt-3 @3xl:px-5">
				<div class="mb-2 flex items-baseline justify-between">
					<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-[#87898c]">
						What members see
					</span>
					<span class="font-mono text-[10px] text-[#87898c]">click a property to edit it</span>
				</div>

				<!-- The floating option panel Discord shows above the box -->
				<div
					class="overflow-hidden rounded-lg border border-[#26282c] bg-[#2b2d31] shadow-[0_8px_24px_rgba(0,0,0,0.35)]"
				>
					<div class="flex items-center gap-2 border-b border-[#1e1f22] px-3 py-2">
						<div class="grid size-6 shrink-0 place-items-center rounded-full bg-[#F1DFDF]">
							<Logo size={15} />
						</div>
						<span class="shrink-0 text-[13px] font-medium text-[#f2f3f5]">/{shownName}</span>
						<span class="min-w-0 truncate text-[11.5px] text-[#949ba4]">
							{description || 'Custom command'}
						</span>
					</div>

					{#if options.length === 0}
						<div class="px-3 py-2.5 text-[11.5px] italic text-[#87898c]">
							No properties yet, the command runs right away.
						</div>
					{:else}
						<div class="max-h-44 overflow-y-auto py-1 @max-3xl:max-h-28">
							{#each ordered as entry (entry.i)}
								{@const meta = SLASH_OPTION_KIND_BY_ID.get(entry.o.kind)}
								{@const Icon = iconFor(meta?.icon ?? 'Square')}
								{@const active = selected === entry.i && !adding}
								<button
									type="button"
									class="flex w-full items-center gap-2.5 px-3 py-1.5 text-left transition-colors {active
										? 'bg-[#393c41]'
										: 'hover:bg-[#34373c]'}"
									use:keepInView={active}
									onclick={() => select(entry.i)}
								>
									<Icon size={13} class="shrink-0 {active ? 'text-[#dbdee1]' : 'text-[#87898c]'}" />
									<span
										class="shrink-0 text-[12.5px] font-medium {active
											? 'text-white'
											: 'text-[#dbdee1]'}"
									>
										{entry.o.name || 'property'}
									</span>
									<span class="min-w-0 flex-1 truncate text-[11.5px] text-[#949ba4]">
										{entry.o.description || meta?.short || ''}
									</span>
									{#if entry.o.required}
										<span
											class="shrink-0 text-[8.5px] font-semibold uppercase tracking-[0.08em] text-[#87898c]"
										>
											required
										</span>
									{/if}
								</button>

								<!-- The focused property expands the way Discord previews it -->
								{#if active}
									{#if (entry.o.choices?.length ?? 0) > 0}
										<div
											class="mb-1 ml-9 mr-3 overflow-hidden rounded-md border border-[#1e1f22] bg-[#232428]"
										>
											{#each (entry.o.choices ?? []).slice(0, 5) as c, ci (ci)}
												<div
													class="flex items-center justify-between gap-3 px-2.5 py-1 text-[11.5px] {ci === 0
														? 'bg-[#34373c] text-[#f2f3f5]'
														: 'text-[#b5bac1]'}"
												>
													<span class="min-w-0 truncate">{c.name || 'choice'}</span>
													<span class="shrink-0 font-mono text-[10px] text-[#87898c]">
														{String(c.value ?? '')}
													</span>
												</div>
											{/each}
											{#if (entry.o.choices?.length ?? 0) > 5}
												<div class="px-2.5 py-1 text-[10.5px] text-[#87898c]">
													+{(entry.o.choices?.length ?? 0) - 5} more
												</div>
											{/if}
										</div>
									{:else if entry.o.kind === 'bool'}
										<div
											class="mb-1 ml-9 mr-3 overflow-hidden rounded-md border border-[#1e1f22] bg-[#232428]"
										>
											<div class="bg-[#34373c] px-2.5 py-1 text-[11.5px] text-[#f2f3f5]">True</div>
											<div class="px-2.5 py-1 text-[11.5px] text-[#b5bac1]">False</div>
										</div>
									{:else if kindHint(entry.o)}
										<div class="mb-1 ml-9 mr-3 px-0.5 pb-0.5 text-[10.5px] italic text-[#87898c]">
											{kindHint(entry.o)}
										</div>
									{/if}
								{/if}
							{/each}
						</div>
					{/if}
				</div>

				<!-- The message bar with the typed command + property pills -->
				<div
					class="mt-2 flex min-h-10 flex-wrap items-center gap-1.5 rounded-lg bg-[#383a40] px-3 py-2"
				>
					<span
						class="inline-flex h-6 shrink-0 items-center rounded bg-[#5865f2]/30 px-1.5 text-[12px] font-medium text-[#c9cdfb]"
					>
						/{shownName}
					</span>
					{#each ordered as entry (entry.i)}
						{@const active = selected === entry.i && !adding}
						<button
							type="button"
							class="inline-flex h-6 items-center rounded px-1.5 text-[11.5px] font-medium transition-colors {active
								? 'bg-[#5865f2] text-white'
								: 'bg-[#26282c] text-[#b5bac1] hover:bg-[#2f3136]'} {entry.o.required || active
								? ''
								: 'opacity-60'}"
							title={entry.o.required ? 'Required' : 'Optional, offered in a picker'}
							onclick={() => select(entry.i)}
						>
							{entry.o.name || 'property'}{entry.o.required ? '' : '?'}
						</button>
					{/each}
					{#if options.length === 0}
						<span class="text-[11.5px] italic text-[#87898c]">press Enter to run</span>
					{/if}
				</div>
			</div>

			<!-- Editor pane -->
			<div class="min-h-0 flex-1 overflow-y-auto px-4 py-4 @3xl:px-5">
				{#if adding || options.length === 0}
					<!-- Type picker: a property starts from what the member provides -->
					<div class="mx-auto w-full max-w-[560px]">
						<div class="mb-1 flex items-center gap-2">
							{#if options.length > 0}
								<button
									type="button"
									class="grid size-6 place-items-center rounded-md border border-line text-muted transition-colors hover:border-line-strong hover:text-ink"
									onclick={() => (adding = false)}
									aria-label="Back to the selected property"
								>
									<ArrowLeft size={12} />
								</button>
							{/if}
							<span class="text-[13px] font-semibold text-ink">
								{options.length === 0 ? 'Add your first property' : 'Add a property'}
							</span>
						</div>
						<p class="mb-3 text-[11.5px] leading-relaxed text-muted">
							Pick what the member provides. Discord renders a native field for it, and every step
							can read it as
							<code class="rounded bg-surface px-1 font-mono text-[10.5px] text-ink">
								{'{{ .Input.name }}'}
							</code>
						</p>
						<div class="grid grid-cols-2 gap-2 @xl:grid-cols-3">
							{#each SLASH_OPTION_KINDS as k (k.id)}
								{@const KIcon = iconFor(k.icon)}
								<button
									type="button"
									class="flex flex-col items-start gap-1 rounded-lg border border-line bg-surface/40 p-3 text-left transition-colors hover:border-line-strong hover:bg-surface"
									onclick={() => addKind(k.id)}
								>
									<span
										class="grid size-7 place-items-center rounded-md border border-line bg-ink-2 text-muted"
									>
										<KIcon size={13} />
									</span>
									<span class="text-[12px] font-medium text-ink">{k.label}</span>
									<span class="text-[10.5px] leading-snug text-faint">{k.short}</span>
								</button>
							{/each}
						</div>
					</div>
				{:else if sel}
					<div class="mx-auto w-full max-w-[560px] space-y-4">
						<!-- Name + required -->
						<div class="flex items-end gap-3">
							<div class="min-w-0 flex-1">
								<div class="mb-1 flex items-baseline justify-between">
									<span
										class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
									>
										Name
									</span>
									<span class="font-mono text-[10px] tabular-nums text-faint">
										{sel.name.length}/32
									</span>
								</div>
								<input
									bind:this={nameEl}
									class="h-8 w-full rounded-md border border-line bg-bg px-2.5 font-mono text-[12.5px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
									placeholder="property-name"
									maxlength="32"
									value={sel.name}
									oninput={(e) =>
										patch(selected, {
											name: normaliseName((e.currentTarget as HTMLInputElement).value)
										})}
								/>
							</div>
							<label
								class="flex h-8 shrink-0 cursor-pointer items-center gap-2 rounded-md border border-line bg-surface/40 px-2.5"
								title={sel.required
									? 'Members must fill this in'
									: 'Offered in the optional picker'}
							>
								<span class="text-[11.5px] text-muted">Required</span>
								<Toggle
									checked={!!sel.required}
									label="Required"
									onchange={(v) => patch(selected, { required: v })}
								/>
							</label>
						</div>

						<!-- Type -->
						<div>
							<span
								class="mb-1 block font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
							>
								Type
							</span>
							<div class="grid grid-cols-3 gap-1.5">
								{#each SLASH_OPTION_KINDS as k (k.id)}
									{@const KIcon = iconFor(k.icon)}
									{@const on = sel.kind === k.id}
									<button
										type="button"
										class="flex items-center gap-1.5 rounded-md border px-2 py-1.5 text-[11px] transition-colors {on
											? 'border-line-strong bg-surface text-ink'
											: 'border-line text-muted hover:border-line-strong/70 hover:text-ink'}"
										title={k.short}
										onclick={() => setKind(selected, k.id)}
									>
										<KIcon size={11} class="shrink-0 {on ? 'text-ink' : 'text-faint'}" />
										<span class="truncate">{k.label}</span>
									</button>
								{/each}
							</div>
							<p class="mt-1 font-mono text-[10px] text-faint">{selMeta?.short ?? ''}</p>
						</div>

						<!-- Description -->
						<div>
							<div class="mb-1 flex items-baseline justify-between">
								<span
									class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
								>
									Description
								</span>
								<span class="font-mono text-[10px] tabular-nums text-faint">
									{sel.description.length}/100
								</span>
							</div>
							<input
								class="h-8 w-full rounded-md border border-line bg-bg px-2.5 text-[12.5px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
								placeholder="Shown under the property while typing in Discord"
								maxlength="100"
								value={sel.description}
								oninput={(e) =>
									patch(selected, { description: (e.currentTarget as HTMLInputElement).value })}
							/>
						</div>

						<!-- How to read what the member typed: copy-paste into any step -->
						<div
							class="flex items-center gap-2 overflow-hidden rounded-md border border-line bg-ink-2/60 px-2.5 py-1.5"
						>
							<span
								class="shrink-0 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
							>
								Use it
							</span>
							<code class="min-w-0 flex-1 truncate font-mono text-[11px] text-ink">
								{`{{ .Input.${sel.name || 'name'} }}`}
							</code>
							<button
								type="button"
								class="inline-flex h-5 shrink-0 items-center gap-1 rounded border border-line px-1.5 font-mono text-[9.5px] uppercase tracking-[0.08em] transition-colors {copied
									? 'border-success/40 text-success'
									: 'text-muted hover:border-line-strong hover:text-ink'}"
								onclick={copyToken}
							>
								{#if copied}
									<Check size={9} />
									copied
								{:else}
									<Copy size={9} />
									copy
								{/if}
							</button>
						</div>

						<!-- Numeric bounds -->
						{#if selMeta?.supportsValueBounds}
							<div class="grid grid-cols-2 gap-2">
								<div>
									<span
										class="mb-1 block font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
									>
										Min value
									</span>
									<NumberField
										placeholder="none"
										value={sel.min_value}
										onChange={(n) => patch(selected, { min_value: n })}
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
										value={sel.max_value}
										onChange={(n) => patch(selected, { max_value: n })}
									/>
								</div>
							</div>
						{/if}

						<!-- Text length bounds -->
						{#if selMeta?.supportsLength}
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
										value={sel.min_length}
										onChange={(n) => patch(selected, { min_length: n })}
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
										value={sel.max_length}
										onChange={(n) => patch(selected, { max_length: n })}
									/>
								</div>
							</div>
						{/if}

						<!-- Channel type filter -->
						{#if selMeta?.supportsChannelTypes}
							<div>
								<span
									class="mb-1 block font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
								>
									Allowed channel types
								</span>
								<div class="flex flex-wrap gap-1">
									{#each SLASH_CHANNEL_TYPES as ct (ct.id)}
										{@const on = (sel.channel_types ?? []).includes(ct.id)}
										<button
											type="button"
											class="inline-flex h-6 items-center rounded border px-1.5 font-mono text-[10.5px] transition-colors {on
												? 'border-line-strong bg-bg text-ink'
												: 'border-line text-faint hover:text-muted'}"
											onclick={() => toggleChannelType(selected, ct.id)}
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
						{#if selMeta?.supportsChoices}
							<div>
								<div class="mb-1 flex items-center justify-between">
									<span
										class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
									>
										Choices
									</span>
									<span class="font-mono text-[10px] tabular-nums text-faint">
										{sel.choices?.length ?? 0}/{MAX_CHOICES}
									</span>
								</div>
								{#if (sel.choices?.length ?? 0) > 0}
									<div class="mb-1.5 space-y-1">
										{#each sel.choices ?? [] as c, ci (ci)}
											<div class="flex items-center gap-1.5">
												<input
													class="h-7 min-w-0 flex-1 rounded-md border border-line bg-bg px-2 text-[12px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
													placeholder="Label members see"
													maxlength="100"
													value={c.name}
													oninput={(e) =>
														patchChoice(selected, ci, {
															name: (e.currentTarget as HTMLInputElement).value
														})}
												/>
												<input
													class="h-7 w-28 rounded-md border border-line bg-bg px-2 font-mono text-[11px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none @xl:w-36"
													placeholder="value"
													value={String(c.value ?? '')}
													oninput={(e) =>
														patchChoice(selected, ci, {
															value: choiceValue(
																sel.kind,
																(e.currentTarget as HTMLInputElement).value
															)
														})}
													onblur={(e) => {
														const el = e.currentTarget as HTMLInputElement;
														el.value = String(sel?.choices?.[ci]?.value ?? '');
													}}
												/>
												<button
													type="button"
													class="grid size-6 shrink-0 place-items-center rounded text-faint transition-colors hover:bg-bg hover:text-danger"
													onclick={() => removeChoice(selected, ci)}
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
									onclick={() => addChoice(selected)}
									disabled={(sel.choices?.length ?? 0) >= MAX_CHOICES}
								>
									<Plus size={10} />
									Add choice
								</button>
								<p class="mt-1 font-mono text-[10px] text-faint">
									with choices, members must pick one (free input is off)
								</p>
							</div>
						{/if}

						<!-- Autocomplete -->
						{#if selMeta?.supportsAutocomplete}
							{@const hasChoices = (sel.choices?.length ?? 0) > 0}
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
									checked={!!sel.autocomplete}
									disabled={hasChoices}
									label="Autocomplete"
									onchange={(v) => patch(selected, { autocomplete: v || undefined })}
								/>
							</label>
						{/if}

						{#if problems[selected]?.length > 0}
							<ul class="space-y-0.5">
								{#each problems[selected] as p (p)}
									<li class="font-mono text-[10.5px] text-danger">· {p}</li>
								{/each}
							</ul>
						{/if}

						<!-- Footer: position + delete -->
						<div class="flex items-center justify-between border-t border-line pt-3">
							<span class="font-mono text-[10px] tabular-nums text-faint">
								#{selected + 1} of {options.length}
							</span>
							<button
								type="button"
								class="inline-flex h-7 items-center gap-1.5 rounded-md border border-line px-2 text-[11.5px] text-muted transition-colors hover:border-danger/40 hover:bg-danger/5 hover:text-danger"
								onclick={() => remove(selected)}
							>
								<Trash2 size={11} />
								Delete property
							</button>
						</div>
					</div>
				{/if}
			</div>
		</div>
	</div>
</div>
