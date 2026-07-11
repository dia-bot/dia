<script lang="ts">
	// The Enter-button responses, fully composed — one complete message per
	// outcome, edited with the SAME MessageEditor as the giveaway message itself:
	// content typed into the bubble, WYSIWYG embeds, buttons (link or wired to a
	// saved automation). Each outcome also picks how it's delivered: a private
	// (ephemeral) reply, a public reply in the channel, a DM, or nothing. What to
	// DO on entry beyond the reply (roles, logging, anything) lives in the
	// built-in "On giveaway entry" automation behind the Advanced button.
	// Mirrors giveaway.EntryConfig / EntryReply; every string is a Go template.
	import type { EntryConfig, EntryReply } from '$lib/giveaway';
	import type { Step } from '$lib/commands/types';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import Check from 'lucide-svelte/icons/check';
	import LogOut from 'lucide-svelte/icons/log-out';
	import Ban from 'lucide-svelte/icons/ban';
	import Bot from 'lucide-svelte/icons/bot';
	import Clock from 'lucide-svelte/icons/clock';
	import Wand2 from 'lucide-svelte/icons/wand-2';
	import EyeOff from 'lucide-svelte/icons/eye-off';
	import MessageSquare from 'lucide-svelte/icons/message-square';
	import Mail from 'lucide-svelte/icons/mail';
	import BellOff from 'lucide-svelte/icons/bell-off';

	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	type AnySpec = any;

	let {
		entry = $bindable(),
		readOnly = false,
		onAdvanced,
		buttonExtras
	}: {
		entry: EntryConfig;
		readOnly?: boolean;
		onAdvanced?: () => void;
		// Per-button action controls (wire a button to a saved automation),
		// rendered inside the button's inline editor — passed through to the
		// MessageEditor, same as the giveaway message's own buttons.
		buttonExtras?: import('svelte').Snippet<[{ component: AnySpec; ri: number; ci: number }]>;
	} = $props();

	type OutcomeKey = keyof EntryConfig;

	// The outcomes, in the order a member is most likely to hit them. fallback is
	// the built-in copy the server uses when the composition renders empty.
	const OUTCOMES: {
		key: OutcomeKey;
		label: string;
		desc: string;
		icon: typeof Check;
		fallback: string;
	}[] = [
		{
			key: 'entered',
			label: 'Entered',
			desc: 'They joined the giveaway. {{ .Entries }} is their ticket count.',
			icon: Check,
			fallback: "🎉 You're entered into the giveaway for {{ .Prize }}!"
		},
		{
			key: 'left',
			label: 'Left / already in',
			desc: 'They were already in and clicked again, so they leave.',
			icon: LogOut,
			fallback: "You've left the giveaway for {{ .Prize }}."
		},
		{
			key: 'not_eligible',
			label: "Doesn't qualify",
			desc: 'They failed a requirement. {{ .Reason }} says why.',
			icon: Ban,
			fallback: '❌ {{ .Reason }}'
		},
		{
			key: 'bots_blocked',
			label: 'Bot blocked',
			desc: 'A bot clicked and bots can’t enter.',
			icon: Bot,
			fallback: "Bots can't enter this giveaway."
		},
		{
			key: 'ended',
			label: 'Already ended',
			desc: 'They clicked after the giveaway ended.',
			icon: Clock,
			fallback: 'This giveaway has already ended.'
		}
	];

	const MODES: { value: string; label: string; icon: typeof EyeOff; desc: string }[] = [
		{ value: '', label: 'Private reply', icon: EyeOff, desc: 'Only they see it (ephemeral).' },
		{ value: 'public', label: 'Public reply', icon: MessageSquare, desc: 'Posted in the channel for everyone.' },
		{ value: 'dm', label: 'DM', icon: Mail, desc: 'Sent as a direct message.' },
		{ value: 'none', label: 'Nothing', icon: BellOff, desc: 'The click is acknowledged silently.' }
	];

	let sel = $state<OutcomeKey>('entered');

	function clone<T>(v: T): T {
		return JSON.parse(JSON.stringify(v)) as T;
	}

	// One synthetic MessageEditor step per outcome. Rebuilt only when the parent
	// replaces the bound entry object (load / discard); the editor's own writes
	// flow steps → entry below, and must not loop back into a rebuild.
	let steps = $state<Record<string, Step>>({});
	let seeded: EntryConfig | null = null;
	$effect(() => {
		if (entry === seeded) return;
		seeded = entry;
		const next: Record<string, Step> = {};
		for (const o of OUTCOMES) {
			const r: EntryReply = entry[o.key] ?? {};
			next[o.key] = {
				id: `entry-${o.key}`,
				kind: 'send_message',
				spec: {
					content: r.content ?? '',
					embeds: clone(r.embeds ?? []),
					components: clone(r.components ?? [])
				}
			};
		}
		steps = next;
	});

	// Editor writes sync back into the bound entry config (the parent's dirty
	// snapshot watches entry, so edits surface in the save dock). In-place field
	// writes keep the entry object identity, so the seeding effect stays quiet.
	// Empty values become undefined, matching the Go side's omitempty, so the
	// round-trip through this editor never makes an untouched giveaway dirty.
	$effect(() => {
		void JSON.stringify(steps);
		for (const o of OUTCOMES) {
			const s = steps[o.key]?.spec as AnySpec;
			if (!s || !entry[o.key]) continue;
			const embeds = (s.embeds as EntryReply['embeds']) ?? [];
			const components = (s.components as EntryReply['components']) ?? [];
			entry[o.key].content = (s.content as string) || undefined;
			entry[o.key].embeds = embeds.length ? embeds : undefined;
			entry[o.key].components = components.length ? components : undefined;
		}
	});

	const selMeta = $derived(OUTCOMES.find((o) => o.key === sel) ?? OUTCOMES[0]);
	const selMode = $derived(entry[sel]?.mode ?? '');
	function setMode(m: string) {
		if (!entry[sel]) return;
		entry[sel].mode = m || undefined;
	}
	// A dot on the tab when an outcome deviates from a plain default reply.
	function customized(key: OutcomeKey): boolean {
		const r = entry[key];
		if (!r) return false;
		return !!(r.mode || (r.embeds ?? []).length > 0 || (r.components ?? []).length > 0);
	}
