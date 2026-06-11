<script lang="ts">
	// Multi-step new-command wizard: Name → Properties → Response.
	//
	// Replaces the inline name-input flow. Walks the whole shape of a slash
	// command — the /name, its Discord properties (typed inputs members fill
	// in), and the first reply — with a live preview of how the command will
	// look in Discord's message bar. One atomic create at the end, then the
	// editor opens for the full flow.
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import {
		newStepID,
		type CommandOption,
		type Definition,
		type Step
	} from '$lib/commands/types';
	import SlashCommandPreview from './SlashCommandPreview.svelte';
	import PropertiesEditor from './PropertiesEditor.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { Dialog } from '$lib/components/ui';
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';

	import SquareSlash from 'lucide-svelte/icons/square-slash';
	import Braces from 'lucide-svelte/icons/braces';
	import MessageSquare from 'lucide-svelte/icons/message-square';
	import Check from 'lucide-svelte/icons/check';
	import ChevronLeft from 'lucide-svelte/icons/chevron-left';
	import ChevronRight from 'lucide-svelte/icons/chevron-right';
	import Sparkles from 'lucide-svelte/icons/sparkles';

	let {
		open = $bindable(false),
		guildId,
		existingNames = []
	}: {
		open?: boolean;
		guildId: string;
		existingNames?: string[];
	} = $props();

	type WizardStep = 0 | 1 | 2;
	const STEPS: { key: WizardStep; label: string; icon: typeof SquareSlash }[] = [
		{ key: 0, label: 'Name', icon: SquareSlash },
		{ key: 1, label: 'Properties', icon: Braces },
		{ key: 2, label: 'Response', icon: MessageSquare }
	];

	let step = $state<WizardStep>(0);
	let name = $state('');
	let description = $state('');
	let options = $state<CommandOption[]>([]);
	let replyContent = $state('Hello {user.mention}!');
	let ephemeral = $state(false);
	let creating = $state(false);
	let createError = $state('');

	let propsEditor = $state<PropertiesEditor | null>(null);

	const NAME_RE = /^[a-z0-9_-]{1,32}$/;
	const nameValid = $derived(NAME_RE.test(name));
	const nameTaken = $derived(existingNames.includes(name));

	// Reset when (re)opened.
	$effect(() => {
		if (!open) return;
		step = 0;
		name = '';
		description = '';
		options = [];
		replyContent = 'Hello {user.mention}!';
		ephemeral = false;
		creating = false;
		createError = '';
	});

	const canAdvance = $derived.by(() => {
		if (step === 0) return nameValid && !nameTaken;
		if (step === 1) return propsEditor?.isValid() ?? true;
		return true;
	});

	function tryGo(target: WizardStep) {
		// Backwards is always allowed; forwards gates on the current step.
		if (target <= step || canAdvance) step = target;
	}

	// Token chips members can click to seed the reply with template values.
	const tokens = $derived.by(() => {
		const base = ['{user.mention}', '{user.name}', '{server}', '{channel}'];
		return [...options.filter((o) => o.name).map((o) => `{input.${o.name}}`), ...base];
	});

	let replyEl = $state<HTMLTextAreaElement | null>(null);
	function insertToken(tok: string) {
		const el = replyEl;
		if (!el) {
			replyContent += tok;
			return;
		}
		const start = el.selectionStart ?? replyContent.length;
		const end = el.selectionEnd ?? replyContent.length;
		replyContent = replyContent.slice(0, start) + tok + replyContent.slice(end);
		requestAnimationFrame(() => {
			el.focus();
			el.selectionStart = el.selectionEnd = start + tok.length;
		});
	}

	async function create() {
		if (creating || !nameValid || nameTaken) return;
		creating = true;
		createError = '';
		try {
			// Discord requires required properties first — order it on the way out.
			const ordered = [...options].sort(
				(a, b) => Number(!!b.required) - Number(!!a.required)
			);
			const reply: Step = {
				id: newStepID(),
				kind: 'reply',
				spec: {
					content: replyContent.trim() || 'Hello {user.mention}!',
					...(ephemeral ? { ephemeral: true } : {})
				}
			};
			const definition: Definition = {
				options: ordered,
				triggers: [{ kind: 'slash' }],
				steps: [reply]
			};
			const res = await api.upsertCommand(guildId, {
				name,
				description,
				enabled: true,
				definition
			});
			open = false;
			await goto(`/servers/${guildId}/commands/${res.id}`);
		} catch (e) {
			createError = e instanceof Error ? e.message : String(e);
		} finally {
			creating = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-w-[680px] gap-0 overflow-hidden p-0">
		<Dialog.Title class="sr-only">New command</Dialog.Title>
		<!-- Header: eyebrow · current step · progress -->
		<div class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-4">
			<div class="grid size-5 place-items-center rounded border border-line bg-surface text-muted">
				<SquareSlash size={11} />
			</div>
			<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
				New command
			</span>
			<div class="h-4 w-px bg-line"></div>
			<span class="text-[12.5px] font-medium text-ink">{STEPS[step].label}</span>
			<span class="font-mono text-[10.5px] tabular-nums text-faint">
				{step + 1} / {STEPS.length}
			</span>
		</div>

		<!-- Step rail -->
		<div class="flex h-9 shrink-0 items-center gap-1 border-b border-line/60 px-3">
			{#each STEPS as s, i (s.key)}
				{@const active = s.key === step}
				{@const done = s.key < step}
				<button
					type="button"
					onclick={() => tryGo(s.key)}
					class="inline-flex h-6 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium transition-colors {active
						? 'bg-surface text-ink'
						: done
							? 'text-muted hover:bg-surface/60 hover:text-ink'
							: 'text-faint hover:text-muted'}"
				>
					{#if done}
						<Check size={11} />
					{:else}
						<s.icon size={11} />
					{/if}
					{s.label}
					{#if i < STEPS.length - 1}
						<span class="ml-1 text-faint/60">›</span>
					{/if}
				</button>
			{/each}
		</div>

		<!-- Body — each step slides in as you advance -->
		<div class="max-h-[60vh] min-h-72 overflow-y-auto px-5 py-5">
			{#key step}
				<div in:fly={{ x: 14, duration: dur(220), easing: cubicOut }}>
					{#if step === 0}
				<div class="space-y-4">
					<div>
						<span
							class="mb-1.5 block font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
						>
							Command name
						</span>
						<div
							class="flex h-10 items-center gap-1 rounded-lg border border-line bg-bg px-3 focus-within:border-line-strong"
						>
							<span class="font-mono text-[15px] text-faint">/</span>
							<!-- svelte-ignore a11y_autofocus -->
							<input
								class="h-full min-w-0 flex-1 bg-transparent font-mono text-[14px] text-ink placeholder:text-faint focus:outline-none"
								placeholder="my-command"
								maxlength="32"
								autofocus
								bind:value={name}
								onkeydown={(e) => e.key === 'Enter' && canAdvance && (step = 1)}
							/>
							{#if name}
								<span
									class="shrink-0 font-mono text-[10px] {nameValid && !nameTaken
										? 'text-success'
										: 'text-danger'}"
								>
									{nameTaken ? 'already exists' : nameValid ? '✓' : 'a–z 0–9 _ -'}
								</span>
							{/if}
						</div>
						<p class="mt-1.5 font-mono text-[10.5px] text-faint">
							lowercase · 1–32 chars · letters, digits, hyphen, underscore
						</p>
					</div>

					<div>
						<div class="mb-1.5 flex items-baseline justify-between">
							<span
								class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
							>
								Description
							</span>
							<span class="font-mono text-[10px] tabular-nums text-faint">
								{description.length}/100
							</span>
						</div>
						<input
							class="h-8 w-full rounded-lg border border-line bg-bg px-3 text-[13px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
							placeholder="What this command does — shown in Discord's command picker"
							maxlength="100"
							bind:value={description}
						/>
					</div>

					<SlashCommandPreview {name} {description} {options} />
				</div>
			{:else if step === 1}
				<div class="space-y-4">
					<p class="text-[12px] leading-relaxed text-muted">
						Properties are the inputs members fill in after typing
						<code class="rounded bg-surface px-1 font-mono text-[11px] text-ink"
							>/{name || 'command'}</code
						>
						— each one is a typed field Discord renders natively. Read them in steps as
						<code class="rounded bg-surface px-1 font-mono text-[11px] text-ink"
							>{'{input.name}'}</code
						>.
					</p>
					<SlashCommandPreview {name} {description} {options} />
					<PropertiesEditor
						bind:this={propsEditor}
						{options}
						onChange={(next) => (options = next)}
					/>
				</div>
			{:else}
				<div class="space-y-4">
					<div>
						<span
							class="mb-1.5 block font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint"
						>
							First reply
						</span>
						<textarea
							bind:this={replyEl}
							class="w-full rounded-lg border border-line bg-bg px-3 py-2 text-[13px] leading-relaxed text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
							rows="4"
							placeholder="Hello {'{user.mention}'}!"
							bind:value={replyContent}
						></textarea>
						<div class="mt-1.5 flex flex-wrap items-center gap-1">
							{#each tokens as tok (tok)}
								<button
									type="button"
									class="inline-flex h-6 items-center rounded border border-line bg-surface px-1.5 font-mono text-[10.5px] text-muted transition-colors hover:border-line-strong hover:text-ink"
									onclick={() => insertToken(tok)}
								>
									{tok}
								</button>
							{/each}
						</div>
					</div>

					<label class="flex items-center justify-between gap-3 rounded-lg border border-line bg-surface/40 px-3 py-2.5">
						<span class="min-w-0">
							<span class="block text-[12.5px] font-medium text-ink">Ephemeral</span>
							<span class="block text-[11px] text-muted">
								Only the member who ran the command sees the reply.
							</span>
						</span>
						<Toggle checked={ephemeral} onchange={(v) => (ephemeral = v)} />
					</label>

					<div class="flex items-start gap-2.5 rounded-lg border border-line bg-ink-2 px-3 py-2.5">
						<Sparkles size={13} class="mt-0.5 shrink-0 text-faint" />
						<p class="text-[11.5px] leading-relaxed text-muted">
							This seeds the first step. After creating you land in the flow editor —
							branch on conditions, grant roles, call APIs, render cards, anything.
						</p>
					</div>
				</div>
			{/if}
				</div>
			{/key}
		</div>

		<!-- Footer -->
		<div class="flex h-12 shrink-0 items-center gap-1.5 border-t border-line px-3">
			{#if step > 0}
				<button
					type="button"
					class="inline-flex h-7 items-center gap-1 rounded-md px-2 text-[12px] font-medium text-muted transition-colors hover:bg-surface hover:text-ink"
					onclick={() => (step = (step - 1) as WizardStep)}
				>
					<ChevronLeft size={12} />
					Back
				</button>
			{/if}
			{#if createError}
				<span class="min-w-0 truncate font-mono text-[10.5px] text-danger" title={createError}>
					{createError}
				</span>
			{/if}
			<div class="ml-auto flex items-center gap-1.5">
				{#if step < STEPS.length - 1}
					<button
						type="button"
						class="inline-flex h-7 items-center gap-1 rounded-md bg-ink px-2.5 text-[12px] font-medium text-bg transition-opacity hover:opacity-90 disabled:opacity-40"
						disabled={!canAdvance}
						onclick={() => (step = (step + 1) as WizardStep)}
					>
						Continue
						<ChevronRight size={12} />
					</button>
				{:else}
					<button
						type="button"
						class="inline-flex h-7 items-center gap-1.5 rounded-md bg-accent px-3 text-[12px] font-medium text-ink transition-colors hover:bg-accent/85 disabled:opacity-50"
						disabled={creating || !nameValid || nameTaken}
						onclick={create}
					>
						{creating ? 'Creating…' : `Create /${name || 'command'}`}
					</button>
				{/if}
			</div>
		</div>
	</Dialog.Content>
</Dialog.Root>
