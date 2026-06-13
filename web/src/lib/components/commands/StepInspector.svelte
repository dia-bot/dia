<script lang="ts">
	import { getContext } from 'svelte';
	import type { Step } from '$lib/commands/types';
	import { STEP_KIND_BY_KIND } from '$lib/commands/types';
	import { AUTOMATION_CTX, EXPR_SCOPE_CTX, stepProducedVar, type ExprScope } from '$lib/commands/expr-meta';
	import ExprField from './ExprField.svelte';
	import StepOutputHint from './StepOutputHint.svelte';
	import MessageEditor from './MessageEditor.svelte';
	import EmbedBuilder from './EmbedBuilder.svelte';
	import EmojiPicker from './EmojiPicker.svelte';
	import NumberField from './NumberField.svelte';
	import MessageRefField from './MessageRefField.svelte';
	import ChannelExprField from './ChannelExprField.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import FieldSelect from './FieldSelect.svelte';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';

	let {
		step,
		embedded = false
	}: {
		step: Step | null;
		// Drawer hosts provide their own header — skip the built-in one.
		embedded?: boolean;
	} = $props();

	// In an automation, waits cap at 1 minute and there's no slash interaction,
	// so a few step editors reword themselves.
	const isAutomation = getContext(AUTOMATION_CTX) === true;
	const scope = getContext<ExprScope | undefined>(EXPR_SCOPE_CTX);

	// Buttons defined earlier in the flow, so a Wait-for can let the admin pick
	// one by its label instead of typing a raw custom id.
	function collectButtons(steps: unknown, out: { value: string; label: string }[]) {
		for (const s of (steps as any[]) ?? []) {
			const sp = (s.spec ?? {}) as any;
			for (const row of sp.components ?? []) {
				for (const c of row.components ?? []) {
					if (c.type === 'button' && c.style !== 'link' && c.on_click !== 'none' && c.custom_id_suffix) {
						out.push({ value: c.custom_id_suffix, label: c.label || c.custom_id_suffix });
					}
				}
			}
			for (const br of [s.then, s.else, s.default, s.on_error]) if (br) collectButtons(br, out);
			for (const cse of s.cases ?? []) collectButtons(cse.do, out);
			for (const ec of s.on_error_cases ?? []) collectButtons(ec.do, out);
			for (const b of sp.branches ?? []) collectButtons(b, out);
		}
	}
	const flowButtons = $derived.by(() => {
		const out: { value: string; label: string }[] = [];
		collectButtons(scope?.steps, out);
		return out;
	});

	// "Remember it as" hint, in plain words per wait kind.
	function rememberHint(wt: string): string {
		if (wt === 'message') return 'Give it a short name so later steps can use their text or reply to it.';
		if (wt === 'reaction') return 'A short name so later steps can use the emoji or who reacted.';
		return 'A short name so later steps know which button was clicked.';
	}

	function getSpec(): any {
		if (!step) return {};
		if (!step.spec) step.spec = {};
		return step.spec as any;
	}

	function set<K extends string>(field: K, value: unknown) {
		if (!step) return;
		const s = getSpec();
		s[field] = value;
		step.spec = { ...s };
	}

	function exprBind(field: string): { value: import('$lib/commands/types').Expr; onChange: (v: import('$lib/commands/types').Expr) => void } {
		return {
			value: ((step?.spec as any)?.[field] ?? { lang: 'tmpl', src: '' }),
			onChange: (v) => set(field, v)
		};
	}

	const kindMeta = $derived(step ? STEP_KIND_BY_KIND.get(step.kind) : null);
	const spec = $derived(getSpec());

	// The variable (and its fields) this step makes available to later steps,
	// resolved live from the spec — drives the "use this later" teaching panel.
	const produced = $derived(stepProducedVar(step));
</script>

