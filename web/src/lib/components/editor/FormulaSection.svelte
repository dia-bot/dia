<script lang="ts">
	// Formulas — the advanced inspector section that makes ANY scalar property
	// data-driven. Each entry binds a property (Layer.bind[key]) to a Go
	// text/template expression evaluated on the server render against the card
	// data (member / level / XP / progress) with math and if/else. The static
	// field stays the drag value + DOM-preview value; the formula overrides it on
	// the real PNG. Single-layer only — formulas are per-layer and the property
	// set differs by type. Styled to match the other inspector sections: the
	// InspectorSection body already provides px-4, so content is full-width here.
	import { getContext } from 'svelte';
	import { EditorStore, EDITOR_CTX } from '$lib/layout/editor.svelte';
	import InspectorSection from './InspectorSection.svelte';
	import Select from '$lib/components/Select.svelte';
	import {
		bindablePropsFor,
		cardFormulaVarsFor,
		CARD_FUNCS,
		type BindableProp
	} from '$lib/layout/vars';
	import { X } from 'lucide-svelte';

	let { context = 'rank' }: { context?: 'welcome' | 'rank' } = $props();
	const editor = getContext<EditorStore>(EDITOR_CTX);

	const layer = $derived(editor.selectedIds.length === 1 ? editor.selected : null);
	const bindable = $derived(layer ? bindablePropsFor(layer.type) : []);
	const bound = $derived(bindable.filter((p) => layer?.bind && p.key in layer.bind));
	const unbound = $derived(bindable.filter((p) => !(layer?.bind && p.key in layer.bind)));
	const vars = $derived(cardFormulaVarsFor(context));

	// The formula textarea the insert palette writes into (kept across chip clicks
	// via onmousedown-preventDefault, so focus never leaves the field).
	let activeEl = $state<HTMLTextAreaElement | null>(null);

	function setFormula(key: string, val: string) {
		editor.setAll((l) => {
			if (!l.bind) l.bind = {};
			l.bind[key] = val;
		});
	}
	function addBinding(p: BindableProp) {
		setFormula(p.key, '');
	}
	function removeBinding(key: string) {
		editor.setAll((l) => {
			if (l.bind) {
				delete l.bind[key];
				if (Object.keys(l.bind).length === 0) l.bind = undefined;
			}
		});
	}
	// Insert a snippet at the focused formula's caret.
	function insert(text: string) {
		const el = activeEl;
		if (!el || !el.isConnected) return;
		const key = el.dataset.key;
		if (!key) return;
		const s = el.selectionStart ?? el.value.length;
		const e = el.selectionEnd ?? el.value.length;
		setFormula(key, el.value.slice(0, s) + text + el.value.slice(e));
		requestAnimationFrame(() => {
			el.focus();
			const pos = s + text.length;
			el.setSelectionRange(pos, pos);
		});
	}

	function placeholder(kind: BindableProp['kind']): string {
		if (kind === 'color') return '{{ if gt .LevelNum 50 }}#FFD700{{ else }}#FF6363{{ end }}';
		if (kind === 'bool') return '{{ if gt .LevelNum 10 }}true{{ else }}false{{ end }}';
		return '{{ round (fmul .ProgressFrac 618) }}';
	}

	const chip =
		'rounded border border-line px-1.5 py-0.5 font-mono text-[10px] text-muted transition-colors hover:border-line-strong hover:text-ink';
</script>

{#if layer}
	<InspectorSection title="Formulas">
		{#snippet action()}
			{#if bound.length}
				<span
					class="rounded-full border border-line px-1.5 font-mono text-[10px] leading-[1.5] tabular-nums text-muted"
				>
					{bound.length}
				</span>
			{/if}
		{/snippet}

		<div class="space-y-3">
			{#if bound.length === 0}
				<p class="text-[11px] leading-relaxed text-faint">
					Drive a property from member data with math and
					<span class="font-mono text-muted">if / else</span>. The static value stays your
					fallback; the formula renders on the live card.
				</p>
			{/if}

			<!-- Bound properties -->
			{#each bound as p (p.key)}
				<div class="space-y-1.5">
					<div class="flex items-center gap-1.5">
						<span class="text-[11px] font-medium text-ink">{p.label}</span>
						<span
							class="rounded border border-line px-1 font-mono text-[9px] uppercase leading-[1.6] tracking-wide text-faint"
						>
							{p.kind}
						</span>
						<button
							type="button"
							onclick={() => removeBinding(p.key)}
							class="ml-auto grid size-5 place-items-center rounded text-faint transition-colors hover:bg-ink-2 hover:text-danger"
							aria-label="Remove {p.label} formula"
							title="Remove formula"
						>
							<X size={13} />
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
						class="w-full resize-y rounded-lg border border-line bg-ink-2 px-2.5 py-2 font-mono text-[11.5px] leading-snug text-ink outline-none transition-all placeholder:text-faint/80 hover:border-faint focus:border-faint focus:ring-2 focus:ring-line-strong"
					></textarea>
				</div>
			{/each}

			<!-- Insert palette: writes into the focused formula (preventDefault keeps
			     the caret). Only shown once there's a formula to insert into. -->
			{#if bound.length}
				<div class="space-y-2 rounded-lg border border-line bg-ink-2/40 p-2.5">
					<div class="space-y-1">
						<div class="font-mono text-[9.5px] uppercase tracking-wide text-faint">Variables</div>
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
					<div class="space-y-1">
						<div class="font-mono text-[9.5px] uppercase tracking-wide text-faint">Functions</div>
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
			{/if}

			<!-- Add a property -->
			{#if unbound.length}
				<Select
					dense
					bind:value={
						() => '',
						(v) => {
							const p = unbound.find((x) => x.key === v);
							if (p) addBinding(p);
						}
					}
					placeholder="Add a formula…"
					options={unbound.map((p) => ({ value: p.key, label: p.label }))}
				/>
			{/if}
		</div>
	</InspectorSection>
{/if}
