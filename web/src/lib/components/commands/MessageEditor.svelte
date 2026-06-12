<script lang="ts">
	// The message composer IS the message: a Discord-accurate surface you edit
	// in place. Content is typed straight into the chat bubble, embeds are
	// WYSIWYG (EmbedBuilder), buttons render exactly as Discord renders them
	// (click one to edit it in a popover), attachments support real picture
	// upload. Mirrors SpecReply / SpecSendMessage on the Go side; every string
	// is a Go template rendered at runtime.
	import { page } from '$app/stores';
	import { uploadImage } from '$lib/api';
	import type { Step } from '$lib/commands/types';
	import EmbedBuilder from './EmbedBuilder.svelte';
	import ImagePicker from './ImagePicker.svelte';
	import VarMenu from './VarMenu.svelte';
	import FieldSelect from './FieldSelect.svelte';
	import EmojiPicker from './EmojiPicker.svelte';
	import EmojiGlyph from './EmojiGlyph.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import { Popover } from '$lib/components/ui';
	import { slide } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';

	import Plus from 'lucide-svelte/icons/plus';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import ExternalLink from 'lucide-svelte/icons/external-link';
	import ChevronDown from 'lucide-svelte/icons/chevron-down';
	import Upload from 'lucide-svelte/icons/upload';
	import FileImage from 'lucide-svelte/icons/file-image';
	import List from 'lucide-svelte/icons/list';
	import Variable from 'lucide-svelte/icons/variable';
	import EyeOff from 'lucide-svelte/icons/eye-off';
	import MousePointerClick from 'lucide-svelte/icons/mouse-pointer-click';

	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	type AnySpec = any;

	let {
		step,
		ephemeral = false,
		embeds = true,
		components = false,
		attachments = false
	}: {
		step: Step;
		// Which message features this step kind supports (mirrors the Go spec).
		ephemeral?: boolean;
		embeds?: boolean;
		components?: boolean;
		attachments?: boolean;
	} = $props();

	function spec(): AnySpec {
		if (!step.spec) step.spec = {};
		return step.spec as AnySpec;
	}
	function set(field: string, value: unknown) {
		const s = { ...spec() };
		if (
			value === '' ||
			value === false ||
			value === undefined ||
			(Array.isArray(value) && value.length === 0)
		)
			delete s[field];
		else s[field] = value;
		step.spec = s;
	}

	const s = $derived(spec());
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const embedList = $derived((s.embeds ?? []) as any[]);
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const rowList = $derived((s.components ?? []) as { components: any[] }[]);
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const attachList = $derived((s.attachments ?? []) as any[]);

	// ── templated-value insertion at the cursor ──────────────────────────────
	let rootEl = $state<HTMLDivElement | null>(null);
	let lastInput: HTMLInputElement | HTMLTextAreaElement | null = null;

	function onFocusIn(e: FocusEvent) {
		const el = e.target as HTMLElement;
		if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) {
			if (el.type !== 'color' && el.type !== 'number' && el.type !== 'file') lastInput = el;
		}
	}

	// Message content wants Discord's emoji syntax: custom emoji tokens
	// ("name:id", "a:name:id") become <:name:id> / <a:name:id>; unicode
	// passes through untouched.
	function contentEmoji(token: string): string {
		const m = /^(a:)?([\w~-]+):(\d{15,21})$/.exec(token.trim());
		if (!m) return token;
		return m[1] ? `<a:${m[2]}:${m[3]}>` : `<:${m[2]}:${m[3]}>`;
	}

	function insertToken(token: string) {
		const el = lastInput && rootEl?.contains(lastInput) ? lastInput : null;
		if (!el) {
			set('content', `${s.content ?? ''}${token}`);
			return;
		}
		const start = el.selectionStart ?? el.value.length;
		const end = el.selectionEnd ?? el.value.length;
		el.value = el.value.slice(0, start) + token + el.value.slice(end);
		el.dispatchEvent(new Event('input', { bubbles: true }));
		el.focus();
		el.selectionStart = el.selectionEnd = start + token.length;
	}

	function autogrow(el: HTMLTextAreaElement) {
		const fit = () => {
			el.style.height = '0';
			el.style.height = `${el.scrollHeight}px`;
		};
		fit();
		el.addEventListener('input', fit);
		return { destroy: () => el.removeEventListener('input', fit) };
	}

	// ── embeds ───────────────────────────────────────────────────────────────
	function addEmbed() {
		if (embedList.length >= 10) return;
		set('embeds', [...embedList, {}]);
	}
	function removeEmbed(i: number) {
		set('embeds', embedList.filter((_, idx) => idx !== i));
	}
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function patchEmbed(i: number, next: any) {
		set('embeds', embedList.map((e, idx) => (idx === i ? next : e)));
	}

	// ── components ───────────────────────────────────────────────────────────
	const SELECT_TYPES = [
		{ value: 'select_string', label: 'Custom options' },
		{ value: 'select_user', label: 'Pick a user' },
		{ value: 'select_role', label: 'Pick a role' },
		{ value: 'select_channel', label: 'Pick a channel' }
	];
	const BUTTON_STYLES: { value: string; swatch: string }[] = [
		{ value: 'primary', swatch: '#5865f2' },
		{ value: 'secondary', swatch: '#4e5058' },
		{ value: 'success', swatch: '#248046' },
		{ value: 'danger', swatch: '#da373c' },
		{ value: 'link', swatch: '#4e5058' }
	];
	const BTN_BG: Record<string, string> = {
		primary: '#5865f2',
		secondary: '#4e5058',
		success: '#248046',
		danger: '#da373c',
		link: '#4e5058'
	};

	function setRows(rows: { components: AnySpec[] }[]) {
		set('components', rows);
	}

	// Random click ids: two buttons (even across copies of the same command)
	// can never collide, and the run id scopes them per invocation anyway.
	function randId(prefix: string): string {
		return `${prefix}_${Math.random().toString(36).slice(2, 7)}`;
	}
	function addButtonRow() {
		if (rowList.length >= 5) return;
		setRows([
			...rowList,
			{
				components: [
					{ type: 'button', style: 'primary', label: 'Button', custom_id_suffix: randId('btn') }
				]
			}
		]);
	}
	function addSelectRow() {
		if (rowList.length >= 5) return;
		setRows([
			...rowList,
			{
				components: [
					{
						type: 'select_string',
						placeholder: 'Make a selection',
						custom_id_suffix: randId('pick'),
						options: [{ label: 'Option 1', value: 'one' }]
					}
				]
			}
		]);
	}
	function addToRow(ri: number) {
		const row = rowList[ri];
		if (rowHasSelect(row) || row.components.length >= 5) return;
		setRows(
			rowList.map((r, idx) =>
				idx === ri
					? {
							components: [
								...r.components,
								{
									type: 'button',
									style: 'secondary',
									label: 'Button',
									custom_id_suffix: `btn${ri + 1}_${r.components.length + 1}`
								}
							]
						}
					: r
			)
		);
	}
	function patchComponent(ri: number, ci: number, patch: AnySpec) {
		setRows(
			rowList.map((r, idx) =>
				idx === ri
					? { components: r.components.map((c, cidx) => (cidx === ci ? { ...c, ...patch } : c)) }
					: r
			)
		);
	}
	function setButtonStyle(ri: number, ci: number, style: string) {
		const cur = rowList[ri].components[ci];
		const patch: AnySpec = { style };
		// Link buttons carry a URL instead of a custom id; swap cleanly.
		// A stale on_click would put a custom_id back on the link button,
		// which Discord rejects.
		if (style === 'link') {
			patch.custom_id_suffix = undefined;
			patch.on_click = undefined;
			patch.url = cur.url ?? '';
		} else {
			patch.url = undefined;
			patch.custom_id_suffix = cur.custom_id_suffix || randId('btn');
		}
		const next = { ...cur, ...patch };
		for (const k of Object.keys(next)) if (next[k] === undefined) delete next[k];
		setRows(
			rowList.map((r, idx) =>
				idx === ri
					? { components: r.components.map((c, cidx) => (cidx === ci ? next : c)) }
					: r
			)
		);
	}
	function removeComponent(ri: number, ci: number) {
		const row = rowList[ri];
		if (row.components.length <= 1) {
			setRows(rowList.filter((_, idx) => idx !== ri));
			return;
		}
		setRows(
			rowList.map((r, idx) =>
				idx === ri ? { components: r.components.filter((_, cidx) => cidx !== ci) } : r
			)
		);
	}
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function rowHasSelect(row: { components: any[] }): boolean {
		return row.components.some((c) => c.type !== 'button');
	}
	function patchSelectOption(ri: number, ci: number, oi: number, patch: AnySpec) {
		const cur = rowList[ri].components[ci];
		patchComponent(ri, ci, {
			options: (cur.options ?? []).map((o: AnySpec, idx: number) =>
				idx === oi ? { ...o, ...patch } : o
			)
		});
	}

	// ── attachments ──────────────────────────────────────────────────────────
	let attachFileEl: HTMLInputElement | null = $state(null);
	let attachUploading = $state(false);
	let attachError = $state('');

	async function onAttachFile(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const file = input.files?.[0];
		if (!file) return;
		attachUploading = true;
		attachError = '';
		try {
			const url = await uploadImage($page.params.id ?? '', file);
			set('attachments', [...attachList, { url, filename: file.name }]);
		} catch (err) {
			attachError = err instanceof Error ? err.message : 'Upload failed';
		} finally {
			attachUploading = false;
			input.value = '';
		}
	}
	function addVarAttachment() {
		set('attachments', [...attachList, { from_var: '', filename: '' }]);
	}
	function patchAttachment(i: number, patch: AnySpec) {
		set('attachments', attachList.map((a, idx) => (idx === i ? { ...a, ...patch } : a)));
	}
	function removeAttachment(i: number) {
		set('attachments', attachList.filter((_, idx) => idx !== i));
	}