</script>

<div class="space-y-2.5">
	<!-- Toolbar: outcome tabs + the Advanced (automation) jump. -->
	<div class="flex flex-wrap items-center justify-between gap-2">
		<div class="flex flex-wrap rounded-md border border-line p-0.5" role="tablist" aria-label="Entry outcome">
			{#each OUTCOMES as o (o.key)}
				<button
					type="button"
					role="tab"
					aria-selected={sel === o.key}
					class="inline-flex items-center gap-1.5 rounded px-2 py-1 text-[11px] font-medium transition-colors {sel === o.key
						? 'bg-surface text-ink'
						: 'text-faint hover:text-muted'}"
					onclick={() => (sel = o.key)}
				>
					<o.icon size={11} />
					{o.label}
					{#if customized(o.key)}<span class="size-1 rounded-full bg-accent"></span>{/if}
				</button>
			{/each}
		</div>
		<button
			type="button"
			class="inline-flex h-7 items-center gap-1.5 rounded-md border border-accent/40 bg-accent/5 px-2.5 text-[11px] font-medium text-accent-ink transition-colors hover:border-accent/70"
			onclick={() => onAdvanced?.()}
			title="Open the built-in on-entry automation to do anything when a member enters"
		>
			<Wand2 size={12} /> Advanced: automate entries
		</button>
	</div>

	<p class="text-[12px] text-muted">{selMeta.desc}</p>

	<!-- Delivery mode -->
	{#if !readOnly}
		<div class="flex flex-wrap items-center gap-1.5">
			{#each MODES as m (m.value)}
				<button
					type="button"
					role="radio"
					aria-checked={selMode === m.value}
					class="inline-flex h-7 items-center gap-1.5 rounded-md border px-2 text-[11px] font-medium transition-colors {selMode === m.value
						? 'border-line-strong bg-surface text-ink'
						: 'border-line text-faint hover:text-muted'}"
					title={m.desc}
					onclick={() => setMode(m.value)}
				>
					<m.icon size={11} /> {m.label}
				</button>
			{/each}
		</div>
	{/if}

	{#if selMode === 'none'}
		<p class="rounded-md border border-line bg-surface px-2.5 py-2 text-[12px] text-muted">
			The click is acknowledged silently and nothing is sent. The on-entry automation still runs.
		</p>
	{:else}
		{#if steps[sel]}
			<div class={readOnly ? 'pointer-events-none opacity-70' : ''}>
				{#key sel}
					<MessageEditor step={steps[sel]} embeds components clickPaths={false} {buttonExtras} />
				{/key}
			</div>
		{/if}
		<p class="text-[11px] leading-relaxed text-muted">
			{#if selMode === 'dm'}
				Sent as a direct message; the click itself is acknowledged silently.
			{:else if selMode === 'public'}
				Posted in the giveaway channel, visible to everyone.
			{:else}
				A private reply only they can see.
			{/if}
			Left empty, the built-in reply is used:
			<code class="rounded bg-surface px-1 font-mono text-[10.5px]">{selMeta.fallback}</code>
		</p>
	{/if}
</div>
