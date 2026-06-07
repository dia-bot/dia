<script lang="ts">
	// The layer list — a Figma-style stack with drag-to-reorder. The store keeps
	// layers back-to-front (index 0 draws first); here we render the REVERSE so the
	// front-most layer sits at the top, matching designer expectations.
	import { getContext } from 'svelte';
	import { DropdownMenu } from 'bits-ui';
	import { EditorStore, EDITOR_CTX } from '$lib/layout/editor.svelte';
	import { MAX_LAYERS, type Layer, type LayerType } from '$lib/layout/schema';
	import { Type, Image, UserCircle, Square, Circle, PenTool, Plus, Eye, EyeOff, Copy, Trash2, GripVertical, Frame } from 'lucide-svelte';

	const editor = getContext<EditorStore>(EDITOR_CTX);
	const ordered = $derived([...editor.layout.layers].reverse()); // front-most first
	// The "canvas" is selected when nothing else is — that's when the inspector
	// shows the document's size + background controls.
	const canvasActive = $derived(editor.selectedIds.length === 0);

	const icons: Record<LayerType, typeof Type> = {
		text: Type,
		image: Image,
		avatar: UserCircle,
		rect: Square,
		ellipse: Circle,
		path: PenTool
	};
	const addItems: { type: LayerType; label: string; icon: typeof Type }[] = [
		{ type: 'text', label: 'Text', icon: Type },
		{ type: 'image', label: 'Image', icon: Image },
		{ type: 'avatar', label: 'Avatar', icon: UserCircle },
		{ type: 'rect', label: 'Rectangle', icon: Square },
		{ type: 'ellipse', label: 'Ellipse', icon: Circle }
	];

	const ROW = 32; // px, matches h-8

	// ── drag-to-reorder ──────────────────────────────────────────
	let listEl = $state<HTMLElement>();
	let dragId = $state<string | null>(null);
	let dropGap = $state(0); // insertion gap in display order, 0..length

	function startDrag(e: PointerEvent, id: string) {
		e.stopPropagation();
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		dragId = id;
		updateGap(e);
	}
	function updateGap(e: PointerEvent) {
		if (!dragId || !listEl) return;
		const rel = e.clientY - listEl.getBoundingClientRect().top;
		dropGap = Math.max(0, Math.min(ordered.length, Math.round(rel / ROW)));
	}
	function endDrag(e: PointerEvent) {
		if (!dragId) return;
		try {
			(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
		} catch {
			/* gone */
		}
		const ids = ordered.map((l) => l.id);
		const from = ids.indexOf(dragId);
		if (from >= 0) {
			ids.splice(from, 1);
			let to = dropGap;
			if (from < to) to -= 1; // removal shifts the target up
			ids.splice(Math.max(0, Math.min(to, ids.length)), 0, dragId);
			editor.setOrder(ids);
		}
		dragId = null;
	}

	function stop(e: MouseEvent) {
		e.stopPropagation();
	}
</script>

<div class="flex h-full flex-col">
	<header class="flex h-9 items-center justify-between gap-2 border-b border-line pr-1 pl-3">
		<span class="eyebrow text-faint">Layers</span>
		<div class="flex items-center gap-1.5">
			<span class="font-mono text-[11px] tabular-nums {editor.atLimit ? 'text-accent-ink' : 'text-faint'}">
				{editor.layout.layers.length}/{MAX_LAYERS}
			</span>
			<DropdownMenu.Root>
				<DropdownMenu.Trigger
					disabled={editor.atLimit}
					title={editor.atLimit ? `Layer limit reached (${MAX_LAYERS})` : 'Add layer'}
					class="flex h-6 w-6 items-center justify-center rounded-md text-faint outline-none transition-colors hover:bg-surface hover:text-ink disabled:cursor-not-allowed disabled:opacity-30 disabled:hover:bg-transparent data-[state=open]:bg-surface data-[state=open]:text-ink"
					aria-label="Add layer"
				>
					<Plus size={15} />
				</DropdownMenu.Trigger>
				<DropdownMenu.Portal>
					<DropdownMenu.Content
						align="end"
						sideOffset={6}
						class="menu-pop z-50 min-w-[160px] overflow-hidden rounded-xl border border-line-strong bg-surface p-1.5 shadow-2xl outline-none"
					>
						{#each addItems as item (item.type)}
							{@const Icon = item.icon}
							<DropdownMenu.Item
								onSelect={() => editor.addLayer(item.type)}
								class="flex cursor-pointer items-center gap-2.5 rounded-lg px-2 py-1.5 text-[13px] text-muted outline-none transition-colors data-[highlighted]:bg-ink-2 data-[highlighted]:text-ink"
							>
								<Icon size={14} class="text-faint" />
								{item.label}
							</DropdownMenu.Item>
						{/each}
					</DropdownMenu.Content>
				</DropdownMenu.Portal>
			</DropdownMenu.Root>
		</div>
	</header>

	<!-- Canvas row: click to deselect everything and edit the document (size + background). -->
	<button
		type="button"
		onclick={() => editor.select(null)}
		class="flex h-8 w-full shrink-0 items-center gap-2 border-b border-line px-3 text-[13px] outline-none transition-colors {canvasActive
			? 'bg-surface text-ink'
			: 'text-muted hover:bg-surface/50'}"
	>
		<Frame size={14} class="shrink-0 {canvasActive ? 'text-accent-ink' : 'text-faint'}" />
		<span class="flex-1 text-left">Canvas</span>
		<span class="font-mono text-[10px] tabular-nums text-faint">
			{editor.layout.width}×{editor.layout.height}
		</span>
	</button>

	<div class="min-h-0 flex-1 overflow-y-auto py-1">
		{#if ordered.length === 0}
			<p class="px-3 py-6 text-center text-[12px] text-faint">No layers yet</p>
		{:else}
			<ul bind:this={listEl} class="relative">
				{#each ordered as layer (layer.id)}
					{@const Icon = icons[layer.type]}
					{@const selected = editor.isSelected(layer.id)}
					<li>
						<div
							role="button"
							tabindex="0"
							onclick={(e) => editor.select(layer.id, e.shiftKey || e.metaKey || e.ctrlKey)}
							onkeydown={(e) => {
								if (e.key === 'Enter' || e.key === ' ') {
									e.preventDefault();
									editor.select(layer.id);
								}
							}}
							class="group flex h-8 cursor-pointer items-center gap-1.5 pr-1 pl-1.5 text-[13px] outline-none transition-[background-color,opacity] {selected
								? 'bg-surface text-ink'
								: 'text-muted hover:bg-surface/50'} {dragId === layer.id ? 'opacity-40' : ''}"
						>
							<!-- drag grip -->
							<button
								type="button"
								onpointerdown={(e) => startDrag(e, layer.id)}
								onpointermove={updateGap}
								onpointerup={endDrag}
								onpointercancel={endDrag}
								onclick={stop}
								aria-label="Drag to reorder"
								class="flex h-6 w-4 shrink-0 cursor-grab touch-none items-center justify-center text-faint opacity-0 transition-opacity group-hover:opacity-100 active:cursor-grabbing"
							>
								<GripVertical size={13} />
							</button>

							<Icon size={14} class="shrink-0 {selected ? 'text-accent-ink' : 'text-faint'}" />
							<span class="min-w-0 flex-1 truncate {layer.hidden ? 'opacity-50' : ''}">{layer.name}</span>

							<div class="flex items-center gap-0.5 opacity-0 transition-opacity group-hover:opacity-100 {selected ? 'opacity-100' : ''}">
								<button
									type="button"
									onclick={(e) => {
										stop(e);
										editor.duplicateLayer(layer.id);
									}}
									disabled={editor.atLimit}
									aria-label="Duplicate"
									class="flex h-6 w-6 items-center justify-center rounded-md text-faint transition-colors hover:bg-ink-2 hover:text-ink disabled:opacity-30 disabled:hover:bg-transparent"
								>
									<Copy size={13} />
								</button>
								<button
									type="button"
									onclick={(e) => {
										stop(e);
										editor.removeLayer(layer.id);
									}}
									aria-label="Delete"
									class="flex h-6 w-6 items-center justify-center rounded-md text-faint transition-colors hover:bg-ink-2 hover:text-danger"
								>
									<Trash2 size={13} />
								</button>
							</div>

							<button
								type="button"
								onclick={(e) => {
									stop(e);
									editor.patch(layer.id, { hidden: !layer.hidden });
								}}
								aria-label={layer.hidden ? 'Show layer' : 'Hide layer'}
								class="flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-muted transition-colors hover:bg-ink-2 hover:text-ink"
							>
								{#if layer.hidden}<EyeOff size={13} />{:else}<Eye size={13} />{/if}
							</button>
						</div>
					</li>
				{/each}

				{#if dragId !== null}
					<div class="pointer-events-none absolute inset-x-1 h-0.5 -translate-y-1/2 rounded-full bg-accent" style="top:{dropGap * ROW}px"></div>
				{/if}
			</ul>
		{/if}
	</div>
</div>
