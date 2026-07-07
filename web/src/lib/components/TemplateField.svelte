<script lang="ts">
	// A reusable message-template editor: a textarea plus a live "Test render"
	// (runs the real engine on the server with sample data) and a collapsible
	// reference of {tokens} + {{ }} logic snippets that insert at the cursor.
	import { api } from '$lib/api';
	import { Loader2, Sparkles, ChevronDown } from 'lucide-svelte';

	let {
		value = $bindable(''),
		label = '',
		placeholder = '',
		guildId,
		variables = [],
		extraVars,
		sample,
		rows = 3
	}: {
		value: string;
		label?: string;
		placeholder?: string;
		guildId: string;
		variables?: { token: string; desc: string }[];
		extraVars?: Record<string, string>;
		// sample renders the "Test render" against a feature data map via the card
		// engine (fields like {{ .Prize }}) instead of the default user/guild scope.
		sample?: Record<string, unknown>;
		rows?: number;
	} = $props();

	let el = $state<HTMLTextAreaElement>();
	let testing = $state(false);
	let result = $state<string | null>(null);
	let error = $state<string | null>(null);
	let showRef = $state(false);

	const snippets = [
		'{{if gt .Guild.MemberCount 100}}🎉{{end}}',
		'{{upper .User.Username}}',
		'{{randInt 1 6}}',
		'{{(getRole "Member").mention}}'
	];

	function insert(token: string) {
		const t = el;
		if (!t) {
			value += token;
			return;
		}
		const s = t.selectionStart ?? value.length;
		const e = t.selectionEnd ?? value.length;
		value = value.slice(0, s) + token + value.slice(e);
		queueMicrotask(() => {
			t.focus();
			const pos = s + token.length;
			t.setSelectionRange(pos, pos);
		});
	}

	async function test() {
		testing = true;
		error = null;
		result = null;
		try {
			const r = await api.templatingPreview(guildId, value, extraVars, sample);
			if (r.error) error = r.error;
			else result = r.rendered;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Preview failed';
		} finally {
			testing = false;
		}
	}
</script>

<div class="space-y-2">
	{#if label}<div class="label">{label}</div>{/if}
	<textarea
		bind:this={el}
		bind:value
		{placeholder}
		{rows}
		class="input w-full resize-y font-mono text-[13px] leading-relaxed"
	></textarea>

	<div class="flex items-center justify-between gap-2">
		<button
			type="button"
			onclick={() => (showRef = !showRef)}
			class="flex items-center gap-1 text-[11px] text-muted transition-colors hover:text-ink"
		>
			<ChevronDown size={12} class="transition-transform {showRef ? 'rotate-180' : ''}" />
			Variables &amp; functions
		</button>
		<button
			type="button"
			onclick={test}
			disabled={testing || !value.trim()}
			class="flex items-center gap-1.5 rounded-md border border-line-strong bg-ink-2 px-2.5 py-1 text-[11px] font-medium text-ink transition-colors hover:border-faint disabled:opacity-40"
		>
			{#if testing}<Loader2 size={12} class="animate-spin" />{:else}<Sparkles size={12} />{/if}
			Test render
		</button>
	</div>

	{#if error}
		<div class="rounded-md border border-accent/40 bg-accent/5 px-2.5 py-1.5 font-mono text-[11px] text-accent-ink">
			{error}
		</div>
	{:else if result !== null}
		<div class="rounded-md border border-line bg-ink-2 px-2.5 py-1.5 text-[13px] whitespace-pre-wrap text-ink">
			{result || '(empty output)'}
		</div>
	{/if}

	{#if showRef}
		<div class="space-y-2 rounded-lg border border-line bg-surface p-2.5">
			<div class="flex flex-wrap gap-1.5">
				{#each variables as v (v.token)}
					<button
						type="button"
						title={v.desc}
						onclick={() => insert(v.token)}
						class="rounded-md border border-line bg-ink-2 px-1.5 py-1 font-mono text-[11px] text-muted transition-colors hover:border-line-strong hover:text-ink"
					>
						{v.token}
					</button>
				{/each}
			</div>
			<div class="border-t border-line pt-2">
				<div class="mb-1.5 text-[10px] font-medium uppercase tracking-wide text-faint">Logic snippets</div>
				<div class="flex flex-wrap gap-1.5">
					{#each snippets as sn (sn)}
						<button
							type="button"
							onclick={() => insert(sn)}
							class="rounded-md border border-line bg-ink-2 px-1.5 py-1 font-mono text-[11px] text-muted transition-colors hover:border-line-strong hover:text-ink"
						>
							{sn}
						</button>
					{/each}
				</div>
			</div>
		</div>
	{/if}
</div>
