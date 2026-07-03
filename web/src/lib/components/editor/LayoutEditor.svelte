<script lang="ts">
	// The reusable Card Studio editor chrome: toolbar + three panes + the
	// server-render compare panel. It publishes the given EditorStore on
	// EDITOR_CTX so the panels bind to it, and fills its parent (h-full). The
	// host (the standalone page or the in-welcome modal) supplies primary actions
	// (Save / Apply) via the `actions` snippet and sizes the container.
	import { getContext, onDestroy, type Snippet } from 'svelte';
	import { DropdownMenu, Popover } from 'bits-ui';
	import { EditorStore, EDITOR_CTX, type Tool } from '$lib/layout/editor.svelte';
	import type { ShapeKind, Layout } from '$lib/layout/schema';
	import { defaultLayout } from '$lib/layout/schema';
	import { cardTemplates, rankStarterLayout, cloneLayout, templateLayout } from '$lib/layout/templates';
	import { layoutPreview, resolveCard } from '$lib/api';
	import { googleFontsHref } from '$lib/layout/fonts';
	import Canvas from '$lib/components/editor/Canvas.svelte';
	import LayersPanel from '$lib/components/editor/LayersPanel.svelte';
	import PropertiesPanel from '$lib/components/editor/PropertiesPanel.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import { Image, X, Loader2, AlertTriangle, MousePointer2, Scaling, Square, Circle, Type, PenTool, Pencil, Spline, Undo2, Redo2, Frame, Shapes, Triangle, Diamond, Pentagon, Hexagon, Star, Minus, Layers, SlidersHorizontal, LayoutTemplate, Keyboard } from 'lucide-svelte';

	const shapeItems: { kind: ShapeKind; label: string; icon: typeof Image }[] = [
		{ kind: 'triangle', label: 'Triangle', icon: Triangle },
		{ kind: 'diamond', label: 'Diamond', icon: Diamond },
		{ kind: 'pentagon', label: 'Pentagon', icon: Pentagon },
		{ kind: 'hexagon', label: 'Hexagon', icon: Hexagon },
		{ kind: 'star', label: 'Star', icon: Star },
		{ kind: 'line', label: 'Line', icon: Minus }
	];

	// Tools grouped (select · vector · shapes · media); a divider is drawn whenever
	// the group changes, for a calmer, more pro tool palette.
	const tools: { id: Tool; label: string; key: string; icon: typeof Image; group: number }[] = [
		{ id: 'select', label: 'Select', key: 'V', icon: MousePointer2, group: 0 },
		{ id: 'scale', label: 'Scale — scales size, text & strokes (K)', key: 'K', icon: Scaling, group: 0 },
		{ id: 'pen', label: 'Pen', key: 'P', icon: PenTool, group: 1 },
		{ id: 'pencil', label: 'Pencil', key: '', icon: Pencil, group: 1 },
		{ id: 'bend', label: 'Bend — click a path/point and drag', key: 'B', icon: Spline, group: 1 },
		{ id: 'rect', label: 'Rectangle', key: 'R', icon: Square, group: 2 },
		{ id: 'ellipse', label: 'Ellipse', key: 'O', icon: Circle, group: 2 },
		{ id: 'text', label: 'Text', key: 'T', icon: Type, group: 2 },
		{ id: 'image', label: 'Image', key: '', icon: Image, group: 3 }
	];

	// The store owner (the page or the modal) publishes the EditorStore on
	// EDITOR_CTX; this chrome and the three panels all read it from there.
	let {
		guildId,
		title = 'Card Studio',
		extraVars,
		context = 'rank',
		actions
	}: {
		guildId: string;
		title?: string;
		extraVars?: Record<string, string>;
		// Which card this studio is designing, gates the rank-only variable chips
		// and the rank starter in the template gallery. Defaults to 'rank' (the most
		// permissive) for the standalone editor.
		context?: 'welcome' | 'rank';
		actions?: Snippet;
	} = $props();

	const store = getContext<EditorStore>(EDITOR_CTX);

	// Publish the host's sample vars + the server-render overlay flag onto the store
	// so the canvas can preview a progress bar and the Esc cascade knows the overlay
	// is up (see the modal's Esc handler).
	$effect(() => {
		store.extraVars = extraVars ?? {};
	});

	// ── template gallery ────────────────────────────────────────────────────────
	// The ready-made starters, plus the flat rank starter when designing a rank card.
	const gallery = $derived<{ id: string; name: string; layout: Layout }[]>([
		...cardTemplates.map((t) => ({ id: t.id, name: t.name, layout: t.layout })),
		...(context === 'rank' ? [{ id: 'rank-starter', name: 'Rank starter', layout: rankStarterLayout() }] : [])
	]);
	let templatesOpen = $state(false);
	let confirmReplace = $state(false);
	let pendingLayout: Layout | null = null;

	// A CSS swatch of a template's background, for the gallery preview.
	function bgSwatch(l: Layout): string {
		const b = l.background;
		if (b.type === 'gradient' && (b.from || b.to))
			return `background:linear-gradient(${(b.angle ?? 0) + 90}deg, ${b.from || '#000'}, ${b.to || '#000'});`;
		if (b.type === 'image' && b.image_url)
			return `background-image:url(${JSON.stringify(b.image_url)});background-size:cover;background-position:center;`;
		return `background:${b.color || '#141417'};`;
	}
	// A structural fingerprint (ignoring non-deterministic ids) so we only warn about
	// replacing a design the user has actually customised, not the fresh starter.
	function seedKey(l: Layout): string {
		return JSON.stringify(l, (k, v) => (k === 'id' || k === 'group' || k === 'groups' ? undefined : v));
	}
	function isPristine(): boolean {
		const seed =
			context === 'rank' ? rankStarterLayout() : context === 'welcome' ? templateLayout('aurora') : defaultLayout();
		return seedKey(store.toJSON()) === seedKey(seed);
	}
	function chooseTemplate(l: Layout) {
		templatesOpen = false;
		if (isPristine()) {
			loadTemplate(l);
			return;
		}
		pendingLayout = cloneLayout(l);
		confirmReplace = true;
	}
	// Load AFTER record() so the whole swap collapses into a single undo step.
	function loadTemplate(l: Layout) {
		store.record();
		store.exitEdit();
		store.select(null);
		store.layout = cloneLayout(l);
	}
	function confirmReplaceNow() {
		if (pendingLayout) loadTemplate(pendingLayout);
		pendingLayout = null;
	}
	function cancelReplace() {
		pendingLayout = null;
	}

	// ── keyboard-shortcuts sheet ────────────────────────────────────────────────
	// Sourced from the tools array (so tool keys never drift) plus the canvas /
	// selection / edit chords handled in Canvas.svelte.
	const shortcutGroups: { group: string; items: [string, string][] }[] = [
		{
			group: 'Tools',
			items: tools
				.filter((t) => t.key)
				.map((t) => [t.key, t.id[0].toUpperCase() + t.id.slice(1)] as [string, string])
		},
		{
			group: 'Canvas',
			items: [
				['Space-drag', 'Pan'],
				['⌘/Ctrl-scroll', 'Zoom'],
				['⌘/Ctrl 0', 'Reset view'],
				['⌘/Ctrl +', 'Zoom in'],
				['⌘/Ctrl -', 'Zoom out']
			]
		},
		{
			group: 'Selection',
			items: [
				['⌘/Ctrl A', 'Select all'],
				['⌘/Ctrl G', 'Group'],
				['⇧⌘/Ctrl G', 'Ungroup'],
				['Arrows', 'Nudge'],
				['⇧ Arrows', 'Nudge ×10'],
				['⌫', 'Delete']
			]
		},
		{
			group: 'Edit',
			items: [
				['⌘/Ctrl Z', 'Undo'],
				['⇧⌘/Ctrl Z', 'Redo'],
				['⌘/Ctrl C', 'Copy'],
				['⌘/Ctrl X', 'Cut'],
				['⌘/Ctrl V', 'Paste'],
				['⌘/Ctrl D', 'Duplicate'],
				['Enter / Esc', 'Finish path'],
				['Double-click', 'Edit object']
			]
		}
	];

	// Unified control variants (shadcn-on-Dia) — identical strings per variant so
	// every editor control reads the same. See PropertiesPanel for the full set.
	const btnBase =
		'inline-flex items-center justify-center gap-1.5 rounded-md text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-line-strong disabled:pointer-events-none disabled:opacity-40';
	const btnSecondary = `${btnBase} border border-line-strong text-ink hover:bg-ink-2`;
	// Ghost icon-square toolbar button (undo/redo).
	const iconGhost = `${btnBase} text-muted hover:bg-surface hover:text-ink`;

	// The Bend tool only appears when there's a path to bend (a path is selected or
	// being edited) — like Figma, where bend belongs to vector-edit mode.
	const canBend = $derived(store.selected?.type === 'path' || !!store.editId);
	const visibleTools = $derived(tools.filter((t) => t.id !== 'bend' || canBend));
	// If the bend target goes away, fall back to Select (don't strand a hidden tool).
	$effect(() => {
		if (!canBend && store.tool === 'bend') store.setTool('select');
	});

	let previewUrl = $state('');
	let previewing = $state(false); // a render is in flight (header spinner)
	let previewError = $state('');
	let renderOpen = $state(false); // the docked, live server-render panel is up

	// On mobile the two side rails become off-canvas drawers (only one open at a
	// time), toggled by floating buttons on the canvas. On md+ they're static panes.
	let layersOpen = $state(false);
	let propsOpen = $state(false);
	function toggleLayers() {
		layersOpen = !layersOpen;
		propsOpen = false;
	}
	function toggleProps() {
		propsOpen = !propsOpen;
		layersOpen = false;
	}
	function closeDrawers() {
		layersOpen = false;
		propsOpen = false;
	}

	// The docked server-render panel is LIVE: once open it re-renders as you edit,
	// swapping the blob URL under a header spinner. A generation counter discards
	// out-of-order responses (mirrors the resolve effect below).
	let renderGen = 0;
	async function doRender() {
		if (!renderOpen) return;
		const my = ++renderGen;
		previewing = true;
		try {
			const url = await layoutPreview(guildId, store.toJSON(), extraVars);
			if (my !== renderGen) {
				URL.revokeObjectURL(url); // superseded by a newer edit / close
				return;
			}
			if (previewUrl) URL.revokeObjectURL(previewUrl);
			previewUrl = url;
			previewError = '';
		} catch (e) {
			if (my !== renderGen) return;
			previewError = e instanceof Error ? e.message : 'Render failed';
		} finally {
			if (my === renderGen) previewing = false;
		}
	}
	function renderServer() {
		if (renderOpen) return; // already open and live-updating
		renderOpen = true; // the live effect fires the first render
	}
	function closePreview() {
		renderOpen = false;
		renderGen++; // discard any in-flight render
		previewing = false;
		clearTimeout(liveTimer);
		if (previewUrl) URL.revokeObjectURL(previewUrl);
		previewUrl = '';
		previewError = '';
	}
	// Live re-render: while the panel is open, deep-touch the layout (register deps
	// WITHOUT stringifying every frame) and debounce a re-render ~700ms after edits
	// settle. The first open renders immediately; edits then coalesce. Reads no state
	// doRender writes, so it never self-triggers. `didFirstRender` is deliberately
	// non-reactive (a plain flag, not a dep).
	let liveTimer: ReturnType<typeof setTimeout>;
	let didFirstRender = false;
	$effect(() => {
		if (!renderOpen) {
			didFirstRender = false;
			clearTimeout(liveTimer);
			return;
		}
		touch(store.layout);
		void extraVars; // re-render when the sample vars change too
		clearTimeout(liveTimer);
		const delay = didFirstRender ? 700 : 0;
		didFirstRender = true;
		liveTimer = setTimeout(() => doRender(), delay);
	});
	// Mirror the panel's open state onto the store so the hosting modal's Esc handler
	// treats an open render panel as the last thing to dismiss before it closes.
	$effect(() => {
		store.overlayOpen = renderOpen;
	});
	// Load the guild's custom (premium) fonts into the document via the FontFace
	// API so the live preview renders them (the static roster comes from Google
	// Fonts above). Tracked by URL so each loads once.
	const loadedFonts = new Set<string>();
	$effect(() => {
		if (typeof document === 'undefined') return;
		for (const f of store.customFonts) {
			if (!f.url || loadedFonts.has(f.url)) continue;
			loadedFonts.add(f.url);
			try {
				const face = new FontFace(f.family, `url(${JSON.stringify(f.url)})`);
				face
					.load()
					.then((loaded) => document.fonts.add(loaded))
					.catch(() => {});
			} catch {
				/* FontFace unsupported — preview falls back to a system font */
			}
		}
	});

	// Resolve card templates ({{.User…}}) on the server so the live canvas matches
	// the bot. Only re-fetches when the set of template strings changes (reading
	// text/src here means geometry drags don't trigger it), debounced.
	let resolveTimer: ReturnType<typeof setTimeout>;
	let resolveGen = 0;
	$effect(() => {
		const ev = extraVars; // reference so the effect re-runs when sample vars change
		const set = new Set<string>();
		for (const l of store.layout.layers) {
			if (l.text && l.text.includes('{{')) set.add(l.text);
			if (l.src && l.src.includes('{{')) set.add(l.src);
		}
		const list = [...set];
		clearTimeout(resolveTimer);
		if (list.length === 0) {
			store.setResolved({});
			return;
		}
		resolveTimer = setTimeout(async () => {
			const my = ++resolveGen;
			try {
				const out = await resolveCard(guildId, list, ev);
				if (my !== resolveGen) return; // a newer resolve superseded this one
				const map: Record<string, string> = {};
				list.forEach((s, i) => (map[s] = out[i] ?? s));
				store.setResolved(map);
			} catch {
				/* keep the last resolved values */
			}
		}, 250);
	});

	// Undo history recorder: coalesce a burst of edits (a drag, a run of
	// keystrokes) into one checkpoint by committing 350ms after activity settles.
	// `touch` deep-reads the layout to register reactive deps WITHOUT allocating a
	// big JSON string every frame (the stringify was the drag-lag culprit on cards
	// with images / long template strings).
	function touch(v: unknown) {
		if (v && typeof v === 'object') {
			for (const k in v as Record<string, unknown>) touch((v as Record<string, unknown>)[k]);
		}
	}
	let histTimer: ReturnType<typeof setTimeout>;
	$effect(() => {
		touch(store.layout);
		clearTimeout(histTimer);
		histTimer = setTimeout(() => store.record(), 350);
	});

	onDestroy(() => {
		clearTimeout(histTimer);
		clearTimeout(resolveTimer);
		clearTimeout(liveTimer);
		if (previewUrl) URL.revokeObjectURL(previewUrl);
	});

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && renderOpen) {
			// Consume Esc: dismiss the overlay, not the whole studio.
			e.preventDefault();
			e.stopPropagation();
			closePreview();
		}
	}
