<script lang="ts">
	// Formulas — the advanced inspector section that makes ANY scalar property
	// data-driven. Each entry binds a property (Layer.bind[key]) to a Go
	// text/template expression evaluated on the server render against the card
	// data (member / level / XP / progress) with math and if/else. The static
	// field stays the drag value + DOM-preview value; the formula overrides it on
	// the real PNG. Single-layer only — formulas are per-layer and the property
	// set differs by type.
	import { getContext } from 'svelte';
	import { EditorStore, EDITOR_CTX } from '$lib/layout/editor.svelte';
	import InspectorSection from './InspectorSection.svelte';
	import {
		bindablePropsFor,
		cardFormulaVarsFor,
		CARD_FUNCS,
		type BindableProp
	} from '$lib/layout/vars';
	import { Plus, X, Variable } from 'lucide-svelte';

	let { context = 'rank' }: { context?: 'welcome' | 'rank' } = $props();
	const editor = getContext<EditorStore>(EDITOR_CTX);

	const layer = $derived(editor.selectedIds.length === 1 ? editor.selected : null);
	const bindable = $derived(layer ? bindablePropsFor(layer.type) : []);
	const bound = $derived(bindable.filter((p) => layer?.bind && p.key in layer.bind));
	const unbound = $derived(bindable.filter((p) => !(layer?.bind && p.key in layer.bind)));
	const vars = $derived(cardFormulaVarsFor(context));

	let picker = $state<string | null>(null); // property whose var/func picker is open
	let addOpen = $state(false);
	let activeEl: HTMLTextAreaElement | null = null;

	function setFormula(key: string, val: string) {
		editor.setAll((l) => {
			if (!l.bind) l.bind = {};
			l.bind[key] = val;
		});
	}
	function addBinding(p: BindableProp) {
		setFormula(p.key, '');
		addOpen = false;
		picker = p.key;
	}
	function removeBinding(key: string) {
		editor.setAll((l) => {
			if (l.bind) {
				delete l.bind[key];
				if (Object.keys(l.bind).length === 0) l.bind = undefined;
			}
		});
		if (picker === key) picker = null;
	}
	// Insert a snippet at the focused formula's caret (or append if unfocused).
	function insert(key: string, text: string) {
		const el = activeEl;
		if (el && el.dataset.key === key) {
			const s = el.selectionStart ?? el.value.length;
			const e = el.selectionEnd ?? el.value.length;
			setFormula(key, el.value.slice(0, s) + text + el.value.slice(e));
			requestAnimationFrame(() => {
				el.focus();
				const pos = s + text.length;
				el.setSelectionRange(pos, pos);
			});
		} else {
			setFormula(key, (layer?.bind?.[key] ?? '') + text);
		}
	}

	function placeholder(kind: BindableProp['kind']): string {
		if (kind === 'color') return '{{ if gt .LevelNum 50 }}#FFD700{{ else }}#FF6363{{ end }}';
		if (kind === 'bool') return '{{ if gt .LevelNum 10 }}true{{ else }}false{{ end }}';
		return '{{ round (fmul .ProgressFrac 618) }}';
	}
</script>

{#if layer}
	<InspectorSection title="Formulas">
		{#snippet action()}
			{#if unbound.length}
				<button
					type="button"
					onclick={() => (addOpen = !addOpen)}
					class="grid size-5 place-items-center rounded text-faint transition-colors hover:bg-surface hover:text-ink"
					aria-label="Add a formula"
					title="Drive a property from a formula"
				>
					<Plus size={13} />
				</button>
			{/if}
		{/snippet}

		<div class="px-4 pb-3">
			{#if bound.length === 0 && !addOpen}
				<p class="text-[11px] leading-relaxed text-faint">
					Drive a property (width, opacity, color, …) from member data with math and
					<span class="font-mono">if/else</span>. Add one with
					<Plus size={11} class="-mt-0.5 inline" />.
				</p>
			{/if}

			<!-- Add-a-property menu -->
			{#if addOpen}
				<div class="mb-2 rounded-lg border border-line bg-ink-2 p-1">
					<div class="flex flex-wrap gap-1">
						{#each unbound as p (p.key)}
							<button
								type="button"
								onclick={() => addBinding(p)}
								class="rounded-md px-2 py-1 text-[11.5px] font-medium text-muted transition-colors hover:bg-surface hover:text-ink"
							>
								{p.label}
							</button>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Bound properties -->
			<div class="space-y-2.5">
				{#each bound as p (p.key)}
					<div>
						<div class="mb-1 flex items-center gap-1.5">
							<Variable size={12} class="text-accent-ink" />
							<span class="text-[11px] font-medium text-ink">{p.label}</span>
							<span class="font-mono text-[10px] text-faint">{p.kind}</span>
							<button
								type="button"
								onclick={() => (picker = picker === p.key ? null : p.key)}
								class="ml-auto rounded px-1 text-[10px] font-medium text-faint transition-colors hover:text-ink"
							>
								{picker === p.key ? 'Hide' : 'Insert'}
							</button>
							<button
								type="button"
								onclick={() => removeBinding(p.key)}
								class="grid size-4 place-items-center rounded text-faint transition-colors hover:bg-surface hover:text-danger"
								aria-label="Remove formula"
							>
								<X size={12} />
							</button>
						</div>
						<textarea
							data-key={p.key}
							rows="2"
							spellcheck="false"
							value={layer.bind?.[p.key] ?? ''}
							placeholder={placeholder(p.kind)}
							onfocus={(e) => (activeEl = e.currentTarget)}
							oninput={(e) => setFormula(p.key, e.currentTarget.value)}
							class="w-full resize-y rounded-md border border-line bg-bg px-2 py-1.5 font-mono text-[11px] leading-snug text-ink placeholder:text-faint/70 focus:border-line-strong focus:outline-none"
						></textarea>

						{#if picker === p.key}
							<div class="mt-1.5 rounded-md border border-line bg-ink-2 p-1.5">
								<div class="mb-1 font-mono text-[9.5px] uppercase tracking-wide text-faint">
									Variables
								</div>
								<div class="flex flex-wrap gap-1">
									{#each vars as v (v.tmpl)}
										<button
											type="button"
											onclick={() => insert(p.key, v.tmpl)}
											title={v.tmpl}
											class="rounded border border-line bg-surface px-1.5 py-0.5 text-[10.5px] text-muted transition-colors hover:border-line-strong hover:text-ink"
										>
											{v.label}
										</button>
									{/each}
								</div>
								<div class="mb-1 mt-2 font-mono text-[9.5px] uppercase tracking-wide text-faint">
									Functions
								</div>
								<div class="flex flex-wrap gap-1">
									{#each CARD_FUNCS as f (f.label)}
										<button
											type="button"
											onclick={() => insert(p.key, f.snippet)}
											title={f.hint}
											class="rounded border border-line bg-surface px-1.5 py-0.5 font-mono text-[10.5px] text-muted transition-colors hover:border-line-strong hover:text-ink"
										>
											{f.label}
										</button>
									{/each}
								</div>
							</div>
						{/if}
					</div>
				{/each}
			</div>
		</div>
	</InspectorSection>
{/if}
