<script lang="ts">
	// The layer list — a Figma-style stack with drag-to-reorder. The store keeps
	// layers back-to-front (index 0 draws first); here we render the REVERSE so the
	// front-most layer sits at the top, matching designer expectations.
	import { getContext, tick } from 'svelte';
	import { DropdownMenu, ContextMenu } from 'bits-ui';
	import { EditorStore, EDITOR_CTX } from '$lib/layout/editor.svelte';
	import { MAX_LAYERS, type Layer, type LayerType } from '$lib/layout/schema';
	import {
		Type,
		Image,
		UserCircle,
		Square,
		Circle,
		PenTool,
		Plus,
		Eye,
		EyeOff,
		Copy,
		Trash2,
		GripVertical,
		Frame,
		Scissors,
		CornerDownRight,
		ArrowUpToLine,
		ArrowDownToLine,
		ChevronUp,
		ChevronDown,
		Group,
		Ungroup,
		PencilLine,
		ClipboardPaste,
		Lock,
		Unlock
	} from 'lucide-svelte';

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
	const addItems: { type: LayerType; label: string; icon: typeof Type; avatar?: boolean }[] = [
		{ type: 'text', label: 'Text', icon: Type },
		{ type: 'image', label: 'Image', icon: Image },
		{ type: 'image', label: 'Avatar', icon: UserCircle, avatar: true },
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

	// ── context-menu helpers ─────────────────────────────────────────────────────
	// Right-clicking a row acts on the current multi-selection if the row is part of
	// it; otherwise it selects just that row first (Figma's behaviour).
	function ensureSelected(id: string) {
		if (!editor.isSelected(id)) editor.select(id);
	}

	// inline rename (double-click the name)
	let renamingId = $state<string | null>(null);
	async function startRename(id: string) {
		renamingId = id;
		await tick();
		const el = document.getElementById(`rn-${id}`) as HTMLInputElement | null;
		el?.focus();
		el?.select();
	}
	function commitRename(id: string, value: string) {
		editor.rename(id, value);
		renamingId = null;
	}
	const menuItem =
		'flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-1.5 text-[13px] text-muted outline-none transition-colors data-[highlighted]:bg-ink-2 data-[highlighted]:text-ink data-[disabled]:pointer-events-none data-[disabled]:opacity-40';
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
								onSelect={() => (item.avatar ? editor.addAvatar() : editor.addLayer(item.type))}
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
	<div class="px-1.5 pt-1.5">
		<button
			type="button"
			onclick={() => editor.select(null)}
			class="group flex h-8 w-full items-center gap-2 rounded-lg px-2 text-[13px] outline-none transition-all duration-100 {canvasActive
				? 'bg-surface text-ink ring-1 ring-line-strong'
				: 'text-muted hover:bg-surface/60'}"
		>
			<Frame size={14} class="shrink-0 {canvasActive ? 'text-ink' : 'text-faint group-hover:text-muted'}" />
			<span class="flex-1 text-left font-medium">Canvas</span>
			<span class="font-mono text-[10px] tabular-nums text-faint">
				{editor.layout.width}×{editor.layout.height}
			</span>
		</button>
	</div>
	<div class="mx-3 my-1.5 h-px bg-line"></div>

	<div class="min-h-0 flex-1 overflow-y-auto py-1">
		{#if ordered.length === 0}
			<p class="px-3 py-6 text-center text-[12px] text-faint">No layers yet</p>
		{:else}
			<ul bind:this={listEl} class="relative">
				{#each ordered as layer (layer.id)}
					{@const Icon = icons[layer.type]}
					{@const selected = editor.isSelected(layer.id)}
					{@const masked = !!editor.maskFor(layer)}
					<li>
						<ContextMenu.Root>
							<ContextMenu.Trigger>
								{#snippet child({ props })}
									<div
										{...props}
										role="button"
										tabindex="0"
										onclick={(e) => editor.select(layer.id, e.shiftKey || e.metaKey || e.ctrlKey)}
										onkeydown={(e) => {
											if (e.key === 'Enter' || e.key === ' ') {
												e.preventDefault();
												editor.select(layer.id);
											}
										}}
										class="group mx-1.5 flex h-8 cursor-pointer items-center gap-1.5 rounded-lg pr-1 text-[13px] outline-none transition-all duration-100 {masked
											? 'pl-6'
											: 'pl-1'} {selected
											? 'bg-surface text-ink ring-1 ring-line-strong'
											: 'text-muted hover:bg-surface/60'} {dragId === layer.id ? 'opacity-40' : ''} {layer.locked
											? 'opacity-70'
											: ''}"
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

										{#if masked}<CornerDownRight size={12} class="-ml-0.5 shrink-0 text-faint" />{/if}
										<Icon
											size={14}
											class="shrink-0 {layer.clip
												? 'text-accent-ink'
												: selected
													? 'text-ink'
													: 'text-faint group-hover:text-muted'}"
										/>

										{#if renamingId === layer.id}
											<input
												id="rn-{layer.id}"
												value={layer.name}
												onpointerdown={(e) => e.stopPropagation()}
												onclick={(e) => e.stopPropagation()}
												onblur={(e) => commitRename(layer.id, e.currentTarget.value)}
												onkeydown={(e) => {
													e.stopPropagation();
													if (e.key === 'Enter') e.currentTarget.blur();
													else if (e.key === 'Escape') renamingId = null;
												}}
												class="min-w-0 flex-1 rounded border border-line-strong bg-ink-2 px-1 py-0.5 text-[13px] text-ink outline-none focus:border-faint"
											/>
										{:else}
											<span
												role="button"
												tabindex="-1"
												class="min-w-0 flex-1 truncate {layer.hidden ? 'opacity-50' : ''} {selected
													? 'font-medium'
													: ''}"
												ondblclick={(e) => {
													stop(e);
													startRename(layer.id);
												}}>{layer.name}</span
											>
										{/if}

										{#if layer.clip}<Scissors size={11} class="shrink-0 text-accent-ink" />{/if}

										<div
											class="flex items-center gap-0.5 transition-opacity {layer.hidden ||
											layer.locked ||
											selected
												? 'opacity-100'
												: 'opacity-0 group-hover:opacity-100'}"
										>
											<button
												type="button"
												onclick={(e) => {
													stop(e);
													editor.toggleLock(layer.id);
												}}
												aria-label={layer.locked ? 'Unlock' : 'Lock'}
												class="flex h-6 w-6 items-center justify-center rounded-md text-faint transition-colors hover:bg-ink-2 hover:text-ink"
											>
												{#if layer.locked}<Lock size={12} />{:else}<Unlock size={12} />{/if}
											</button>
											<button
												type="button"
												onclick={(e) => {
													stop(e);
													editor.patch(layer.id, { hidden: !layer.hidden });
												}}
												aria-label={layer.hidden ? 'Show layer' : 'Hide layer'}
												class="flex h-6 w-6 items-center justify-center rounded-md text-muted transition-colors hover:bg-ink-2 hover:text-ink"
											>
												{#if layer.hidden}<EyeOff size={13} />{:else}<Eye size={13} />{/if}
											</button>
										</div>
									</div>
								{/snippet}
							</ContextMenu.Trigger>
							<ContextMenu.Portal>
								<ContextMenu.Content
									class="menu-pop z-50 min-w-[210px] rounded-xl border border-line-strong bg-surface p-1.5 shadow-2xl outline-none"
								>
									{#if layer.clip}
										<ContextMenu.Item class={menuItem} onSelect={() => editor.releaseMask(layer.id)}>
											<Scissors size={14} class="text-faint" /> Release mask
										</ContextMenu.Item>
									{:else}
										<ContextMenu.Item
											class={menuItem}
											onSelect={() => {
												ensureSelected(layer.id);
												editor.useAsMask();
											}}
										>
											<Scissors size={14} class="text-faint" /> Use as mask
										</ContextMenu.Item>
									{/if}

									<ContextMenu.Separator class="my-1 h-px bg-line" />

									<ContextMenu.Item class={menuItem} onSelect={() => editor.bringToFront(layer.id)}>
										<ArrowUpToLine size={14} class="text-faint" /> Bring to front
									</ContextMenu.Item>
									<ContextMenu.Item class={menuItem} onSelect={() => editor.reorder(layer.id, 1)}>
										<ChevronUp size={14} class="text-faint" /> Bring forward
									</ContextMenu.Item>
									<ContextMenu.Item class={menuItem} onSelect={() => editor.reorder(layer.id, -1)}>
										<ChevronDown size={14} class="text-faint" /> Send backward
									</ContextMenu.Item>
									<ContextMenu.Item class={menuItem} onSelect={() => editor.sendToBack(layer.id)}>
										<ArrowDownToLine size={14} class="text-faint" /> Send to back
									</ContextMenu.Item>

									<ContextMenu.Separator class="my-1 h-px bg-line" />

									<ContextMenu.Item
										class={menuItem}
										disabled={!editor.canGroup}
										onSelect={() => editor.group()}
									>
										<Group size={14} class="text-faint" /> Group selection
									</ContextMenu.Item>
									<ContextMenu.Item
										class={menuItem}
										disabled={!editor.canUngroup}
										onSelect={() => editor.ungroup()}
									>
										<Ungroup size={14} class="text-faint" /> Ungroup
									</ContextMenu.Item>

									<ContextMenu.Separator class="my-1 h-px bg-line" />

									<ContextMenu.Item class={menuItem} onSelect={() => startRename(layer.id)}>
										<PencilLine size={14} class="text-faint" /> Rename
									</ContextMenu.Item>
									<ContextMenu.Item
										class={menuItem}
										disabled={editor.atLimit}
										onSelect={() => editor.duplicateLayer(layer.id)}
									>
										<Copy size={14} class="text-faint" /> Duplicate
									</ContextMenu.Item>
									<ContextMenu.Item class={menuItem} onSelect={() => editor.paste()}>
										<ClipboardPaste size={14} class="text-faint" /> Paste
									</ContextMenu.Item>
									<ContextMenu.Item class={menuItem} onSelect={() => editor.toggleLock(layer.id)}>
										{#if layer.locked}<Unlock size={14} class="text-faint" /> Unlock{:else}<Lock
												size={14}
												class="text-faint"
											/> Lock{/if}
									</ContextMenu.Item>
									<ContextMenu.Item
										class={menuItem}
										onSelect={() => editor.patch(layer.id, { hidden: !layer.hidden })}
									>
										{#if layer.hidden}<Eye size={14} class="text-faint" /> Show{:else}<EyeOff
												size={14}
												class="text-faint"
											/> Hide{/if}
									</ContextMenu.Item>

									<ContextMenu.Separator class="my-1 h-px bg-line" />

									<ContextMenu.Item
										class="{menuItem} data-[highlighted]:!bg-danger/15 data-[highlighted]:!text-danger"
										onSelect={() => {
											ensureSelected(layer.id);
											editor.removeSelected();
										}}
									>
										<Trash2 size={14} /> Delete
									</ContextMenu.Item>
								</ContextMenu.Content>
							</ContextMenu.Portal>
						</ContextMenu.Root>
					</li>
				{/each}

				{#if dragId !== null}
					<div class="pointer-events-none absolute inset-x-2 h-0.5 -translate-y-1/2 rounded-full bg-ink" style="top:{dropGap * ROW}px"></div>
				{/if}
			</ul>
		{/if}
	</div>
</div>
