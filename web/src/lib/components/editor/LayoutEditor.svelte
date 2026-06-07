<script lang="ts">
	// The reusable Card Studio editor chrome: toolbar + three panes + the
	// server-render compare panel. It publishes the given EditorStore on
	// EDITOR_CTX so the panels bind to it, and fills its parent (h-full). The
	// host (the standalone page or the in-welcome modal) supplies primary actions
	// (Save / Apply) via the `actions` snippet and sizes the container.
	import { getContext, onDestroy, type Snippet } from 'svelte';
	import { DropdownMenu } from 'bits-ui';
	import { EditorStore, EDITOR_CTX, type Tool } from '$lib/layout/editor.svelte';
	import type { ShapeKind } from '$lib/layout/schema';
	import { layoutPreview, resolveCard } from '$lib/api';
	import { googleFontsHref } from '$lib/layout/fonts';
	import Canvas from '$lib/components/editor/Canvas.svelte';
	import LayersPanel from '$lib/components/editor/LayersPanel.svelte';
	import PropertiesPanel from '$lib/components/editor/PropertiesPanel.svelte';
	import { Image, X, Loader2, AlertTriangle, MousePointer2, Square, Circle, Type, UserCircle, PenTool, Pencil, Undo2, Redo2, Frame, Shapes, Triangle, Diamond, Pentagon, Hexagon, Star, Minus } from 'lucide-svelte';

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
		{ id: 'pen', label: 'Pen', key: 'P', icon: PenTool, group: 1 },
		{ id: 'pencil', label: 'Pencil', key: '', icon: Pencil, group: 1 },
		{ id: 'rect', label: 'Rectangle', key: 'R', icon: Square, group: 2 },
		{ id: 'ellipse', label: 'Ellipse', key: 'O', icon: Circle, group: 2 },
		{ id: 'text', label: 'Text', key: 'T', icon: Type, group: 2 },
		{ id: 'image', label: 'Image', key: '', icon: Image, group: 3 },
		{ id: 'avatar', label: 'Avatar', key: '', icon: UserCircle, group: 3 }
	];

	// The store owner (the page or the modal) publishes the EditorStore on
	// EDITOR_CTX; this chrome and the three panels all read it from there.
	let {
		guildId,
		title = 'Card Studio',
		extraVars,
		actions
	}: { guildId: string; title?: string; extraVars?: Record<string, string>; actions?: Snippet } =
		$props();

	const store = getContext<EditorStore>(EDITOR_CTX);

	let previewUrl = $state('');
	let previewing = $state(false);
	let previewError = $state('');

	async function renderServer() {
		if (previewing) return;
		previewing = true;
		previewError = '';
		try {
			const url = await layoutPreview(guildId, store.toJSON(), extraVars);
			if (previewUrl) URL.revokeObjectURL(previewUrl);
			previewUrl = url;
		} catch (e) {
			previewError = e instanceof Error ? e.message : 'Render failed';
		} finally {
			previewing = false;
		}
	}
	function closePreview() {
		if (previewUrl) URL.revokeObjectURL(previewUrl);
		previewUrl = '';
		previewError = '';
	}
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
	let histTimer: ReturnType<typeof setTimeout>;
	$effect(() => {
		JSON.stringify(store.layout); // track every nested change
		clearTimeout(histTimer);
		histTimer = setTimeout(() => store.record(), 350);
	});

	onDestroy(() => {
		clearTimeout(histTimer);
		clearTimeout(resolveTimer);
		if (previewUrl) URL.revokeObjectURL(previewUrl);
	});

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && (previewUrl || previewError)) {
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
	<div class="studio-bar relative z-20 flex h-12 shrink-0 items-center gap-3 border-b border-line bg-surface px-3">
		<div class="flex items-center gap-2">
			<span class="grid h-6 w-6 place-items-center rounded-md bg-ink-2 text-ink ring-1 ring-line">
				<Frame size={14} />
			</span>
			<span class="text-[13px] font-semibold tracking-tight text-ink">{title}</span>
		</div>
		<span class="rounded border border-line-strong bg-ink-2 px-1.5 py-0.5 font-mono text-[10px] tabular-nums text-faint">
			{store.layout.width}×{store.layout.height}
		</span>
		<div class="ml-1 flex items-center gap-0.5">
			<button
				type="button"
				onclick={() => store.undo()}
				disabled={!store.canUndo}
				title="Undo (⌘Z)"
				aria-label="Undo"
				class="grid h-7 w-7 place-items-center rounded-md text-muted transition-colors hover:bg-surface hover:text-ink disabled:opacity-30 disabled:hover:bg-transparent"
			>
				<Undo2 size={14} />
			</button>
			<button
				type="button"
				onclick={() => store.redo()}
				disabled={!store.canRedo}
				title="Redo (⇧⌘Z)"
				aria-label="Redo"
				class="grid h-7 w-7 place-items-center rounded-md text-muted transition-colors hover:bg-surface hover:text-ink disabled:opacity-30 disabled:hover:bg-transparent"
			>
				<Redo2 size={14} />
			</button>
		</div>
		<div class="flex-1"></div>
		<button
			type="button"
			onclick={renderServer}
			disabled={previewing}
			class="flex h-7 items-center gap-1.5 rounded-md border border-line-strong px-2.5 text-[12px] font-medium text-muted transition-colors hover:bg-surface hover:text-ink disabled:opacity-50"
		>
			{#if previewing}<Loader2 size={13} class="animate-spin" />{:else}<Image size={13} />{/if}
			Server render
		</button>
		{@render actions?.()}
	</div>

	<!-- Three-pane body -->
	<div class="flex min-h-0 flex-1">
		<aside class="w-56 shrink-0 overflow-y-auto border-r border-line bg-surface">
			<LayersPanel />
		</aside>

		<div class="canvas-pit relative min-w-0 flex-1">
			<Canvas />

			<!-- Tool palette — pick a tool, then drag on the canvas to draw it. -->
			<div class="pointer-events-none absolute bottom-4 left-1/2 z-10 -translate-x-1/2">
				<div
					class="pointer-events-auto flex items-center gap-0.5 rounded-2xl border border-line-strong bg-surface/70 p-1 shadow-2xl backdrop-blur-xl"
				>
					{#each tools as t, i (t.id)}
						{@const Icon = t.icon}
						{#if i > 0 && t.group !== tools[i - 1].group}
							<span class="mx-0.5 h-5 w-px shrink-0 bg-line-strong"></span>
						{/if}
						<button
							type="button"
							onclick={() => store.setTool(t.id)}
							title={t.key ? `${t.label} (${t.key})` : t.label}
							aria-label={t.label}
							aria-pressed={store.tool === t.id}
							class="flex h-8 w-8 items-center justify-center rounded-lg transition-all {store.tool ===
							t.id
								? 'bg-surface text-ink shadow-[inset_0_1px_0_rgba(255,255,255,0.06)] ring-1 ring-line-strong'
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
							class="flex h-8 w-8 items-center justify-center rounded-lg outline-none transition-all {store.tool ===
							'shape'
								? 'bg-surface text-ink shadow-[inset_0_1px_0_rgba(255,255,255,0.06)] ring-1 ring-line-strong'
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

			{#if previewUrl || previewError}
				<div class="absolute inset-0 z-20 grid place-items-center bg-ink-2/80 p-8 backdrop-blur-sm">
					<div class="card w-full max-w-3xl overflow-hidden bg-surface shadow-2xl">
						<div class="flex items-center justify-between gap-4 border-b border-line px-4 py-3">
							<div>
								<div class="text-[13px] font-semibold text-ink">Server render</div>
								<div class="mt-0.5 text-[11px] text-muted">The exact PNG the bot would post.</div>
							</div>
							<button
								type="button"
								onclick={closePreview}
								class="grid h-7 w-7 place-items-center rounded-md text-muted transition-colors hover:bg-ink-2 hover:text-ink"
								aria-label="Close render"
							>
								<X size={15} />
							</button>
						</div>
						<div class="grid place-items-center bg-ink-2 p-4">
							{#if previewError}
								<div class="flex items-center gap-2 py-10 text-[13px] text-danger">
									<AlertTriangle size={15} /> {previewError}
								</div>
							{:else}
								<img src={previewUrl} alt="Server-rendered card" class="max-h-[60vh] w-auto max-w-full rounded-lg border border-line" />
							{/if}
						</div>
					</div>
				</div>
			{/if}
		</div>

		<aside class="w-72 shrink-0 overflow-y-auto border-l border-line bg-surface">
			<PropertiesPanel />
		</aside>
	</div>
</div>

<style>
	/* The studio uses the dashboard's own clean palette (white text on neutral
	   charcoal surfaces, like the sidebar) — no custom colour overrides, just the
	   structural polish + a soft entrance. */
	.studio-theme {
		animation: studio-in 0.24s cubic-bezier(0.16, 1, 0.3, 1);
	}
	@keyframes studio-in {
		from {
			opacity: 0;
			transform: scale(0.985) translateY(4px);
		}
		to {
			opacity: 1;
			transform: scale(1) translateY(0);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.studio-theme {
			animation: none;
		}
	}

	/* Toolbar: a faint top sheen for depth (over bg-surface). */
	.studio-bar {
		background-image: linear-gradient(to bottom, rgba(255, 255, 255, 0.03), rgba(255, 255, 255, 0));
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
