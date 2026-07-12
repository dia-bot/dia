<script lang="ts">
	// The full editor for one ticket type (category): its panel button, channel
	// behaviour, access, every composed message surface, form, transcript,
	// feedback, auto-close and automation hooks. Pure form — the host (the
	// ticket-type modal) provides the chrome.
	import type { CategoryConfig, MessageSpec } from '$lib/tickets/types';
	import { newFormField } from '$lib/tickets/types';
	import Field from '$lib/components/Field.svelte';
	import Select from '$lib/components/Select.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import NumberField from '$lib/components/ui/NumberField.svelte';
	import TemplateField from '$lib/components/TemplateField.svelte';
	import AutomationPicker from '$lib/components/commands/AutomationPicker.svelte';
	import TicketMessageEditor from '$lib/components/tickets/TicketMessageEditor.svelte';
	import { ChevronDown, ChevronRight, Trash2, Plus } from 'lucide-svelte';

	let {
		category,
		guildId,
		panelStyle = 'buttons'
	}: {
		category: CategoryConfig;
		guildId: string;
		// The parent setup's layout: with a dropdown the option's emoji and
		// description are edited here; with buttons the panel composer owns the
		// button's whole look.
		panelStyle?: string;
	} = $props();

	// Advanced message surfaces are collapsed until needed (the built-in message
	// is used while a surface stays uncomposed).
	let showClosed = $state(false);
	let showCloseReq = $state(false);
	let showWarn = $state(false);
	let showButtons = $state(false);

	const styleOptions = [
		{ value: 'primary', label: 'Primary (blurple)' },
		{ value: 'secondary', label: 'Secondary (grey)' },
		{ value: 'success', label: 'Success (green)' },
		{ value: 'danger', label: 'Danger (red)' }
	];
	const sysStyleOptions = [{ value: '', label: 'Default' }, ...styleOptions];
	const modeOptions = [
		{ value: 'channel', label: 'Private channel' },
		{ value: 'thread', label: 'Private thread' }
	];
	const inputCls =
		'w-full rounded-md border border-line bg-bg px-3 py-2 text-sm text-ink outline-none focus:border-line-strong';

	// What composed buttons can be wired to, per surface.
	const welcomeBindings = [
		{ value: 'claim', label: 'Claim ticket' },
		{ value: 'close', label: 'Close ticket' }
	];
	const closedBindings = [
		{ value: 'reopen', label: 'Reopen' },
		{ value: 'delete', label: 'Delete channel' },
		{ value: 'transcript', label: 'Transcript' }
	];
	const closeReqBindings = [
		{ value: 'accept', label: 'Accept & close' },
		{ value: 'deny', label: 'Keep open' }
	];

	// Warn when the opening message composes buttons but none closes the ticket
	// (there is no /ticket close; without a Close binding the ticket can only be
	// closed via a close request or auto-close).
	function actionableButtons(spec: MessageSpec): string[] {
		return (spec.components ?? [])
			.flatMap((r) => r.components ?? [])
			.filter((c) => (c.type ?? 'button') === 'button' && c.style !== 'link' && !c.url && !!c.custom_id_suffix)
			.map((c) => c.custom_id_suffix as string);
	}
	const welcomeMissingClose = $derived.by(() => {
		const suffixes = actionableButtons(category.welcome);
		if (suffixes.length === 0) return false; // system row fallback covers it
		return !suffixes.some((s) => category.welcome.button_bindings?.[s] === 'close');
	});

	// The fallback system row (used when a surface composes no buttons) can still
	// be restyled here. Close never hides; Claim visibility is the toggle above.
	const sysButtons: { key: keyof CategoryConfig['buttons']; label: string; hideable: boolean; hint: string }[] = [
		{ key: 'claim', label: 'Claim', hideable: false, hint: 'On the opening message' },
		{ key: 'close', label: 'Close', hideable: false, hint: 'On the opening message' },
		{ key: 'reopen', label: 'Reopen', hideable: true, hint: 'On the closed message' },
		{ key: 'delete', label: 'Delete', hideable: true, hint: 'On the closed message' },
		{ key: 'transcript', label: 'Transcript', hideable: true, hint: 'On the closed message' }
	];

	function addFormField() {
		if (!category.form) category.form = [];
		if (category.form.length >= 5) return;
		category.form = [...category.form, newFormField()];
	}
	function removeFormField(i: number) {
		category.form = (category.form ?? []).filter((_, idx) => idx !== i);
	}
