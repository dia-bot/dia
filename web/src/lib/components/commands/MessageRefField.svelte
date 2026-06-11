<script lang="ts">
	// Message-target field: a template input PLUS a "from a previous step"
	// picker. Picking an earlier message-producing step auto-names its output
	// (sets `into` if missing) and fills this field with {{ .Vars.x.id }} —
	// no hand-wiring of variables. Channel can be auto-filled alongside.
	import { getContext } from 'svelte';
	import { EXPR_SCOPE_CTX, type ExprScope } from '$lib/commands/expr-meta';
	import { STEP_KIND_BY_KIND, type Expr, type Step } from '$lib/commands/types';
	import ExprField from './ExprField.svelte';
	import { Popover } from '$lib/components/ui';

	import Link2 from 'lucide-svelte/icons/link-2';

	let {
		step,
		value,
		onChange,
		onChannel,
		placeholder = ''
	}: {
		step: Step; // the step being edited (references must come BEFORE it)
		value: Expr | undefined;
		onChange: (v: Expr) => void;
		// When provided, picking a step also fills the channel field.
		onChannel?: (v: Expr) => void;
		placeholder?: string;
	} = $props();

	const scope = getContext<ExprScope | undefined>(EXPR_SCOPE_CTX);

	// Steps whose output is a message reference ({id, channel_id}).
	const PRODUCERS = new Set(['send_message', 'embed_send', 'message_fetch']);

	type Producer = { step: Step; label: string; detail: string };

	// Walk the tree in execution order, collecting producers until we reach
	// the step being edited.
	const producers = $derived.by(() => {
		const out: Producer[] = [];
		let done = false;
		const walk = (steps: Step[] | undefined) => {
			for (const s of steps ?? []) {
				if (done) return;
				if (s.id === step.id) {
					done = true;
					return;
				}
				if (PRODUCERS.has(s.kind)) {
					const meta = STEP_KIND_BY_KIND.get(s.kind);
					// eslint-disable-next-line @typescript-eslint/no-explicit-any
					const sp = (s.spec ?? {}) as any;
					out.push({
						step: s,
						label: meta?.label ?? s.kind,
						detail: sp.content || sp.embed?.title || sp.into || ''
					});
				}
				walk(s.then);
				walk(s.else);
				for (const c of s.cases ?? []) walk(c.do);
				walk(s.default);
				walk(s.on_error);
				for (const ec of s.on_error_cases ?? []) walk(ec.do);
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
				for (const br of ((s.spec ?? {}) as any).branches ?? []) walk(br as Step[]);
			}
		};
		walk(scope?.steps);
		return out;
	});

	let open = $state(false);

	function pick(p: Producer) {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const sp = (p.step.spec ?? {}) as any;
		let into: string = sp.into ?? '';
		if (!into) {
			into = `msg_${p.step.id.slice(-4).toLowerCase()}`;
			p.step.spec = { ...sp, into };
		}
		onChange({ lang: 'tmpl', src: `{{ .Vars.${into}.id }}` });
		onChannel?.({ lang: 'tmpl', src: `{{ .Vars.${into}.channel_id }}` });
		open = false;
	}
</script>

<div class="flex items-start gap-1.5">
	<div class="min-w-0 flex-1">
		<ExprField {value} {onChange} {placeholder} />
	</div>
	{#if producers.length > 0}
		<Popover.Root bind:open>
			<Popover.Trigger
				class="grid h-8 w-8 shrink-0 place-items-center rounded-md border border-line bg-bg text-muted transition-colors hover:border-line-strong hover:text-ink data-[state=open]:border-line-strong"
			>
				<Link2 size={13} />
			</Popover.Trigger>
			<Popover.Content class="w-72 p-1" align="end">
				<div
					class="px-2 pb-1 pt-1.5 font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground"
				>
					From a previous step
				</div>
				{#each producers as p (p.step.id)}
					<button
						type="button"
						class="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left transition-colors hover:bg-secondary"
						onclick={() => pick(p)}
					>
						<span class="shrink-0 text-[11.5px] font-medium text-foreground">{p.label}</span>
						<span class="min-w-0 flex-1 truncate font-mono text-[10px] text-muted-foreground">
							{p.detail}
						</span>
					</button>
				{/each}
			</Popover.Content>
		</Popover.Root>
	{/if}
</div>
