<script lang="ts">
	// The Enter-button replies, edited in place — the "the preview IS the editor"
	// surface, one compact ephemeral bubble per outcome. When a member clicks
	// Enter, Dia decides an outcome (entered, left, denied, blocked, ended) and
	// sends the matching ephemeral reply; each is typed straight into its bubble
	// here. What to actually DO on entry (give a role, log a denial, DM them) lives
	// in the built-in "On giveaway entry" automation, one click away on the
	// Advanced button. Mirrors giveaway.EntryConfig; every string is a Go template.
	import type { EntryConfig } from '$lib/giveaway';
	import EmojiText from '$lib/components/commands/EmojiText.svelte';
	import EmojiPicker from '$lib/components/commands/EmojiPicker.svelte';
	import VarMenu from '$lib/components/commands/VarMenu.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import Check from 'lucide-svelte/icons/check';
	import LogOut from 'lucide-svelte/icons/log-out';
	import Ban from 'lucide-svelte/icons/ban';
	import Bot from 'lucide-svelte/icons/bot';
	import Clock from 'lucide-svelte/icons/clock';
	import Wand2 from 'lucide-svelte/icons/wand-2';

	let {
		entry = $bindable(),
		readOnly = false,
		onAdvanced
	}: {
		entry: EntryConfig;
		readOnly?: boolean;
		onAdvanced?: () => void;
	} = $props();

	// The outcomes, in the order a member is most likely to hit them. key is the
	// EntryConfig field; the placeholder is the built-in default (Go side) so an
	// empty field previews truthfully.
	const OUTCOMES: {
		key: keyof EntryConfig;
		label: string;
		desc: string;
		icon: typeof Check;
		placeholder: string;
	}[] = [
		{
			key: 'entered',
			label: 'Entered',
			desc: 'They joined the giveaway.',
			icon: Check,
			placeholder: "🎉 You're entered into the giveaway for {{ .Prize }}!"
		},
		{
			key: 'left',
			label: 'Left / already in',
			desc: 'They were already in and clicked again, so they leave.',
			icon: LogOut,
			placeholder: "You've left the giveaway for {{ .Prize }}."
		},
		{
			key: 'not_eligible',
			label: "Doesn't qualify",
			desc: 'They failed a requirement. {{ .Reason }} says why.',
			icon: Ban,
			placeholder: '❌ {{ .Reason }}'
		},
		{
			key: 'bots_blocked',
			label: 'Bot blocked',
			desc: 'A bot clicked and bots can’t enter.',
			icon: Bot,
			placeholder: "Bots can't enter this giveaway."
		},
		{
			key: 'ended',
			label: 'Already ended',
			desc: 'They clicked after the giveaway ended.',
			icon: Clock,
			placeholder: 'This giveaway has already ended.'
		}
	];

	// ── shared token insertion into the focused inline field ──────────────────
	// Same machinery as MessageEditor / AnnounceEditor.
	let rootEl = $state<HTMLDivElement | null>(null);
	type RichHost = HTMLElement & { __insertToken?: (t: string) => void };
	let lastField: RichHost | null = null;
	function onFocusIn(e: FocusEvent) {
		const el = e.target as HTMLElement;
		if (el?.isContentEditable && '__insertToken' in el) lastField = el as RichHost;
	}
	function insertToken(token: string) {
		const el = lastField && rootEl?.contains(lastField) ? lastField : null;
		el?.__insertToken?.(token);
	}
	function insertEmoji(token: string) {
		const m = /^(a:)?([\w~-]+):(\d{15,21})$/.exec(token.trim());
		insertToken(m ? (m[1] ? `<a:${m[2]}:${m[3]}>` : `<:${m[2]}:${m[3]}>`) : token);
	}
</script>

<div bind:this={rootEl} class="space-y-2" onfocusin={onFocusIn}>
	<!-- Toolbar: the shared insert pickers + the Advanced (automation) jump. -->
	<div class="flex flex-wrap items-center justify-between gap-2">
		<button
			type="button"
			class="inline-flex h-7 items-center gap-1.5 rounded-md border border-accent/40 bg-accent/5 px-2.5 text-[11px] font-medium text-accent-ink transition-colors hover:border-accent/70"
			onclick={() => onAdvanced?.()}
			title="Open the built-in on-entry automation to do anything when a member enters"
		>
			<Wand2 size={12} /> Advanced: automate entries
		</button>
		{#if !readOnly}
			<div class="flex items-center gap-2">
				<EmojiPicker
					value=""
					returnFocusOnPick={false}
					onChange={(t) => t && insertEmoji(t)}
					class="grid h-6 w-7 shrink-0 place-items-center rounded border border-line text-faint transition-colors hover:border-line-strong hover:text-muted data-[state=open]:border-line-strong"
				/>
				<VarMenu onPick={insertToken} />
			</div>
		{/if}
	</div>

	<!-- One editable ephemeral reply per outcome. -->
	<div class="space-y-2.5">
		{#each OUTCOMES as o (o.key)}
			<div>
				<div class="mb-1 flex items-center gap-1.5">
					<span class="grid size-4 place-items-center text-muted"><o.icon size={12} /></span>
					<span class="text-[12px] font-semibold text-ink">{o.label}</span>
					<span class="truncate text-[11px] text-muted">{o.desc}</span>
				</div>
				<div class={readOnly ? 'pointer-events-none opacity-70' : ''}>
					<div class="rounded-lg bg-[#313338] px-3 py-2.5">
						<div class="flex gap-3">
							<div class="mt-0.5 grid size-7 shrink-0 select-none place-items-center rounded-full bg-[#F1DFDF]">
								<Logo size={18} />
							</div>
							<div class="min-w-0 flex-1">
								<div class="flex items-baseline gap-1.5">
									<span class="text-[12.5px] font-medium text-[#f2f3f5]">Dia</span>
									<span class="rounded-[3px] bg-[#5865f2] px-1 py-px text-[8px] font-semibold uppercase text-white">App</span>
								</div>
								<EmojiText
									class="w-full text-[13px] leading-[1.45] text-[#dbdee1]"
									placeholder={o.placeholder}
									value={entry[o.key] ?? ''}
									onChange={(v) => (entry[o.key] = v)}
								/>
								<p class="mt-1 text-[10px] italic text-[#949ba4]">Only they can see this reply.</p>
							</div>
						</div>
					</div>
				</div>
			</div>
		{/each}
	</div>
</div>