</script>

<div class="space-y-5">
	<!-- Identity. The panel button's look (label, emoji, colour) is edited on the
	     button itself in the panel composer; only the dropdown layout needs its
	     option fields here. -->
	<div class="grid gap-4 sm:grid-cols-2">
		<Field label="Name" hint="Shown in the queue, logs and {'{{ .Ticket.Category }}'}">
			<input class={inputCls} bind:value={category.label} placeholder="General support" />
		</Field>
		{#if panelStyle === 'select'}
			<Field label="Option emoji" hint="On the dropdown option">
				<input class={inputCls} bind:value={category.emoji} placeholder="🎫" />
			</Field>
			<Field label="Option description" hint="Shown under the dropdown option">
				<input class={inputCls} bind:value={category.description} placeholder="Questions and general help" />
			</Field>
		{/if}
	</div>

	<!-- Channel / thread -->
	<div class="grid gap-4 sm:grid-cols-2">
		<Field label="Open as"><Select bind:value={category.open_mode} options={modeOptions} /></Field>
		<Field label="Channel category" hint="Where channel-mode tickets are created">
			<ChannelPicker
				kind="all"
				value={category.parent_id ?? ''}
				placeholder="Server default"
				onChange={(v) => (category.parent_id = v as string)}
			/>
		</Field>
		<Field label="Channel name" hint="A template — {'{{ .Ticket.Number }}'}">
			<TemplateField
				{guildId}
				value={category.name_scheme ?? ''}
				variables={[]}
				extraVars={{}}
				rows={1}
				placeholder={'ticket-{{ printf "%04d" .Ticket.Number }}'}
			/>
		</Field>
	</div>

	<!-- Access + limits -->
	<div class="grid gap-4 sm:grid-cols-2">
		<Field label="Support roles" hint="Added to every ticket of this type">
			<RolePicker
				multiple
				value={category.support_role_ids ?? []}
				onChange={(v) => (category.support_role_ids = v as string[])}
			/>
		</Field>
		<Field label="Ping on open" hint="Roles pinged when a ticket opens">
			<RolePicker
				multiple
				value={category.ping_role_ids ?? []}
				onChange={(v) => (category.ping_role_ids = v as string[])}
			/>
		</Field>
		<Field label="Required roles" hint="Member must hold one to open (blank = anyone)">
			<RolePicker
				multiple
				value={category.required_role_ids ?? []}
				onChange={(v) => (category.required_role_ids = v as string[])}
			/>
		</Field>
		<div class="grid grid-cols-2 gap-3">
			<Field label="Max open / user" hint="0 = unlimited"><NumberField bind:value={category.max_open_per_user} min={0} /></Field>
			<Field label="Cooldown (s)" hint="0 = none"><NumberField bind:value={category.cooldown_seconds} min={0} /></Field>
		</div>
	</div>

	<label class="flex items-center gap-3 text-sm text-ink">
		<Toggle bind:checked={category.claim_enabled} label="Claiming" />
		Let staff claim this ticket type
	</label>
	<label class="flex items-center gap-3 text-sm text-ink">
		<Toggle bind:checked={category.ping_opener} label="Ping opener" />
		Ping the opener in the first message
	</label>

	<!-- Opening message -->
	<div class="space-y-3 border-t border-line pt-4">
		<p class="eyebrow">Opening message</p>
		<p class="text-xs text-muted">
			Posted in the new ticket. Compose it like any message — click a button to set what it does:
			claim, close, run one of your automations, or open a link.
		</p>
		{#if welcomeMissingClose}
			<div class="rounded-md border border-accent/40 bg-accent/5 px-2.5 py-1.5 text-[12px] text-accent-ink">
				No button closes the ticket. Set one button's action to <span class="font-medium">Close ticket</span>,
				or the ticket can only be closed with a close request or auto-close.
			</div>
		{/if}
		<TicketMessageEditor spec={category.welcome} id={category.id + '-welcome'} bindings={welcomeBindings} />
	</div>

	<!-- Closed message -->
	<div class="space-y-3 border-t border-line pt-4">
		<button type="button" class="flex items-center gap-2 text-left" onclick={() => (showClosed = !showClosed)}>
			{#if showClosed}<ChevronDown class="h-4 w-4 text-muted" />{:else}<ChevronRight class="h-4 w-4 text-muted" />{/if}
			<p class="eyebrow">Closed message <span class="text-faint">(blank = built-in card)</span></p>
		</button>
		{#if showClosed}
			<p class="text-xs text-muted">
				Posted when the ticket closes. Use {'{{ .Ticket.Closer }}'} and {'{{ .Ticket.Reason }}'}.
				Buttons can reopen, delete, generate a transcript, or run an automation; compose none and the
				built-in Reopen / Delete / Transcript row is added for you.
			</p>
			<TicketMessageEditor spec={category.closed} id={category.id + '-closed'} bindings={closedBindings} />
		{/if}
	</div>

	<!-- Close request message -->
	<div class="space-y-3 border-t border-line pt-4">
		<button type="button" class="flex items-center gap-2 text-left" onclick={() => (showCloseReq = !showCloseReq)}>
			{#if showCloseReq}<ChevronDown class="h-4 w-4 text-muted" />{:else}<ChevronRight class="h-4 w-4 text-muted" />{/if}
			<p class="eyebrow">Close request message <span class="text-faint">(blank = built-in card)</span></p>
		</button>
		{#if showCloseReq}
			<p class="text-xs text-muted">
				Posted by /ticket closerequest to ask the opener to confirm. Use {'{{ .Actor.Mention }}'}
				(the requester), {'{{ .Ticket.Reason }}'} and {'{{ .Ticket.Deadline }}'}. Buttons can accept
				or decline; compose none and the built-in Accept / Keep-open row is added for you.
			</p>
			<TicketMessageEditor spec={category.close_request} id={category.id + '-closereq'} bindings={closeReqBindings} />
		{/if}
	</div>

	<!-- Fallback system buttons -->
	<div class="space-y-3 border-t border-line pt-4">
		<button type="button" class="flex items-center gap-2 text-left" onclick={() => (showButtons = !showButtons)}>
			{#if showButtons}<ChevronDown class="h-4 w-4 text-muted" />{:else}<ChevronRight class="h-4 w-4 text-muted" />{/if}
			<p class="eyebrow">Fallback control buttons <span class="text-faint">(used when a message composes no buttons)</span></p>
		</button>
		{#if showButtons}
			<div class="space-y-2">
				{#each sysButtons as sb (sb.key)}
					<div class="grid items-end gap-3 rounded-md border border-line p-3 sm:grid-cols-[1fr_1fr_1fr_auto]">
						<Field label={sb.label} hint={sb.hint}>
							<input class={inputCls} bind:value={category.buttons[sb.key].label} placeholder={sb.label} />
						</Field>
						<Field label="Emoji"><input class={inputCls} bind:value={category.buttons[sb.key].emoji} placeholder="—" /></Field>
						<Field label="Style"><Select bind:value={category.buttons[sb.key].style} options={sysStyleOptions} /></Field>
						{#if sb.hideable}
							<label class="flex items-center gap-2 pb-2 text-sm text-ink">
								<Toggle bind:checked={category.buttons[sb.key].hide} label="Hide" /> Hide
							</label>
						{:else}
							<div></div>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</div>

	<!-- Pre-open form -->
	<div class="space-y-3 border-t border-line pt-4">
		<div class="flex items-center justify-between">
			<p class="eyebrow">Pre-open form <span class="text-faint">(up to 5 questions)</span></p>
			<button
				type="button"
				class="flex items-center gap-1 text-xs text-accent-ink hover:underline disabled:opacity-40"
				disabled={(category.form?.length ?? 0) >= 5}
				onclick={addFormField}
			>
				<Plus class="h-3.5 w-3.5" /> Add question
			</button>
		</div>
		{#each category.form ?? [] as ff, i (ff.id)}
			<div class="rounded-md border border-line p-3">
				<div class="grid gap-3 sm:grid-cols-2">
					<Field label="Question"><input class={inputCls} bind:value={ff.label} placeholder="What do you need help with?" /></Field>
					<Field label="Placeholder"><input class={inputCls} bind:value={ff.placeholder} /></Field>
					<Field label="Style">
						<Select
							bind:value={ff.style}
							options={[
								{ value: 'short', label: 'Short' },
								{ value: 'paragraph', label: 'Paragraph' }
							]}
						/>
					</Field>
					<div class="flex items-end justify-between">
						<label class="flex items-center gap-2 text-sm text-ink">
							<Toggle bind:checked={ff.required} label="Required" /> Required
						</label>
						<button type="button" class="text-muted hover:text-accent-ink" onclick={() => removeFormField(i)}>
							<Trash2 class="h-4 w-4" />
						</button>
					</div>
				</div>
			</div>
		{/each}
	</div>

	<!-- Transcript + feedback -->
	<div class="grid gap-4 border-t border-line pt-4 sm:grid-cols-2">
		<div class="space-y-2">
			<p class="eyebrow">Transcript</p>
			<label class="flex items-center gap-2 text-sm text-ink">
				<Toggle bind:checked={category.transcript.enabled} label="Transcript" /> Generate on close
			</label>
			<label class="flex items-center gap-2 text-sm text-ink">
				<Toggle bind:checked={category.transcript.dm_opener} label="DM transcript" /> DM it to the opener
			</label>
		</div>
		<div class="space-y-2">
			<p class="eyebrow">Feedback</p>
			<label class="flex items-center gap-2 text-sm text-ink">
				<Toggle bind:checked={category.feedback.enabled} label="Feedback" /> Ask for a rating on close
			</label>
			<Field label="Prompt" hint="Used by the built-in DM card">
				<input class={inputCls} bind:value={category.feedback.prompt} placeholder="How was your support experience?" />
			</Field>
			<Field label="Thanks reply" hint="Shown after rating (blank = star summary)">
				<input class={inputCls} bind:value={category.feedback.thanks_message} placeholder={'Thanks! You rated us {{ .Ticket.Rating }}/5.'} />
			</Field>
		</div>
	</div>
	{#if category.feedback.enabled}
		<div class="space-y-2">
			<p class="text-xs text-muted">
				Compose the DM sent above the rating select (blank = built-in card with the prompt).
			</p>
			<TicketMessageEditor spec={category.feedback.message} id={category.id + '-feedback'} />
		</div>
	{/if}

	<!-- Auto-close -->
	<div class="space-y-2 border-t border-line pt-4">
		<p class="eyebrow">Auto-close on inactivity</p>
		<label class="flex items-center gap-2 text-sm text-ink">
			<Toggle bind:checked={category.auto_close.enabled} label="Auto-close" /> Close inactive tickets automatically
		</label>
		{#if category.auto_close.enabled}
			<div class="grid grid-cols-2 gap-3">
				<Field label="Inactive minutes"><NumberField bind:value={category.auto_close.inactivity_minutes} min={5} /></Field>
				<Field label="Warn grace (min)" hint="0 = close without warning"><NumberField bind:value={category.auto_close.warn_minutes} min={0} /></Field>
			</div>
			<div class="space-y-2 pt-1">
				<button type="button" class="flex items-center gap-2 text-left" onclick={() => (showWarn = !showWarn)}>
					{#if showWarn}<ChevronDown class="h-4 w-4 text-muted" />{:else}<ChevronRight class="h-4 w-4 text-muted" />{/if}
					<p class="eyebrow">Warning message <span class="text-faint">(blank = built-in line)</span></p>
				</button>
				{#if showWarn}
					<TicketMessageEditor spec={category.auto_close.warn_message} id={category.id + '-warn'} />
				{/if}
			</div>
		{/if}
	</div>

	<!-- Automations -->
	<div class="space-y-3 border-t border-line pt-4">
		<p class="eyebrow">Automations</p>
		<p class="text-xs text-muted">
			Launch a saved automation at any moment of this ticket type's lifecycle. The flow sees the
			same variables as the matching ticket trigger, so one automation works from either.
		</p>
		<div class="grid gap-4 sm:grid-cols-2">
			<Field label="On open">
				<AutomationPicker value={category.on_open_automation ?? ''} onChange={(v) => (category.on_open_automation = v)} />
			</Field>
			<Field label="On claim">
				<AutomationPicker value={category.on_claim_automation ?? ''} onChange={(v) => (category.on_claim_automation = v)} />
			</Field>
			<Field label="On close request">
				<AutomationPicker value={category.on_close_request_automation ?? ''} onChange={(v) => (category.on_close_request_automation = v)} />
			</Field>
			<Field label="On reopen">
				<AutomationPicker value={category.on_reopen_automation ?? ''} onChange={(v) => (category.on_reopen_automation = v)} />
			</Field>
			<Field label="On close">
				<AutomationPicker value={category.on_close_automation ?? ''} onChange={(v) => (category.on_close_automation = v)} />
			</Field>
			<Field label="On rating">
				<AutomationPicker value={category.on_rate_automation ?? ''} onChange={(v) => (category.on_rate_automation = v)} />
			</Field>
		</div>
	</div>
</div>
