<script lang="ts">
	// The winner announcement, edited in place — the same "the preview IS the
	// editor" surface as the message composer (MessageEditor). What the bot posts
	// when a giveaway is drawn renders here exactly as Discord shows it: the ended
	// embed (title + winners + host + footer), the in-channel congrats message, an
	// optional jump button and the winner DM — every user string typed straight
	// into the surface. The system-filled parts (the winners list, the host) show
	// sample values so the drawn state reads true. Mirrors AnnounceConfig on the Go
	// side; every string is a Go template rendered at runtime.
	import type { AnnounceConfig } from '$lib/giveaway';
	import type { Step } from '$lib/commands/types';
	import EmojiText from '$lib/components/commands/EmojiText.svelte';
	import EmojiPicker from '$lib/components/commands/EmojiPicker.svelte';
	import VarMenu from '$lib/components/commands/VarMenu.svelte';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import ExternalLink from 'lucide-svelte/icons/external-link';
	import AtSign from 'lucide-svelte/icons/at-sign';
	import Link2 from 'lucide-svelte/icons/link-2';
	import Mail from 'lucide-svelte/icons/mail';
	import Check from 'lucide-svelte/icons/check';

	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	type AnySpec = any;

	let {
		announce = $bindable(),
		accent = '#FF6363',
		readOnly = false,
		buttonExtras
	}: {
		announce: AnnounceConfig;
		accent?: string;
		readOnly?: boolean;
		// Per-button action controls for the winner DM's buttons (wire one to a
		// saved automation), passed through to its MessageEditor.
		buttonExtras?: import('svelte').Snippet<[{ component: AnySpec; ri: number; ci: number }]>;
	} = $props();

	// Which drawn state the preview shows — the winners path (congrats + winners)
	// or the no-winners path (the fallback message). Both are edited here so an
	// admin sees, and tunes, exactly what posts either way.
	let drawState = $state<'winners' | 'none'>('winners');

	// System-filled sample values for the parts the bot writes itself (so the card
	// reads like a real drawn giveaway, not a blank template).
	const SAMPLE_WINNERS = '@alex, @sam';
	const SAMPLE_HOST = '@you';

	// ── shared token insertion into the focused inline field ──────────────────
	// Same machinery as MessageEditor: the emoji / variable pickers drop their
	// token into whichever inline surface was last focused.
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
	// Custom-emoji token ("name:id") → Discord markup; unicode passes through.
	function insertEmoji(token: string) {
		const m = /^(a:)?([\w~-]+):(\d{15,21})$/.exec(token.trim());
		insertToken(m ? (m[1] ? `<a:${m[2]}:${m[3]}>` : `<:${m[2]}:${m[3]}>`) : token);
	}

	// Toggle chip styling (ping / jump / DM), matching the composer's inline pills.
	function chip(on: boolean): string {
		return on
			? 'border-line-strong bg-surface text-ink'
			: 'border-line text-faint hover:text-muted';
	}

	function clone<T>(v: T): T {
		return JSON.parse(JSON.stringify(v)) as T;
	}

	// The winner DM is a fully-composed message, edited with the shared
	// MessageEditor bound to this synthetic step. Rebuilt only when the parent
	// replaces the bound announce object (load / discard); the editor's writes
	// flow step → announce below and must not loop back into a rebuild.
	let dmStep = $state<Step>({ id: 'ann-dm', kind: 'send_dm', spec: {} });
	let seeded: AnnounceConfig | null = null;
	$effect(() => {
		if (announce === seeded) return;
		seeded = announce;
		dmStep = {
			id: 'ann-dm',
			kind: 'send_dm',
			spec: {
				content: announce.dm_message ?? '',
				embeds: clone(announce.dm_embeds ?? []),
				components: clone(announce.dm_components ?? [])
			}
		};
	});
	// Empty embed/button lists become undefined, matching the Go side's
	// omitempty, so the round-trip never makes an untouched giveaway dirty.
	$effect(() => {
		void JSON.stringify(dmStep);
		const s = dmStep.spec as AnySpec;
		if (!s) return;
		const embeds = (s.embeds as AnnounceConfig['dm_embeds']) ?? [];
		const components = (s.components as AnnounceConfig['dm_components']) ?? [];
		announce.dm_message = (s.content as string) ?? '';
		announce.dm_embeds = embeds.length ? embeds : undefined;
		announce.dm_components = components.length ? components : undefined;
	});
</script>

