<script lang="ts">
	// The spacious, dedicated formula editor for Card Studio — a two-pane dialog so
	// the "make any property data-driven" surface has room to breathe instead of
	// crowding the narrow inspector. Left: every bindable property for the layer,
	// grouped, with a dot marking the ones that already have a formula. Right: the
	// editor for the selected property (a big code field, expected-output hint,
	// and click-to-insert variable / function / enum palettes). Writes
	// Layer.bind[key] via editor.setAll; an empty formula clears the binding.
	import { getContext } from 'svelte';
	import { EditorStore, EDITOR_CTX } from '$lib/layout/editor.svelte';
	import { Dialog } from '$lib/components/ui';
	import {
		bindablePropsFor,
		cardFormulaVarsFor,
		CARD_FUNCS,
		BIND_GROUPS,
		BG_BINDABLE_PROPS,
		type BindableProp
	} from '$lib/layout/vars';
	import { X, FunctionSquare, Trash2 } from 'lucide-svelte';

	let {
		open = $bindable(false),
		context = 'rank',
		initialKey = '',
		target = 'layer'
	}: {
		open?: boolean;
		context?: 'welcome' | 'rank';
		initialKey?: string;
		target?: 'layer' | 'background';
	} = $props();

	const editor = getContext<EditorStore>(EDITOR_CTX);

	const isBg = $derived(target === 'background');
	// The bind object being edited: the canvas background's, or the single
	// selected layer's. `subject` is truthy whenever there is something to edit.
	const layer = $derived(isBg ? null : editor.selectedIds.length === 1 ? editor.selected : null);
	const subject = $derived(isBg ? editor.layout.background : layer);
	const bindObj = $derived(isBg ? editor.layout.background.bind : layer?.bind);
	const fields = $derived(isBg ? BG_BINDABLE_PROPS : layer ? bindablePropsFor(layer.type) : []);
	const groups = $derived(
		BIND_GROUPS.map((name) => ({ name, items: fields.filter((p) => p.group === name) })).filter(
			(g) => g.items.length
		)
	);
	const vars = $derived(cardFormulaVarsFor(context));

	let selectedKey = $state('');
	// Keep a valid selection. On the moment of OPENING, honour an explicit
	// initialKey (the chip you clicked) even though the modal instance persists
	// across opens; otherwise only re-pick when the current selection is no longer
	// a valid property for this layer. `wasOpen` is a plain (non-reactive) latch so
	// this doesn't fight the user clicking a different property in the left list.
	let wasOpen = false;
	$effect(() => {
		const justOpened = open && !wasOpen;
		wasOpen = open;
		if (!open) return;
		const want = fields.find((p) => p.key === initialKey);
		if (justOpened && want) {
			selectedKey = want.key;
			return;
		}
		if (fields.some((p) => p.key === selectedKey)) return;
		const firstBound = fields.find((p) => bindObj && p.key in bindObj);
		selectedKey = (want ?? firstBound ?? fields[0])?.key ?? '';
	});
	// Mirror the dialog's open state onto the editor so Canvas suppresses its
	// window key handlers while this modal is up (a keypress here must not mutate
	// the layer underneath). Reset on unmount so the flag can't stick.
	$effect(() => {
		editor.formulaOpen = open;
		return () => {
			editor.formulaOpen = false;
		};
	});
	const selected = $derived(fields.find((p) => p.key === selectedKey) ?? null);

	function isBound(key: string): boolean {
		return !!(bindObj && key in bindObj && (bindObj[key] ?? '').trim() !== '');
	}
	// Mutate a bind map in place: set the key, or delete it (nulling the map when
	// empty) for an empty formula. Shared by the layer and background targets.
	function writeBind(holder: { bind?: Record<string, string> }, key: string, val: string) {
		if (val.trim() === '') {
			if (holder.bind) {
				delete holder.bind[key];
				if (Object.keys(holder.bind).length === 0) holder.bind = undefined;
			}
		} else {
			if (!holder.bind) holder.bind = {};
			holder.bind[key] = val;
		}
	}
	function setFormula(key: string, val: string) {
		if (isBg) {
			// Same reactive layout mutation as setAll (which just mutates in place),
			// so the studio's dirty/undo tracking picks it up.
			writeBind(editor.layout.background, key, val);
		} else {
			editor.setAll((l) => writeBind(l, key, val));
		}
	}

	let ta = $state<HTMLTextAreaElement | null>(null);
	function insert(text: string) {
		const el = ta;
		if (!el || !selected) return;
		const s = el.selectionStart ?? el.value.length;
		const e = el.selectionEnd ?? el.value.length;
		setFormula(selected.key, el.value.slice(0, s) + text + el.value.slice(e));
		requestAnimationFrame(() => {
			el.focus();
			const pos = s + text.length;
			el.setSelectionRange(pos, pos);
		});
	}

	function outHint(p: BindableProp): string {
		switch (p.kind) {
			case 'number':
				return 'Outputs a number.';
			case 'color':
				return 'Outputs a hex color, e.g. #FFD700.';
			case 'bool':
				return 'Outputs true or false.';
			case 'enum':
				return 'Outputs one of the values below.';
			default:
				return 'Outputs text.';
		}
	}

	const chip =
		'rounded border border-line bg-surface px-1.5 py-0.5 font-mono text-[10.5px] text-muted transition-colors hover:border-line-strong hover:text-ink';
	const label = 'mb-1 font-mono text-[9.5px] uppercase tracking-wide text-faint';
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-w-[880px] gap-0 overflow-hidden p-0" showClose={false}>
		<Dialog.Title class="sr-only">Formulas</Dialog.Title>

		<!-- Header -->
		<div class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-4">
			<div class="grid size-5 place-items-center rounded border border-line bg-surface text-muted">
				<FunctionSquare size={11} />
			</div>
			<span class="text-[12.5px] font-medium text-ink">Formulas</span>
			{#if isBg}
				<span class="truncate text-[11.5px] text-muted">· canvas background</span>
			{:else if layer}
				<span class="truncate text-[11.5px] text-muted">· {layer.name || layer.type}</span>
			{/if}
			<button
				type="button"
				class="ml-auto grid size-6 place-items-center rounded-md text-muted transition-colors hover:bg-surface hover:text-ink"
				onclick={() => (open = false)}
				aria-label="Close"
			>
				<X size={14} />
			</button>
		</div>

		<!-- Two-pane body -->
		<div class="flex h-[560px] max-h-[72vh]">
			<!-- Left: every bindable property, grouped. -->
			<div class="w-[236px] shrink-0 overflow-y-auto border-r border-line py-1.5">
				{#each groups as g (g.name)}
					<div class="px-3 pb-1 pt-2.5 {label}">{g.name}</div>
					{#each g.items as p (p.key)}
						<button
							type="button"
							onclick={() => (selectedKey = p.key)}
							class="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[12px] transition-colors {selectedKey ===
							p.key
								? 'bg-surface font-medium text-ink'
								: 'text-muted hover:bg-surface/50 hover:text-ink'}"
						>
							<span
								class="size-1.5 shrink-0 rounded-full {isBound(p.key) ? 'bg-accent' : 'bg-faint/30'}"
								title={isBound(p.key) ? 'Has a formula' : 'No formula'}
							></span>
							<span class="min-w-0 flex-1 truncate">{p.label}</span>
						</button>
					{/each}
				{/each}
			</div>

			<!-- Right: the selected property's editor. -->
			<div class="min-w-0 flex-1 overflow-y-auto">
				{#if selected && subject}
					<div class="space-y-4 p-5">
						<div class="flex items-center gap-2">
							<h3 class="text-[13px] font-semibold text-ink">{selected.label}</h3>
							<span class="rounded border border-line px-1.5 py-0.5 font-mono text-[10px] text-faint">
								{selected.kind}
							</span>
							{#if isBound(selected.key)}
								<button
									type="button"
									onclick={() => setFormula(selected.key, '')}
									class="ml-auto inline-flex items-center gap-1.5 rounded-md border border-line-strong px-2 py-1 text-[11.5px] font-medium text-muted transition-colors hover:border-danger hover:text-danger"
								>
									<Trash2 size={12} /> Remove
								</button>
							{/if}
						</div>

						<p class="text-[11.5px] leading-relaxed text-faint">
							{outHint(selected)} The static value stays your fallback; the formula renders on the
							live server card (not the editor preview).
						</p>

						<textarea
							bind:this={ta}
							rows="6"
							spellcheck="false"
							value={bindObj?.[selected.key] ?? ''}
							placeholder={'{{ round (fmul .ProgressFrac 618) }}'}
							oninput={(e) => setFormula(selected.key, e.currentTarget.value)}
							class="w-full resize-y rounded-lg border border-line bg-ink-2 px-3 py-2.5 font-mono text-[12px] leading-relaxed text-ink outline-none transition-all placeholder:text-faint/70 hover:border-faint focus:border-faint focus:ring-2 focus:ring-line-strong"
						></textarea>

						{#if selected.kind === 'enum' && selected.values}
							<div>
								<div class={label}>Valid values</div>
								<div class="flex flex-wrap gap-1">
									{#each selected.values as v (v)}
										<button
											type="button"
											onmousedown={(e) => e.preventDefault()}
											onclick={() => insert(v)}
											class={chip}
										>
											{v}
										</button>
									{/each}
								</div>
							</div>
						{/if}

						<div>
							<div class={label}>Variables</div>
							<div class="flex flex-wrap gap-1">
								{#each vars as v (v.tmpl)}
									<button
										type="button"
										title={v.tmpl}
										onmousedown={(e) => e.preventDefault()}
										onclick={() => insert(v.tmpl)}
										class={chip}
									>
										{v.label}
									</button>
								{/each}
							</div>
						</div>

						<div>
							<div class={label}>Functions</div>
							<div class="flex flex-wrap gap-1">
								{#each CARD_FUNCS as f (f.label)}
									<button
										type="button"
										title={f.hint}
										onmousedown={(e) => e.preventDefault()}
										onclick={() => insert(f.snippet)}
										class={chip}
									>
										{f.label}
									</button>
								{/each}
							</div>
						</div>
					</div>
				{:else}
					<div class="grid h-full place-items-center px-6 text-center text-[12px] text-faint">
						Select a single layer to add formulas.
					</div>
				{/if}
			</div>
		</div>
	</Dialog.Content>
</Dialog.Root>
