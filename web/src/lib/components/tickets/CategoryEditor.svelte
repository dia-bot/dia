<script lang="ts">
	import type { CategoryConfig } from '$lib/tickets/types';
	import { newFormField, TICKET_TEMPLATE_VARS } from '$lib/tickets/types';
	import Field from '$lib/components/Field.svelte';
	import Select from '$lib/components/Select.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import NumberField from '$lib/components/ui/NumberField.svelte';
	import TemplateField from '$lib/components/TemplateField.svelte';
	import EmbedBuilder from '$lib/components/commands/EmbedBuilder.svelte';
	import AutomationPicker from '$lib/components/commands/AutomationPicker.svelte';
	import { ChevronDown, ChevronRight, Trash2, Plus } from 'lucide-svelte';

	let {
		category,
		guildId,
		index,
		onRemove
	}: {
		category: CategoryConfig;
		guildId: string;
		index: number;
		onRemove: () => void;
	} = $props();

	// svelte-ignore state_referenced_locally
	let open = $state(index === 0); // first category expanded by default; toggled thereafter

	const styleOptions = [
		{ value: 'primary', label: 'Primary (blurple)' },
		{ value: 'secondary', label: 'Secondary (grey)' },
		{ value: 'success', label: 'Success (green)' },
		{ value: 'danger', label: 'Danger (red)' }
	];
	const modeOptions = [
		{ value: 'channel', label: 'Private channel' },
		{ value: 'thread', label: 'Private thread' }
	];
	const inputCls =
		'w-full rounded-md border border-line bg-bg px-3 py-2 text-sm text-ink outline-none focus:border-line-strong';

	function addFormField() {
		if (!category.form) category.form = [];
		if (category.form.length >= 5) return;
		category.form = [...category.form, newFormField()];
	}
	function removeFormField(i: number) {
		category.form = (category.form ?? []).filter((_, idx) => idx !== i);
	}
</script>

<div class="rounded-lg border border-line bg-surface">
	<div class="flex items-center gap-3 px-4 py-3">
		<button
			type="button"
			class="flex flex-1 items-center gap-2 text-left"
			onclick={() => (open = !open)}
		>
			{#if open}<ChevronDown class="h-4 w-4 text-muted" />{:else}<ChevronRight
					class="h-4 w-4 text-muted"
				/>{/if}
			<span class="text-lg">{category.emoji || '🎫'}</span>
			<span class="font-medium text-ink">{category.label || 'Untitled category'}</span>
			<span class="text-xs text-faint">· {category.open_mode === 'thread' ? 'thread' : 'channel'}</span>
		</button>
		<button
			type="button"
			class="text-muted hover:text-accent-ink"
			title="Remove category"
			onclick={onRemove}
		>
			<Trash2 class="h-4 w-4" />
		</button>
	</div>

	{#if open}
		<div class="space-y-5 border-t border-line px-4 py-4">
			<!-- Button / entry -->
			<div class="grid gap-4 sm:grid-cols-2">
				<Field label="Label"><input class={inputCls} bind:value={category.label} placeholder="General support" /></Field>
				<Field label="Emoji" hint="Unicode or <:name:id>"><input class={inputCls} bind:value={category.emoji} placeholder="🎫" /></Field>
				<Field label="Description" hint="Shown on the select option"><input class={inputCls} bind:value={category.description} placeholder="Questions and general help" /></Field>
				<Field label="Button color"><Select bind:value={category.button_style} options={styleOptions} /></Field>
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
				<Field label="Message content" hint="Posted in the new ticket channel">
					<TemplateField
						{guildId}
						value={category.welcome.content ?? ''}
						variables={[]}
						extraVars={{}}
						rows={2}
						placeholder={'{{ .User.Mention }}'}
					/>
				</Field>
				<label class="flex items-center gap-3 text-sm text-ink">
					<Toggle bind:checked={category.welcome.use_embed} label="Use embed" />
					Include an embed
				</label>
				{#if category.welcome.use_embed}
					<EmbedBuilder embed={category.welcome.embed} onChange={(next) => (category.welcome.embed = next)} />
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

			<!-- Transcript + feedback + auto-close -->
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
					<Field label="Prompt"><input class={inputCls} bind:value={category.feedback.prompt} placeholder="How was your support experience?" /></Field>
				</div>
			</div>

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
				{/if}
			</div>

			<!-- Automations -->
			<div class="grid gap-4 border-t border-line pt-4 sm:grid-cols-2">
				<Field label="Run automation on open" hint="Launches a saved automation">
					<AutomationPicker value={category.on_open_automation ?? ''} onChange={(v) => (category.on_open_automation = v)} />
				</Field>
				<Field label="Run automation on close">
					<AutomationPicker value={category.on_close_automation ?? ''} onChange={(v) => (category.on_close_automation = v)} />
				</Field>
			</div>
		</div>
	{/if}
</div>
