<script lang="ts">
	// Binds one ticket MessageSpec to the shared WYSIWYG MessageEditor (the same
	// composer welcome / giveaways use). Content, embeds and button rows are
	// edited in place; clicking a non-link button exposes an automation picker so
	// the button can launch a saved automation (spec.button_actions), mirroring
	// the giveaway editor's action buttons.
	import type { MessageSpec, TicketComponent } from '$lib/tickets/types';
	import type { Step } from '$lib/commands/types';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import AutomationPicker from '$lib/components/commands/AutomationPicker.svelte';

	let { spec, id }: { spec: MessageSpec; id: string } = $props();

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

	function actionFor(suffix: string): string {
		return spec.button_actions?.[suffix] ?? '';
	}
	function setAction(suffix: string, autoId: string) {
		if (!spec.button_actions) spec.button_actions = {};
		if (autoId) spec.button_actions[suffix] = autoId;
		else delete spec.button_actions[suffix];
	}
</script>

{#snippet buttonAction({ component }: { component: TicketComponent; ri: number; ci: number })}
	{@const suffix = component.custom_id_suffix}
	{#if suffix && component.style !== 'link' && !component.url}
		<div class="mt-2 space-y-1.5">
			<span class="font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
				Run automation on click
			</span>
			<AutomationPicker value={actionFor(suffix)} onChange={(v) => setAction(suffix, v)} />
		</div>
	{/if}
{/snippet}

<MessageEditor {step} embeds components clickPaths={false} buttonExtras={buttonAction} />
