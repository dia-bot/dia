<script lang="ts">
	// A reference modal for the Go text/template language every user-facing
	// string in Dia renders through: the placeholder syntax, control flow
	// (if / range / with), the variables in scope on the current surface, and
	// the full pure-function catalogue (mirrored from
	// internal/templating/funcs.go via expr-meta's TMPL_FUNCTIONS). Purely
	// documentation — the pickers still insert; this explains.
	import { fade, scale } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';
	import {
		TMPL_FUNCTIONS,
		TMPL_CATEGORIES,
		type TmplVar
	} from '$lib/commands/expr-meta';
	import { BookOpen, X } from 'lucide-svelte';

	let {
		open = $bindable(false),
		// The variables in scope on the surface that opened the guide (e.g. the
		// giveaway scope). Shown as its own section above the shared catalogue.
		variables = [],
		variablesLabel = 'Variables in scope',
		// Guild lookups (getRole / getChannel) only exist on the command /
		// automation engine; simple surfaces (giveaway strings, card formulas)
		// render on the pure engine without them.
		lookups = true
	}: {
		open?: boolean;
		variables?: TmplVar[];
		variablesLabel?: string;
		lookups?: boolean;
	} = $props();

	const funcs = $derived(
		lookups ? TMPL_FUNCTIONS : TMPL_FUNCTIONS.filter((f) => f.name !== 'getRole' && f.name !== 'getChannel')
	);

	// Syntax primer rows: pattern → what it does → example.
	const SYNTAX: { code: string; short: string }[] = [
		{ code: '{{ .Prize }}', short: 'Insert a value. The leading dot is the scope; names are case-sensitive.' },
		{ code: '{{ upper .Prize }}', short: 'Call a function on a value (function first, arguments after).' },
		{ code: '{{ printf "%d tickets" .Entries }}', short: 'Format several values into one string.' },
		{ code: '{{ if gt .Entries 1 }}…{{ end }}', short: 'Show a part only when a condition holds.' },
		{ code: '{{ if .Reason }}…{{ else }}…{{ end }}', short: 'Branch on whether a value is set.' },
		{ code: '{{ range .Items }}{{ . }}{{ end }}', short: 'Repeat a part for every element of a list ({{ . }} is the element).' },
		{ code: '{{ with .Member }}{{ .Nick }}{{ end }}', short: 'Rebind the dot to a value inside the block.' },
		{ code: '{{ default "someone" .Host }}', short: 'Fall back when a value is empty.' }
	];

	const EXAMPLES: { code: string; short: string }[] = [
		{
			code: '🎉 {{ .Prize }} — ends {{ .Ends }}',
			short: 'Values drop into plain text anywhere.'
		},
		{
			code: '{{ if gt .Entries 1 }}You have {{ .Entries }} entries!{{ else }}Good luck!{{ end }}',
			short: 'Different copy for bonus-ticket holders.'
		},
		{
			code: '{{ upper (slice .Prize 0 1) }}{{ slice .Prize 1 (len .Prize) }}',
			short: 'Nest calls with parentheses (capitalise the first letter).'
		}
	];

	function close() {
		open = false;
	}
	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && open) close();
	}
</script>

<svelte:window onkeydown={onKeydown} />