</script>

<!-- Load the card fonts so the preview shows the same faces the renderer uses. -->
<svelte:head>
	<link rel="stylesheet" href={googleFontsHref()} />
</svelte:head>

<svelte:window onkeydown={onKeydown} />

<div class="studio-theme flex h-full flex-col bg-ink-2 text-ink">
	<!-- Toolbar -->
	<div class="studio-bar relative z-20 flex h-12 shrink-0 items-center gap-2 border-b border-line bg-surface px-2 md:gap-3 md:px-3">
		<div class="flex items-center gap-2">
			<span class="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-ink-2 text-ink ring-1 ring-line">
				<Frame size={14} />
			</span>
			<span class="hidden text-[13px] font-semibold tracking-tight text-ink sm:inline">{title}</span>
		</div>
		<span class="hidden rounded border border-line-strong bg-ink-2 px-1.5 py-0.5 font-mono text-[10px] tabular-nums text-faint sm:inline-block">
			{store.layout.width}×{store.layout.height}
		</span>
		<div class="ml-1 flex items-center gap-0.5">
			<button
				type="button"
				onclick={() => store.undo()}
				disabled={!store.canUndo}
				title="Undo (⌘Z)"
				aria-label="Undo"
				class="{iconGhost} h-7 w-7"
			>
				<Undo2 size={14} />
			</button>
			<button
				type="button"
				onclick={() => store.redo()}
				disabled={!store.canRedo}
				title="Redo (⇧⌘Z)"
				aria-label="Redo"
				class="{iconGhost} h-7 w-7"
			>
				<Redo2 size={14} />
			</button>
		</div>
		<!-- Template gallery: pick a ready-made starter (portalled so it never clips). -->
		<Popover.Root bind:open={templatesOpen}>
			<Popover.Trigger title="Start from a template" class="{btnSecondary} ml-1 h-7 gap-1.5 px-2">
				<LayoutTemplate size={13} />
				<span class="hidden sm:inline">Templates</span>
			</Popover.Trigger>
			<Popover.Portal>
				<Popover.Content
					side="bottom"
					align="start"
					sideOffset={8}
					class="menu-pop z-50 w-72 rounded-xl border border-line-strong bg-surface p-2 shadow-2xl outline-none"
				>
					<div class="px-1 pb-1.5 pt-0.5 text-[11px] font-semibold text-ink">Templates</div>
					<div class="grid grid-cols-2 gap-1.5">
						{#each gallery as t (t.id)}
							<button
								type="button"
								onclick={() => chooseTemplate(t.layout)}
								class="group flex flex-col gap-1.5 rounded-lg border border-line p-1.5 text-left transition-colors hover:border-line-strong hover:bg-ink-2"
							>
								<span class="block h-12 w-full rounded-md border border-line-strong" style={bgSwatch(t.layout)}></span>
								<span class="flex items-center justify-between gap-1">
									<span class="truncate text-[11px] font-medium text-ink">{t.name}</span>
									<span class="shrink-0 font-mono text-[10px] tabular-nums text-faint">{t.layout.layers.length}</span>
								</span>
							</button>
						{/each}
					</div>
				</Popover.Content>
			</Popover.Portal>
		</Popover.Root>

		<div class="flex-1"></div>
		<button
			type="button"
			onclick={() => (renderOpen ? closePreview() : renderServer())}
			title="Live server render"
			aria-pressed={renderOpen}
			class="{btnSecondary} h-8 px-2.5 {renderOpen ? 'border-faint bg-ink-2 text-ink' : ''}"
		>
			{#if previewing}<Loader2 size={13} class="animate-spin" />{:else}<Image size={13} />{/if}
			<span class="hidden sm:inline">Server render</span>
		</button>
		<!-- Keyboard-shortcuts sheet (also opened by Shift+/ on the canvas). -->
		<Popover.Root bind:open={() => store.shortcutsOpen, (v) => (store.shortcutsOpen = v)}>
			<Popover.Trigger title="Keyboard shortcuts (?)" aria-label="Keyboard shortcuts" class="{iconGhost} h-8 w-8">
				<Keyboard size={15} />
			</Popover.Trigger>
			<Popover.Portal>
				<Popover.Content
					side="bottom"
					align="end"
					sideOffset={8}
					class="menu-pop z-50 w-80 rounded-xl border border-line-strong bg-surface p-3 shadow-2xl outline-none"
				>
					<div class="pb-2 text-[11px] font-semibold text-ink">Keyboard shortcuts</div>
					<div class="grid grid-cols-2 gap-x-4 gap-y-3">
						{#each shortcutGroups as grp (grp.group)}
							<div>
								<div class="mb-1 font-mono text-[9.5px] uppercase tracking-[0.14em] text-faint">{grp.group}</div>
								<dl class="space-y-1">
									{#each grp.items as [key, label] (label)}
										<div class="flex items-center justify-between gap-2">
											<dt class="truncate text-[11px] text-muted">{label}</dt>
											<dd class="shrink-0 rounded border border-line-strong bg-ink-2 px-1.5 py-0.5 font-mono text-[10px] text-faint">{key}</dd>
										</div>
									{/each}
								</dl>
							</div>
						{/each}
					</div>
				</Popover.Content>
			</Popover.Portal>
		</Popover.Root>

		{@render actions?.()}
	</div>

	<!-- Three-pane body. On mobile the two rails become off-canvas drawers (one at a
	     time) toggled by the floating buttons on the canvas; on md+ they're static. -->
	<div class="relative flex min-h-0 flex-1">
		<aside
			class="studio-rail absolute inset-y-0 left-0 z-40 w-64 max-w-[78%] -translate-x-full overflow-y-auto border-r border-line bg-surface shadow-2xl transition-transform duration-200 md:static md:z-auto md:w-60 md:max-w-none md:translate-x-0 md:shadow-none md:transition-none {layersOpen
				? 'translate-x-0'
				: ''}"
		>
			<LayersPanel />
		</aside>

		<!-- Mobile drawer backdrop. -->
		{#if layersOpen || propsOpen}
			<button
				type="button"
				aria-label="Close panels"
				onclick={closeDrawers}
				class="absolute inset-0 z-30 bg-black/40 md:hidden"
			></button>
		{/if}

		<div class="canvas-pit relative min-w-0 flex-1">
			<Canvas />

			<!-- Mobile-only floating toggles for the Layers / Properties drawers. -->
			<button
				type="button"
				onclick={toggleLayers}
				aria-label="Toggle layers panel"
				aria-pressed={layersOpen}
				class="absolute left-3 top-3 z-10 grid h-9 w-9 place-items-center rounded-lg border border-line-strong bg-surface/80 text-muted shadow-lg backdrop-blur-md transition-colors hover:text-ink md:hidden"
			>
				<Layers size={16} />
			</button>
			<button
				type="button"
				onclick={toggleProps}
				aria-label="Toggle properties panel"
				aria-pressed={propsOpen}
				class="absolute right-3 top-3 z-10 grid h-9 w-9 place-items-center rounded-lg border border-line-strong bg-surface/80 text-muted shadow-lg backdrop-blur-md transition-colors hover:text-ink md:hidden"
			>
				<SlidersHorizontal size={16} />
			</button>

			<!-- Tool palette — pick a tool, then drag on the canvas to draw it. -->
			<div class="pointer-events-none absolute bottom-4 left-1/2 z-10 -translate-x-1/2">
				<div
					class="pointer-events-auto flex items-center gap-0.5 rounded-2xl border border-line-strong bg-surface/70 p-1 shadow-2xl backdrop-blur-xl"
				>
					{#each visibleTools as t, i (t.id)}
						{@const Icon = t.icon}
						{#if i > 0 && t.group !== visibleTools[i - 1].group}
							<span class="mx-0.5 h-5 w-px shrink-0 bg-line-strong"></span>
						{/if}
						<button
							type="button"
							onclick={() => store.setTool(t.id)}
							title={t.key ? `${t.label} (${t.key})` : t.label}
							aria-label={t.label}
							aria-pressed={store.tool === t.id}
							class="grid h-8 w-8 place-items-center rounded-md outline-none transition-colors focus-visible:ring-2 focus-visible:ring-line-strong {store.tool ===
							t.id
								? 'bg-ink-2 text-ink ring-1 ring-line-strong'
								: 'text-muted hover:bg-ink-2 hover:text-ink'}"
						>
							<Icon size={16} />
						</button>
					{/each}

					<span class="mx-0.5 h-5 w-px shrink-0 bg-line-strong"></span>
					<DropdownMenu.Root>
						<DropdownMenu.Trigger
							title="Shapes — pick one, then drag on the canvas to draw it"
							aria-label="Shapes"
							class="grid h-8 w-8 place-items-center rounded-md outline-none transition-colors focus-visible:ring-2 focus-visible:ring-line-strong {store.tool ===
							'shape'
								? 'bg-ink-2 text-ink ring-1 ring-line-strong'
								: 'text-muted hover:bg-ink-2 hover:text-ink data-[state=open]:bg-ink-2 data-[state=open]:text-ink'}"
						>
							<Shapes size={16} />
						</DropdownMenu.Trigger>
						<DropdownMenu.Portal>
							<DropdownMenu.Content
								side="top"
								sideOffset={10}
								class="menu-pop z-50 min-w-[150px] rounded-xl border border-line-strong bg-surface p-1.5 shadow-2xl outline-none"
							>
								{#each shapeItems as sh (sh.kind)}
									{@const Icon = sh.icon}
									<DropdownMenu.Item
										onSelect={() => store.setShape(sh.kind)}
										class="flex cursor-pointer items-center gap-2.5 rounded-lg px-2 py-1.5 text-[13px] text-muted outline-none transition-colors data-[highlighted]:bg-ink-2 data-[highlighted]:text-ink"
									>
										<Icon size={14} class="text-faint" /> {sh.label}
									</DropdownMenu.Item>
								{/each}
							</DropdownMenu.Content>
						</DropdownMenu.Portal>
					</DropdownMenu.Root>
				</div>
			</div>

			<!-- Live server-render panel: docked bottom-right (non-blocking) so the canvas
			     stays editable; it re-renders as you edit, with a header spinner. -->
			{#if renderOpen}
				<div class="absolute bottom-4 right-4 z-20 w-[min(26rem,calc(100%-2rem))] overflow-hidden rounded-xl border border-line-strong bg-surface shadow-2xl">
					<div class="flex items-center justify-between gap-3 border-b border-line px-3 py-2">
						<div class="flex min-w-0 items-center gap-2">
							<span class="truncate text-[12px] font-semibold text-ink">Server render</span>
							{#if previewing}<Loader2 size={12} class="shrink-0 animate-spin text-faint" />{/if}
						</div>
						<button
							type="button"
							onclick={closePreview}
							class="grid h-6 w-6 shrink-0 place-items-center rounded-md text-muted transition-colors hover:bg-ink-2 hover:text-ink"
							aria-label="Close render"
						>
							<X size={14} />
						</button>
					</div>
					<div class="grid place-items-center bg-ink-2 p-3">
						{#if previewError}
							<div class="flex items-center gap-2 py-6 text-[12px] text-danger">
								<AlertTriangle size={14} /> {previewError}
							</div>
						{:else if previewUrl}
							<img src={previewUrl} alt="Server-rendered card" class="max-h-[42vh] w-auto max-w-full rounded-lg border border-line" />
						{:else}
							<div class="flex items-center gap-2 py-6 text-[12px] text-muted">
								<Loader2 size={14} class="animate-spin" /> Rendering…
							</div>
						{/if}
					</div>
					<div class="border-t border-line px-3 py-1.5 text-[10.5px] text-faint">The exact PNG the bot would post, updated as you edit.</div>
				</div>
			{/if}
		</div>

		<aside
			class="studio-rail absolute inset-y-0 right-0 z-40 w-80 max-w-[85%] translate-x-full overflow-y-auto border-l border-line bg-surface shadow-2xl transition-transform duration-200 md:static md:z-auto md:w-72 md:max-w-none md:translate-x-0 md:shadow-none md:transition-none {propsOpen
				? 'translate-x-0'
				: ''}"
		>
			<PropertiesPanel {context} />
		</aside>
	</div>
</div>

<!-- Replacing a customised design with a template is confirmed here. -->
<ConfirmDialog
	bind:open={confirmReplace}
	title="Replace current design?"
	description="Loading this template replaces the card you've designed. You can undo it afterwards."
	cancelLabel="Keep editing"
	confirmLabel="Replace"
	oncancel={cancelReplace}
	onconfirm={confirmReplaceNow}
/>

<style>
	/* The studio uses the dashboard's own clean palette (white text on neutral
	   charcoal surfaces, like the sidebar) — no custom colour overrides. The modal
	   container pops the whole studio in; the chrome below adds a short staggered
	   reveal so the toolbar, canvas and rails settle in sequence (this also gives the
	   standalone editor page an entrance). Disabled under reduce-motion. */
	@media (prefers-reduced-motion: no-preference) {
		.studio-bar {
			animation: studio-bar-in 320ms cubic-bezier(0.16, 1, 0.3, 1) both;
		}
		.canvas-pit {
			animation: studio-canvas-in 460ms cubic-bezier(0.16, 1, 0.3, 1) 70ms both;
		}
		.studio-rail {
			/* opacity only — the rails already own a translate for the mobile drawer. */
			animation: studio-fade-in 380ms ease 150ms both;
		}
	}
	@keyframes studio-bar-in {
		from {
			opacity: 0;
			transform: translateY(-8px);
		}
		to {
			opacity: 1;
			transform: none;
		}
	}
	@keyframes studio-canvas-in {
		from {
			opacity: 0;
			transform: translateY(10px) scale(0.995);
		}
		to {
			opacity: 1;
			transform: none;
		}
	}
	@keyframes studio-fade-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}

	/* Toolbar: a faint top sheen for depth (over bg-surface). */
	.studio-bar {
		background-image: linear-gradient(to bottom, rgba(255, 255, 255, 0.03), rgba(255, 255, 255, 0));
	}

	/* Side rails: a faint lit top edge so the panels read as crafted, elevated
	   surfaces (matches the toolbar sheen + the .card depth motif). */
	.studio-rail {
		box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.03);
	}

	/* The canvas "pit": the dark recessed backdrop with a faint dot grid, so the
	   floating card stage reads as elevated. */
	.canvas-pit {
		background-color: var(--color-ink-2);
		background-image: radial-gradient(circle, rgba(255, 255, 255, 0.045) 1px, transparent 1px);
		background-size: 22px 22px;
		background-position: center;
	}
</style>
