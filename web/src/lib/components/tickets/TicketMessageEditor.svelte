<script lang="ts">
	// Binds one ticket MessageSpec to the shared WYSIWYG MessageEditor (the same
	// composer welcome / giveaways use). Content, embeds and button rows are
	// edited in place; clicking a button exposes an inline action picker (the
	// giveaway pattern): a system action for this surface (Claim / Close /
	// Reopen / …), one of your saved automations, or nothing. A link-style
	// button just opens its URL.
	import type { MessageSpec, TicketComponent } from '$lib/tickets/types';
	import type { Step } from '$lib/commands/types';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import AutomationPicker from '$lib/components/commands/AutomationPicker.svelte';

	let {
		spec,
		id,
		bindings = []
	}: {
		spec: MessageSpec;
		id: string;
		// The system actions composable buttons can be wired to on this surface,
		// e.g. [{ value: 'claim', label: 'Claim ticket' }, { value: 'close', … }].
		bindings?: { value: string; label: string }[];
	} = $props();

	// The editor owns a Step whose spec starts from the bound MessageSpec; the
	// effect below copies edits back. It only reads step.spec (the editor swaps
	// in a fresh object per change), so writing `spec` can't loop it.
	// svelte-ignore state_referenced_locally
	let step = $state<Step>({
		id: 'tkt-' + id,
		kind: 'send_message',
		spec: {
			content: spec.content ?? '',
			embeds: JSON.parse(JSON.stringify(spec.embeds ?? [])),
			components: JSON.parse(JSON.stringify(spec.components ?? []))
		}
	});
	$effect(() => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const s = step.spec as any;
		spec.content = s.content ?? '';
		spec.embeds = s.embeds ?? [];
		spec.components = s.components ?? [];
	});

	// A button's current mode: a system binding, a saved automation, or nothing.
	function buttonMode(suffix: string): string {
		const b = spec.button_bindings?.[suffix];
		if (b) return 'bind:' + b;
		if (spec.button_actions && suffix in spec.button_actions) return 'auto';
		return 'none';
	}
	function setButtonMode(suffix: string, mode: string) {
		if (!spec.button_bindings) spec.button_bindings = {};
		if (!spec.button_actions) spec.button_actions = {};
		if (mode.startsWith('bind:')) {
			spec.button_bindings[suffix] = mode.slice('bind:'.length);
			delete spec.button_actions[suffix];
			spec.button_actions = { ...spec.button_actions };
		} else {
			delete spec.button_bindings[suffix];
			spec.button_bindings = { ...spec.button_bindings };
			if (mode === 'auto') {
				if (!(suffix in spec.button_actions)) spec.button_actions[suffix] = '';
			} else {
				delete spec.button_actions[suffix];
				spec.button_actions = { ...spec.button_actions };
			}
		}
	}
	const radioCls = (active: boolean) =>
		`flex-1 rounded px-2 py-0.5 text-[10px] font-medium transition-colors ${active ? 'bg-secondary text-foreground' : 'text-muted-foreground hover:text-foreground'}`;
</script>

{#snippet buttonAction({ component }: { component: TicketComponent; ri: number; ci: number })}
	{@const suffix = component.custom_id_suffix}
	{#if suffix && component.style !== 'link' && !component.url}
		{@const mode = buttonMode(suffix)}
		<div class="mt-2 space-y-1.5">
			<span class="font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground">Action</span>
			<div class="flex flex-wrap rounded-md border border-input p-0.5" role="radiogroup" aria-label="Button action">
				{#each bindings as b (b.value)}
					<button
						type="button"
						role="radio"
						aria-checked={mode === 'bind:' + b.value}
						class={radioCls(mode === 'bind:' + b.value)}
						onclick={() => setButtonMode(suffix, 'bind:' + b.value)}
					>
						{b.label}
					</button>
				{/each}
				<button
					type="button"
					role="radio"
					aria-checked={mode === 'auto'}
					class={radioCls(mode === 'auto')}
					onclick={() => setButtonMode(suffix, 'auto')}
				>
					Run automation
				</button>
				<button
					type="button"
					role="radio"
					aria-checked={mode === 'none'}
					class={radioCls(mode === 'none')}
					onclick={() => setButtonMode(suffix, 'none')}
				>
					Nothing
				</button>
			</div>
			{#if mode === 'auto'}
				<AutomationPicker
					value={spec.button_actions?.[suffix] ?? ''}
					onChange={(v) => {
						if (!spec.button_actions) spec.button_actions = {};
						spec.button_actions[suffix] = v;
					}}
				/>
			{/if}
		</div>
	{/if}
{/snippet}

<MessageEditor {step} embeds components clickPaths={false} buttonExtras={buttonAction} />
