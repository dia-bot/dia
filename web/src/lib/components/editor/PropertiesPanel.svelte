<script lang="ts">
	// Right-hand inspector for the layout editor. Reads the shared EditorStore from
	// context and mutates the selected layer (or the canvas/background when nothing
	// is selected) directly through its reactive $state proxy. Dense, hairline-ruled,
	// pro-editor styling — no bordered-card stacks, no per-heading icons.
	import { getContext } from 'svelte';
	import { EditorStore, EDITOR_CTX, type AlignEdge } from '$lib/layout/editor.svelte';
	import { SIZE_PRESETS, clampCanvas } from '$lib/layout/schema';
	import { CARD_FONTS } from '$lib/layout/fonts';
	import type { BackgroundType, Mask, HandleMode } from '$lib/layout/schema';
	import Select from '$lib/components/Select.svelte';
	import ColorPicker from '$lib/components/ui/ColorPicker.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ImageInput from '$lib/components/editor/ImageInput.svelte';
	import { uploadFont, deleteFont } from '$lib/api';
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
		X
	} from 'lucide-svelte';

	const alignButtons: { edge: AlignEdge; label: string; icon: typeof Group }[] = [
		{ edge: 'left', label: 'Align left', icon: AlignStartVertical },
		{ edge: 'hcenter', label: 'Align horizontal centers', icon: AlignCenterVertical },
		{ edge: 'right', label: 'Align right', icon: AlignEndVertical },
		{ edge: 'top', label: 'Align top', icon: AlignStartHorizontal },
		{ edge: 'vcenter', label: 'Align vertical centers', icon: AlignCenterHorizontal },
		{ edge: 'bottom', label: 'Align bottom', icon: AlignEndHorizontal }
	];

	const editor = getContext<EditorStore>(EDITOR_CTX);

	const bgTypes: { value: BackgroundType; label: string }[] = [
		{ value: 'solid', label: 'Solid' },
		{ value: 'gradient', label: 'Gradient' },
		{ value: 'image', label: 'Image' }
	];

	// Background sub-objects can be undefined on first switch; ensure a default so
	// the bound ColorPicker/number inputs always have something to write to.
	// setCanvasSize clamps to the shared resolution budget so the canvas can be any
	// aspect ratio without letting the server-side render allocate unbounded memory.
	function setCanvasSize(w: number, h: number) {
		const c = clampCanvas(w, h);
		editor.layout.width = c.width;
		editor.layout.height = c.height;
	}
	const presetValue = () =>
		SIZE_PRESETS.find(
			(p) => p.width === editor.layout.width && p.height === editor.layout.height
		)?.label ?? 'custom';

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
			const sel = editor.selected;
			if (sel?.type === 'text') sel.font_family = f.family; // apply the new font
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

	function setBgType(t: BackgroundType) {
		const bg = editor.layout.background;
		bg.type = t;
		if (t === 'solid' && bg.color === undefined) bg.color = '#141417';
		if (t === 'gradient') {
			if (bg.from === undefined) bg.from = '#FF6363';
			if (bg.to === undefined) bg.to = '#B244FC';
			if (bg.angle === undefined) bg.angle = 45;
		}
		if (t === 'image' && bg.blur === undefined) bg.blur = 0;
	}
</script>

<!-- Reusable bits ──────────────────────────────────────────────────────────── -->

