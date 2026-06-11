<script lang="ts">
	// Figma's paint picker: clicking a fill row's swatch opens THE colour picker,
	// and the paint TYPE is chosen inside it via a row of small preview tabs —
	// Solid · Linear · Radial · Angular · Diamond · Image (Figma's "Guide to fills").
	// Solid shows the colour controls; gradients show a draggable stop strip (click
	// to add a stop, drag to move, Delete/× to remove) with the selected stop edited
	// by the same colour controls; Image shows the upload field + Fill/Fit/Tile mode.
	import { Popover } from 'bits-ui';
	import { Image as ImageIcon, Trash2 } from 'lucide-svelte';
	import { inspectorAnchor } from '$lib/layout/inspectorAnchor';
	import ColorArea from '$lib/components/ui/ColorArea.svelte';
	import ImageInput from '$lib/components/editor/ImageInput.svelte';
	import Select from '$lib/components/Select.svelte';
	import { resolveSrc, type Paint, type PaintType, type GradientStop } from '$lib/layout/schema';

	let {
		paint,
		guildId,
		patch,
		convert,
		setStops,
		types,
		anchor = true
	}: {
		paint: Paint;
		guildId: string;
		patch: (p: Partial<Paint>) => void; // unsorted write (mid-drag)
		convert: (t: PaintType) => void; // switch type, seeding defaults
		setStops: (stops: GradientStop[]) => void; // sorted commit
		// restrict the offered paint types (e.g. ['solid'] for a shadow colour —
		// the popover then hides the type tabs but keeps the identical controls)
		types?: PaintType[];
		// anchor to the inspector's left edge (Figma-style, the default); pass
		// false for a picker already nested inside another popover, which opens
		// under its own trigger instead of covering the host popover.
		anchor?: boolean;
	} = $props();

	const ALL_TYPES: { t: PaintType; label: string }[] = [
		{ t: 'solid', label: 'Solid' },
		{ t: 'linear', label: 'Linear' },
		{ t: 'radial', label: 'Radial' },
		{ t: 'angular', label: 'Angular' },
		{ t: 'diamond', label: 'Diamond' },
		{ t: 'image', label: 'Image' }
	];
	const TYPES = $derived(types ? ALL_TYPES.filter((x) => types.includes(x.t)) : ALL_TYPES);
	const isGradient = $derived(
		paint.type === 'linear' || paint.type === 'radial' || paint.type === 'angular' || paint.type === 'diamond'
	);

	// ── selected gradient stop ──
	let selStop = $state(0);
	$effect(() => {
		const n = paint.stops?.length ?? 0;
		if (selStop >= n) selStop = Math.max(0, n - 1);
	});

	function rgba(hex: string, a: number): string {
		let h = (hex ?? '#000000').replace('#', '');
		if (h.length === 3) h = h.split('').map((c) => c + c).join('');
		h = h.padEnd(6, '0').slice(0, 6);
		return `rgba(${parseInt(h.slice(0, 2), 16) || 0},${parseInt(h.slice(2, 4), 16) || 0},${parseInt(h.slice(4, 6), 16) || 0},${a})`;
	}
	const stripCss = $derived(
		`linear-gradient(90deg, ${(paint.stops ?? [])
			.map((st) => `${rgba(st.color, st.alpha ?? 1)} ${(st.pos * 100).toFixed(1)}%`)
			.join(', ')})`
	);

	// the selected stop's colour+alpha as one hex for ColorArea (#RRGGBB[AA])
	function stopHex(st: GradientStop): string {
		const al = st.alpha ?? 1;
		if (al >= 1) return st.color.toUpperCase();
		return (
			st.color.toUpperCase() +
			Math.round(al * 255)
				.toString(16)
				.padStart(2, '0')
				.toUpperCase()
		);
	}
	function setStopHex(hex: string) {
		const m = /^#([0-9a-fA-F]{6})([0-9a-fA-F]{2})?$/.exec(hex);
		if (!m) return;
		const stops = (paint.stops ?? []).map((st, i) =>
			i === selStop
				? { ...st, color: '#' + m[1].toUpperCase(), alpha: m[2] ? parseInt(m[2], 16) / 255 : 1 }
				: st
		);
		patch({ stops }); // keep order while editing; positions didn't change
	}

	// clicking the empty strip adds a stop at that position with the interpolated colour
	function addStopAt(pos: number) {
		const stops = paint.stops ?? [];
		let before: GradientStop | undefined;
		let after: GradientStop | undefined;
		for (const st of stops) {
			if (st.pos <= pos) before = st;
			else if (!after) after = st;
		}
		const f =
			before && after && after.pos > before.pos ? (pos - before.pos) / (after.pos - before.pos) : 0;
		const lerp = (x: number, y: number) => Math.round(x + (y - x) * f);
		const hex = (c: string) => {
			const h = c.replace('#', '').padEnd(6, '0');
			return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) || 0);
		};
		let color = before?.color ?? after?.color ?? '#FFFFFF';
		let alpha = before?.alpha ?? 1;
		if (before && after) {
			const [r1, g1, b1] = hex(before.color);
			const [r2, g2, b2] = hex(after.color);
			color =
				'#' +
				[lerp(r1, r2), lerp(g1, g2), lerp(b1, b2)]
					.map((n) => n.toString(16).padStart(2, '0'))
					.join('')
					.toUpperCase();
			alpha = (before.alpha ?? 1) + ((after.alpha ?? 1) - (before.alpha ?? 1)) * f;
		}
		const next = [...stops, { pos, color, alpha }].sort((a, b) => a.pos - b.pos);
		setStops(next);
		selStop = next.findIndex((st) => st.pos === pos);
	}

	function removeStop(i: number) {
		if ((paint.stops?.length ?? 0) <= 2) return;
		setStops((paint.stops ?? []).filter((_, j) => j !== i));
		selStop = Math.max(0, selStop - (i <= selStop ? 1 : 0));
	}

	// ── stop dragging (position is committed sorted on release) ──
	let stripEl = $state<HTMLDivElement>();
	let dragging = $state<number | null>(null);
	function stopDown(e: PointerEvent, i: number) {
		e.stopPropagation();
		selStop = i;
		dragging = i;
		(e.currentTarget as Element).setPointerCapture(e.pointerId);
	}
	function stopMove(e: PointerEvent) {
		if (dragging === null || !stripEl) return;
		const r = stripEl.getBoundingClientRect();
		const pos = Math.min(1, Math.max(0, (e.clientX - r.left) / r.width));
		const stops = (paint.stops ?? []).map((st, j) => (j === dragging ? { ...st, pos } : st));
		patch({ stops }); // unsorted while dragging so the handle index stays stable
	}
	function stopUp() {
		if (dragging === null) return;
		const pos = paint.stops?.[dragging]?.pos ?? 0;
		dragging = null;
		const sorted = (paint.stops ?? []).slice().sort((a, b) => a.pos - b.pos);
		setStops(sorted);
		selStop = sorted.findIndex((st) => st.pos === pos);
	}
	function stripClick(e: MouseEvent) {
		if (!stripEl) return;
		const r = stripEl.getBoundingClientRect();
		addStopAt(Math.min(1, Math.max(0, (e.clientX - r.left) / r.width)));
	}

	const checker =
		'background-image:linear-gradient(45deg,#5557 25%,transparent 25%,transparent 75%,#5557 75%),linear-gradient(45deg,#5557 25%,transparent 25%,transparent 75%,#5557 75%);background-size:8px 8px;background-position:0 0,4px 4px;';

	// ── trigger chip + label ──
	function chipCss(p: Paint): string {
		switch (p.type) {
			case 'solid':
				return `background:${p.color ?? '#000'};`;
			case 'linear':
				return `background:linear-gradient(${p.angle ?? 180}deg, ${stopsCss(p)});`;
			case 'radial':
			case 'diamond':
				return `background:radial-gradient(closest-side, ${stopsCss(p)});`;
			case 'angular':
				return `background:conic-gradient(from ${p.angle ?? 0}deg, ${stopsCss(p)});`;
			case 'image': {
				const src = resolveSrc(p.src);
				return src
					? `background-image:url("${src}"); background-size:cover; background-position:center;`
					: 'background:#444;';
			}
		}
	}
	function stopsCss(p: Paint): string {
		return (p.stops ?? []).map((st) => `${st.color} ${(st.pos * 100).toFixed(0)}%`).join(', ');
	}
	const label = $derived(
		paint.type === 'solid'
			? (paint.color ?? '').toUpperCase() || 'Solid'
			: (TYPES.find((x) => x.t === paint.type)?.label ?? paint.type)
	);

	// type-tab thumbnails: small grayscale previews like Figma's picker
	function tabCss(t: PaintType): string {
		switch (t) {
			case 'solid':
				return `background:${paint.type === 'solid' ? (paint.color ?? '#d9d9d9') : '#d9d9d9'};`;
			case 'linear':
				return 'background:linear-gradient(180deg,#fff,#666);';
			case 'radial':
				return 'background:radial-gradient(closest-side,#fff,#666);';
			case 'angular':
				return 'background:conic-gradient(#fff,#666,#fff);';
			default:
				return '';
		}
	}