</script>

<div bind:this={rootEl} class="space-y-2" onfocusin={onFocusIn}>
	<!-- Composer toolbar -->
	<div class="flex items-center justify-between gap-2">
		<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
			Message
		</span>
		<div class="flex items-center gap-2">
			{#if ephemeral}
				<button
					type="button"
					class="inline-flex h-6 items-center gap-1 rounded border px-1.5 font-mono text-[10px] font-medium transition-colors {s.ephemeral
						? 'border-line-strong bg-surface text-ink'
						: 'border-line text-faint hover:text-muted'}"
					title="Only the invoker sees the reply"
					onclick={() => set('ephemeral', !s.ephemeral)}
				>
					<EyeOff size={10} />
					ephemeral
				</button>
			{/if}
			<EmojiPicker
				value=""
				onChange={(t) => t && insertToken(contentEmoji(t))}
				class="grid h-6 w-7 shrink-0 place-items-center rounded border border-line text-faint transition-colors hover:border-line-strong hover:text-muted data-[state=open]:border-line-strong"
			/>
			<VarMenu onPick={insertToken} />
		</div>
	</div>

	<!-- ── The message, as Discord renders it. Everything edits in place. ── -->
	<div class="rounded-lg bg-[#313338] px-4 py-3">
		<div class="flex gap-3.5">
			<!-- Bot avatar — the real Dia mark, like Discord shows it -->
			<div
				class="mt-0.5 grid size-9 shrink-0 select-none place-items-center rounded-full bg-[#F1DFDF]"
			>
				<Logo size={24} />
			</div>
			<div class="min-w-0 flex-1">
				<div class="flex items-baseline gap-1.5">
					<span class="text-[13.5px] font-medium text-[#f2f3f5]">Dia</span>
					<span
						class="rounded-[3px] bg-[#5865f2] px-1 py-px text-[8.5px] font-semibold uppercase text-white"
					>
						App
					</span>
					<span class="text-[10px] text-[#949ba4]">today at 4:20 PM</span>
				</div>

				<!-- Content — typed straight into the bubble -->
				<textarea
					use:autogrow
					rows="1"
					class="dc-content w-full resize-none text-[13px] leading-[1.45] text-[#dbdee1]"
					maxlength="2000"
					placeholder={'Say something… {user.mention}, properties, markdown — all templates work'}
					value={s.content ?? ''}
					oninput={(e) => set('content', (e.currentTarget as HTMLTextAreaElement).value)}
				></textarea>

				<!-- Attachments -->
				{#if attachments && attachList.length > 0}
					<div class="mt-1.5 space-y-1.5">
						{#each attachList as a, i (i)}
							<div
								transition:slide={{ duration: dur(180), easing: cubicOut }}
								class="group/att flex items-center gap-2.5 rounded-md border border-[#1e1f22] bg-[#2b2d31] p-2"
							>
								{#if /^https?:\/\//.test(a.url ?? '')}
									<img
										src={a.url}
										alt={a.filename || 'attachment'}
										class="size-10 shrink-0 rounded object-cover"
									/>
								{:else}
									<span
										class="grid size-10 shrink-0 place-items-center rounded bg-white/[0.04] text-[#949ba4]"
									>
										{#if a.from_var !== undefined && !a.url}
											<Variable size={14} />
										{:else}
											<FileImage size={14} />
										{/if}
									</span>
								{/if}
								<div class="min-w-0 flex-1">
									<input
										class="dc-content block w-full text-[12.5px] font-medium text-[#00a8fc]"
										placeholder="file.png"
										value={a.filename ?? ''}
										oninput={(e) =>
											patchAttachment(i, {
												filename: (e.currentTarget as HTMLInputElement).value
											})}
									/>
									<input
										class="dc-content block w-full font-mono text-[10px] text-[#949ba4]"
										placeholder="variable (image_render / image_load) or https://…"
										value={a.from_var || a.url || ''}
										oninput={(e) => {
											const v = (e.currentTarget as HTMLInputElement).value;
											if (/^https?:\/\//.test(v)) patchAttachment(i, { url: v, from_var: '' });
											else patchAttachment(i, { from_var: v, url: '' });
										}}
									/>
								</div>
								<button
									type="button"
									class="grid size-6 shrink-0 place-items-center rounded text-[#949ba4] opacity-0 transition-all hover:text-[#fa777c] group-hover/att:opacity-100"
									onclick={() => removeAttachment(i)}
									aria-label="Remove attachment"
								>
									<Trash2 size={11} />
								</button>
							</div>
						{/each}
					</div>
				{/if}

				<!-- Embeds — WYSIWYG, edited in place -->
				{#if embeds && embedList.length > 0}
					<div class="mt-1.5 space-y-1.5">
						{#each embedList as e, i (i)}
							<div transition:slide={{ duration: dur(200), easing: cubicOut }}>
								<EmbedBuilder
									embed={e}
									onChange={(next) => patchEmbed(i, next)}
									onRemove={() => removeEmbed(i)}
								/>
							</div>
						{/each}
					</div>
				{/if}

				<!-- Buttons & selects — rendered like Discord, click to edit -->
				{#if components && rowList.length > 0}
					<div class="mt-2 space-y-2">
						{#each rowList as row, ri (ri)}
							<div
								transition:slide={{ duration: dur(180), easing: cubicOut }}
								class="flex flex-wrap items-center gap-2"
							>
								{#each row.components as c, ci (ci)}
									{#if c.type === 'button'}
										<Popover.Root>
											<Popover.Trigger>
												{#snippet child({ props })}
													<button
														{...props}
														class="inline-flex h-8 items-center gap-1.5 rounded-[3px] px-3.5 text-[12.5px] font-medium text-white transition-[filter] hover:brightness-110"
														style="background: {BTN_BG[c.style ?? 'primary'] ?? BTN_BG.primary}"
													>
														{#if c.emoji}<EmojiGlyph emoji={c.emoji} size={17} />{/if}
														<span class="max-w-40 truncate">{c.label || 'Button'}</span>
														{#if c.style === 'link'}<ExternalLink size={11} class="opacity-80" />{/if}
													</button>
												{/snippet}
											</Popover.Trigger>
											<Popover.Content class="w-72 p-3" align="start">
												<div class="mb-2 flex items-center justify-between">
													<span
														class="font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground"
													>
														Button
													</span>
													<button
														type="button"
														class="inline-flex h-5 items-center gap-1 rounded px-1.5 text-[10.5px] font-medium text-destructive transition-colors hover:bg-destructive/10"
														onclick={() => removeComponent(ri, ci)}
													>
														<Trash2 size={10} />
														Remove
													</button>
												</div>
												<!-- Style swatches -->
												<div class="mb-2.5 flex items-center gap-1.5">
													{#each BUTTON_STYLES as st (st.value)}
														<button
															type="button"
															class="grid h-7 flex-1 place-items-center rounded-[3px] text-[10px] font-medium text-white transition-all {(c.style ?? 'primary') === st.value
																? 'ring-2 ring-white ring-offset-2 ring-offset-popover'
																: 'opacity-60 hover:opacity-100'}"
															style="background: {st.swatch}"
															title={st.value}
															onclick={() => setButtonStyle(ri, ci, st.value)}
														>
															{#if st.value === 'link'}<ExternalLink size={10} />{/if}
														</button>
													{/each}
												</div>
												<div class="flex items-center gap-1.5">
													<input
														class="h-7 min-w-0 flex-1 rounded-md border border-input bg-background px-2 text-[12px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
														placeholder="Label"
														maxlength="80"
														value={c.label ?? ''}
														oninput={(e) =>
															patchComponent(ri, ci, {
																label: (e.currentTarget as HTMLInputElement).value
															})}
													/>
													<EmojiPicker
														value={c.emoji ?? ''}
														onChange={(v) => patchComponent(ri, ci, { emoji: v })}
													/>
												</div>
												{#if c.style === 'link'}
													<input
														class="mt-1.5 h-7 w-full rounded-md border border-input bg-background px-2 font-mono text-[11px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
														placeholder="https:// where the button links"
														value={c.url ?? ''}
														oninput={(e) =>
															patchComponent(ri, ci, {
																url: (e.currentTarget as HTMLInputElement).value
															})}
													/>
												{:else if c.custom_id_manual}
													<input
														class="mt-1.5 h-7 w-full rounded-md border border-input bg-background px-2 font-mono text-[11px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
														placeholder="custom id — templates work: vote_{'{{ .Vars.idx }}'}"
														value={c.custom_id_suffix ?? ''}
														oninput={(e) =>
															patchComponent(ri, ci, {
																custom_id_suffix: (e.currentTarget as HTMLInputElement).value
															})}
													/>
													<p class="mt-1.5 text-[10px] leading-snug text-muted-foreground">
														Manual id — for advanced routing and future automations. The
														canvas click-path dot is off for this button; pair it with a
														Wait-for step yourself.
													</p>
													<button
														type="button"
														class="mt-1 text-[10px] font-medium text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
														onclick={() =>
															patchComponent(ri, ci, {
																custom_id_manual: undefined,
																custom_id_suffix: randId('btn')
															})}
													>
														Use the automatic id instead
													</button>
												{:else}
													<div class="mt-2 flex items-center justify-between gap-2">
														<span class="font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
															On click
														</span>
														<div class="flex rounded-md border border-input p-0.5" role="radiogroup" aria-label="On click">
															<button
																type="button"
																role="radio"
																aria-checked={c.on_click !== 'none'}
																class="rounded px-2 py-0.5 text-[10px] font-medium transition-colors {c.on_click !== 'none'
																	? 'bg-secondary text-foreground'
																	: 'text-muted-foreground hover:text-foreground'}"
																onclick={() => patchComponent(ri, ci, { on_click: undefined })}
															>
																Runs a path
															</button>
															<button
																type="button"
																role="radio"
																aria-checked={c.on_click === 'none'}
																class="rounded px-2 py-0.5 text-[10px] font-medium transition-colors {c.on_click === 'none'
																	? 'bg-secondary text-foreground'
																	: 'text-muted-foreground hover:text-foreground'}"
																onclick={() =>
																	patchComponent(ri, ci, {
																		on_click: 'none',
																		custom_id_suffix: c.custom_id_suffix || randId('btn')
																	})}
															>
																Nothing
															</button>
														</div>
													</div>
													{#if c.on_click === 'none'}
														<p class="mt-1.5 text-[10px] leading-snug text-muted-foreground">
															Clicks are acknowledged silently and nothing runs. Works forever,
															even after the flow ends.
														</p>
													{:else}
													<p class="mt-1.5 flex items-start gap-1 text-[10px] leading-snug text-muted-foreground">
														<MousePointerClick size={10} class="mt-px shrink-0" />
														<span>
															Drag this button's dot on the canvas to choose what
															happens when it's clicked.
														</span>
													</p>
													<button
														type="button"
														class="mt-1 text-[10px] font-medium text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
														title="Advanced — set your own id for routing / future automations"
														onclick={() => patchComponent(ri, ci, { custom_id_manual: true })}
													>
														Configure custom id
													</button>
													{/if}
												{/if}
											</Popover.Content>
										</Popover.Root>
									{:else}
										<!-- Select menu — full-width bar, click to edit -->
										<Popover.Root>
											<Popover.Trigger
												class="flex h-9 w-full max-w-sm items-center justify-between gap-2 rounded-[3px] border border-[#1e1f22] bg-[#1e1f22] px-2.5 text-left text-[12.5px] text-[#949ba4] transition-colors hover:border-[#404249]"
											>
												<span class="truncate">{c.placeholder || 'Make a selection'}</span>
												<ChevronDown size={14} class="shrink-0" />
											</Popover.Trigger>
											<Popover.Content class="w-80 p-3" align="start">
												<div class="mb-2 flex items-center justify-between">
													<span
														class="font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground"
													>
														Select menu
													</span>
													<button
														type="button"
														class="inline-flex h-5 items-center gap-1 rounded px-1.5 text-[10.5px] font-medium text-destructive transition-colors hover:bg-destructive/10"
														onclick={() => removeComponent(ri, ci)}
													>
														<Trash2 size={10} />
														Remove
													</button>
												</div>
												<div class="grid grid-cols-2 gap-1.5">
													<FieldSelect
														class="h-7 text-[11.5px]"
														value={c.type}
														options={SELECT_TYPES}
														onChange={(v) => {
															const patch: AnySpec = { type: v };
															if (v === 'select_string' && !c.options)
																patch.options = [{ label: 'Option 1', value: 'one' }];
															patchComponent(ri, ci, patch);
														}}
													/>
													<input
														class="h-7 rounded-md border border-input bg-background px-2 font-mono text-[11px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
														placeholder="pick id"
														value={c.custom_id_suffix ?? ''}
														oninput={(e) =>
															patchComponent(ri, ci, {
																custom_id_suffix: (e.currentTarget as HTMLInputElement).value
															})}
													/>
												</div>
												<input
													class="mt-1.5 h-7 w-full rounded-md border border-input bg-background px-2 text-[12px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
													placeholder="Placeholder text"
													value={c.placeholder ?? ''}
													oninput={(e) =>
														patchComponent(ri, ci, {
															placeholder: (e.currentTarget as HTMLInputElement).value
														})}
												/>
												{#if c.type === 'select_string'}
													<div class="mt-2 space-y-1">
														{#each c.options ?? [] as o, oi (oi)}
															<div class="flex items-center gap-1.5">
																<EmojiPicker
																	class="grid h-6 w-7 shrink-0 place-items-center rounded border border-input bg-background text-[13px] transition-colors hover:border-ring/60 data-[state=open]:border-ring"
																	value={o.emoji ?? ''}
																	onChange={(v) => patchSelectOption(ri, ci, oi, { emoji: v })}
																/>
																<input
																	class="h-6 min-w-0 flex-1 rounded border border-input bg-background px-1.5 text-[11px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
																	placeholder="Option label"
																	value={o.label ?? ''}
																	oninput={(e) =>
																		patchSelectOption(ri, ci, oi, {
																			label: (e.currentTarget as HTMLInputElement).value
																		})}
																/>
																<input
																	class="h-6 w-20 rounded border border-input bg-background px-1.5 font-mono text-[10.5px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
																	placeholder="value"
																	value={o.value ?? ''}
																	oninput={(e) =>
																		patchSelectOption(ri, ci, oi, {
																			value: (e.currentTarget as HTMLInputElement).value
																		})}
																/>
																<button
																	type="button"
																	class="grid size-5 shrink-0 place-items-center rounded text-muted-foreground transition-colors hover:text-destructive"
																	onclick={() =>
																		patchComponent(ri, ci, {
																			options: (c.options ?? []).filter(
																				(_: unknown, idx: number) => idx !== oi
																			)
																		})}
																	aria-label="Remove option"
																>
																	<Trash2 size={10} />
																</button>
															</div>
														{/each}
														<button
															type="button"
															class="inline-flex h-6 items-center gap-1 rounded px-1 text-[10.5px] font-medium text-muted-foreground transition-colors hover:text-foreground disabled:opacity-40"
															disabled={(c.options ?? []).length >= 25}
															onclick={() =>
																patchComponent(ri, ci, {
																	options: [
																		...(c.options ?? []),
																		{ label: `Option ${(c.options ?? []).length + 1}`, value: '' }
																	]
																})}
														>
															<Plus size={10} />
															option
														</button>
													</div>
												{/if}
											</Popover.Content>
										</Popover.Root>
									{/if}
								{/each}
								{#if !rowHasSelect(row) && row.components.length < 5}
									<button
										type="button"
										class="grid h-8 w-9 place-items-center rounded-[3px] border border-dashed border-white/15 text-[#6d6f78] transition-colors hover:border-white/30 hover:text-[#b5bac1]"
										title="Add a button to this row"
										onclick={() => addToRow(ri)}
									>
										<Plus size={13} />
									</button>
								{/if}
							</div>
						{/each}
					</div>
				{/if}

				{#if ephemeral && s.ephemeral}
					<p class="mt-2 flex items-center gap-1.5 text-[10.5px] italic text-[#949ba4]">
						<EyeOff size={10} />
						Only the invoker can see this — ephemeral
					</p>
				{/if}
			</div>
		</div>
	</div>

	<!-- Composer toolbar — what you can add to this message. Lives OUTSIDE the
	     Discord surface so it's always obvious and never reads as part of it. -->
	<div class="flex flex-wrap items-center gap-1.5">
		{#if embeds}
			<button
				type="button"
				class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line bg-bg px-2.5 text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:bg-surface hover:text-ink disabled:pointer-events-none disabled:opacity-40"
				onclick={addEmbed}
				disabled={embedList.length >= 10}
			>
				<!-- An embed, drawn as one: rounded card with the left accent bar. -->
				<svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
					<rect
						x="0.75"
						y="1.4"
						width="10.5"
						height="9.2"
						rx="1.75"
						stroke="currentColor"
						stroke-width="1.2"
					/>
					<rect x="2.6" y="3.2" width="1.3" height="5.6" rx="0.65" fill="currentColor" />
					<path
						d="M5.7 4.6h3.4M5.7 7.4h2.2"
						stroke="currentColor"
						stroke-width="1.1"
						stroke-linecap="round"
						opacity="0.65"
					/>
				</svg>
				Embed
				<span class="font-mono text-[9.5px] tabular-nums text-faint">{embedList.length}/10</span>
			</button>
		{/if}
		{#if components}
			<button
				type="button"
				class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line bg-bg px-2.5 text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:bg-surface hover:text-ink disabled:pointer-events-none disabled:opacity-40"
				onclick={addButtonRow}
				disabled={rowList.length >= 5}
			>
				<MousePointerClick size={12} />
				Buttons
			</button>
			<button
				type="button"
				class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line bg-bg px-2.5 text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:bg-surface hover:text-ink disabled:pointer-events-none disabled:opacity-40"
				onclick={addSelectRow}
				disabled={rowList.length >= 5}
			>
				<List size={12} />
				Select menu
			</button>
		{/if}
		{#if attachments}
			<input
				bind:this={attachFileEl}
				type="file"
				accept="image/png,image/jpeg,image/webp,image/gif"
				class="hidden"
				onchange={onAttachFile}
			/>
			<button
				type="button"
				class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line bg-bg px-2.5 text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:bg-surface hover:text-ink disabled:opacity-50"
				disabled={attachUploading}
				onclick={() => attachFileEl?.click()}
			>
				<Upload size={12} />
				{attachUploading ? 'Uploading…' : 'Upload image'}
			</button>
			<button
				type="button"
				class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line bg-bg px-2.5 text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:bg-surface hover:text-ink"
				title="Attach an image from a variable (image_render / image_load)"
				onclick={addVarAttachment}
			>
				<Variable size={12} />
				From variable
			</button>
		{/if}
	</div>

	{#if attachError}
		<p class="text-[10.5px] text-danger">{attachError}</p>
	{/if}
</div>

<style>
	.dc-content {
		background: transparent;
		border: none;
		outline: none;
		border-radius: 3px;
	}
	.dc-content::placeholder {
		color: #6d6f78;
	}
	.dc-content:hover {
		background: rgba(255, 255, 255, 0.03);
	}
	.dc-content:focus {
		background: rgba(255, 255, 255, 0.04);
	}
</style>