{#if !step}
	<div class="flex h-full flex-col items-center justify-center p-6 text-center text-sm text-muted">
		<div class="rounded-full border border-line bg-ink-2 p-3">
			<span class="font-mono text-xs">≡</span>
		</div>
		<p class="mt-3">Select a step on the left to edit its spec.</p>
	</div>
{:else}
	<div class="flex h-full flex-col overflow-y-auto">
		{#if !embedded}
			<div class="border-b border-line p-4">
				<div class="font-mono text-[10px] uppercase tracking-wider text-faint">Step kind</div>
				<div class="mt-1 flex items-center gap-2">
					<span class="text-sm font-semibold text-ink">{kindMeta?.label ?? step.kind}</span>
					<code class="font-mono text-[10px] text-muted">{step.kind}</code>
				</div>
				<p class="mt-1.5 text-xs text-muted">{kindMeta?.short ?? ''}</p>
			</div>
		{/if}

		{#if embedded && kindMeta?.short}
			<!-- Plain "what this does" line so non-technical admins always know
			     what the step they're editing actually does. -->
			<p class="border-b border-line bg-surface/30 px-4 py-2.5 text-[11.5px] leading-snug text-muted">
				{kindMeta.short}
			</p>
		{/if}

		<div class="flex-1 space-y-4 p-4">
			{#if step.kind === 'reply' || step.kind === 'edit_reply'}
				<MessageEditor
					{step}
					ephemeral={step.kind === 'reply'}
					embeds
					components
					attachments
				/>
			{:else if step.kind === 'defer_reply'}
				<label class="flex items-center justify-between gap-3">
					<span class="text-sm">Ephemeral defer</span>
					<Toggle
						checked={!!spec.ephemeral}
						onchange={(v) => set('ephemeral', v)}
					/>
				</label>
				<p class="hint">Defer if your worst-case path exceeds 3 seconds. The validator auto-inserts this when needed.</p>
			{:else if step.kind === 'send_message'}
				<Field label="Channel"><ChannelExprField {...exprBind('channel')} /></Field>
				<MessageEditor {step} embeds components attachments />
				<Field label="Reply-to message id" hint="Optional — make the bot reply to a specific message"><ExprField {...exprBind('reply_to')} placeholder="(none)" /></Field>
				<Field label="Save to variable" hint="The created message id will be stored under this name.">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'send_dm'}
				<Field label="User"><ExprField {...exprBind('user')} placeholder={'{{ .User.ID }}'} /></Field>
				<MessageEditor {step} embeds components attachments />
			{:else if step.kind === 'embed_send'}
				<Field label="Channel"><ChannelExprField {...exprBind('channel')} /></Field>
				<EmbedBuilder
					embed={spec.embed ?? {}}
					onChange={(next) => set('embed', next)}
				/>
				<Field label="Save to variable" hint="The created message id will be stored under this name.">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'message_edit'}
				<Field label="What to edit">
					<FieldSelect
						value={spec.target ?? ''}
						onChange={(v) => set('target', v)}
						options={[
							{ value: 'reply', label: "The command's reply" },
							{ value: '', label: 'A specific message' }
						]}
					/>
				</Field>
				{#if spec.target !== 'reply'}
					<Field label="Message" hint="Pick it from a previous step with the link button, or template an id.">
						<MessageRefField {step} {...exprBind('message')} onChannel={(v) => set('channel', v)} />
					</Field>
					<Field label="Channel"><ChannelExprField {...exprBind('channel')} /></Field>
				{/if}
				<MessageEditor {step} embeds components />
			{:else if step.kind === 'message_fetch'}
				<Field label="Message" hint="Pick from a previous step, or template an id.">
					<MessageRefField {step} {...exprBind('message')} onChannel={(v) => set('channel', v)} />
				</Field>
				<Field label="Channel"><ChannelExprField {...exprBind('channel')} /></Field>
				<Field label="Save to variable" hint="Name it, then use its text, author and more in later steps (shown below).">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'message_delete' || step.kind === 'pin_add' || step.kind === 'pin_remove'}
				<Field label="Message" hint="Pick from a previous step, or template an id.">
					<MessageRefField {step} {...exprBind('message')} onChannel={(v) => set('channel', v)} />
				</Field>
				<Field label="Channel"><ChannelExprField {...exprBind('channel')} /></Field>
				{#if step.kind === 'message_delete'}
					<Field label="Reason (audit log)">
						<input class="input" value={spec.reason ?? ''} oninput={(e) => set('reason', (e.currentTarget as HTMLInputElement).value)} />
					</Field>
				{/if}
			{:else if step.kind === 'message_purge'}
				<Field label="Channel"><ChannelExprField {...exprBind('channel')} /></Field>
				<Field label="How many recent messages to scan" hint="Bulk delete caps at 100; messages older than 14 days are skipped.">
					<NumberField min={1} max={100} value={spec.limit ?? 50} onChange={(n) => set('limit', n ?? 50)} />
				</Field>
				<Field label="Only from user" hint="Optional filter"><ExprField {...exprBind('from_user')} placeholder="(anyone)" /></Field>
				<label class="flex items-center justify-between gap-3">
					<span class="text-sm">Bots only</span>
					<Toggle checked={!!spec.bots_only} onchange={(v) => set('bots_only', v)} />
				</label>
				<Field label="Containing text" hint="Optional — only delete messages that contain this text.">
					<input class="input" value={spec.contains ?? ''} oninput={(e) => set('contains', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Reason (audit log)">
					<input class="input" value={spec.reason ?? ''} oninput={(e) => set('reason', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Save deleted count to">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'message_crosspost'}
				<Field label="Message" hint="Pick the announcement from a previous step, or template an id.">
					<MessageRefField {step} {...exprBind('message')} onChannel={(v) => set('channel', v)} />
				</Field>
				<Field label="Announcement channel"><ChannelExprField {...exprBind('channel')} /></Field>
			{:else if step.kind === 'react_clear'}
				<Field label="Message" hint="Pick from a previous step, or template an id.">
					<MessageRefField {step} {...exprBind('message')} onChannel={(v) => set('channel', v)} />
				</Field>
				<Field label="Channel"><ChannelExprField {...exprBind('channel')} /></Field>
				<Field label="Emoji" hint="Leave empty to clear ALL reactions">
					<div class="flex items-center gap-1.5">
						<EmojiPicker value={spec.emoji ?? ''} onChange={(v) => set('emoji', v)} />
						<input
							class="input min-w-0 flex-1"
							placeholder="(all reactions)"
							value={spec.emoji ?? ''}
							oninput={(e) => set('emoji', (e.currentTarget as HTMLInputElement).value)}
						/>
					</div>
				</Field>
			{:else if step.kind === 'react_add' || step.kind === 'react_remove'}
				<Field label="Message" hint="Pick from a previous step, or template an id.">
					<MessageRefField {step} {...exprBind('message')} onChannel={(v) => set('channel', v)} />
				</Field>
				<Field label="Channel"><ChannelExprField {...exprBind('channel')} /></Field>
				<Field label="Emoji">
					<div class="flex items-center gap-1.5">
						<EmojiPicker value={spec.emoji ?? ''} onChange={(v) => set('emoji', v)} />
						<input
							class="input min-w-0 flex-1"
							placeholder="👍 or name:id"
							value={spec.emoji ?? ''}
							oninput={(e) => set('emoji', (e.currentTarget as HTMLInputElement).value)}
						/>
					</div>
				</Field>
				{#if step.kind === 'react_remove'}
					<Field label="Whose reaction" hint="Empty = the bot's own reaction">
						<ExprField {...exprBind('user')} placeholder="(the bot)" />
					</Field>
				{/if}
			{:else if step.kind === 'role_add' || step.kind === 'role_remove'}
				<Field label="User"><ExprField {...exprBind('user')} placeholder={'{{ .User.ID }}'} /></Field>
				<Field label="Role id"><ExprField {...exprBind('role')} /></Field>
				<Field label="Reason">
					<input class="input" value={spec.reason ?? ''} oninput={(e) => set('reason', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'member_nickname'}
				<Field label="User"><ExprField {...exprBind('user')} placeholder={'{{ .User.ID }}'} /></Field>
				<Field label="Nickname">
					<input class="input" value={spec.nickname ?? ''} oninput={(e) => set('nickname', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Reason">
					<input class="input" value={spec.reason ?? ''} oninput={(e) => set('reason', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'member_kick' || step.kind === 'member_unban'}
				<Field label="User"><ExprField {...exprBind('user')} placeholder={'{{ .User.ID }}'} /></Field>
				<Field label="Reason">
					<input class="input" value={spec.reason ?? ''} oninput={(e) => set('reason', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'member_fetch'}
				<Field label="User"><ExprField {...exprBind('user')} placeholder={'{{ .Input.target }}'} /></Field>
				<Field label="Save to variable" hint="Name it, then use their roles, nickname, join date and more in later steps (shown below).">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'voice_set'}
				<Field label="User"><ExprField {...exprBind('user')} placeholder={'{{ .User.ID }}'} /></Field>
				<label class="flex items-center justify-between gap-3">
					<span class="text-sm">Server-mute</span>
					<Toggle checked={!!spec.mute} onchange={(v) => set('mute', v)} />
				</label>
				<label class="flex items-center justify-between gap-3">
					<span class="text-sm">Server-deafen</span>
					<Toggle checked={!!spec.deafen} onchange={(v) => set('deafen', v)} />
				</label>
				<Field label="Reason">
					<input class="input" value={spec.reason ?? ''} oninput={(e) => set('reason', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'thread_member'}
				<Field label="Thread"><ExprField {...exprBind('thread')} placeholder={'{{ .Vars.thread.id }}'} /></Field>
				<Field label="User"><ExprField {...exprBind('user')} placeholder={'{{ .User.ID }}'} /></Field>
				<Field label="Action">
					<FieldSelect
						value={spec.action ?? 'add'}
						onChange={(v) => set('action', v)}
						options={[
							{ value: 'add', label: 'Add to thread' },
							{ value: 'remove', label: 'Remove from thread' }
						]}
					/>
				</Field>
			{:else if step.kind === 'invite_create'}
				<Field label="Channel"><ChannelExprField {...exprBind('channel')} /></Field>
				<Field label="Expires after" hint="Like 24h or 7d. Blank = never expires.">
					<input class="input" placeholder="24h" value={spec.max_age ?? ''} oninput={(e) => set('max_age', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Max uses" hint="0 = unlimited">
					<NumberField min={0} max={100} value={spec.max_uses ?? 0} onChange={(n) => set('max_uses', n ?? 0)} />
				</Field>
				<label class="flex items-center justify-between gap-3">
					<span class="text-sm">Temporary membership</span>
					<Toggle checked={!!spec.temporary} onchange={(v) => set('temporary', v)} />
				</label>
				<Field label="Save to variable" hint="Name it, then use the invite link in later steps (shown below).">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'member_ban'}
				<Field label="User"><ExprField {...exprBind('user')} placeholder={'{{ .User.ID }}'} /></Field>
				<Field label="Delete recent messages (days, 0-7)">
					<NumberField
						min={0}
						max={7}
						suffix="days"
						value={spec.delete_message_days ?? 0}
						onChange={(n) => set('delete_message_days', n)}
					/>
				</Field>
				<Field label="Reason">
					<input class="input" value={spec.reason ?? ''} oninput={(e) => set('reason', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'member_timeout'}
				<Field label="User"><ExprField {...exprBind('user')} placeholder={'{{ .User.ID }}'} /></Field>
				<Field label="Duration" hint="Like 10m, 1h or 7d. Up to 28 days.">
					<input class="input" placeholder="10m" value={spec.duration ?? ''} oninput={(e) => set('duration', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Reason">
					<input class="input" value={spec.reason ?? ''} oninput={(e) => set('reason', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'channel_create'}
				<Field label="Name">
					<input class="input" value={spec.name ?? ''} oninput={(e) => set('name', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Type">
					<FieldSelect
						value={spec.type ?? 'text'}
						onChange={(v) => set('type', v)}
						options={[
							{ value: 'text', label: 'Text' },
							{ value: 'voice', label: 'Voice' },
							{ value: 'category', label: 'Category' },
							{ value: 'announcement', label: 'Announcement' },
							{ value: 'forum', label: 'Forum' },
							{ value: 'stage', label: 'Stage' }
						]}
					/>
				</Field>
				<Field label="Parent category"><ExprField {...exprBind('parent')} placeholder="(optional)" /></Field>
				<Field label="Topic">
					<input class="input" value={spec.topic ?? ''} oninput={(e) => set('topic', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Save to variable">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'channel_edit'}
				<Field label="Channel"><ChannelExprField {...exprBind('channel')} /></Field>
				<Field label="Name">
					<input class="input" value={spec.name ?? ''} oninput={(e) => set('name', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Topic">
					<input class="input" value={spec.topic ?? ''} oninput={(e) => set('topic', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'channel_delete'}
				<Field label="Channel"><ChannelExprField {...exprBind('channel')} /></Field>
				<Field label="Reason">
					<input class="input" value={spec.reason ?? ''} oninput={(e) => set('reason', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'thread_create'}
				<Field label="Channel"><ChannelExprField {...exprBind('channel')} /></Field>
				<Field label="From message id" hint="(optional) — thread under an existing message"><ExprField {...exprBind('message')} placeholder="(new thread)" /></Field>
				<Field label="Name">
					<input class="input" value={spec.name ?? ''} oninput={(e) => set('name', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Auto-archive minutes">
					<NumberField
						min={0}
						suffix="min"
						value={spec.auto_archive_minutes ?? 60}
						onChange={(n) => set('auto_archive_minutes', n)}
					/>
				</Field>
				<label class="flex items-center justify-between gap-3">
					<span class="text-sm">Private</span>
					<Toggle checked={!!spec.private} onchange={(v) => set('private', v)} />
				</label>
				<Field label="Save to variable">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'thread_archive'}
				<Field label="Thread"><ExprField {...exprBind('thread')} /></Field>
				<label class="flex items-center justify-between gap-3">
					<span class="text-sm">Lock it too</span>
					<Toggle checked={!!spec.locked} onchange={(v) => set('locked', v)} />
				</label>
			{:else if step.kind === 'voice_move'}
				<Field label="User"><ExprField {...exprBind('user')} placeholder={'{{ .User.ID }}'} /></Field>
				<Field label="Target channel" hint="Empty = disconnect them"><ExprField {...exprBind('channel')} /></Field>
			{:else if step.kind === 'image_render'}
				<Field label="Template id" hint="Studio template — manage them on the index page.">
					<NumberField min={0} value={spec.template_id ?? 0} onChange={(n) => set('template_id', n)} />
				</Field>
				<Field label="Save bytes to" hint="The rendered PNG bytes land in this variable so image_attach + reply pick them up.">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Variable overrides (JSON)" hint={'Map of layout-vars to template strings. Example: {"user.name": "{{ .User.Username }}"}'}>
					<textarea
						class="input font-mono text-[12px]"
						rows="3"
						value={JSON.stringify(spec.vars ?? {}, null, 2)}
						oninput={(e) => {
							try {
								set('vars', JSON.parse((e.currentTarget as HTMLTextAreaElement).value));
							} catch {/* ignore until valid */}
						}}
					></textarea>
				</Field>
			{:else if step.kind === 'image_attach'}
				<Field label="From variable">
					<input class="input" value={spec.from_var ?? ''} oninput={(e) => set('from_var', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Filename">
					<input class="input" value={spec.filename ?? ''} oninput={(e) => set('filename', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'image_load'}
				<Field label="Source URL"><ExprField {...exprBind('source')} placeholder="https://…/image.png" /></Field>
				<Field label="Save to">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Max bytes">
					<NumberField
						min={0}
						step={1024}
						suffix="B"
						value={spec.max_bytes ?? 8388608}
						onChange={(n) => set('max_bytes', n)}
					/>
				</Field>
			{:else if step.kind === 'set_var'}
				<Field label="Variable name">
					<input class="input" value={spec.name ?? ''} oninput={(e) => set('name', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Value"><ExprField {...exprBind('value')} /></Field>
			{:else if step.kind === 'incr_var'}
				<Field label="Variable name">
					<input class="input" value={spec.name ?? ''} oninput={(e) => set('name', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="By">
					<NumberField value={spec.by ?? 1} onChange={(n) => set('by', n)} />
				</Field>
			{:else if step.kind === 'pick_random'}
				<Field
					label="Pick from"
					hint={'A list value, or a string split on newlines/commas — e.g. "Yes,No,Maybe" or {{ .Vars.entrants }}'}
				>
					<ExprField {...exprBind('from')} placeholder={'Yes,No,Ask again later'} />
				</Field>
				<Field label="How many">
					<NumberField min={1} max={25} value={spec.count ?? 1} onChange={(n) => set('count', n ?? 1)} />
				</Field>
				<Field label="Save to variable">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'json_parse'}
				<Field label="JSON text" hint="From a saved value, a form answer, or any value.">
					<ExprField {...exprBind('value')} placeholder={'{{ .Vars.raw }}'} />
				</Field>
				<Field label="Save to variable" hint="Name it, then read its fields in later steps (shown below).">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'kv_get' || step.kind === 'kv_set' || step.kind === 'kv_delete'}
				<Field label="Key" hint="A name to store this value under.">
					<input class="input" value={spec.key ?? ''} oninput={(e) => set('key', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Scope">
					<FieldSelect
						value={spec.scope ?? 'guild'}
						onChange={(v) => set('scope', v)}
						options={[
							{ value: 'guild', label: 'Server-wide' },
							{ value: 'member', label: 'Per member' }
						]}
					/>
				</Field>
				{#if spec.scope === 'member'}
					<Field label="Owner id" hint="Defaults to the invoker"><ExprField {...exprBind('owner_id')} placeholder={'{{ .User.ID }}'} /></Field>
				{/if}
				{#if step.kind === 'kv_set'}
					<Field label="Value"><ExprField {...exprBind('value')} /></Field>
					<Field label="Expires after" hint="Optional — like 1h or 7d. Blank = never expires.">
						<input class="input" value={spec.ttl ?? ''} oninput={(e) => set('ttl', (e.currentTarget as HTMLInputElement).value)} />
					</Field>
				{:else if step.kind === 'kv_get'}
					<Field label="Save to">
						<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
					</Field>
				{/if}
			{:else if step.kind === 'http_request'}
				<Field label="Method">
					<FieldSelect
						value={spec.method ?? 'GET'}
						onChange={(v) => set('method', v)}
						options={['GET', 'POST', 'PUT', 'PATCH', 'DELETE'].map((m) => ({
							value: m,
							label: m
						}))}
					/>
				</Field>
				<Field label="URL">
					<input class="input font-mono text-[12px]" value={spec.url ?? ''} oninput={(e) => set('url', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Timeout (ms)">
					<NumberField
						min={0}
						step={250}
						suffix="ms"
						value={spec.timeout_ms ?? 5000}
						onChange={(n) => set('timeout_ms', n)}
					/>
				</Field>
				<label class="flex items-center justify-between gap-3">
					<span class="text-sm">Parse response as JSON</span>
					<Toggle checked={!!spec.parse_json} onchange={(v) => set('parse_json', v)} />
				</label>
				<Field label="Save to">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'if'}
				<Field label="Condition" hint="The 'then' steps run only when this is true."><ExprField {...exprBind('cond')} placeholder={'{{ eq .User.Username "admin" }}'} /></Field>
			{:else if step.kind === 'switch'}
				<Field label="On"><ExprField {...exprBind('on')} /></Field>
				<p class="hint">Add cases via the canvas — each can edit its own `when` value.</p>
			{:else if step.kind === 'loop'}
				<Field label="Over"><ExprField {...exprBind('over')} placeholder={'comma-separated, or {{.Args}}'} /></Field>
				<Field label="Item variable name">
					<input class="input" value={spec.as ?? 'item'} oninput={(e) => set('as', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Index variable name (optional)">
					<input class="input" value={spec.index_as ?? ''} oninput={(e) => set('index_as', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Max iterations">
					<NumberField min={1} max={1000} value={spec.max_iter ?? 100} onChange={(n) => set('max_iter', n)} />
				</Field>
			{:else if step.kind === 'parallel'}
				<Field label="Join">
					<FieldSelect
						value={spec.join ?? 'all'}
						onChange={(v) => set('join', v)}
						options={[
							{ value: 'all', label: 'All — wait for every branch' },
							{ value: 'race', label: 'Race — first done cancels siblings' }
						]}
					/>
				</Field>
			{:else if step.kind === 'wait'}
				<Field label="Duration" hint={isAutomation ? 'Like 10s or 30s. Up to 1 minute.' : 'Like 30s, 5m or 1h.'}>
					<input class="input" value={spec.duration ?? '30s'} oninput={(e) => set('duration', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'wait_for'}
				{@const wt = spec.trigger ?? 'component'}
				<p class="-mt-1 mb-1 text-[11.5px] leading-snug text-muted">
					Pause here until something happens, then keep going. If nothing happens in time, the
					<span class="text-ink">on&nbsp;timeout</span> path runs instead.
				</p>

				<Field label="What should it wait for?">
					<FieldSelect
						value={wt}
						onChange={(v) => {
							set('trigger', v);
							if (!spec.into) set('into', v === 'message' ? 'reply' : v === 'reaction' ? 'reaction' : 'click');
						}}
						options={isAutomation
							? [
									{ value: 'component', label: 'A button is clicked', description: 'On a message this flow sent' },
									{ value: 'message', label: 'A message is sent', description: 'Wait for someone to write something' },
									{ value: 'reaction', label: 'A reaction is added', description: 'Wait for an emoji reaction' },
									{ value: 'modal', label: 'A form is submitted', description: 'After you open a form' }
								]
							: [
									{ value: 'component', label: 'A button / select click' },
									{ value: 'modal', label: 'A form submission' }
								]}
					/>
				</Field>

				{#if wt === 'component'}
					<Field
						label="Which button?"
						hint={flowButtons.length === 0
							? 'Tip: add buttons to a Send message step above, then pick one here.'
							: 'Pick a button you sent earlier in the flow.'}
					>
						<FieldSelect
							value={spec.custom_id_suffix ?? ''}
							onChange={(v) => set('custom_id_suffix', v)}
							options={[{ value: '', label: 'Any button I sent' }, ...flowButtons]}
						/>
					</Field>
				{:else if wt === 'modal'}
					<Field label="Form name" hint="A short label so the form's answers come back here. The default is fine.">
						<input class="input" placeholder="form" value={spec.custom_id_suffix ?? ''} oninput={(e) => set('custom_id_suffix', (e.currentTarget as HTMLInputElement).value)} />
					</Field>
				{/if}

				<Field label="Who can do it?">
					{#if isAutomation}
						{@const who = spec.from_user?.src === '{{ .User.ID }}' ? 'member' : spec.from_user?.src ? 'custom' : 'anyone'}
						<FieldSelect
							value={who}
							onChange={(v) => {
								if (v === 'anyone') set('from_user', { lang: 'tmpl', src: '' });
								else if (v === 'member') set('from_user', { lang: 'tmpl', src: '{{ .User.ID }}' });
								else
									set('from_user', {
										lang: 'tmpl',
										src: spec.from_user?.src === '{{ .User.ID }}' ? '' : spec.from_user?.src || ''
									});
							}}
							options={[
								{ value: 'anyone', label: 'Anyone' },
								{ value: 'member', label: 'The member this is about', description: 'Whoever triggered the automation' },
								{ value: 'custom', label: 'A specific person…', description: 'Enter a user id or @mention' }
							]}
						/>
						{#if who === 'custom'}
							<div class="mt-1.5"><ExprField {...exprBind('from_user')} placeholder="a user id" /></div>
						{/if}
					{:else}
						<ExprField {...exprBind('from_user')} placeholder="(anyone)" />
					{/if}
				</Field>

				{#if wt === 'message' || wt === 'reaction'}
					{@const cm = spec.channel_mode ?? 'any'}
					<Field label="Where?">
						<FieldSelect
							value={cm}
							onChange={(v) => set('channel_mode', v)}
							options={[
								{ value: 'any', label: 'Anywhere in the server' },
								{ value: 'current', label: 'The same channel', description: 'Where this automation is acting' },
								{ value: 'only', label: 'Only certain channels' },
								{ value: 'except', label: 'Everywhere except some' }
							]}
						/>
					</Field>
					{#if cm === 'only' || cm === 'except'}
						<Field
							label="Which channels?"
							hint={cm === 'except'
								? 'Watch everywhere except these.'
								: 'Only watch these channels.'}
						>
							<ChannelPicker
								multiple
								value={spec.channels ?? []}
								onChange={(v) => set('channels', v)}
								placeholder={cm === 'except' ? 'Channels to skip' : 'Channels to watch'}
							/>
						</Field>
					{/if}
				{/if}

				{#if wt === 'reaction'}
					<Field label="Which emoji?" hint="Leave blank to accept any emoji.">
						<input class="input" placeholder="👍" value={spec.emoji ?? ''} oninput={(e) => set('emoji', (e.currentTarget as HTMLInputElement).value)} />
					</Field>
				{/if}

				{#if isAutomation}
					<Field label="How long to wait?">
						<FieldSelect
							value={spec.timeout ?? '30s'}
							onChange={(v) => set('timeout', v)}
							options={[
								{ value: '10s', label: '10 seconds' },
								{ value: '30s', label: '30 seconds' },
								{ value: '60s', label: '1 minute', description: 'The maximum' }
							]}
						/>
					</Field>
				{:else}
					<Field label="Timeout" hint="Go duration, e.g. 30s, 5m.">
						<input class="input" value={spec.timeout ?? '10m'} oninput={(e) => set('timeout', (e.currentTarget as HTMLInputElement).value)} />
					</Field>
				{/if}

				<Field label="Remember it as" hint={rememberHint(wt)}>
					<input
						class="input"
						placeholder={wt === 'message' ? 'reply' : wt === 'reaction' ? 'reaction' : 'click'}
						value={spec.into ?? ''}
						oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)}
					/>
				</Field>

				{#if isAutomation}
					<p class="px-0.5 text-[10.5px] leading-snug text-faint">
						{#if wt === 'component' || wt === 'modal'}
							The steps after this run when it arrives, and you can reply right there.
						{:else}
							The steps after this run when it arrives (use Send message there).
						{/if}
						Drag the node's right dot to build the <span class="text-muted">on&nbsp;timeout</span> path.
					</p>
				{/if}
			{:else if step.kind === 'exit'}
				<Field label="Reason (logged)">
					<input class="input" value={spec.reason ?? ''} oninput={(e) => set('reason', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'fail'}
				<Field label="Error message">
					<input class="input" value={spec.message ?? ''} oninput={(e) => set('message', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{:else if step.kind === 'run_command'}
				<Field label="Command name">
					<input class="input" value={spec.command ?? ''} oninput={(e) => set('command', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<label class="flex items-center justify-between gap-3">
					<span class="text-sm">Inherit scope</span>
					<Toggle checked={spec.inherit_scope ?? true} onchange={(v) => set('inherit_scope', v)} />
				</label>
			{:else if step.kind === 'audit_note'}
				<Field label="Action">
					<input class="input" value={spec.action ?? ''} oninput={(e) => set('action', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Detail (JSON object)"><ExprField {...exprBind('detail')} placeholder={'{"foo": "bar"}'} /></Field>
			{:else if step.kind === 'modal_open'}
				<Field label="Title">
					<input class="input" maxlength="45" value={spec.title ?? ''} oninput={(e) => set('title', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<Field label="Form name" hint="A short label so the answers come back here. The default is fine.">
					<input class="input font-mono text-[12px]" placeholder="form" value={spec.custom_id_suffix ?? ''} oninput={(e) => set('custom_id_suffix', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
				<div>
					<div class="mb-1 flex items-center justify-between">
						<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
							Fields
						</span>
						<span class="font-mono text-[10px] tabular-nums text-faint">
							{(spec.fields ?? []).length}/5
						</span>
					</div>
					<div class="space-y-1.5">
						{#each spec.fields ?? [] as f, fi (fi)}
							<div class="rounded-md border border-line bg-bg p-2">
								<div class="flex items-center gap-1.5">
									<input
										class="input h-6 min-w-0 flex-1 text-[12px]"
										placeholder="Label the member sees"
										maxlength="45"
										value={f.label ?? ''}
										oninput={(e) => {
											const fields = [...(spec.fields ?? [])];
											fields[fi] = { ...fields[fi], label: (e.currentTarget as HTMLInputElement).value };
											set('fields', fields);
										}}
									/>
									<FieldSelect
										class="h-6 w-28 shrink-0 text-[11px]"
										value={f.style ?? 'short'}
										options={[
											{ value: 'short', label: 'One line' },
											{ value: 'paragraph', label: 'Paragraph' }
										]}
										onChange={(v) => {
											const fields = [...(spec.fields ?? [])];
											fields[fi] = { ...fields[fi], style: v };
											set('fields', fields);
										}}
									/>
									<button
										type="button"
										class="grid size-6 shrink-0 place-items-center rounded text-faint hover:bg-surface hover:text-danger"
										aria-label="Remove field"
										onclick={() => set('fields', (spec.fields ?? []).filter((_: unknown, idx: number) => idx !== fi))}
									>
										✕
									</button>
								</div>
								<div class="mt-1.5 flex items-center gap-1.5">
									<input
										class="input h-6 min-w-0 flex-1 font-mono text-[11px]"
										placeholder="answer_id (a short name for this answer)"
										value={f.custom_id ?? ''}
										oninput={(e) => {
											const fields = [...(spec.fields ?? [])];
											fields[fi] = { ...fields[fi], custom_id: (e.currentTarget as HTMLInputElement).value };
											set('fields', fields);
										}}
									/>
									<label class="flex shrink-0 items-center gap-1.5">
										<span class="text-[11px] text-muted">required</span>
										<Toggle
											checked={!!f.required}
											onchange={(v) => {
												const fields = [...(spec.fields ?? [])];
												fields[fi] = { ...fields[fi], required: v };
												set('fields', fields);
											}}
										/>
									</label>
								</div>
								<input
									class="input mt-1.5 h-6 w-full text-[11.5px]"
									placeholder="Placeholder text (optional)"
									maxlength="100"
									value={f.placeholder ?? ''}
									oninput={(e) => {
										const fields = [...(spec.fields ?? [])];
										fields[fi] = { ...fields[fi], placeholder: (e.currentTarget as HTMLInputElement).value };
										set('fields', fields);
									}}
								/>
							</div>
						{/each}
					</div>
					<button
						type="button"
						class="mt-1.5 inline-flex h-7 items-center gap-1.5 rounded-md border border-dashed border-line bg-bg px-2 text-[11.5px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink disabled:opacity-40"
						disabled={(spec.fields ?? []).length >= 5}
						onclick={() =>
							set('fields', [
								...(spec.fields ?? []),
								{ custom_id: `field_${(spec.fields ?? []).length + 1}`, label: '', style: 'short', required: true }
							])}
					>
						+ Add field
					</button>
				</div>
				<Field label="Save answers to" hint="Submission lands in this variable.">
					<input class="input" value={spec.into ?? ''} oninput={(e) => set('into', (e.currentTarget as HTMLInputElement).value)} />
				</Field>
			{/if}

			{#if produced}
				<StepOutputHint {produced} />
			{/if}

			<details class="rounded-md border border-line bg-ink-2/40">
				<summary class="cursor-pointer px-3 py-2 text-xs font-medium text-muted hover:text-ink">Raw spec (JSON)</summary>
				<textarea
					class="input m-3 font-mono text-[12px]"
					rows="6"
					value={JSON.stringify(spec, null, 2)}
					oninput={(e) => {
						try {
							const v = JSON.parse((e.currentTarget as HTMLTextAreaElement).value);
							if (step) step.spec = v;
						} catch {/* ignore until valid */}
					}}
				></textarea>
			</details>
		</div>
	</div>
{/if}