{#snippet sectionLabel(text: string)}
	<div class="px-4 pb-1.5 pt-3 text-[10px] font-medium uppercase tracking-[0.09em] text-faint">
		{text}
	</div>
{/snippet}

{#snippet num(label: string, value: number, set: (n: number) => void, opts: { min?: number; step?: number } = {})}
	<label class="flex items-center gap-2">
		<span
			use:scrub={{ get: () => value, set, step: opts.step ?? 1, min: opts.min }}
			title="Drag to change"
			class="w-4 shrink-0 cursor-ew-resize select-none text-xs text-muted hover:text-ink"
		>{label}</span>
		<input
			type="number"
			value={value ?? 0}
			min={opts.min}
			step={opts.step ?? 1}
			oninput={(e) => set(e.currentTarget.valueAsNumber || 0)}
			class="h-8 w-full rounded-lg border border-line-strong bg-ink-2 px-2.5 text-sm tabular-nums text-ink outline-none transition-all hover:border-faint focus:border-faint focus:ring-2 focus:ring-line-strong"
		/>
	</label>
{/snippet}

<!-- A compact labelled row: caption on the left, control on the right. -->
{#snippet row(caption: string)}
	<span class="w-16 shrink-0 text-xs text-muted">{caption}</span>
{/snippet}

{#snippet slider(value: number, set: (n: number) => void, min: number, max: number, step: number)}
	<div class="flex items-center gap-3">
		<input
			type="range"
			{min}
			{max}
			{step}
			value={value ?? 0}
			oninput={(e) => set(e.currentTarget.valueAsNumber)}
			class="range h-1.5 w-full"
		/>
		<input
			type="number"
			{min}
			{max}
			{step}
			value={value ?? 0}
			oninput={(e) => set(e.currentTarget.valueAsNumber || 0)}
			class="h-8 w-14 shrink-0 rounded-lg border border-line-strong bg-ink-2 px-1.5 text-center text-xs tabular-nums text-ink outline-none transition-all hover:border-faint focus:border-faint focus:ring-2 focus:ring-line-strong"
		/>
	</div>
{/snippet}

<!-- ColorPicker bound through get/set so optional (string | undefined) fields stay
     type-safe and always feed the picker a defined hex. -->
{#snippet color(value: string | undefined, set: (hex: string) => void)}
	<ColorPicker
		bind:value={() => value ?? '#FFFFFF', (v) => set(v)}
		class="w-full justify-start"
	/>
{/snippet}

<!-- Generic 2..3-way segmented control. items: [value, label][] -->
{#snippet segmented(current: string, items: [string, string][], set: (v: string) => void)}
	<div class="flex gap-0.5 rounded-lg border border-line-strong bg-ink-2 p-1">
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

<!-- Panel ──────────────────────────────────────────────────────────────────── -->

<aside class="flex h-full w-full flex-col overflow-y-auto bg-surface text-sm">
	{#if editor.selectedIds.length > 1}
		<!-- ── Multiple layers selected: align / distribute / group ──────────── -->
		<header class="border-b border-line px-4 py-3">
			<h2 class="text-sm font-semibold text-ink">{editor.selectedIds.length} layers selected</h2>
			<p class="mt-0.5 text-xs text-muted">Align, distribute, or group them.</p>
		</header>

		{@render sectionLabel('Align')}
		<div class="grid grid-cols-6 gap-1 px-4 pb-3">
			{#each alignButtons as a (a.edge)}
				{@const Icon = a.icon}
				<button
					type="button"
					title={a.label}
					aria-label={a.label}
					onclick={() => editor.align(a.edge)}
					class="grid h-8 place-items-center rounded-md border border-line-strong text-muted transition-colors hover:border-faint hover:text-ink"
				>
					<Icon size={15} />
				</button>
			{/each}
		</div>

		{@render sectionLabel('Distribute')}
		<div class="grid grid-cols-2 gap-1 px-4 pb-4">
			<button
				type="button"
				title="Distribute horizontally"
				onclick={() => editor.distribute('h')}
				disabled={editor.selectedIds.length < 3}
				class="flex h-8 items-center justify-center gap-1.5 rounded-md border border-line-strong text-xs text-muted transition-colors hover:border-faint hover:text-ink disabled:opacity-40"
			>
				<AlignHorizontalDistributeCenter size={15} /> Horizontal
			</button>
			<button
				type="button"
				title="Distribute vertically"
				onclick={() => editor.distribute('v')}
				disabled={editor.selectedIds.length < 3}
				class="flex h-8 items-center justify-center gap-1.5 rounded-md border border-line-strong text-xs text-muted transition-colors hover:border-faint hover:text-ink disabled:opacity-40"
			>
				<AlignVerticalDistributeCenter size={15} /> Vertical
			</button>
		</div>

		<div class="border-t border-line"></div>
		<div class="flex gap-2 px-4 py-3">
			{#if editor.canUngroup}
				<button
					type="button"
					onclick={() => editor.ungroup()}
					class="flex flex-1 items-center justify-center gap-1.5 rounded-md border border-line-strong px-2 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-ink-2"
				>
					<Ungroup size={13} /> Ungroup
				</button>
			{:else}
				<button
					type="button"
					onclick={() => editor.group()}
					class="flex flex-1 items-center justify-center gap-1.5 rounded-md border border-line-strong px-2 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-ink-2"
				>
					<Group size={13} /> Group
				</button>
			{/if}
		</div>

		<footer class="mt-auto border-t border-line px-4 py-3">
			<button
				type="button"
				onclick={() => editor.removeSelected()}
				class="flex w-full items-center justify-center gap-1.5 rounded-md border border-line-strong px-2 py-1.5 text-xs font-medium text-muted transition-colors hover:border-accent hover:text-accent-ink"
			>
				<Trash2 size={13} /> Delete {editor.selectedIds.length} layers
			</button>
		</footer>
	{:else if !editor.selected}
		<!-- ── No selection: canvas + background ─────────────────────────────── -->
		<header class="border-b border-line px-4 py-3">
			<h2 class="text-sm font-semibold text-ink">Canvas</h2>
			<p class="mt-0.5 text-xs text-muted">Nothing selected — editing the document.</p>
		</header>

		{@render sectionLabel('Dimensions')}
		<div class="px-4 pb-2">
			<Select
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
		</div>
		<div class="grid grid-cols-2 gap-2 px-4 pb-1">
			{@render num('W', editor.layout.width, (n) => setCanvasSize(n, editor.layout.height), { min: 1 })}
			{@render num('H', editor.layout.height, (n) => setCanvasSize(editor.layout.width, n), { min: 1 })}
		</div>
		<p class="px-4 pb-3 text-[11px] text-faint">Any aspect ratio · capped to ~4M px for fast rendering</p>

		<div class="border-t border-line"></div>
		{@render sectionLabel('Background')}
		<div class="space-y-3 px-4 pb-4">
			{@render segmented(
				editor.layout.background.type,
				bgTypes.map((b) => [b.value, b.label]),
				(v) => setBgType(v as BackgroundType)
			)}

			{#if editor.layout.background.type === 'solid'}
				<div class="flex items-center justify-between gap-3">
					{@render row('Fill')}
					{@render color(editor.layout.background.color, (v) => (editor.layout.background.color = v))}
				</div>
			{:else if editor.layout.background.type === 'gradient'}
				<div class="flex items-center justify-between gap-3">
					{@render row('From')}
					{@render color(editor.layout.background.from, (v) => (editor.layout.background.from = v))}
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('To')}
					{@render color(editor.layout.background.to, (v) => (editor.layout.background.to = v))}
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Angle')}
					{@render num('°', editor.layout.background.angle ?? 0, (n) => (editor.layout.background.angle = n))}
				</div>
			{:else}
				<div class="block">
					<span class="mb-1 block text-xs text-muted">Image</span>
					<ImageInput
						value={editor.layout.background.image_url ?? ''}
						onChange={(v) => (editor.layout.background.image_url = v)}
						guildId={editor.guildId}
						placeholder="https://… or upload"
					/>
				</div>
				<div>
					<span class="mb-1.5 block text-xs text-muted">Blur</span>
					{@render slider(
						editor.layout.background.blur ?? 0,
						(n) => (editor.layout.background.blur = n),
						0,
						40,
						1
					)}
				</div>
			{/if}
		</div>
	{:else}
		<!-- ── Layer selected ────────────────────────────────────────────────── -->
		{@const layer = editor.selected}
		<header class="flex items-center justify-between gap-2 border-b border-line px-4 py-3">
			<h2 class="truncate text-sm font-semibold text-ink">{layer.name}</h2>
			<span
				class="shrink-0 rounded border border-line-strong px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide text-faint"
			>
				{layer.type}
			</span>
		</header>

		{@render sectionLabel('Position & size')}
		<div class="grid grid-cols-2 gap-2 px-4 pb-1">
			{@render num('X', layer.x, (n) => (layer.x = Math.round(n)))}
			{@render num('Y', layer.y, (n) => (layer.y = Math.round(n)))}
			{@render num('W', layer.w, (n) => (layer.w = Math.max(8, Math.round(n))), { min: 8 })}
			{@render num('H', layer.h, (n) => (layer.h = Math.max(8, Math.round(n))), { min: 8 })}
		</div>
		<div class="px-4 pb-3 pt-2">
			<span class="mb-1.5 block text-xs text-muted">Opacity</span>
			{@render slider(layer.opacity ?? 1, (n) => (layer.opacity = n), 0, 1, 0.01)}
		</div>
		<div class="flex items-center justify-between gap-3 px-4 pb-3">
			{@render row('Rotate')}
			{@render num('°', layer.rotation ?? 0, (n) => (layer.rotation = n))}
		</div>

		<div class="border-t border-line"></div>

		{#if layer.type === 'text'}
			{@render sectionLabel('Text')}
			<div class="space-y-3 px-4 pb-4">
				<div>
					<textarea
						rows="3"
						value={layer.text ?? ''}
						oninput={(e) => (layer.text = e.currentTarget.value)}
						class="w-full resize-y rounded-lg border border-line-strong bg-ink-2 px-2.5 py-2 text-sm leading-snug text-ink outline-none transition-all hover:border-faint focus:border-faint focus:ring-2 focus:ring-line-strong"
					></textarea>
					<p class="mt-1 text-[11px] text-faint">
						Variables: <span class="font-mono text-muted">{'{{.User.Username}}'} {'{{.Count}}'} {'{{.User.Avatar}}'}</span> · supports
						<span class="font-mono text-muted">{'{{if}}'}</span> logic
					</p>
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Font')}
					<div class="w-full">
						<Select
							bind:value={() => layer.font_family ?? '', (v) => (layer.font_family = v)}
							options={[
								{ value: '', label: 'Default (Lato)' },
								...CARD_FONTS.map((f) => ({ value: f.family, label: f.family })),
								...editor.customFonts.map((f) => ({ value: f.family, label: `${f.family} (custom)` }))
							]}
						/>
					</div>
				</div>
				<!-- Custom (premium) fonts: upload + manage -->
				{#if editor.premium}
					<div class="space-y-1.5">
						<button
							type="button"
							onclick={() => fontFile?.click()}
							disabled={fontBusy}
							class="flex w-full items-center justify-center gap-1.5 rounded-md border border-line-strong px-2 py-1.5 text-xs font-medium text-muted transition-colors hover:border-faint hover:text-ink disabled:opacity-50"
						>
							{#if fontBusy}<Loader2 size={13} class="animate-spin" />{:else}<Upload size={13} />{/if}
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
							<div class="flex items-center justify-between gap-2 rounded-md bg-ink-2 px-2 py-1 text-[11px] text-muted">
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
						Upload your own fonts with <span class="text-accent-ink">Premium</span>.
					</p>
				{/if}
				<div class="flex items-center justify-between gap-3">
					{@render row('Size')}
					{@render num('px', layer.font_size ?? 0, (n) => (layer.font_size = n), { min: 1 })}
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Weight')}
					<div class="w-full">
						<Select
							bind:value={
								() => String(layer.font_weight ?? 400), (v) => (layer.font_weight = Number(v))
							}
							options={[
								{ value: '400', label: 'Regular' },
								{ value: '700', label: 'Bold' }
							]}
						/>
					</div>
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Color')}
					{@render color(layer.color, (v) => (layer.color = v))}
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Align')}
					<div class="w-full">
						{@render segmented(
							layer.align ?? 'left',
							[
								['left', 'Left'],
								['center', 'Center'],
								['right', 'Right']
							],
							(v) => (layer.align = v as 'left' | 'center' | 'right')
						)}
					</div>
				</div>
			</div>
		{:else if layer.type === 'image'}
			{@render sectionLabel('Image')}
			<div class="space-y-3 px-4 pb-4">
				<div class="block">
					<span class="mb-1 block text-xs text-muted">Source</span>
					<ImageInput
						value={layer.src ?? ''}
						onChange={(v) => (layer.src = v)}
						guildId={editor.guildId}
						placeholder="https://… or {'{{.User.Avatar}}'}"
					/>
					<span class="mt-1 block text-[11px] text-faint">
						Supports <span class="font-mono text-muted">{'{{.User.Avatar}}'}</span>
					</span>
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Fit')}
					<div class="w-full">
						<Select
							bind:value={
								() => layer.fit ?? 'cover', (v) => (layer.fit = v as 'cover' | 'contain')
							}
							options={[
								{ value: 'cover', label: 'Cover' },
								{ value: 'contain', label: 'Contain' }
							]}
						/>
					</div>
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Mask')}
					<div class="w-full">
						<Select
							bind:value={() => layer.mask ?? '', (v) => (layer.mask = v as Mask)}
							options={[
								{ value: '', label: 'None (rounded)' },
								{ value: 'circle', label: 'Circle' },
								{ value: 'ellipse', label: 'Ellipse' }
							]}
						/>
					</div>
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Radius')}
					{@render num('px', layer.radius ?? 0, (n) => (layer.radius = Math.max(0, n)), { min: 0 })}
				</div>
			</div>
		{:else if layer.type === 'avatar'}
			{@render sectionLabel('Avatar')}
			<div class="space-y-3 px-4 pb-4">
				<div class="block">
					<span class="mb-1 block text-xs text-muted">Source</span>
					<ImageInput
						value={layer.src ?? ''}
						onChange={(v) => (layer.src = v)}
						guildId={editor.guildId}
						placeholder="{'{{.User.Avatar}}'} or upload"
					/>
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Shape')}
					<div class="w-full">
						<Select
							bind:value={
								() => layer.shape ?? 'circle', (v) => (layer.shape = v as 'circle' | 'rounded')
							}
							options={[
								{ value: 'circle', label: 'Circle' },
								{ value: 'rounded', label: 'Rounded' }
							]}
						/>
					</div>
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Ring')}
					{@render color(layer.ring_color, (v) => (layer.ring_color = v))}
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Ring width')}
					{@render num('px', layer.ring_width ?? 0, (n) => (layer.ring_width = Math.max(0, n)), {
						min: 0
					})}
				</div>
				{#if layer.shape === 'rounded'}
					<div class="flex items-center justify-between gap-3">
						{@render row('Radius')}
						{@render num('px', layer.radius ?? 0, (n) => (layer.radius = Math.max(0, n)), { min: 0 })}
					</div>
				{/if}
			</div>
		{:else if layer.type === 'rect'}
			{@render sectionLabel('Shape')}
			<div class="space-y-3 px-4 pb-4">
				<div class="flex items-center justify-between gap-3">
					{@render row('Fill')}
					{@render color(layer.fill, (v) => (layer.fill = v))}
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Radius')}
					{@render num('px', layer.radius ?? 0, (n) => (layer.radius = Math.max(0, n)), { min: 0 })}
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Stroke')}
					{@render color(layer.stroke_color, (v) => (layer.stroke_color = v))}
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Stroke W')}
					{@render num('px', layer.stroke_width ?? 0, (n) => (layer.stroke_width = Math.max(0, n)), { min: 0 })}
				</div>
			</div>
		{:else if layer.type === 'ellipse'}
			{@render sectionLabel('Ellipse')}
			<div class="space-y-3 px-4 pb-4">
				<div class="flex items-center justify-between gap-3">
					{@render row('Fill')}
					{@render color(layer.fill, (v) => (layer.fill = v))}
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Stroke')}
					{@render color(layer.stroke_color, (v) => (layer.stroke_color = v))}
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Stroke W')}
					{@render num('px', layer.stroke_width ?? 0, (n) => (layer.stroke_width = Math.max(0, n)), { min: 0 })}
				</div>
			</div>
		{:else if layer.type === 'path'}
			<!-- Point inspector: shown while a node is focused in path-edit mode. -->
			{#if editor.activePathNode}
				{@const node = editor.activePathNode}
				{@render sectionLabel('Point')}
				<div class="space-y-3 px-4 pb-3">
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
						{@render num('X', node.x, (n) => editor.setActiveNodeX(n))}
						{@render num('Y', node.y, (n) => editor.setActiveNodeY(n))}
					</div>
					<button
						type="button"
						onclick={() => editor.deleteActiveNode()}
						disabled={(layer.nodes?.length ?? 0) <= 2}
						class="flex w-full items-center justify-center gap-1.5 rounded-md border border-line-strong px-2 py-1.5 text-xs font-medium text-muted transition-colors hover:border-accent hover:text-accent-ink disabled:opacity-40"
					>
						<Trash2 size={13} /> Delete point
					</button>
				</div>
				<div class="border-t border-line"></div>
			{/if}

			{@render sectionLabel('Path')}
			<div class="space-y-3 px-4 pb-4">
				<div class="flex items-center justify-between gap-3">
					{@render row('Stroke')}
					{@render color(layer.stroke_color, (v) => (layer.stroke_color = v))}
				</div>
				<div class="flex items-center justify-between gap-3">
					{@render row('Width')}
					{@render num('px', layer.stroke_width ?? 0, (n) => (layer.stroke_width = Math.max(0, n)), { min: 0 })}
				</div>
				<div class="flex items-center justify-between gap-3">
					<span class="text-xs text-muted">Closed</span>
					<Toggle bind:checked={() => layer.closed ?? false, (v) => editor.setClosed(v)} />
				</div>
				<div class="flex items-center justify-between gap-3">
					<span class="text-xs text-muted">Fill</span>
					<Toggle bind:checked={() => editor.fillEnabled, (v) => editor.setFillEnabled(v)} />
				</div>
				{#if editor.fillEnabled}
					<div class="flex items-center justify-between gap-3">
						{@render row('Fill color')}
						{@render color(layer.fill, (v) => (layer.fill = v))}
					</div>
				{/if}
				<button
					type="button"
					onclick={() => editor.reversePath()}
					class="flex w-full items-center justify-center gap-1.5 rounded-md border border-line-strong px-2 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-ink-2"
				>
					<Repeat2 size={13} /> Reverse direction
				</button>
				<p class="text-[11px] text-faint">{layer.nodes?.length ?? 0} nodes</p>
			</div>
		{/if}

		<!-- Footer actions -->
		<footer class="mt-auto flex gap-2 border-t border-line bg-surface px-4 py-3">
			<button
				type="button"
				onclick={() => editor.duplicateLayer(layer.id)}
				class="flex flex-1 items-center justify-center gap-1.5 rounded-md border border-line-strong px-2 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-ink-2"
			>
				<Copy size={13} />
				Duplicate
			</button>
			<button
				type="button"
				onclick={() => editor.removeLayer(layer.id)}
				class="flex flex-1 items-center justify-center gap-1.5 rounded-md border border-line-strong px-2 py-1.5 text-xs font-medium text-muted transition-colors hover:border-accent hover:text-accent-ink"
			>
				<Trash2 size={13} />
				Delete
			</button>
		</footer>
	{/if}
</aside>

<style>
	/* Restrained native range track/thumb so the slider reads like an inspector
	   control, not a chunky default. */
	.range {
		-webkit-appearance: none;
		appearance: none;
		background: var(--color-line-strong);
		border-radius: 9999px;
		outline: none;
	}
	.range::-webkit-slider-thumb {
		-webkit-appearance: none;
		appearance: none;
		height: 14px;
		width: 14px;
		border-radius: 9999px;
		background: var(--color-ink);
		border: 2px solid var(--color-surface);
		cursor: pointer;
	}
	.range::-moz-range-thumb {
		height: 12px;
		width: 12px;
		border-radius: 9999px;
		background: var(--color-ink);
		border: 2px solid var(--color-surface);
		cursor: pointer;
	}
</style>
