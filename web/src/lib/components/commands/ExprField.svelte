<script lang="ts">
	import { getContext } from 'svelte';
	import type { CommandOption, Expr, VarDecl } from '$lib/commands/types';
	import {
		TMPL_FUNCTIONS,
		TMPL_CATEGORIES,
		TMPL_STATIC_VARS,
		TMPL_ERROR_VARS,
		TMPL_SNIPPETS,
		EXPR_SCOPE_CTX,
		type TmplFunc,
		type TmplVar,
		type ExprScope
	} from '$lib/commands/expr-meta';
	import FunctionSquare from 'lucide-svelte/icons/function-square';
	import Variable from 'lucide-svelte/icons/variable';
	import X from 'lucide-svelte/icons/x';

	type Props = {
		value: Expr | undefined;
		onChange: (v: Expr) => void;
		placeholder?: string;
		label?: string;
		hint?: string;
		// Drives the in-scope variable picker. The editor passes whatever the
		// command defines plus whether we're inside an on_error subtree.
		options?: CommandOption[];
		variables?: VarDecl[];
		inErrorScope?: boolean;
	};

	let {
		value,
		onChange,
		placeholder = '{{ .User.Username }}',
		label = '',
		hint = '',
		options,
		variables,
		inErrorScope = false
	}: Props = $props();

	// Fall back to the editor's scope context when not given explicit props —
	// keeps the 30+ call sites in StepInspector working untouched.
	const scope = getContext<ExprScope | undefined>(EXPR_SCOPE_CTX);
	const effectiveOptions = $derived(options ?? scope?.options ?? []);
	const effectiveVariables = $derived(variables ?? scope?.variables ?? []);

	const src = $derived(value?.src ?? '');
	const lang = $derived(value?.lang ?? 'tmpl');

	let inputEl: HTMLInputElement | null = $state(null);
	let pickerOpen = $state(false);
	let pickerTab = $state<'funcs' | 'vars' | 'snippets'>('funcs');
	let pickerCategory = $state<TmplFunc['category'] | 'all'>('all');
	let pickerFilter = $state('');

	function update(s: string) {
		onChange({ lang: 'tmpl', src: s });
	}

	function insertAtCursor(snippet: string) {
		if (!inputEl) {
			update((src ?? '') + snippet);
			return;
		}
		const start = inputEl.selectionStart ?? src.length;
		const end = inputEl.selectionEnd ?? src.length;
		const before = src.slice(0, start);
		const after = src.slice(end);
		const next = before + snippet + after;
		update(next);
		queueMicrotask(() => {
			if (!inputEl) return;
			inputEl.focus();
			const caret = start + snippet.length;
			inputEl.setSelectionRange(caret, caret);
		});
	}

	function insertVar(v: TmplVar) {
		insertAtCursor(`{{ ${v.path} }}`);
	}

	function insertFunc(f: TmplFunc) {
		// If user has highlighted text, wrap it; otherwise drop a stub.
		const start = inputEl?.selectionStart ?? 0;
		const end = inputEl?.selectionEnd ?? 0;
		if (start !== end && inputEl) {
			const sel = src.slice(start, end);
			const replacement = `{{ ${f.name} ${sel} }}`;
			const next = src.slice(0, start) + replacement + src.slice(end);
			update(next);
			queueMicrotask(() => {
				if (!inputEl) return;
				inputEl.focus();
				const caret = start + replacement.length;
				inputEl.setSelectionRange(caret, caret);
			});
		} else {
			insertAtCursor(`{{ ${f.name} }}`);
		}
	}

	// Dynamic vars derived from the command's options + declared variables.
	const dynamicVars = $derived([
		...effectiveOptions.map<TmplVar>((o) => ({
			path: `.Input.${o.name}`,
			label: `Input.${o.name}`,
			type: o.kind,
			short: o.description || `Slash argument (${o.kind})`
		})),
		...effectiveVariables.map<TmplVar>((v) => ({
			path: `.Vars.${v.name}`,
			label: `Vars.${v.name}`,
			type: v.type,
			short: `Declared variable (${v.type})`
		}))
	]);

	const scopeVars = $derived([
		...TMPL_STATIC_VARS,
		...dynamicVars,
		...(inErrorScope ? TMPL_ERROR_VARS : [])
	]);

	const filteredFuncs = $derived(
		TMPL_FUNCTIONS.filter((f) => {
			if (pickerCategory !== 'all' && f.category !== pickerCategory) return false;
			if (!pickerFilter) return true;
			const q = pickerFilter.toLowerCase();
			return (
				f.name.toLowerCase().includes(q) ||
				f.short.toLowerCase().includes(q) ||
				f.signature.toLowerCase().includes(q)
			);
		})
	);

	const filteredVars = $derived(
		scopeVars.filter((v) => {
			if (!pickerFilter) return true;
			const q = pickerFilter.toLowerCase();
			return v.path.toLowerCase().includes(q) || v.short.toLowerCase().includes(q);
		})
	);

	// Quick "balanced braces" check — flags obviously broken syntax.
	const braceIssue = $derived(checkBraces(src));

	function checkBraces(s: string): string {
		let depth = 0;
		for (let i = 0; i < s.length - 1; i++) {
			if (s[i] === '{' && s[i + 1] === '{') {
				depth++;
				i++;
			} else if (s[i] === '}' && s[i + 1] === '}') {
				depth--;
				if (depth < 0) return 'unmatched }}';
				i++;
			}
		}
		if (depth > 0) return 'unclosed {{';
		return '';
	}

	function onKeyDown(e: KeyboardEvent) {
		if (e.ctrlKey && e.key === ' ') {
			e.preventDefault();
			pickerOpen = true;
		}
		// Auto-pair {{ → {{ }} with caret in the middle.
		if (e.key === '{' && inputEl) {
			const pos = inputEl.selectionStart ?? 0;
			if (src[pos - 1] === '{') {
				e.preventDefault();
				const before = src.slice(0, pos);
				const after = src.slice(pos);
				const next = before + '{ }}' + after;
				update(next);
				queueMicrotask(() => {
					if (!inputEl) return;
					const caret = pos + 2;
					inputEl.setSelectionRange(caret, caret);
				});
			}
		}
	}