</script>

<Popover.Root>
	<Popover.Trigger
		class="flex h-7 min-w-0 flex-1 items-center gap-2 rounded-md border border-line bg-ink-2 px-1.5 text-xs text-ink outline-none transition-colors hover:border-line-strong data-[state=open]:border-faint"
	>
		<span class="relative h-4 w-4 shrink-0 overflow-hidden rounded-sm border border-line-strong">
			<span class="absolute inset-0" style={checker}></span>
			<span class="absolute inset-0" style={chipCss(paint)}></span>
		</span>
		<span class="flex-1 truncate text-left">{label}</span>
	</Popover.Trigger>
	<Popover.Portal>
		<Popover.Content
			customAnchor={anchor === false ? undefined : inspectorAnchor}
			side={anchor === false ? 'bottom' : 'left'}
			align={anchor === false ? 'center' : 'start'}
			sideOffset={anchor === false ? 6 : 10}
			collisionPadding={12}
			class="menu-pop z-50 w-64 rounded-xl border border-line-strong bg-surface shadow-2xl shadow-black/60 ring-1 ring-black/40 outline-none"
		>
			<!-- header: Custom / Libraries tabs + close (Figma's picker header) -->
			<div class="flex items-center justify-between border-b border-line px-2.5 py-1.5">
				<div class="flex items-center gap-1 text-xs font-medium">
					<span class="rounded px-1.5 py-0.5 text-ink">Custom</span>
					<span class="cursor-default px-1.5 py-0.5 text-faint" title="Shared libraries — coming soon">Libraries</span>
				</div>
				<Popover.Close class="grid h-5 w-5 place-items-center rounded text-faint transition-colors hover:bg-ink-2 hover:text-ink" aria-label="Close">
					<svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M3 3l6 6M9 3l-6 6" /></svg>
				</Popover.Close>
			</div>

			<!-- paint-type tabs: Solid · Linear · Radial · Angular · Diamond · Image -->
			{#if TYPES.length > 1}
			<div class="flex items-center gap-1 px-2.5 pb-1 pt-2">
				{#each TYPES as ty (ty.t)}
					<button
						type="button"
						title={ty.label}
						aria-label={ty.label}
						aria-pressed={paint.type === ty.t}
						onclick={() => convert(ty.t)}
						class="relative grid h-6 w-6 place-items-center overflow-hidden rounded-md border transition-all {paint.type === ty.t
							? 'border-faint ring-2 ring-line-strong'
							: 'border-line-strong opacity-70 hover:opacity-100'}"
					>
						{#if ty.t === 'image'}
							<span class="grid h-full w-full place-items-center bg-ink-2 text-muted"><ImageIcon size={13} /></span>
						{:else if ty.t === 'diamond'}
							<!-- stepped concentric diamonds — CSS has no diamond gradient -->
							<svg viewBox="0 0 16 16" class="h-full w-full" aria-hidden="true">
								<rect width="16" height="16" fill="#666" />
								<polygon points="8,1 15,8 8,15 1,8" fill="#999" />
								<polygon points="8,4 12,8 8,12 4,8" fill="#ccc" />
								<polygon points="8,6.5 9.5,8 8,9.5 6.5,8" fill="#fff" />
							</svg>
						{:else}
							<span class="absolute inset-0" style={tabCss(ty.t)}></span>
						{/if}
					</button>
				{/each}
				<span class="ml-auto text-[11px] text-faint">{TYPES.find((x) => x.t === paint.type)?.label}</span>
			</div>
			{/if}

			<div class="space-y-2.5 px-2.5 pb-2.5 pt-1.5">
				{#if paint.type === 'solid'}
					<ColorArea
						bind:value={() => (paint.color ?? '#D9D9D9').toUpperCase(), (v) => patch({ color: v })}
					/>
				{:else if isGradient}
					<!-- gradient strip: click to add a stop, drag to move, × to remove -->
					<div class="space-y-1.5">
						<div
							bind:this={stripEl}
							role="presentation"
							onclick={stripClick}
							class="relative h-6 w-full cursor-copy touch-none overflow-visible rounded-md border border-line-strong"
						>
							<div class="absolute inset-0 overflow-hidden rounded-[5px]">
								<div class="absolute inset-0" style={checker}></div>
								<div class="absolute inset-0" style="background:{stripCss}"></div>
							</div>
							{#each paint.stops ?? [] as st, si (si)}
								<button
									type="button"
									aria-label="Stop at {(st.pos * 100).toFixed(0)}%"
									onpointerdown={(e) => stopDown(e, si)}
									onpointermove={stopMove}
									onpointerup={stopUp}
									onpointercancel={stopUp}
									onclick={(e) => e.stopPropagation()}
									class="absolute top-1/2 h-4 w-4 -translate-x-1/2 -translate-y-1/2 cursor-ew-resize rounded-full border-2 shadow {selStop === si
										? 'z-10 scale-125 border-white ring-1 ring-black/50'
										: 'border-white/80'}"
									style="left:{st.pos * 100}%; background:{st.color};"
								></button>
							{/each}
						</div>
						<div class="flex items-center justify-between gap-2">
							{#if paint.type === 'linear' || paint.type === 'angular'}
								<label class="flex h-7 w-24 items-center rounded-md border border-line bg-ink-2 pl-2 pr-1 text-xs focus-within:border-faint">
									<input
										type="number"
										value={paint.angle ?? (paint.type === 'linear' ? 180 : 0)}
										oninput={(e) => {
											const n = e.currentTarget.valueAsNumber;
											if (!Number.isNaN(n)) patch({ angle: n });
										}}
										class="w-full min-w-0 bg-transparent tabular-nums text-ink outline-none"
									/>
									<span class="select-none pr-1 text-[10px] text-faint">°</span>
								</label>
							{:else}
								<span></span>
							{/if}
							<button
								type="button"
								title="Remove the selected stop"
								aria-label="Remove the selected stop"
								disabled={(paint.stops?.length ?? 0) <= 2}
								onclick={() => removeStop(selStop)}
								class="grid h-7 w-7 place-items-center rounded-md text-faint transition-colors hover:bg-ink-2 hover:text-danger disabled:pointer-events-none disabled:opacity-40"
							>
								<Trash2 size={13} />
							</button>
						</div>
					</div>
					{#if paint.stops?.[selStop]}
						<ColorArea
							bind:value={() => stopHex(paint.stops![selStop]), (v) => setStopHex(v)}
						/>
					{/if}
				{:else}
					<!-- image fill: preview + source + Figma's Fill / Fit / Tile modes -->
					{#if resolveSrc(paint.src)}
						<div
							class="h-24 w-full rounded-lg border border-line-strong"
							style="background-image:url('{resolveSrc(paint.src)}'); background-size:{paint.fit === 'contain'
								? 'contain'
								: paint.fit === 'tile'
									? '64px auto'
									: 'cover'}; background-position:center; background-repeat:{paint.fit === 'tile'
								? 'repeat'
								: 'no-repeat'};"
						></div>
					{/if}
					<ImageInput
						value={paint.src ?? ''}
						onChange={(v) => patch({ src: v })}
						{guildId}
						placeholder="https://… or upload"
					/>
					<Select
						dense
						bind:value={() => paint.fit ?? 'cover', (v) => patch({ fit: v as Paint['fit'] })}
						options={[
							{ value: 'cover', label: 'Fill' },
							{ value: 'contain', label: 'Fit' },
							{ value: 'tile', label: 'Tile' }
						]}
					/>
				{/if}
			</div>
		</Popover.Content>
	</Popover.Portal>
</Popover.Root>