<div bind:this={rootEl} class="space-y-2" onfocusin={onFocusIn}>
	<!-- Toolbar: which drawn state to preview + the shared insert pickers. -->
	<div class="flex flex-wrap items-center justify-between gap-2">
		<div class="flex rounded-md border border-line p-0.5" role="radiogroup" aria-label="Preview state">
			<button
				type="button"
				role="radio"
				aria-checked={drawState === 'winners'}
				class="rounded px-2 py-0.5 text-[11px] font-medium transition-colors {drawState === 'winners'
					? 'bg-surface text-ink'
					: 'text-faint hover:text-muted'}"
				onclick={() => (drawState = 'winners')}
			>
				Winners drawn
			</button>
			<button
				type="button"
				role="radio"
				aria-checked={drawState === 'none'}
				class="rounded px-2 py-0.5 text-[11px] font-medium transition-colors {drawState === 'none'
					? 'bg-surface text-ink'
					: 'text-faint hover:text-muted'}"
				onclick={() => (drawState = 'none')}
			>
				No winners
			</button>
		</div>
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

	<!-- The announcement, as Discord renders it. Every string edits in place. -->
	<div class={readOnly ? 'pointer-events-none opacity-70' : ''}>
		<div class="rounded-lg bg-[#313338] px-4 py-3">
			<div class="flex gap-3.5">
				<div class="mt-0.5 grid size-9 shrink-0 select-none place-items-center rounded-full bg-[#F1DFDF]">
					<Logo size={24} />
				</div>
				<div class="min-w-0 flex-1">
					<div class="flex items-baseline gap-1.5">
						<span class="text-[13.5px] font-medium text-[#f2f3f5]">Dia</span>
						<span class="rounded-[3px] bg-[#5865f2] px-1 py-px text-[8.5px] font-semibold uppercase text-white">App</span>
						<span class="text-[10px] text-[#949ba4]">today at 4:20 PM</span>
					</div>

					<!-- Congrats / no-winners message (posted in-channel with the draw). -->
					{#if drawState === 'winners'}
						<EmojiText
							class="w-full text-[13px] leading-[1.45] text-[#dbdee1]"
							placeholder={'Congratulations {{ .Winners }}! You won…'}
							value={announce.message ?? ''}
							onChange={(v) => (announce.message = v)}
						/>
					{:else}
						<EmojiText
							class="w-full text-[13px] leading-[1.45] text-[#dbdee1]"
							placeholder="Not enough valid entries to draw a winner for…"
							value={announce.no_winners_message ?? ''}
							onChange={(v) => (announce.no_winners_message = v)}
						/>
					{/if}

					<!-- Ended embed: the giveaway message flips to this when drawn. -->
					<div class="mt-1.5 overflow-hidden rounded-[4px] bg-[#2b2d31]" style="border-left: 4px solid {accent}">
						<div class="px-3 py-2.5">
							<EmojiText
								class="text-[15px] font-semibold text-white"
								multiline={false}
								placeholder={'🎉 {{ .Prize }}'}
								value={announce.ended_title ?? ''}
								onChange={(v) => (announce.ended_title = v)}
							/>
							<div class="mt-2">
								<div class="text-[12px] font-semibold text-white">Winners</div>
								<div class="text-[13px] text-[#dbdee1]">
									{drawState === 'winners' ? SAMPLE_WINNERS : 'No valid entries.'}
								</div>
							</div>
							<div class="mt-2">
								<div class="text-[12px] font-semibold text-white">Hosted by</div>
								<div class="text-[13px] text-[#dbdee1]">{SAMPLE_HOST}</div>
							</div>
							<div class="mt-2 flex items-baseline gap-1 text-[11px] text-[#949ba4]">
								<EmojiText
									class="text-[11px] text-[#949ba4]"
									multiline={false}
									placeholder="Ended"
									value={announce.ended_footer ?? ''}
									onChange={(v) => (announce.ended_footer = v)}
								/>
								<span class="shrink-0">· just now</span>
							</div>
						</div>
					</div>

					<!-- Jump button — rendered only when the toggle is on. -->
					{#if announce.jump_button}
						<div class="mt-2">
							<span class="inline-flex items-center gap-1.5 rounded-[3px] border border-[#4e5058] px-3 py-1.5 text-[12.5px] font-medium text-white">
								Jump to giveaway <ExternalLink size={11} class="opacity-80" />
							</span>
						</div>
					{/if}

					<p class="mt-2 text-[10.5px] italic text-[#949ba4]">
						Winners and host are filled in when the giveaway is drawn (sample shown).
					</p>
				</div>
			</div>
		</div>
	</div>

	<!-- Behaviour toggles — the affordances that change what posts, next to the
	     surface they affect (not a separate form). -->
	{#if !readOnly}
		<div class="flex flex-wrap items-center gap-1.5">
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md border px-2 text-[11px] font-medium transition-colors {chip(announce.ping_winners)}"
				title="Ping the winners in the announcement"
				onclick={() => (announce.ping_winners = !announce.ping_winners)}
			>
				<AtSign size={11} /> Ping winners {#if announce.ping_winners}<Check size={11} class="text-accent-ink" />{/if}
			</button>
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md border px-2 text-[11px] font-medium transition-colors {chip(announce.jump_button)}"
				title="Add a link button back to the giveaway message"
				onclick={() => (announce.jump_button = !announce.jump_button)}
			>
				<Link2 size={11} /> Jump button {#if announce.jump_button}<Check size={11} class="text-accent-ink" />{/if}
			</button>
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md border px-2 text-[11px] font-medium transition-colors {chip(announce.dm_winners)}"
				title="DM each winner when they win"
				onclick={() => (announce.dm_winners = !announce.dm_winners)}
			>
				<Mail size={11} /> DM winners {#if announce.dm_winners}<Check size={11} class="text-accent-ink" />{/if}
			</button>
		</div>
	{/if}

	<!-- Winner DM — a fully-composed message (content, embeds, buttons), edited
	     with the same MessageEditor as everything else, shown when DM is on. -->
	{#if announce.dm_winners}
		<div>
			<div class="mb-1 flex items-center gap-1.5 font-mono text-[10px] uppercase tracking-[0.14em] text-faint">
				<Mail size={11} /> Winner DM
			</div>
			<div class={readOnly ? 'pointer-events-none opacity-70' : ''}>
				<MessageEditor step={dmStep} embeds components clickPaths={false} {buttonExtras} />
			</div>
			<p class="mt-1 text-[11px] text-muted">
				Sent to each winner. Buttons open a link or run one of your automations.
			</p>
		</div>
	{/if}
</div>