</script>

<div class="mb-3">
	{#if label}
		<div class="mb-1 flex items-center gap-2">
			<span class="font-mono text-[10px] font-medium uppercase tracking-[0.12em] text-faint">
				{label}
			</span>
			{#if braceIssue}
				<span class="font-mono text-[10px] text-danger">· {braceIssue}</span>
			{/if}
		</div>
	{/if}
	<div class="relative">
		<input
			bind:this={inputEl}
			class="h-8 w-full rounded-md border border-line bg-bg px-2 pr-20 font-mono text-[12.5px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
			class:border-danger={!!braceIssue}
			{placeholder}
			value={src}
			oninput={(e) => update((e.currentTarget as HTMLInputElement).value)}
			onkeydown={onKeyDown}
			autocomplete="off"
			spellcheck="false"
		/>
		<div class="absolute right-1 top-1/2 flex -translate-y-1/2 items-center gap-0.5">
			<button
				type="button"
				class="grid size-6 place-items-center rounded text-faint transition-colors hover:bg-surface hover:text-ink"
				title="Insert variable (Ctrl+Space)"
				aria-label="Insert variable"
				onclick={() => {
					pickerTab = 'vars';
					pickerOpen = !pickerOpen || pickerTab !== 'vars';
				}}
			>
				<Variable size={12} />
			</button>
			<button
				type="button"
				class="grid size-6 place-items-center rounded text-faint transition-colors hover:bg-surface hover:text-ink"
				title="Insert function"
				aria-label="Insert function"
				onclick={() => {
					pickerTab = 'funcs';
					pickerOpen = !pickerOpen || pickerTab !== 'funcs';
				}}
			>
				<FunctionSquare size={12} />
			</button>
			<span
				class="pointer-events-none ml-1 font-mono text-[9.5px] uppercase tracking-[0.14em] text-faint"
				title="Template language"
			>
				{lang === 'literal' ? 'literal' : 'tmpl'}
			</span>
		</div>

		{#if pickerOpen}
			<div
				class="absolute left-0 right-0 top-[calc(100%+4px)] z-20 flex max-h-72 flex-col overflow-hidden rounded-md border border-line bg-bg shadow-lg"
			>
				<div class="flex h-7 shrink-0 items-center gap-2 border-b border-line/60 bg-surface/40 px-2">
					{#each [{ id: 'funcs', label: 'Functions' }, { id: 'vars', label: 'Variables' }, { id: 'snippets', label: 'Snippets' }] as t (t.id)}
						<button
							type="button"
							class="rounded px-1.5 py-0.5 font-mono text-[10px] font-medium uppercase tracking-[0.12em] transition-colors {pickerTab === t.id
								? 'bg-bg text-ink'
								: 'text-faint hover:text-ink'}"
							onclick={() => (pickerTab = t.id as 'funcs' | 'vars' | 'snippets')}
						>
							{t.label}
						</button>
					{/each}
					<input
						class="ml-auto h-5 w-40 rounded border border-line bg-bg px-1.5 text-[11px] placeholder:text-faint focus:border-line-strong focus:outline-none"
						placeholder="Filter…"
						bind:value={pickerFilter}
					/>
					<button
						type="button"
						class="grid size-5 place-items-center rounded text-faint hover:bg-bg hover:text-ink"
						onclick={() => (pickerOpen = false)}
						aria-label="Close"
					>
						<X size={11} />
					</button>
				</div>

				<div class="min-h-0 flex-1 overflow-y-auto">
					{#if pickerTab === 'funcs'}
						<div class="flex h-7 shrink-0 items-center gap-1 overflow-x-auto border-b border-line/40 bg-bg px-2">
							{#each [{ id: 'all', label: 'All' }, ...TMPL_CATEGORIES] as c (c.id)}
								<button
									type="button"
									class="shrink-0 rounded px-1.5 py-0.5 font-mono text-[10px] font-medium uppercase tracking-[0.12em] transition-colors {pickerCategory === c.id
										? 'bg-surface text-ink'
										: 'text-faint hover:text-ink'}"
									onclick={() => (pickerCategory = c.id as TmplFunc['category'] | 'all')}
								>
									{c.label}
								</button>
							{/each}
						</div>
						<div class="divide-y divide-line/40">
							{#each filteredFuncs as f (f.name)}
								<button
									type="button"
									class="flex w-full items-center gap-2 px-2.5 py-1 text-left transition-colors hover:bg-surface/60"
									onclick={() => insertFunc(f)}
								>
									<code class="shrink-0 font-mono text-[11px] font-medium text-ink">{f.name}</code>
									<span class="shrink-0 font-mono text-[10px] text-faint">{f.signature}</span>
									<span class="min-w-0 flex-1 truncate text-[11px] text-muted">{f.short}</span>
								</button>
							{/each}
							{#if filteredFuncs.length === 0}
								<p class="px-3 py-4 text-center font-mono text-[10.5px] text-faint">
									No function matches "{pickerFilter}"
								</p>
							{/if}
						</div>
					{:else if pickerTab === 'vars'}
						<div class="divide-y divide-line/40">
							{#each filteredVars as v (v.path)}
								<button
									type="button"
									class="flex w-full items-center gap-2 px-2.5 py-1 text-left transition-colors hover:bg-surface/60"
									onclick={() => insertVar(v)}
								>
									<code class="shrink-0 font-mono text-[11px] font-medium text-ink">{v.path}</code>
									<span class="shrink-0 font-mono text-[10px] text-faint">{v.type}</span>
									<span class="min-w-0 flex-1 truncate text-[11px] text-muted">{v.short}</span>
								</button>
							{/each}
							{#if filteredVars.length === 0}
								<p class="px-3 py-4 text-center font-mono text-[10.5px] text-faint">
									No variable matches "{pickerFilter}"
								</p>
							{/if}
						</div>
					{:else}
						<div class="divide-y divide-line/40">
							{#each TMPL_SNIPPETS as s (s.id)}
								<button
									type="button"
									class="flex w-full items-start gap-2 px-2.5 py-1.5 text-left transition-colors hover:bg-surface/60"
									onclick={() => insertAtCursor(s.insert)}
								>
									<span class="shrink-0 text-[11.5px] font-medium text-ink">{s.label}</span>
									<code class="min-w-0 flex-1 truncate font-mono text-[10.5px] text-muted">{s.insert}</code>
								</button>
							{/each}
						</div>
					{/if}
				</div>
			</div>
		{/if}
	</div>

	{#if hint}
		<p class="mt-1 font-mono text-[10px] text-faint">{hint}</p>
	{/if}
</div>
