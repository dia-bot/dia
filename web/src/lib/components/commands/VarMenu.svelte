<script lang="ts">
	// "Insert value" menu for templated message fields. Every string in a
	// custom command renders through Go text/template, so this offers the
	// values in scope: the command's properties ({{ .Input.<name> }}), declared
	// variables ({{ .Vars.<name> }}), and the user / server / channel context.
	import { getContext } from 'svelte';
	import {
		EXPR_SCOPE_CTX,
		TMPL_STATIC_VARS,
		collectProducedVars,
		type ExprScope
	} from '$lib/commands/expr-meta';
	import { Popover } from '$lib/components/ui';

	import Braces from 'lucide-svelte/icons/braces';

	let {
		onPick
	}: {
		onPick: (token: string) => void;
	} = $props();

	let open = $state(false);

	const scope = getContext<ExprScope | undefined>(EXPR_SCOPE_CTX);

	const groups = $derived.by(() => {
		const out: { label: string; items: { token: string; short: string }[] }[] = [];
		const opts = scope?.options ?? [];
		if (opts.length > 0) {
			out.push({
				label: 'Properties',
				items: opts
					.filter((o) => o.name)
					.map((o) => ({ token: `{{ .Input.${o.name} }}`, short: o.description || o.kind }))
			});
		}
		const extra = scope?.extraVars ?? [];
		if (extra.length > 0) {
			out.push({
				label: 'Trigger',
				items: extra.map((v) => ({ token: `{{ ${v.path} }}`, short: v.short }))
			});
		}
		const vars = scope?.variables ?? [];
		if (vars.length > 0) {
			out.push({
				label: 'Variables',
				items: vars
					.filter((v) => v.name)
					.map((v) => ({ token: `{{ .Vars.${v.name} }}`, short: v.type }))
			});
		}
		// Values earlier steps save (a sent message, a fetched member, a form
		// answer, …) and their fields, so they can be dropped straight in here.
		const stepVars = collectProducedVars(scope?.steps);
		if (stepVars.length > 0) {
			out.push({
				label: 'From earlier steps',
				items: stepVars.map((v) => ({ token: `{{ ${v.path} }}`, short: v.short }))
			});
		}
		out.push({
			label: 'Context',
			items: TMPL_STATIC_VARS.map((v) => ({ token: `{{ ${v.path} }}`, short: v.short }))
		});
		return out;
	});

	function pick(token: string) {
		open = false;
		onPick(token);
	}
</script>

<Popover.Root bind:open>
	<Popover.Trigger
		class="inline-flex h-6 items-center gap-1 rounded border border-line bg-bg px-1.5 font-mono text-[10px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink"
	>
		<Braces size={10} />
		Insert value
	</Popover.Trigger>
	<Popover.Content class="max-h-72 w-72 overflow-y-auto p-1" align="end">
		{#each groups as g (g.label)}
			<div
				class="px-2 pb-0.5 pt-2 font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground"
			>
				{g.label}
			</div>
			{#each g.items as item (item.token)}
				<button
					type="button"
					class="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left transition-colors hover:bg-secondary"
					onclick={() => pick(item.token)}
				>
					<code class="shrink-0 font-mono text-[10.5px] text-foreground">{item.token}</code>
					<span class="min-w-0 truncate text-[10.5px] text-muted-foreground">{item.short}</span>
				</button>
			{/each}
		{/each}
	</Popover.Content>
</Popover.Root>
