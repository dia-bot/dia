<script lang="ts">
	// Compact sidebar entry for the formula system. The actual editing happens in
	// the spacious FormulaModal (the inspector is too narrow for it); here we just
	// show what's bound at a glance and open the editor. Single-layer only.
	import { getContext } from 'svelte';
	import { EditorStore, EDITOR_CTX } from '$lib/layout/editor.svelte';
	import InspectorSection from './InspectorSection.svelte';
	import FormulaModal from './FormulaModal.svelte';
	import { bindablePropsFor } from '$lib/layout/vars';
	import { FunctionSquare } from 'lucide-svelte';

	let { context = 'rank' }: { context?: 'welcome' | 'rank' } = $props();
	const editor = getContext<EditorStore>(EDITOR_CTX);

	const layer = $derived(editor.selectedIds.length === 1 ? editor.selected : null);
	const boundProps = $derived(
		layer
			? bindablePropsFor(layer.type).filter(
					(p) => layer.bind && p.key in layer.bind && (layer.bind[p.key] ?? '').trim() !== ''
				)
			: []
	);

	let open = $state(false);
	let initialKey = $state('');
	function edit(key = '') {
		initialKey = key;
		open = true;
	}
</script>

{#if layer}
	<InspectorSection title="Formulas">
		{#snippet action()}
			{#if boundProps.length}
				<span
					class="rounded-full border border-line px-1.5 font-mono text-[10px] leading-[1.5] tabular-nums text-muted"
				>
					{boundProps.length}
				</span>
			{/if}
		{/snippet}

		<div class="space-y-2">
			{#if boundProps.length}
				<!-- Bound properties: click one to jump straight to its formula. -->
				<div class="flex flex-wrap gap-1">
					{#each boundProps as p (p.key)}
						<button
							type="button"
							onclick={() => edit(p.key)}
							class="inline-flex items-center gap-1 rounded border border-line bg-ink-2 px-1.5 py-0.5 text-[11px] text-muted transition-colors hover:border-line-strong hover:text-ink"
							title="Edit {p.label} formula"
						>
							<span class="size-1 rounded-full bg-accent"></span>
							{p.label}
						</button>
					{/each}
				</div>
				<button
					type="button"
					onclick={() => edit()}
					class="inline-flex h-7 w-full items-center justify-center gap-1.5 rounded-md border border-line-strong text-xs font-medium text-ink transition-colors hover:bg-ink-2"
				>
					<FunctionSquare size={13} /> Edit formulas
				</button>
			{:else}
				<button
					type="button"
					onclick={() => edit()}
					class="inline-flex h-7 w-full items-center justify-center gap-1.5 rounded-md border border-line-strong text-xs font-medium text-ink transition-colors hover:bg-ink-2"
				>
					<FunctionSquare size={13} /> Add a formula
				</button>
				<p class="text-[11px] leading-relaxed text-faint">
					Drive any property (width, color, stroke, …) from member data with math and
					<span class="font-mono text-muted">if / else</span>.
				</p>
			{/if}
		</div>
	</InspectorSection>

	<FormulaModal bind:open {context} {initialKey} />
{/if}
