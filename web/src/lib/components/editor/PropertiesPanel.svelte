<script lang="ts">
	// Right-hand inspector for the layout editor — a Figma-style Design panel. Reads
	// the shared EditorStore from context and edits the selection (single OR many)
	// through `editor.common`/`editor.setAll` so a multi-select shows the SAME full
	// inspector with "Mixed" placeholders where values differ. When nothing is
	// selected it edits the canvas/document. Dense, hairline-ruled, no bordered-card
	// stacks, no gradients.
	//
	// Every numeric input is the SAME recognizable `field` (icon/short-glyph +
	// number, bg-ink-2) — there are no range sliders anywhere. Object operations
	// (edit / mask / boolean / flatten / select-matching) live in a header toolbar
	// of tooltipped icon buttons plus ONE portalled "all operations" dropdown, so
	// nothing clips inside this scrollable aside. Per-effect settings open in a
	// portalled popover so they never clip either.
	import { getContext } from 'svelte';
	import { DropdownMenu, Tooltip, Popover } from 'bits-ui';
	import { EditorStore, EDITOR_CTX, type AlignEdge, type PaintTarget } from '$lib/layout/editor.svelte';
	import { inspectorAnchor } from '$lib/layout/inspectorAnchor';
	import { SIZE_PRESETS, clampCanvas, EFFECT_LABELS } from '$lib/layout/schema';
	import { CARD_FONTS } from '$lib/layout/fonts';
	import type {
		Effect,
		Paint,
		HandleMode,
		StrokeAlign,
		StrokeStyle,
		StrokeCap,
		StrokeJoin,
		ClipMode,
		BoolOp,
		EffectType,
		Align,
		VAlign,
		TextCase,
		TextDecoration
	} from '$lib/layout/schema';
	import { cardVarsFor } from '$lib/layout/vars';
	import Select from '$lib/components/Select.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ImageInput from '$lib/components/editor/ImageInput.svelte';
	import InspectorSection from '$lib/components/editor/InspectorSection.svelte';
	import StrokeStyleSelect from '$lib/components/editor/StrokeStyleSelect.svelte';
	import BrushSelect from '$lib/components/editor/BrushSelect.svelte';
	import StrokeSidesSelect from '$lib/components/editor/StrokeSidesSelect.svelte';
	import StrokeProfileSelect from '$lib/components/editor/StrokeProfileSelect.svelte';
	import ArrowCapSelect from '$lib/components/editor/ArrowCapSelect.svelte';
	import { uploadFont, deleteFont } from '$lib/api';
	import PaintPicker from '$lib/components/editor/PaintPicker.svelte';
	import { scrub } from '$lib/actions/scrub';
	import {
		Copy,
		Trash2,
		Group,
		Ungroup,
		AlignStartVertical,
		AlignCenterVertical,
		AlignEndVertical,
		AlignStartHorizontal,
		AlignCenterHorizontal,
		AlignEndHorizontal,
		AlignHorizontalDistributeCenter,
		AlignVerticalDistributeCenter,
		Repeat2,
		Upload,
		Loader2,
		Scissors,
		SquarePen,
		Eye,
		EyeOff,
		Plus,
		Minus,
		ChevronUp,
		ChevronDown,
		X,
		Check,
		Spline,
		Waypoints,
		Boxes,
		SquareDashedBottom,
		Contrast,
		Ellipsis,
		Type,
		AlignLeft,
		AlignCenter,
		AlignRight,
		ArrowUpToLine,
		AlignVerticalJustifyCenter,
		ArrowDownToLine,
		CaseSensitive,
		CaseUpper,
		CaseLower,
		Underline,
		Strikethrough,
		SlidersHorizontal,
		Scaling,
		Link2,
		Unlink,
		Blend,
		Crop,
		SunMedium,
		Eclipse,
		Activity,
		Waves,
		ArrowLeft,
		ArrowRight
	} from 'lucide-svelte';
	import { slide, fade } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';

	const editor = getContext<EditorStore>(EDITOR_CTX);

	// Which card is being designed, scopes the click-to-insert variable chips
	// (welcome hides the rank-only level/XP/progress tokens). Defaults to 'rank'.
	let { context = 'rank' }: { context?: 'welcome' | 'rank' } = $props();
	const varChips = $derived(cardVarsFor(context));

	// The Typography <textarea>, so a variable chip can insert its token at the
	// caret (keeping focus) instead of appending to the end.
	let textArea = $state<HTMLTextAreaElement>();
	function insertVar(tmpl: string) {
		const cur = editor.common((l) => l.text ?? '') ?? '';
		const el = textArea;
		const s = el?.selectionStart ?? cur.length;
		const en = el?.selectionEnd ?? s;
		const next = cur.slice(0, s) + tmpl + cur.slice(en);
		editor.setAll((l) => (l.text = next));
		// Restore focus + place the caret after the inserted token once the
		// controlled value has flushed to the DOM.
		requestAnimationFrame(() => {
			if (!el) return;
			el.focus();
			const pos = s + tmpl.length;
			el.setSelectionRange(pos, pos);
		});
	}

	const alignButtons: { edge: AlignEdge; label: string; icon: typeof Group }[] = [
		{ edge: 'left', label: 'Align left', icon: AlignStartVertical },
		{ edge: 'hcenter', label: 'Align horizontal centers', icon: AlignCenterVertical },
		{ edge: 'right', label: 'Align right', icon: AlignEndVertical },
		{ edge: 'top', label: 'Align top', icon: AlignStartHorizontal },
		{ edge: 'vcenter', label: 'Align vertical centers', icon: AlignCenterHorizontal },
		{ edge: 'bottom', label: 'Align bottom', icon: AlignEndHorizontal }
	];

	// Identity of the current inspector context, so the panel body cross-fades when
	// the selection changes (the Svelte-native equivalent of motion's AnimatePresence).
	const panelKey = $derived(
		editor.selectedIds.length > 1
			? `multi:${editor.selectedIds.length}`
			: (editor.selected?.id ?? 'canvas')
	);

	// Friendly type badge — a path layer reads as "Vector".
	function typeLabel(t: string): string {
		return t === 'path' ? 'Vector' : t[0].toUpperCase() + t.slice(1);
	}

	// Boolean ops for the "all operations" combine section.
	const boolOps: [BoolOp, string][] = [
		['union', 'Union'],
		['subtract', 'Subtract'],
		['intersect', 'Intersect'],
		['exclude', 'Exclude']
	];

	const clipModes: [ClipMode, string, typeof Group][] = [
		['alpha', 'Alpha', Blend],
		['vector', 'Vector', Crop],
		['luminance', 'Luminance', SunMedium]
	];

	const effectTypeOptions = (
		['drop_shadow', 'inner_shadow', 'layer_blur', 'background_blur'] as EffectType[]
	).map((t) => ({ value: t, label: EFFECT_LABELS[t] }));

	// Items inside the portalled header dropdown reuse this class string (copied
	// from LayersPanel's `menuItem`, so highlighting/disabled match the rest).
	const menuItem =
		'flex w-full cursor-pointer items-center gap-2.5 rounded-md px-2 py-1.5 text-[13px] text-muted outline-none transition-colors data-[highlighted]:bg-ink-2 data-[highlighted]:text-ink data-[disabled]:pointer-events-none data-[disabled]:opacity-40';

	// ── Unified control variants (shadcn-on-Dia) ──────────────────────────────
	// One set of button class strings, applied identically wherever the same
	// variant intent appears, so every control in the editor reads the same.
	const btnBase =
		'inline-flex items-center justify-center gap-1.5 rounded-md text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-line-strong disabled:pointer-events-none disabled:opacity-40';
	const btnSecondary = `${btnBase} border border-line-strong text-ink hover:bg-ink-2`;
	const btnGhost = `${btnBase} text-muted hover:bg-ink-2 hover:text-ink`;
	const btnDestructive = `${btnBase} border border-line-strong text-muted hover:border-danger hover:text-danger`;
	// icon-square base: add the active treatment (`border-faint bg-ink-2 text-ink`)
	// or the rest treatment per-use; sizes (h-7 w-7 / h-8 w-8) are appended inline.
	const btnIcon = btnBase;

	// stepVal nudges a numeric field by ±step (the custom up/down stepper that replaces
	// the browser's native number spinner), clamped to min/max and de-floated.
	function stepVal(
		value: number | undefined,
		opts: { min?: number; max?: number; step?: number },
		dir: 1 | -1
	): number {
		const step = opts.step ?? 1;
		let n = (value ?? 0) + dir * step;
		if (opts.min != null) n = Math.max(opts.min, n);
		if (opts.max != null) n = Math.min(opts.max, n);
		return Math.round(n * 1e6) / 1e6;
	}

	// Whether the selection can carry a corner radius (rect / image, or a rounded
	// avatar). Multi-aware via selectionType + a common shape check for avatars.
	const showCorners = $derived(
		editor.selectionType === 'rect' || editor.selectionType === 'image'
	);

	// setCanvasSize clamps to the shared resolution budget so the canvas can be any
	// aspect ratio without letting the server-side render allocate unbounded memory.
	function setCanvasSize(w: number, h: number) {
		const c = clampCanvas(w, h);
		editor.layout.width = c.width;
		editor.layout.height = c.height;
	}
	const presetValue = () =>
		SIZE_PRESETS.find((p) => p.width === editor.layout.width && p.height === editor.layout.height)
			?.label ?? 'custom';

	// Typed scale factor for the Resize tool's Apply (e.g. 1.0, 1.2, 0.75).
	let scaleInput = $state(1);

	// Which Stroke-settings tab is active — DERIVED from the selection's stroke TYPE, so the
	// Basic/Dynamic/Brush tabs work as Figma's mutually-exclusive type switcher (clicking one
	// applies it via editor.setStrokeMode, which seeds defaults).
	const strokeTab = $derived(editor.strokeMode);

	// Opacity reads/writes as a whole-number percent (0..100) → the field, not a slider.
	const opacityPct = $derived.by(() => {
		const c = editor.common((l) => l.opacity ?? 1);
		return c === undefined ? undefined : Math.round(c * 100);
	});

	// ── custom (premium) font upload ──────────────────────────────────────────
	let fontFile = $state<HTMLInputElement>();
	let fontBusy = $state(false);
	let fontErr = $state('');
	async function onFontUpload(file: File | null | undefined) {
		if (!file) return;
		fontBusy = true;
		fontErr = '';
		try {
			const f = await uploadFont(editor.guildId, file);
			editor.addFont(f);
			// Apply the new family to every selected text layer.
			editor.setAll((l) => {
				if (l.type === 'text') l.font_family = f.family;
			});
		} catch (e) {
			fontErr = e instanceof Error ? e.message : 'Upload failed';
		} finally {
			fontBusy = false;
		}
	}
	async function onFontDelete(family: string) {
		try {
			await deleteFont(editor.guildId, family);
			editor.removeFont(family);
		} catch {
			/* leave it; the next list refresh will reconcile */
		}
	}

	// ── effect colour ↔ one solid paint ────────────────────────────────────────
	// A shadow's colour + opacity edit as a single #RRGGBB[AA] through the SAME
	// PaintPicker dropdown as every other colour in the editor (solid-only mode).
	function effectPaint(e: Effect): Paint {
		const c = (e.color ?? '#000000').toUpperCase();
		if (/^#[0-9A-F]{8}$/.test(c)) return { type: 'solid', color: c }; // legacy 8-digit colour
		const op = Math.round(Math.min(1, Math.max(0, e.opacity ?? 0.25)) * 255);
		return {
			type: 'solid',
			color: op >= 255 ? c : c + op.toString(16).padStart(2, '0').toUpperCase()
		};
	}
	function setEffectColor(i: number, hex: string | undefined) {
		if (!hex) return;
		const m = /^#([0-9a-fA-F]{6})([0-9a-fA-F]{2})?$/.exec(hex);
		if (!m) return;
		editor.updateEffect(i, {
			color: '#' + m[1].toUpperCase(),
			opacity: m[2] ? parseInt(m[2], 16) / 255 : 1
		});
	}
</script>

<!-- Reusable bits ──────────────────────────────────────────────────────────── -->

<!-- glyphIcon: the Figma-style 12px property glyphs that lucide doesn't have — corner
     radius (uniform + each corner), rotation angle, line height, letter spacing,
     stroke weight, dash/gap and blur/spread. Referenced from `field` via 'i:<name>'. -->
{#snippet glyphIcon(name: string)}
	<svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
		{#if name === 'rotate'}
			<path d="M3.5 3.5 V12.5 H12.5" />
			<path d="M3.5 7.5 A5 5 0 0 1 8.5 12.5" stroke-opacity="0.55" />
		{:else if name === 'radius'}
			<path d="M3.5 12.5 V8.5 A5 5 0 0 1 8.5 3.5 H12.5" />
		{:else if name === 'tl'}
			<path d="M4 12.5 V8 A4 4 0 0 1 8 4 H12.5" />
		{:else if name === 'tr'}
			<path d="M3.5 4 H8 A4 4 0 0 1 12 8 V12.5" />
		{:else if name === 'br'}
			<path d="M12 3.5 V8 A4 4 0 0 1 8 12 H3.5" />
		{:else if name === 'bl'}
			<path d="M12.5 12 H8 A4 4 0 0 1 4 8 V3.5" />
		{:else if name === 'lineheight'}
			<path d="M3 2.7 H13 M3 13.3 H13" />
			<path d="M8 5 V11 M6.3 6.6 L8 4.9 L9.7 6.6 M6.3 9.4 L8 11.1 L9.7 9.4" />
		{:else if name === 'letterspacing'}
			<path d="M5.4 9.6 L8 3.4 L10.6 9.6 M6.3 7.8 H9.7" />
			<path d="M3 11.4 V14 M13 11.4 V14 M3 12.7 H13" />
		{:else if name === 'weight'}
			<path d="M3 3.8 H13" stroke-width="1" />
			<path d="M3 7.8 H13" stroke-width="1.8" />
			<path d="M3 12 H13" stroke-width="2.8" />
		{:else if name === 'dash'}
			<path d="M2.5 8 H13.5" stroke-width="1.8" stroke-dasharray="3 2.2" />
		{:else if name === 'gapline'}
			<path d="M4.5 3.5 V12.5 M11.5 3.5 V12.5" />
			<path d="M6.8 8 H9.2" stroke-opacity="0.55" />
		{:else if name === 'blur'}
			<path d="M8 3.2 A4.8 4.8 0 0 0 8 12.8" />
			<path d="M8 3.2 A4.8 4.8 0 0 1 8 12.8" stroke-dasharray="1.6 2" />
		{:else if name === 'spread'}
			<circle cx="8" cy="8" r="2.2" />
			<circle cx="8" cy="8" r="5.4" stroke-dasharray="2 2.4" />
		{/if}
	</svg>
{/snippet}

<!-- field: THE one numeric field used everywhere — a recognizable "glyph + number"
     control on a bg-ink-2 chip. `glyph` is a short string label (e.g. 'W'), an
     'i:<name>' custom glyph (see glyphIcon), or a lucide icon component; drag the
     glyph to scrub. A `value` of undefined means the selected layers disagree →
     empty input with a "Mixed" placeholder. -->
{#snippet field(
	glyph: string | typeof Group,
	value: number | undefined,
	set: (n: number) => void,
	opts: { min?: number; max?: number; step?: number; suffix?: string } = {}
)}
	<label
		class="group flex h-7 min-w-0 items-stretch overflow-hidden rounded-md border border-line bg-ink-2 transition-all hover:border-line-strong focus-within:border-faint focus-within:ring-2 focus-within:ring-line-strong"
	>
		<span
			use:scrub={{ get: () => value ?? 0, set, step: opts.step ?? 1, min: opts.min, max: opts.max }}
			title="Drag to change"
			class="grid w-6 shrink-0 cursor-ew-resize select-none place-items-center border-r border-line/70 bg-white/[0.02] text-[10px] font-semibold leading-none text-faint transition-colors group-hover:text-muted group-focus-within:text-ink"
		>
			{#if typeof glyph === 'string'}{#if glyph.startsWith('i:')}{@render glyphIcon(glyph.slice(2))}{:else}{glyph}{/if}{:else}{@const I = glyph}<I size={12} strokeWidth={2} />{/if}
		</span>
		<input
			type="number"
			value={value === undefined ? '' : value}
			placeholder={value === undefined ? 'Mixed' : undefined}
			min={opts.min}
			max={opts.max}
			step={opts.step ?? 1}
			oninput={(e) => set(e.currentTarget.valueAsNumber || 0)}
			class="w-full min-w-0 bg-transparent px-2 text-xs tabular-nums text-ink outline-none placeholder:text-faint"
		/>
		{#if opts.suffix}<span class="grid shrink-0 select-none place-items-center pr-2 text-[10px] text-faint">{opts.suffix}</span>{/if}
		<!-- Custom up/down stepper (replaces the native number spinner; reveals on hover). -->
		<span
			class="flex shrink-0 flex-col self-stretch border-l border-line/70 opacity-0 transition-opacity duration-100 group-hover:opacity-100 group-focus-within:opacity-100"
		>
			<button
				type="button"
				tabindex="-1"
				aria-label="Increment"
				onclick={() => set(stepVal(value, opts, 1))}
				class="grid w-[18px] flex-1 place-items-center text-faint transition-colors hover:bg-white/10 hover:text-ink"
			>
				<ChevronUp size={9} strokeWidth={3} />
			</button>
			<button
				type="button"
				tabindex="-1"
				aria-label="Decrement"
				onclick={() => set(stepVal(value, opts, -1))}
				class="grid w-[18px] flex-1 place-items-center border-t border-line/50 text-faint transition-colors hover:bg-white/10 hover:text-ink"
			>
				<ChevronDown size={9} strokeWidth={3} />
			</button>
		</span>
	</label>
{/snippet}

<!-- A compact labelled row: caption on the left, control on the right. -->
{#snippet row(caption: string)}
	<span class="w-16 shrink-0 text-xs text-muted">{caption}</span>
{/snippet}

<!-- A roomier label column for the Stroke-settings popover (Figma's two-column rows:
     a wider label gutter so "Frequency" / "Profile" / "Direction" never truncate). -->
{#snippet prow(caption: string)}
	<span class="w-24 shrink-0 text-xs text-muted">{caption}</span>
{/snippet}

<!-- paintRows: THE paint-stack editor — the identical row list (PaintPicker chip +
     per-paint opacity + hide + remove, top paint first like Figma) for every paint
     slot: a layer's Fill, its Stroke, and the canvas Background. -->
{#snippet paintRows(target: PaintTarget, empty: string)}
	{@const ps = editor.paints(target)}
	{#if ps.length}
		<div class="space-y-1.5">
			{#each ps.slice().reverse() as p, ri (ps.length - 1 - ri)}
				{@const pi = ps.length - 1 - ri}
				<div class="flex items-center gap-1.5 {p.hidden ? 'opacity-50' : ''}">
					<PaintPicker
						paint={p}
						guildId={editor.guildId}
						patch={(patch) => editor.setPaint(target, pi, patch)}
						convert={(t) => editor.convertPaint(target, pi, t)}
						setStops={(stops) => editor.setPaintStops(target, pi, stops)}
					/>
					<div class="w-20 shrink-0">
						{@render field(Contrast, Math.round((p.opacity ?? 1) * 100), (n) =>
							editor.setPaint(target, pi, { opacity: Math.min(1, Math.max(0, n / 100)) }), {
							min: 0,
							max: 100
						})}
					</div>
					<button
						type="button"
						title={p.hidden ? 'Show paint' : 'Hide paint'}
						aria-label={p.hidden ? 'Show paint' : 'Hide paint'}
						onclick={() => editor.togglePaintHidden(target, pi)}
						class="{btnGhost} h-7 w-7 shrink-0"
					>
						{#if p.hidden}<EyeOff size={13} />{:else}<Eye size={13} />{/if}
					</button>
					<button
						type="button"
						title="Remove paint"
						aria-label="Remove paint"
						onclick={() => editor.removePaint(target, pi)}
						class="{btnBase} h-7 w-7 shrink-0 text-faint hover:bg-ink-2 hover:text-danger"
					>
						<X size={13} />
					</button>
				</div>
			{/each}
		</div>
	{:else}
		<p class="text-[11px] text-faint">{empty}</p>
	{/if}
{/snippet}

<!-- Generic segmented control. items: [value, label][]. current === '' (Mixed)
     simply leaves every segment unselected. -->
{#snippet segmented(current: string, items: [string, string][], set: (v: string) => void)}
	<div class="flex gap-0.5 rounded-lg border border-line bg-ink-2 p-1">
		{#each items as [val, lbl] (val)}
			<button
				type="button"
				onclick={() => set(val)}
				class="flex-1 rounded-md px-2 py-1 text-xs font-medium capitalize transition-all duration-100 {current ===
				val
					? 'bg-surface text-ink shadow-sm ring-1 ring-line-strong'
					: 'text-muted hover:bg-surface/50 hover:text-ink'}"
			>
				{lbl}
			</button>
		{/each}
	</div>
{/snippet}

<!-- Icon segmented control. items: [value, title, icon][] — a lucide icon (or a
     short text glyph when no good icon exists) per segment; title is the tooltip /
     aria-label. current === '' (Mixed) leaves every segment unselected. -->
{#snippet segIcons(
	current: string,
	items: [string, string, typeof Group | string][],
	set: (v: string) => void
)}
	<div class="flex gap-0.5 rounded-lg border border-line bg-ink-2 p-1">
		{#each items as [val, title, icon] (val)}
			<button
				type="button"
				{title}
				aria-label={title}
				onclick={() => set(val)}
				class="grid h-6 flex-1 place-items-center rounded-md transition-all duration-100 {current ===
				val
					? 'bg-surface text-ink shadow-sm ring-1 ring-line-strong'
					: 'text-muted hover:bg-surface/50 hover:text-ink'}"
			>
				{#if typeof icon === 'string'}<span class="text-[11px] font-medium">{icon}</span>{:else}{@const I =
						icon}<I size={14} />{/if}
			</button>
		{/each}
	</div>
{/snippet}

<!-- strokeGlyph: a tiny SVG that DEPICTS a stroke option (style/cap/join) using the very
     property it sets, so each button reads at a glance — like Figma's icon controls. -->
{#snippet strokeGlyph(kind: string, val: string)}
	<svg width="16" height="16" viewBox="0 0 18 18" fill="none" stroke="currentColor" aria-hidden="true">
		{#if kind === 'style'}
			<line x1="2.5" y1="9" x2="15.5" y2="9" stroke-width="2" stroke-linecap="round" stroke-dasharray={val === 'dashed' ? '3.5 3' : 'none'} />
		{:else if kind === 'cap'}
			<!-- heavy stroke ending at the faint end-guide; the shape PAST the guide reads at a
			     glance: flush = Butt, rounded bulge = Round, square block = Square. -->
			<line x1="11" y1="3.5" x2="11" y2="14.5" stroke-width="1.3" stroke-opacity="0.4" />
			<path d="M3 9 H 11" stroke-width="6.5" stroke-linecap={val as 'butt' | 'round' | 'square'} />
			{:else}
				<!-- heavy right-angle whose OUTER corner shows the join: sharp point = Miter,
				     flat chamfer = Bevel, rounded = Round (exaggerated by the heavy weight). -->
				<path d="M6 15 V 6 H 15" stroke-width="5.5" stroke-linecap="butt" stroke-linejoin={val as 'miter' | 'bevel' | 'round'} />
			{/if}
	</svg>
{/snippet}
<!-- iconSeg: a COMPACT row of small icon buttons (Figma's cap/join controls) — fixed-width
     squares, not stretched, so the switcher stays small. label → tooltip. -->
{#snippet iconSeg(current: string, kind: string, items: [string, string][], set: (v: string) => void)}
	<div class="flex shrink-0 gap-0.5 rounded-md border border-line bg-ink-2 p-0.5">
		{#each items as [val, title] (val)}
			<button
				type="button"
				{title}
				aria-label={title}
				onclick={() => set(val)}
				class="grid h-6 w-6 place-items-center rounded transition-colors {current === val
					? 'bg-surface text-ink shadow-sm ring-1 ring-line-strong'
					: 'text-faint hover:text-ink'}"
			>
				{@render strokeGlyph(kind, val)}
			</button>
		{/each}
	</div>
{/snippet}

<!-- Figma-style Fill: the "+" in the header adds a paint; rows edit the stack. -->
{#snippet fillSection()}
	<InspectorSection title="Fill">
		{#snippet action()}
			<button
				type="button"
				title="Add fill"
				aria-label="Add fill"
				onclick={() => editor.addPaint('fill')}
				class="{btnGhost} h-5 w-5"
			>
				<Plus size={14} />
			</button>
		{/snippet}
		{@render paintRows('fill', 'No fill. Add one with +.')}
	</InspectorSection>
{/snippet}

<!-- Figma-style Stroke: the "+" in the header adds it, then colour · Weight · Position
     (box shapes) · Style · Join · Cap. `showPosition` for box shapes, `showCaps` for paths,
     `advanced` for the settings popover (off for text), `sides` for per-side rect strokes. -->
{#snippet strokeSection(showPosition: boolean, showCaps: boolean, advanced: boolean, sides: boolean)}
	<InspectorSection title="Stroke">
		{#snippet action()}
			{#if editor.hasStroke}
				<div class="flex items-center gap-0.5">
					<!-- Figma: "+" on an existing stroke stacks ANOTHER paint onto it. -->
					<button
						type="button"
						title="Add stroke paint"
						aria-label="Add stroke paint"
						onclick={() => editor.addPaint('stroke')}
						class="{btnGhost} h-5 w-5"
					>
						<Plus size={14} />
					</button>
					{#if advanced}
					<Popover.Root>
						<Popover.Trigger
							title="Advanced stroke settings"
							aria-label="Advanced stroke settings"
							class="{btnGhost} grid h-5 w-5 place-items-center data-[state=open]:bg-ink-2 data-[state=open]:text-ink"
						>
							<SlidersHorizontal size={13} />
						</Popover.Trigger>
						<Popover.Portal>
							<Popover.Content
								customAnchor={inspectorAnchor}
								side="left"
								align="start"
								sideOffset={10}
								collisionPadding={12}
								class="menu-pop z-50 w-64 space-y-2 rounded-xl border border-line-strong bg-surface p-3 shadow-2xl shadow-black/60 ring-1 ring-black/40 outline-none"
								>
								<div class="flex items-center justify-between border-b border-line pb-2">
									<span class="text-xs font-semibold text-ink">Stroke settings</span>
									<Popover.Close class="{btnGhost} grid h-5 w-5 place-items-center" aria-label="Close">
										<X size={13} />
									</Popover.Close>
								</div>

								{@render segmented(
									strokeTab,
									[
										['basic', 'Basic'],
										['dynamic', 'Dynamic'],
										['brush', 'Brush']
									],
									(v) => editor.setStrokeMode(v as 'basic' | 'dynamic' | 'brush')
								)}

								{#if strokeTab === 'basic'}
									<div class="space-y-2">
										<div class="flex items-center justify-between gap-3">
											{@render prow('Style')}
											<div class="min-w-0 flex-1">
												<StrokeStyleSelect value={editor.strokeStyle} set={(v) => editor.setStrokeStyle(v)} />
											</div>
										</div>
										{#if showCaps && !editor.strokeDashed}
											<div class="flex items-center justify-between gap-3">
												{@render prow('Profile')}
												<div class="min-w-0 flex-1">
													<StrokeProfileSelect value={editor.widthProfile} set={(v) => editor.setWidthProfile(v)} />
												</div>
											</div>
										{/if}
										{#if editor.strokeDashed}
											<div class="flex items-center justify-between gap-3">
												{@render prow('Dash')}
												{@render field('i:dash', editor.dash, (n) => editor.setDash(n), { min: 0, suffix: 'px' })}
											</div>
											<div class="flex items-center justify-between gap-3">
												{@render prow('Gap')}
												{@render field('i:gapline', editor.gap, (n) => editor.setGap(n), { min: 0, suffix: 'px' })}
											</div>
										{/if}
										{#if showCaps || editor.strokeDashed}
											<div class="flex items-center justify-between gap-3">
												{@render prow(showCaps ? 'Cap' : 'Dash cap')}
												{@render iconSeg(
													editor.strokeCap,
													'cap',
													[
														['butt', 'None'],
														['round', 'Round'],
														['square', 'Square']
													],
													(v) => editor.setStrokeCap(v as StrokeCap)
												)}
											</div>
										{/if}
										<div class="flex items-center justify-between gap-3">
											{@render prow('Join')}
											{@render iconSeg(
												editor.strokeJoin,
												'join',
												[
													['miter', 'Miter'],
													['bevel', 'Bevel'],
													['round', 'Round']
												],
												(v) => editor.setStrokeJoin(v as StrokeJoin)
											)}
										</div>
										{#if editor.strokeJoin === 'miter'}
											<div class="flex items-center justify-between gap-3">
												{@render prow('Miter angle')}
												{@render field('∠', editor.miterAngle, (n) => editor.setMiterAngle(n), {
													min: 0,
													max: 180,
													suffix: '°'
												})}
											</div>
										{/if}
										{#if showCaps && !editor.strokeDashed}
											<div class="flex items-center justify-between gap-3">
												{@render prow('Start point')}
												<div class="min-w-0 flex-1">
													<ArrowCapSelect flip value={editor.startCap} set={(v) => editor.setStartCap(v)} />
												</div>
											</div>
											<div class="flex items-center justify-between gap-3">
												{@render prow('End point')}
												<div class="min-w-0 flex-1">
													<ArrowCapSelect value={editor.endCap} set={(v) => editor.setEndCap(v)} />
												</div>
											</div>
										{/if}
									</div>
								{:else if strokeTab === 'dynamic'}
									<div class="space-y-2">
										<div class="flex items-center justify-between gap-3">
											{@render prow('Frequency')}
											{@render field(Activity, editor.dynamicFrequency, (n) => editor.setDynamicFrequency(n), {
												min: 0,
												max: 100,
												suffix: '%'
											})}
										</div>
										<div class="flex items-center justify-between gap-3">
											{@render prow('Wiggle')}
											{@render field(Waves, editor.dynamicWiggle, (n) => editor.setDynamicWiggle(n), {
												min: 0,
												max: 200,
												suffix: '%'
											})}
										</div>
										<div class="flex items-center justify-between gap-3">
											{@render prow('Smoothen')}
											{@render field(Spline, editor.dynamicSmoothen, (n) => editor.setDynamicSmoothen(n), {
												min: 0,
												max: 100,
												suffix: '%'
											})}
										</div>
										<p class="pt-0.5 text-[11px] text-faint">A hand-drawn wobble along the vector path.</p>
									</div>
								{:else}
			<div class="space-y-2">
				<div class="flex items-center justify-between gap-3">
					{@render prow('Brush')}
					<div class="min-w-0 flex-1">
						<BrushSelect value={editor.brushName} set={(v) => editor.setBrushName(v)} />
					</div>
				</div>
				{#if editor.brushKind === 'scatter'}
					<div class="flex items-center justify-between gap-3">
						{@render prow('Gap')}
						{@render field('i:gapline', editor.scatterGap, (n) => editor.setScatterGap(n), { min: 0.05, max: 8, step: 0.05, suffix: '×' })}
					</div>
					<div class="flex items-center justify-between gap-3">
						{@render prow('Wiggle')}
						{@render field(Waves, editor.scatterWiggle, (n) => editor.setScatterWiggle(n), { min: 0, max: 100, suffix: '%' })}
					</div>
					<div class="flex items-center justify-between gap-3">
						{@render prow('Size jitter')}
						{@render field(Scaling, editor.scatterSize, (n) => editor.setScatterSize(n), { min: 0, max: 100, suffix: '%' })}
					</div>
					<div class="flex items-center justify-between gap-3">
						{@render prow('Rotation')}
						{@render field('i:rotate', editor.scatterRotation, (n) => editor.setScatterRotation(n), { min: -180, max: 180, suffix: '°' })}
					</div>
					<div class="flex items-center justify-between gap-3">
						{@render prow('Angular jitter')}
						{@render field('∠', editor.scatterAngular, (n) => editor.setScatterAngular(n), { min: 0, max: 180, suffix: '°' })}
					</div>
					<p class="pt-0.5 text-[11px] text-faint">A mark stippled along the stroke.</p>
				{:else}
					<div class="flex items-center justify-between gap-3">
						{@render prow('Direction')}
						<div class="flex gap-0.5 rounded-md border border-line bg-ink-2 p-0.5">
							<button type="button" title="Backward" aria-label="Backward" onclick={() => editor.setBrushDirection('backward')} class="grid h-6 w-9 place-items-center rounded transition-colors {editor.brushDirection === 'backward' ? 'bg-surface text-ink shadow-sm ring-1 ring-line-strong' : 'text-faint hover:text-ink'}"><ArrowLeft size={14} /></button>
							<button type="button" title="Forward" aria-label="Forward" onclick={() => editor.setBrushDirection('forward')} class="grid h-6 w-9 place-items-center rounded transition-colors {editor.brushDirection === 'forward' ? 'bg-surface text-ink shadow-sm ring-1 ring-line-strong' : 'text-faint hover:text-ink'}"><ArrowRight size={14} /></button>
						</div>
					</div>
					<p class="pt-0.5 text-[11px] text-faint">A calligraphic stroke stretched along the path.</p>
				{/if}
			</div>
								{/if}
							</Popover.Content>
						</Popover.Portal>
					</Popover.Root>
					{/if}
					<button
						type="button"
						title="Remove stroke"
						aria-label="Remove stroke"
						onclick={() => editor.removeStroke()}
						class="{btnGhost} h-5 w-5"
					>
						<Minus size={14} />
					</button>
				</div>
			{:else}
				<button
					type="button"
					title="Add stroke"
					aria-label="Add stroke"
					onclick={() => editor.addStroke()}
					class="{btnGhost} h-5 w-5"
				>
					<Plus size={14} />
				</button>
			{/if}
		{/snippet}
		{#if editor.hasStroke}
			<div class="space-y-2.5">
				{@render paintRows('stroke', 'No stroke paint. Add one with +.')}
				<div class="flex items-center justify-between gap-3">
					{@render row('Weight')}
					<div class="flex items-center gap-1.5">
						{@render field(
							'i:weight',
							editor.common((l) => l.stroke_width ?? 0),
							(n) => editor.setAll((l) => (l.stroke_width = Math.max(0, n))),
							{ min: 0 }
						)}
						{#if sides}
							<StrokeSidesSelect
								isSide={(s) => editor.isStrokeSide(s)}
								toggle={(s) => editor.toggleStrokeSide(s)}
								reset={() => editor.setAllStrokeSides()}
							/>
						{/if}
					</div>
				</div>
				{#if showPosition}
					<div class="flex items-center justify-between gap-3">
						{@render row('Position')}
						<div class="w-full">
							<Select
								dense
								bind:value={
									() => editor.strokeAlign,
									(v) => editor.setStrokeAlign(v as StrokeAlign)
								}
								options={[
									{ value: 'inside', label: 'Inside' },
									{ value: 'center', label: 'Center' },
									{ value: 'outside', label: 'Outside' }
								]}
							/>
						</div>
					</div>
				{/if}
			</div>
		{:else}
			<p class="text-[11px] text-faint">No border. Add one with +.</p>
		{/if}
	</InspectorSection>
{/snippet}

<!-- A tooltipped object-ops toolbar icon button. Uses bits-ui's `child` pattern so
     the trigger IS our own button (no nested/double button). -->
{#snippet tipBtn(
	label: string,
	active: boolean,
	disabled: boolean,
	onclick: () => void,
	icon: typeof Group
)}
	{@const Icon = icon}
	<Tooltip.Root>
		<Tooltip.Trigger>
			{#snippet child({ props })}
				<button
					{...props}
					type="button"
					aria-label={label}
					{disabled}
					{onclick}
					class="{btnIcon} h-8 w-8 border {active
						? 'border-faint bg-ink-2 text-ink'
						: 'border-line-strong text-muted hover:border-faint hover:text-ink'}"
				>
					<Icon size={15} />
				</button>
			{/snippet}
		</Tooltip.Trigger>
		<Tooltip.Portal>
			<Tooltip.Content
				sideOffset={6}
				class="z-[60] rounded-md border border-line-strong bg-ink-2 px-2 py-1 text-[11px] font-medium text-ink shadow-lg"
			>
				{label}
			</Tooltip.Content>
		</Tooltip.Portal>
	</Tooltip.Root>
{/snippet}

<!-- boolGlyph: Figma-style boolean-op icons (lucide has no squares-* set). -->
{#snippet boolGlyph(op: string)}
	<svg width="14" height="14" viewBox="0 0 18 18" fill="none" aria-hidden="true">
		{#if op === 'union'}
			<rect x="2.5" y="2.5" width="9" height="9" rx="2" fill="currentColor" />
			<rect x="6.5" y="6.5" width="9" height="9" rx="2" fill="currentColor" />
		{:else if op === 'subtract'}
			<rect x="2.5" y="2.5" width="9" height="9" rx="2" fill="currentColor" />
			<rect
				x="6.5"
				y="6.5"
				width="9"
				height="9"
				rx="2"
				fill="var(--color-surface)"
				stroke="currentColor"
				stroke-width="1.5"
			/>
		{:else if op === 'intersect'}
			<rect x="2.5" y="2.5" width="9" height="9" rx="2" stroke="currentColor" stroke-width="1.5" />
			<rect x="6.5" y="6.5" width="9" height="9" rx="2" stroke="currentColor" stroke-width="1.5" />
			<rect x="6.5" y="6.5" width="5" height="5" fill="currentColor" />
		{:else}
			<rect x="2.5" y="2.5" width="9" height="9" rx="2" fill="currentColor" />
			<rect x="6.5" y="6.5" width="9" height="9" rx="2" fill="currentColor" />
			<rect x="6.5" y="6.5" width="5" height="5" fill="var(--color-surface)" />
		{/if}
	</svg>
{/snippet}

<!-- resizeView: the Scale tool's dedicated transform inspector (Figma-style) — a
     different right-sidebar focused purely on resizing/transforming the selection. -->
{#snippet resizeView()}
	{@const one = editor.selected}
	{@const multi = editor.selectedIds.length > 1}
	<header class="flex items-center justify-between gap-2 border-b border-line px-4 py-3">
		<div class="flex min-w-0 items-center gap-2">
			<Scaling size={15} class="shrink-0 text-ink" />
			<h2 class="truncate text-sm font-semibold text-ink">Resize</h2>
		</div>
		<span class="shrink-0 text-xs text-faint">
			{multi ? `${editor.selectedIds.length} layers` : one ? typeLabel(one.type) : ''}
		</span>
	</header>

	<InspectorSection title="Dimensions">
		<div class="flex items-center gap-1.5">
			<div class="min-w-0 flex-1">
				{@render field('W', editor.common((l) => l.w), (n) => editor.resizeW(n), { min: 8 })}
			</div>
			<button
				type="button"
				title={editor.aspectLocked ? 'Aspect ratio locked' : 'Lock aspect ratio'}
				aria-label="Lock aspect ratio"
				aria-pressed={editor.aspectLocked}
				onclick={() => editor.toggleAspect()}
				class="{btnIcon} h-8 w-8 shrink-0 border {editor.aspectLocked
					? 'border-faint bg-ink-2 text-ink'
					: 'border-line-strong text-muted hover:border-faint hover:text-ink'}"
			>
				{#if editor.aspectLocked}<Link2 size={14} />{:else}<Unlink size={14} />{/if}
			</button>
			<div class="min-w-0 flex-1">
				{@render field('H', editor.common((l) => l.h), (n) => editor.resizeH(n), { min: 8 })}
			</div>
		</div>
		<p class="mt-2 text-[11px] text-faint">
			{editor.aspectLocked
				? 'Width & height scale together.'
				: 'Width & height resize independently.'}
		</p>
	</InspectorSection>

	<InspectorSection title="Scale">
		<div class="flex items-center gap-1.5">
			<div class="flex-1">
				{@render field('×', scaleInput, (n) => (scaleInput = n), { step: 0.1, min: 0.01 })}
			</div>
			<button
				type="button"
				title="Scale the selection by this factor"
				onclick={() => editor.scaleSelection(scaleInput)}
				class="{btnSecondary} h-7 shrink-0 px-3"
			>
				Apply
			</button>
		</div>
		<div class="mt-2 grid grid-cols-2 gap-2">
			<button type="button" onclick={() => editor.scaleSelection(0.5)} class="{btnSecondary} h-7">
				50%
			</button>
			<button type="button" onclick={() => editor.scaleSelection(2)} class="{btnSecondary} h-7">
				200%
			</button>
		</div>
		<p class="mt-2 text-[11px] text-faint">
			Type a factor (1.0 = no change, 1.2 = +20%) and Apply, or use a preset. Scales size, text,
			strokes & radius about each layer's centre.
		</p>
	</InspectorSection>

	<InspectorSection title="Position">
		<div class="grid grid-cols-2 gap-2">
			{@render field('X', editor.common((l) => l.x), (n) =>
				editor.setAll((l) => (l.x = Math.round(n)))
			)}
			{@render field('Y', editor.common((l) => l.y), (n) =>
				editor.setAll((l) => (l.y = Math.round(n)))
			)}
		</div>
		<div class="mt-2">
			{@render field('i:rotate', editor.common((l) => l.rotation ?? 0), (n) =>
				editor.setAll((l) => (l.rotation = n)), { suffix: '°' }
			)}
		</div>
	</InspectorSection>

	{#if multi}
		<InspectorSection title="Align">
			<div class="grid grid-cols-6 gap-1">
				{#each alignButtons as a (a.edge)}
					{@const Icon = a.icon}
					<button
						type="button"
						title={a.label}
						aria-label={a.label}
						onclick={() => editor.align(a.edge)}
						class="{btnIcon} h-8 border border-line-strong text-muted hover:border-faint hover:text-ink"
					>
						<Icon size={15} />
					</button>
				{/each}
			</div>
		</InspectorSection>
		<InspectorSection title="Distribute">
			<div class="grid grid-cols-2 gap-1">
				<button
					type="button"
					title="Even horizontal spacing (3+ layers)"
					onclick={() => editor.distribute('h')}
					disabled={editor.selectedIds.length < 3}
					class="{btnSecondary} h-8"
				>
					<AlignHorizontalDistributeCenter size={15} /> Horizontal
				</button>
				<button
					type="button"
					title="Even vertical spacing (3+ layers)"
					onclick={() => editor.distribute('v')}
					disabled={editor.selectedIds.length < 3}
					class="{btnSecondary} h-8"
				>
					<AlignVerticalDistributeCenter size={15} /> Vertical
				</button>
			</div>
		</InspectorSection>
	{/if}

	<footer class="mt-auto border-t border-line bg-surface px-4 py-3">
		<p class="text-[11px] text-faint">
			Tip: drag the canvas handles with the Scale tool to resize visually.
		</p>
	</footer>
{/snippet}

<!-- Panel ──────────────────────────────────────────────────────────────────── -->

<aside data-inspector class="flex h-full w-full flex-col overflow-y-auto bg-surface text-sm">
	{#key panelKey}
		<div class="flex min-h-full flex-col" in:fade={{ duration: 120, easing: cubicOut }}>
			{#if editor.selectedLayers.length >= 1}
				<!-- ── Unified layer inspector (handles 1 AND many) ────────────────────── -->
				{@const one = editor.selected}
				{@const multi = editor.selectedIds.length > 1}

				{#if editor.tool === 'scale'}
					{@render resizeView()}
				{:else}
				<header class="flex items-center justify-between gap-2 border-b border-line px-4 py-3">
					{#if multi}
						<div class="min-w-0">
							<h2 class="truncate text-sm font-semibold text-ink">
								{editor.selectedIds.length} layers
							</h2>
							<p class="mt-0.5 text-xs text-faint">{editor.selectionType ?? 'Mixed'} layers</p>
						</div>
					{:else if one}
						<h2 class="truncate text-sm font-semibold text-ink">{one.name}</h2>
						<span
							class="shrink-0 rounded border border-line-strong px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide text-faint"
						>
							{typeLabel(one.type)}
						</span>
					{/if}
				</header>

				<!-- ── Object-operations toolbar (always shown) ────────────────────── -->
				<Tooltip.Provider delayDuration={250}>
					<div class="flex items-center gap-1 border-b border-line px-4 py-2">
						{#if editor.canEditObject}
							{@render tipBtn(
								editor.editId ? 'Done editing' : 'Edit object',
								!!editor.editId,
								false,
								() => (editor.editId ? editor.exitEdit() : editor.editSelected()),
								SquarePen
							)}
						{/if}

						{@render tipBtn(
							editor.isMask ? 'Release mask' : 'Use as mask',
							editor.isMask,
							!editor.isMask && !editor.canMask,
							() => editor.toggleMask(),
							Scissors
						)}
						{@render tipBtn(
							'Flatten to vector path',
							false,
							!editor.canFlatten,
							() => editor.flatten(),
							Waypoints
						)}
						{@render tipBtn(
							`Select all ${editor.selectionType ?? 'matching'} layers`,
							false,
							false,
							() => editor.selectMatching(),
							Boxes
						)}

						<div class="flex-1"></div>

						<!-- One comprehensive "all operations" menu (portalled, never clips). -->
						<DropdownMenu.Root>
							<DropdownMenu.Trigger
								title="All operations"
								aria-label="All operations"
								class="{btnIcon} h-8 w-8 border border-line-strong text-muted hover:border-faint hover:text-ink data-[state=open]:border-faint data-[state=open]:text-ink"
							>
								<Ellipsis size={15} />
							</DropdownMenu.Trigger>
							<DropdownMenu.Portal>
								<DropdownMenu.Content
									align="end"
									sideOffset={6}
									class="menu-pop z-50 min-w-[210px] rounded-xl border border-line-strong bg-surface p-1.5 shadow-2xl shadow-black/60 ring-1 ring-black/40 outline-none"
								>
									{#if editor.canEditObject}
										<DropdownMenu.Item
											class={menuItem}
											onSelect={() => (editor.editId ? editor.exitEdit() : editor.editSelected())}
										>
											<SquarePen size={14} class="text-faint" />
											{editor.editId ? 'Done editing' : 'Edit object'}
										</DropdownMenu.Item>
										<DropdownMenu.Separator class="my-1 h-px bg-line" />
									{/if}

									<div
										class="px-2 py-1 text-[10px] font-medium uppercase tracking-[0.09em] text-faint"
									>
										Mask
									</div>
									{#if !editor.isMask}
										<DropdownMenu.Item
											class={menuItem}
											disabled={!editor.canMask}
											onSelect={() => editor.useAsMask()}
										>
											<Scissors size={14} class="text-faint" /> Use as mask
										</DropdownMenu.Item>
									{:else}
										<DropdownMenu.Item class={menuItem} onSelect={() => editor.toggleMask()}>
											<Scissors size={14} class="text-faint" /> Release mask
										</DropdownMenu.Item>
									{/if}
									{#each clipModes as [val, lbl, Icon] (val)}
										<DropdownMenu.Item
											class={menuItem}
											disabled={!editor.isMask && !editor.canMask}
											closeOnSelect={false}
											onSelect={() => editor.maskAs(val)}
										>
											<Icon size={14} class="text-faint" />
											{lbl}
											{#if editor.isMask && editor.clipMode === val}
												<Check size={14} class="ml-auto text-ink" />
											{/if}
										</DropdownMenu.Item>
									{/each}
									<DropdownMenu.Item
										class={menuItem}
										disabled={!editor.isMask}
										closeOnSelect={false}
										onSelect={() => editor.setClipInvert(!editor.clipInvert)}
									>
										<Eclipse size={14} class="text-faint" />
										Invert
										{#if editor.isMask && editor.clipInvert}
											<Check size={14} class="ml-auto text-ink" />
										{/if}
									</DropdownMenu.Item>

									<DropdownMenu.Separator class="my-1 h-px bg-line" />
									<div
										class="px-2 py-1 text-[10px] font-medium uppercase tracking-[0.09em] text-faint"
									>
										Combine
									</div>
									{#each boolOps as [val, lbl] (val)}
										<DropdownMenu.Item
											class={menuItem}
											disabled={!editor.canBoolean && !editor.isBoolean}
											closeOnSelect={false}
											onSelect={() => editor.applyBoolean(val)}
										>
											<span class="grid w-3.5 shrink-0 place-items-center text-faint">
												{@render boolGlyph(val)}
											</span>
											{lbl}
											{#if editor.isBoolean && editor.boolOp === val}
												<Check size={14} class="ml-auto text-ink" />
											{/if}
										</DropdownMenu.Item>
									{/each}
									{#if editor.isBoolean}
										<DropdownMenu.Item class={menuItem} onSelect={() => editor.clearBoolean()}>
											<Ungroup size={14} class="text-faint" /> Release combine
										</DropdownMenu.Item>
									{/if}

									<DropdownMenu.Separator class="my-1 h-px bg-line" />
									<DropdownMenu.Item
										class={menuItem}
										disabled={!editor.canFlatten}
										onSelect={() => editor.flatten()}
									>
										<Waypoints size={14} class="text-faint" /> Flatten to vector
									</DropdownMenu.Item>
									<DropdownMenu.Item class={menuItem} onSelect={() => editor.selectMatching()}>
										<Boxes size={14} class="text-faint" /> Select all
										{editor.selectionType ?? 'matching'} layers
									</DropdownMenu.Item>
								</DropdownMenu.Content>
							</DropdownMenu.Portal>
						</DropdownMenu.Root>
					</div>
				</Tooltip.Provider>

				<!-- ── Position ──────────────────────────────────────────────────── -->
				<InspectorSection title="Position">
					<div class="grid grid-cols-2 gap-2">
						{@render field('X', editor.common((l) => l.x), (n) =>
							editor.setAll((l) => (l.x = Math.round(n)))
						)}
						{@render field('Y', editor.common((l) => l.y), (n) =>
							editor.setAll((l) => (l.y = Math.round(n)))
						)}
						{@render field(
							'W',
							editor.common((l) => l.w),
							(n) => editor.setAll((l) => (l.w = Math.max(8, Math.round(n)))),
							{ min: 8 }
						)}
						{@render field(
							'H',
							editor.common((l) => l.h),
							(n) => editor.setAll((l) => (l.h = Math.max(8, Math.round(n)))),
							{ min: 8 }
						)}
					</div>
					<div class="mt-2 grid grid-cols-2 gap-2">
						{@render field('i:rotate', editor.common((l) => l.rotation ?? 0), (n) =>
							editor.setAll((l) => (l.rotation = n)), { suffix: '°' }
						)}
						{@render field(Contrast, opacityPct, (n) =>
							editor.setAll((l) => (l.opacity = Math.min(1, Math.max(0, n / 100)))), {
							min: 0,
							max: 100,
							suffix: '%'
						})}
					</div>
				</InspectorSection>

				<!-- ── Appearance (corner radius) ────────────────────────────────── -->
				{#if showCorners}
					<InspectorSection title="Corners">
						<div class="flex items-center gap-2">
							<div class="flex-1">
								{@render field(
									'i:radius',
									editor.common((l) =>
										Array.isArray(l.corners) && l.corners.length === 4 ? l.corners[0] : (l.radius ?? 0)
									),
									(n) => editor.setAllCorners(n),
									{ min: 0 }
								)}
							</div>
							<button
								type="button"
								title={editor.cornersActive ? 'Uniform radius' : 'Independent corners'}
								aria-label={editor.cornersActive ? 'Uniform radius' : 'Independent corners'}
								aria-pressed={editor.cornersActive}
								onclick={() => (editor.cornersActive ? editor.collapseCorners() : editor.expandCorners())}
								class="{btnIcon} h-7 w-7 shrink-0 border {editor.cornersActive
									? 'border-faint bg-ink-2 text-ink'
									: 'border-line-strong text-muted hover:border-faint hover:text-ink'}"
							>
								<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" aria-hidden="true"><rect x="2.6" y="2.6" width="10.8" height="10.8" rx="3.6" stroke-opacity="0.3" /><path d="M2.6 9.2 V 6.2 A 3.6 3.6 0 0 1 6.2 2.6 H 9.2" stroke-linecap="round" /></svg>
							</button>
						</div>
						{#if editor.cornersActive}
							<div
								class="mt-2 grid grid-cols-2 gap-2"
								transition:slide={{ duration: 180, easing: cubicOut }}
							>
								{@render field(
									'i:tl',
									editor.common((l) => l.corners?.[0] ?? l.radius ?? 0),
									(n) => editor.setCorner(0, n),
									{ min: 0 }
								)}
								{@render field(
									'i:tr',
									editor.common((l) => l.corners?.[1] ?? l.radius ?? 0),
									(n) => editor.setCorner(1, n),
									{ min: 0 }
								)}
								{@render field(
									'i:bl',
									editor.common((l) => l.corners?.[3] ?? l.radius ?? 0),
									(n) => editor.setCorner(3, n),
									{ min: 0 }
								)}
								{@render field(
									'i:br',
									editor.common((l) => l.corners?.[2] ?? l.radius ?? 0),
									(n) => editor.setCorner(2, n),
									{ min: 0 }
								)}
							</div>
						{/if}
					</InspectorSection>
				{/if}

				<!-- ── Type-specific sections (gated on selectionType, multi-aware) ── -->
				{#if editor.selectionType === 'text'}
					<InspectorSection title="Typography">
						<div class="space-y-3">
							<div>
								<textarea
									bind:this={textArea}
									rows="3"
									value={editor.common((l) => l.text ?? '') ?? ''}
									placeholder={editor.common((l) => l.text ?? '') === undefined ? 'Mixed' : undefined}
									oninput={(e) => {
										const v = e.currentTarget.value;
										editor.setAll((l) => (l.text = v));
									}}
									class="w-full resize-y rounded-lg border border-line bg-ink-2 px-2.5 py-2 text-sm leading-snug text-ink outline-none transition-all placeholder:text-faint hover:border-faint focus:border-faint focus:ring-2 focus:ring-line-strong"
								></textarea>
								<!-- Click-to-insert variable chips (insert at the caret, keep focus). -->
								<div class="mt-1.5 flex flex-wrap gap-1">
									{#each varChips as v (v.tmpl)}
										<button
											type="button"
											title={v.tmpl}
											onmousedown={(e) => e.preventDefault()}
											onclick={() => insertVar(v.tmpl)}
											class="rounded border border-line px-1.5 py-0.5 font-mono text-[10px] text-muted transition-colors hover:border-line-strong hover:text-ink"
										>
											{v.label}
										</button>
									{/each}
								</div>
							</div>

							<Select
								dense
								bind:value={
									() => editor.common((l) => l.font_family ?? '') ?? '',
									(v) => editor.setAll((l) => (l.font_family = v))
								}
								placeholder={editor.common((l) => l.font_family ?? '') === undefined
									? 'Mixed'
									: 'Default (Lato)'}
								options={[
									{ value: '', label: 'Default (Lato)' },
									...CARD_FONTS.map((f) => ({ value: f.family, label: f.family })),
									...editor.customFonts.map((f) => ({
										value: f.family,
										label: `${f.family} (custom)`
									}))
								]}
							/>

							<!-- Custom (premium) fonts: upload + manage -->
							{#if editor.premium}
								<div class="space-y-1.5">
									<button
										type="button"
										onclick={() => fontFile?.click()}
										disabled={fontBusy}
										class="{btnSecondary} w-full px-2 py-1.5"
									>
										{#if fontBusy}<Loader2 size={13} class="animate-spin" />{:else}<Upload
												size={13}
											/>{/if}
										Upload font (TTF/OTF)
									</button>
									<input
										bind:this={fontFile}
										type="file"
										accept=".ttf,.otf,font/ttf,font/otf"
										class="hidden"
										onchange={(e) => {
											onFontUpload(e.currentTarget.files?.[0]);
											e.currentTarget.value = '';
										}}
									/>
									{#if fontErr}<p class="text-[11px] text-danger">{fontErr}</p>{/if}
									{#each editor.customFonts as f (f.family)}
										<div
											class="flex items-center justify-between gap-2 rounded-md bg-ink-2 px-2 py-1 text-[11px] text-muted"
										>
											<span class="truncate" style="font-family:'{f.family}', sans-serif;">{f.family}</span>
											<button
												type="button"
												onclick={() => onFontDelete(f.family)}
												aria-label="Remove font"
												class="grid h-5 w-5 shrink-0 place-items-center rounded text-faint transition-colors hover:bg-surface hover:text-danger"
											>
												<X size={12} />
											</button>
										</div>
									{/each}
								</div>
							{:else}
								<p class="text-[11px] text-faint">
									Upload your own fonts with <span class="text-ink">Premium</span>.
								</p>
							{/if}

							<div class="grid grid-cols-2 gap-2">
								{@render field(
									Type,
									editor.common((l) => l.font_size ?? 0),
									(n) => editor.setAll((l) => (l.font_size = Math.max(1, Math.round(n)))),
									{ min: 1 }
								)}
								<Select
									dense
									bind:value={
										() => {
											const c = editor.common((l) => l.font_weight ?? 400);
											return c === undefined ? '' : String(c);
										},
										(v) => editor.setAll((l) => (l.font_weight = Number(v)))
									}
									placeholder="Mixed"
									options={[
										{ value: '400', label: 'Regular' },
										{ value: '700', label: 'Bold' }
									]}
								/>
							</div>

							<div class="grid grid-cols-2 gap-2">
								{@render field(
									'i:lineheight',
									editor.common((l) => l.line_height ?? 1.3),
									(n) => editor.setAll((l) => (l.line_height = Math.max(0, n))),
									{ min: 0, step: 0.1 }
								)}
								{@render field('i:letterspacing', editor.common((l) => l.letter_spacing ?? 0), (n) =>
									editor.setAll((l) => (l.letter_spacing = n))
								)}
							</div>

							<!-- Alignment: horizontal + vertical icon groups on one row. -->
							<div class="grid grid-cols-2 gap-2">
								{@render segIcons(
									editor.common((l) => l.align ?? 'left') ?? '',
									[
										['left', 'Align left', AlignLeft],
										['center', 'Align center', AlignCenter],
										['right', 'Align right', AlignRight]
									],
									(v) => editor.setAll((l) => (l.align = v as Align))
								)}
								{@render segIcons(
									editor.common((l) => l.valign ?? 'top') ?? '',
									[
										['top', 'Align top', ArrowUpToLine],
										['middle', 'Align middle', AlignVerticalJustifyCenter],
										['bottom', 'Align bottom', ArrowDownToLine]
									],
									(v) => editor.setAll((l) => (l.valign = v as VAlign))
								)}
							</div>

							<!-- Case + Decoration on one row. -->
							<div class="grid grid-cols-2 gap-2">
								{@render segIcons(
									editor.common((l) => l.text_case ?? 'none') ?? '',
									[
										['none', 'No case change', '–'],
										['upper', 'Uppercase', CaseUpper],
										['lower', 'Lowercase', CaseLower],
										['title', 'Title case', 'Ag']
									],
									(v) => editor.setAll((l) => (l.text_case = v as TextCase))
								)}
								{@render segIcons(
									editor.common((l) => l.text_decoration ?? 'none') ?? '',
									[
										['none', 'No decoration', '–'],
										['underline', 'Underline', Underline],
										['strike', 'Strikethrough', Strikethrough]
									],
									(v) => editor.setAll((l) => (l.text_decoration = v as TextDecoration))
								)}
							</div>
						</div>
					</InspectorSection>
					<!-- Text fill is the SAME paint stack as a shape's fill (gradient /
					     image text renders via glyph masks; the legacy colour stays synced). -->
					{@render fillSection()}
					{@render strokeSection(false, false, true, false)}
				{:else if editor.selectionType === 'image'}
					<InspectorSection title="Image">
						<div class="space-y-3">
							<div class="block">
								<span class="mb-1 block text-xs text-muted">Source</span>
								<ImageInput
									value={editor.common((l) => l.src ?? '') ?? ''}
									onChange={(v) => editor.setAll((l) => (l.src = v))}
									guildId={editor.guildId}
									placeholder={editor.common((l) => l.src ?? '') === undefined
										? 'Mixed'
										: 'https://… or {{.User.Avatar}}'}
								/>
								<!-- One-click presets for the two bound image sources. -->
								<div class="mt-1.5 flex flex-wrap gap-1">
									<button
										type="button"
										title="{'{{.User.Avatar}}'}"
										onclick={() => editor.setAll((l) => (l.src = '{{.User.Avatar}}'))}
										class="rounded border border-line px-1.5 py-0.5 text-[10px] text-muted transition-colors hover:border-line-strong hover:text-ink"
									>
										Member avatar
									</button>
									<button
										type="button"
										title="{'{{.Guild.Icon}}'}"
										onclick={() => editor.setAll((l) => (l.src = '{{.Guild.Icon}}'))}
										class="rounded border border-line px-1.5 py-0.5 text-[10px] text-muted transition-colors hover:border-line-strong hover:text-ink"
									>
										Server icon
									</button>
								</div>
							</div>
							<div class="flex items-center justify-between gap-3">
								{@render row('Fit')}
								<div class="w-full">
									<Select
										dense
										bind:value={
											() => editor.common((l) => l.fit ?? 'cover') ?? '',
											(v) => editor.setAll((l) => (l.fit = v as 'cover' | 'contain'))
										}
										placeholder="Mixed"
										options={[
											{ value: 'cover', label: 'Cover' },
											{ value: 'contain', label: 'Contain' }
										]}
									/>
								</div>
							</div>
						</div>
					</InspectorSection>
					<!-- Stroke (border) — Figma-style: an image is ROUNDED via the corner
					     radius (Appearance section) and BORDERED via this stroke. The legacy
					     circle/ellipse "Mask" and the "Ring" are gone (redundant): for a circle,
					     set the corner radius to max. -->
					{@render strokeSection(true, false, true, false)}
				{:else if editor.selectionType === 'rect' || editor.selectionType === 'ellipse'}
					{@render fillSection()}
					{@render strokeSection(true, false, true, editor.selectionType === 'rect')}
					{#if editor.selectionType === 'rect'}
						<!-- Bind a rect to XP progress: on the live rank card its width fills to
						     {{.Progress}}; welcome cards (no progress) render it full width. -->
						<InspectorSection title="Progress bar">
							<label class="flex items-center justify-between gap-2">
								<span class="text-xs text-muted">Bind width to XP progress</span>
								<Toggle
									checked={editor.common((l) => !!l.progress) ?? false}
									onchange={(v) => editor.setAll((l) => (l.progress = v || undefined))}
									label="Bind width to XP progress"
								/>
							</label>
							<p class="mt-1.5 text-[11px] text-faint">
								On the live card this bar fills to the member's XP progress. Welcome cards render it
								full width.
							</p>
						</InspectorSection>
					{/if}
				{:else if editor.selectionType === 'path'}
					<!-- Path controls are single-selection only. -->
					{#if editor.selectedIds.length === 1 && one}
						{#if editor.activePathNode}
							{@const node = editor.activePathNode}
							<InspectorSection title="Point">
								<div class="space-y-3">
									<div class="w-full">
										{@render segmented(
											node.m ?? 'corner',
											[
												['corner', 'Corner'],
												['mirror', 'Smooth'],
												['asym', 'Asym']
											],
											(v) => editor.setNodeType(editor.activeNode ?? 0, v as HandleMode)
										)}
									</div>
									<div class="grid grid-cols-2 gap-2">
										{@render field('X', node.x, (n) => editor.setActiveNodeX(n))}
										{@render field('Y', node.y, (n) => editor.setActiveNodeY(n))}
									</div>
									<button
										type="button"
										onclick={() => editor.deleteActiveNode()}
										disabled={(one.nodes?.length ?? 0) <= 2}
										class="{btnDestructive} w-full px-2 py-1.5"
									>
										<Trash2 size={13} /> Delete point
									</button>
								</div>
							</InspectorSection>
						{/if}

						{@render fillSection()}
						{@render strokeSection(false, true, true, false)}

						<InspectorSection title="Path">
							<div class="space-y-3">
								<button
									type="button"
									onclick={() => editor.reversePath()}
									class="{btnSecondary} w-full px-2 py-1.5"
								>
									<Repeat2 size={13} /> Reverse direction
								</button>
								<p class="text-[11px] text-faint">
									{one.nodes?.length ?? 0} nodes · {one.closed ? 'closed' : 'open'} — close a path on
									the canvas: click its first point with the pen, or drag an endpoint onto the
									other end.
								</p>
							</div>
						</InspectorSection>
					{:else}
						<!-- Multiple vector layers: fill + stroke (both setAll cleanly). -->
						{@render fillSection()}
						{@render strokeSection(false, true, true, false)}
					{/if}
				{/if}

				<!-- ── Effects (single-selection only) ─────────────────────────────── -->
				{#if editor.selectedIds.length === 1}
					<InspectorSection title="Effects">
						{#snippet action()}
							<button
								type="button"
								title="Add drop shadow"
								aria-label="Add drop shadow"
								onclick={() => editor.addEffect('drop_shadow')}
								class="{btnGhost} h-5 w-5"
							>
								<Plus size={14} />
							</button>
						{/snippet}

						{#if editor.effects.length === 0}
							<p class="text-[11px] text-faint">No effects. Add a shadow or blur with +.</p>
						{:else}
							<div class="space-y-2">
								{#each editor.effects as e, i (i)}
									<div class="flex items-center gap-1.5 {e.hidden ? 'opacity-50' : ''}">
										<button
											type="button"
											title={e.hidden ? 'Show effect' : 'Hide effect'}
											aria-label={e.hidden ? 'Show effect' : 'Hide effect'}
											onclick={() => editor.toggleEffectHidden(i)}
											class="{btnGhost} h-8 w-8 shrink-0"
										>
											{#if e.hidden}<EyeOff size={14} />{:else}<Eye size={14} />{/if}
										</button>
										<div class="min-w-0 flex-1">
											<Select
												dense
												bind:value={
													() => e.type,
													(v) => editor.updateEffect(i, { type: v as EffectType })
												}
												options={effectTypeOptions}
											/>
										</div>

										<!-- Per-effect settings live in a portalled popover so they never clip. -->
										<Popover.Root>
											<Popover.Trigger
												title="Effect settings"
												aria-label="Effect settings"
												class="{btnGhost} h-8 w-8 shrink-0 data-[state=open]:bg-ink-2 data-[state=open]:text-ink"
											>
												<SlidersHorizontal size={14} />
											</Popover.Trigger>
											<Popover.Portal>
												<Popover.Content
													customAnchor={inspectorAnchor}
													side="left"
													align="start"
													sideOffset={10}
													collisionPadding={12}
													class="menu-pop z-50 w-60 rounded-xl border border-line-strong bg-surface p-3 shadow-2xl shadow-black/60 ring-1 ring-black/40 outline-none"
													>
																									{#if e.type === 'drop_shadow' || e.type === 'inner_shadow'}
																										<div class="space-y-2">
																											<div class="grid grid-cols-2 gap-2">
																												{@render field('X', e.x ?? 0, (n) => editor.updateEffect(i, { x: n }))}
																												{@render field('Y', e.y ?? 0, (n) => editor.updateEffect(i, { y: n }))}
																											</div>
																											<div class="grid grid-cols-2 gap-2">
																												{@render field(
																													'i:blur',
																													e.radius ?? 0,
																													(n) => editor.updateEffect(i, { radius: Math.max(0, n) }),
																													{ min: 0 }
																												)}
																												{@render field('i:spread', e.spread ?? 0, (n) =>
																													editor.updateEffect(i, { spread: n })
																												)}
																											</div>
																											<!-- The shadow colour + opacity edit as ONE solid paint through
																											     the same dropdown as every other colour (alpha = opacity). -->
																											<div class="flex items-center justify-between gap-3">
																												{@render row('Color')}
																												<PaintPicker
																													paint={effectPaint(e)}
																													types={['solid']}
																													anchor={false}
																													guildId={editor.guildId}
																													patch={(p) => setEffectColor(i, p.color)}
																													convert={() => {}}
																													setStops={() => {}}
																												/>
																											</div>
																										</div>
																									{:else}
																										<div class="space-y-2">
																											{@render field(
																												'i:blur',
																												e.radius ?? 0,
																												(n) => editor.updateEffect(i, { radius: Math.max(0, n) }),
																												{ min: 0, suffix: 'px' }
																											)}
																											{#if e.type === 'background_blur'}
																												<p class="text-[11px] text-faint">
																													Blurs what's behind — needs a semi-transparent fill.
																												</p>
																											{/if}
																										</div>
																									{/if}
												</Popover.Content>
											</Popover.Portal>
										</Popover.Root>

										<button
											type="button"
											title="Remove effect"
											aria-label="Remove effect"
											onclick={() => editor.removeEffect(i)}
											class="{btnBase} h-8 w-8 shrink-0 text-faint hover:bg-ink-2 hover:text-danger"
										>
											<X size={14} />
										</button>
									</div>
								{/each}
							</div>
						{/if}
					</InspectorSection>
				{/if}

				<!-- ── Align / Distribute / Arrange (multi-selection) ──────────────── -->
				{#if editor.selectedIds.length >= 2}
					<InspectorSection title="Align">
						<div class="grid grid-cols-6 gap-1">
							{#each alignButtons as a (a.edge)}
								{@const Icon = a.icon}
								<button
									type="button"
									title={a.label}
									aria-label={a.label}
									onclick={() => editor.align(a.edge)}
									class="{btnIcon} h-8 border border-line-strong text-muted hover:border-faint hover:text-ink"
								>
									<Icon size={15} />
								</button>
							{/each}
						</div>
					</InspectorSection>

					<InspectorSection title="Distribute">
						<p class="pb-1.5 text-[11px] text-faint">Even out the spacing between 3+ layers.</p>
						<div class="grid grid-cols-2 gap-1">
							<button
								type="button"
								title="Even horizontal spacing — equalise the left-to-right gaps between 3+ layers"
								onclick={() => editor.distribute('h')}
								disabled={editor.selectedIds.length < 3}
								class="{btnSecondary} h-8"
							>
								<AlignHorizontalDistributeCenter size={15} /> Horizontal
							</button>
							<button
								type="button"
								title="Even vertical spacing — equalise the top-to-bottom gaps between 3+ layers"
								onclick={() => editor.distribute('v')}
								disabled={editor.selectedIds.length < 3}
								class="{btnSecondary} h-8"
							>
								<AlignVerticalDistributeCenter size={15} /> Vertical
							</button>
						</div>
					</InspectorSection>

					<InspectorSection title="Arrange">
						<div class="flex gap-2">
							{#if editor.canUngroup}
								<button
									type="button"
									onclick={() => editor.ungroup()}
									class="{btnSecondary} flex-1 px-2 py-1.5"
								>
									<Ungroup size={13} /> Ungroup
								</button>
							{:else}
								<button
									type="button"
									onclick={() => editor.group()}
									class="{btnSecondary} flex-1 px-2 py-1.5"
								>
									<Group size={13} /> Group
								</button>
							{/if}
						</div>
					</InspectorSection>
				{/if}

				<!-- ── Footer ──────────────────────────────────────────────────────── -->
				{#if multi}
					<footer class="mt-auto border-t border-line bg-surface px-4 py-3">
						<button
							type="button"
							onclick={() => editor.removeSelected()}
							class="{btnDestructive} w-full px-2 py-1.5"
						>
							<Trash2 size={13} /> Delete {editor.selectedIds.length} layers
						</button>
					</footer>
				{:else if one}
					<footer class="mt-auto flex gap-2 border-t border-line bg-surface px-4 py-3">
						<button
							type="button"
							onclick={() => editor.duplicateLayer(one.id)}
							class="{btnSecondary} flex-1 px-2 py-1.5"
						>
							<Copy size={13} /> Duplicate
						</button>
						<button
							type="button"
							onclick={() => editor.removeLayer(one.id)}
							class="{btnDestructive} flex-1 px-2 py-1.5"
						>
							<Trash2 size={13} /> Delete
						</button>
					</footer>
				{/if}
				{/if}
			{:else}
				<!-- ── No selection: canvas + background ─────────────────────────────── -->
				<header class="border-b border-line px-4 py-3">
					<h2 class="text-sm font-semibold text-ink">Canvas</h2>
					<p class="mt-0.5 text-xs text-muted">Nothing selected — editing the document.</p>
				</header>

				<InspectorSection title="Dimensions">
					<Select
						dense
						bind:value={
							() => presetValue(),
							(v) => {
								const p = SIZE_PRESETS.find((x) => x.label === v);
								if (p) setCanvasSize(p.width, p.height);
							}
						}
						options={[
							{ value: 'custom', label: 'Custom size' },
							...SIZE_PRESETS.map((p) => ({ value: p.label, label: p.label }))
						]}
					/>
					<div class="mt-2 grid grid-cols-2 gap-2">
						{@render field('W', editor.layout.width, (n) => setCanvasSize(n, editor.layout.height), {
							min: 1
						})}
						{@render field('H', editor.layout.height, (n) => setCanvasSize(editor.layout.width, n), {
							min: 1
						})}
					</div>
					<p class="mt-2 text-[11px] text-faint">
						Any aspect ratio · capped to ~4M px for fast rendering
					</p>
				</InspectorSection>

				<!-- The canvas background is the SAME paint stack as a layer's fill —
				     solids, all four gradients, images, stacked and individually hideable. -->
				<InspectorSection title="Background">
					{#snippet action()}
						<button
							type="button"
							title="Add background paint"
							aria-label="Add background paint"
							onclick={() => editor.addPaint('bg')}
							class="{btnGhost} h-5 w-5"
						>
							<Plus size={14} />
						</button>
					{/snippet}
					<div class="space-y-2.5">
						{@render paintRows('bg', 'No background. Add a paint with +.')}
						{#if editor.paints('bg').some((p) => p.type === 'image' && !p.hidden)}
							<div class="flex items-center justify-between gap-3">
								{@render row('Blur')}
								{@render field(
									'i:blur',
									editor.layout.background.blur ?? 0,
									(n) => (editor.layout.background.blur = Math.max(0, n)),
									{ min: 0, suffix: 'px' }
								)}
							</div>
						{/if}
					</div>
				</InspectorSection>
			{/if}
		</div>
	{/key}
</aside>