{#if open}
	<div class="fixed inset-0 z-[70] grid place-items-center p-4">
		<button
			type="button"
			class="absolute inset-0 h-full w-full cursor-default bg-black/40"
			aria-label="Dismiss"
			onclick={close}
			transition:fade={{ duration: dur(150) }}
		></button>

		<div
			class="relative flex max-h-[85vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-line bg-surface shadow-2xl"
			transition:scale={{ duration: dur(200), start: 0.95, opacity: 0, easing: cubicOut }}
			role="dialog"
			aria-label="Template guide"
		>
			<div class="flex shrink-0 items-center gap-2 border-b border-line px-4 py-3">
				<BookOpen size={13} class="text-muted" />
				<span class="text-[13px] font-semibold text-ink">Template guide</span>
				<span class="hidden text-[12px] text-muted sm:inline">— every text field renders through these</span>
				<button
					type="button"
					onclick={close}
					class="ml-auto grid size-7 place-items-center rounded-md text-muted hover:bg-ink-2 hover:text-ink"
					aria-label="Close"
				>
					<X size={14} />
				</button>
			</div>

			<div class="min-h-0 flex-1 overflow-y-auto px-4 py-4">
				<!-- Syntax -->
				<div class="mb-1 font-mono text-[10px] uppercase tracking-[0.14em] text-faint">Syntax</div>
				<p class="mb-2 text-[12px] leading-relaxed text-muted">
					Anything between <code class="rounded bg-bg px-1 font-mono text-[11px]">{'{{'}</code> and
					<code class="rounded bg-bg px-1 font-mono text-[11px]">{'}}'}</code> is evaluated when the message
					sends; everything else is literal text (markdown and emoji included). Templates only read values
					and format text — they never perform actions.
				</p>
				<div class="mb-4 overflow-hidden rounded-md border border-line">
					{#each SYNTAX as row, i (row.code)}
						<div class="grid gap-1 px-2.5 py-1.5 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] {i > 0 ? 'border-t border-line' : ''}">
							<code class="break-all font-mono text-[11px] text-ink">{row.code}</code>
							<span class="text-[11.5px] leading-snug text-muted">{row.short}</span>
						</div>
					{/each}
				</div>

				<!-- Variables in scope -->
				{#if variables.length > 0}
					<div class="mb-1 font-mono text-[10px] uppercase tracking-[0.14em] text-faint">{variablesLabel}</div>
					<div class="mb-4 overflow-hidden rounded-md border border-line">
						{#each variables as v, i (v.path)}
							<div class="grid gap-1 px-2.5 py-1.5 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] {i > 0 ? 'border-t border-line' : ''}">
								<code class="font-mono text-[11px] text-ink">{'{{ ' + v.path + ' }}'}</code>
								<span class="text-[11.5px] leading-snug text-muted">
									{v.short}
									<span class="font-mono text-[10px] text-faint">({v.type})</span>
								</span>
							</div>
						{/each}
					</div>
				{/if}

				<!-- Functions -->
				<div class="mb-1 font-mono text-[10px] uppercase tracking-[0.14em] text-faint">Functions</div>
				<p class="mb-2 text-[12px] leading-relaxed text-muted">
					Call as <code class="rounded bg-bg px-1 font-mono text-[11px]">{'{{ func arg1 arg2 }}'}</code>;
					nest with parentheses. All pure — they can't send, grant or change anything.
				</p>
				{#each TMPL_CATEGORIES as cat (cat.id)}
					{@const list = funcs.filter((f) => f.category === cat.id)}
					{#if list.length > 0}
						<div class="mb-1 mt-3 text-[11px] font-semibold text-ink">{cat.label}</div>
						<div class="overflow-hidden rounded-md border border-line">
							{#each list as f, i (f.name)}
								<div class="grid gap-1 px-2.5 py-1.5 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] {i > 0 ? 'border-t border-line' : ''}">
									<code class="break-all font-mono text-[11px] text-ink">{f.signature}</code>
									<span class="text-[11.5px] leading-snug text-muted">{f.short}</span>
								</div>
							{/each}
						</div>
					{/if}
				{/each}

				<!-- Examples -->
				<div class="mb-1 mt-4 font-mono text-[10px] uppercase tracking-[0.14em] text-faint">Examples</div>
				<div class="overflow-hidden rounded-md border border-line">
					{#each EXAMPLES as ex, i (ex.code)}
						<div class="px-2.5 py-2 {i > 0 ? 'border-t border-line' : ''}">
							<code class="block break-all font-mono text-[11px] text-ink">{ex.code}</code>
							<span class="mt-0.5 block text-[11.5px] text-muted">{ex.short}</span>
						</div>
					{/each}
				</div>

				<p class="mt-3 text-[11px] leading-relaxed text-faint">
					A template that errors falls back to its literal text rather than breaking the message. Use each
					field's Test render to check against real sample values.
				</p>
			</div>
		</div>
	</div>
{/if}
