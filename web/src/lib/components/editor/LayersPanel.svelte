<script lang="ts">
	// The layers panel — a Figma-style hierarchical tree with drag-to-reorder. The
	// store keeps layers back-to-front (index 0 draws first); `editor.tree` derives
	// the display list (front-most first) where each contiguous group becomes a
	// container row with its members nested one level under it. A mask group shows
	// the stencil at the bottom of the group with a scissors badge; its masked
	// siblings get a small "into-mask" arrow.
	import { getContext, tick } from 'svelte';
	import { DropdownMenu, ContextMenu } from 'bits-ui';
	import { EditorStore, EDITOR_CTX, type LayerRow } from '$lib/layout/editor.svelte';
	import { MAX_LAYERS, type LayerType } from '$lib/layout/schema';
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
		Combine,
		Frame,
		Scissors,
		CornerDownRight,
		ArrowUpToLine,
		ArrowDownToLine,
		ChevronUp,
		ChevronDown,
		ChevronRight,
		Group,
		Ungroup,
		PencilLine,
		ClipboardPaste,
		Lock,
		Unlock,
		FolderOpen
	} from 'lucide-svelte';
	import { flip } from 'svelte/animate';
	import { fade } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';

	const editor = getContext<EditorStore>(EDITOR_CTX);
	const rows = $derived(editor.tree); // front-most first; containers + nested leaves
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

	const ROW = 32; // px, matches h-8 — every tree row (container or leaf) is one ROW

	// ── selection ────────────────────────────────────────────────────────────────
	function clickGroup(gid: string) {
		if (consumeDragClick()) return; // a drop just fired this click → don't reselect
		editor.selectGroup(gid);
	}
	function clickLeaf(e: MouseEvent, id: string) {
		if (consumeDragClick()) return;
		if (e.shiftKey || e.metaKey || e.ctrlKey) editor.select(id, true); // additive toggle
		else editor.selectOne(id); // pick exactly this layer (not its whole group)
	}
	function groupSelected(row: Extract<LayerRow, { kind: 'group' }>): boolean {
		return row.childIds.length > 0 && row.childIds.every((id) => editor.isSelected(id));
	}

	// ── group eye / lock (toggle every member) ─────────────────────────────────────
	function toggleGroupHidden(row: Extract<LayerRow, { kind: 'group' }>) {
		const hide = !row.hidden;
		for (const id of row.childIds) editor.patch(id, { hidden: hide });
	}
	function toggleGroupLock(row: Extract<LayerRow, { kind: 'group' }>) {
		const lock = !row.locked;
		for (const id of row.childIds) {
			const l = editor.layout.layers.find((x) => x.id === id);
			if (l) l.locked = lock;
		}
	}
	function deleteGroup(gid: string) {
		editor.selectGroup(gid);
		editor.removeSelected();
	}

	// ── drag-to-reorder (nesting-aware) ────────────────────────────────────────────
	// Grab a row anywhere (no dedicated grip): pointerdown arms a pending drag that
	// only becomes a real reorder once the pointer crosses a small slop — so a plain
	// click still selects, and a touch that scrolls the list cancels it. Figma-style.
	const DRAG_SLOP = 4; // px of movement before a press turns into a drag
	let listEl = $state<HTMLElement>();
	let drag = $state<{ kind: 'leaf' | 'group'; id: string } | null>(null);
	let pending: { kind: 'leaf' | 'group'; id: string; x: number; y: number; pointerId: number } | null =
		null;
	let justDragged = false; // a drop just happened → swallow the click that follows it
	let dropGap = $state(0); // insertion gap in the render list, 0..rows.length
	let dropGroup = $state<string | null>(null); // group the drop would nest into

	function consumeDragClick(): boolean {
		if (!justDragged) return false;
		justDragged = false;
		return true;
	}

	function flatIndexOf(id: string): number {
		return editor.layout.layers.findIndex((l) => l.id === id);
	}
	function groupEnd(gid: string): number {
		const arr = editor.layout.layers;
		let lo = arr.findIndex((l) => l.group === gid);
		if (lo < 0) return arr.length;
		let hi = lo;
		while (hi < arr.length && arr[hi].group === gid) hi++;
		return hi;
	}

	function rowPointerDown(e: PointerEvent, kind: 'leaf' | 'group', id: string) {
		if (e.button !== 0) return; // left button only — right-click opens the context menu
		justDragged = false;
		pending = { kind, id, x: e.clientX, y: e.clientY, pointerId: e.pointerId };
	}
	function rowPointerMove(e: PointerEvent) {
		if (drag) {
			updateGap(e);
			return;
		}
		if (!pending || e.pointerId !== pending.pointerId) return;
		if (Math.abs(e.clientX - pending.x) + Math.abs(e.clientY - pending.y) < DRAG_SLOP) return;
		// Crossed the slop → promote to a real drag and capture the pointer to the row.
		try {
			(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		} catch {
			/* element gone */
		}
		drag = { kind: pending.kind, id: pending.id };
		updateGap(e);
	}
	function rowPointerUp(e: PointerEvent) {
		if (drag) {
			justDragged = true; // the trailing click selects the dropped row — suppress it
			endDrag(e);
		}
		pending = null;
	}
	function rowPointerCancel(e: PointerEvent) {
		// A cancelled pointer (touch scroll-steal, OS gesture takeover, blur) must ABORT
		// the drag — never commit a reorder at the last gap. Only pointerup commits.
		if (drag) {
			try {
				(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
			} catch {
				/* element gone */
			}
		}
		drag = null;
		dropGroup = null;
		pending = null;
	}
	function updateGap(e: PointerEvent) {
		if (!drag || !listEl) return;
		const rel = e.clientY - listEl.getBoundingClientRect().top;
		const gap = Math.max(0, Math.min(rows.length, Math.round(rel / ROW)));
		dropGap = gap;
		// Decide whether the drop nests into a group (only a leaf can nest).
		dropGroup = null;
		if (drag.kind === 'leaf') {
			const above = rows[gap - 1];
			const below = rows[gap];
			if (above?.kind === 'leaf' && below?.kind === 'leaf' && above.group && above.group === below.group)
				dropGroup = above.group; // between two children of one group
			else if (above?.kind === 'group' && !above.collapsed && below?.kind === 'leaf' && below.group === above.id)
				dropGroup = above.id; // just under an expanded group header → front of it
		}
	}
	function endDrag(e: PointerEvent) {
		if (!drag) return;
		try {
			(e.currentTarget as HTMLElement).releasePointerCapture(e.pointerId);
		} catch {
			/* gone */
		}
		const below = rows[dropGap];
		// Insert the dragged unit just in front of the unit below the gap (or at the
		// very back when the gap is past the last row). moveLayer/moveGroup clamp to
		// keep groups contiguous, so this rough target is enough.
		let flatIndex: number;
		if (!below) flatIndex = 0;
		else flatIndex = below.kind === 'leaf' ? flatIndexOf(below.id) + 1 : groupEnd(below.id);
		if (drag.kind === 'group') {
			editor.moveGroup(drag.id, flatIndex);
			editor.selectGroup(drag.id); // match moveLayer, which selects what it moved
		} else {
			editor.moveLayer(drag.id, flatIndex, dropGroup);
		}
		drag = null;
		dropGroup = null;
	}
	function endDragGlobal() {
		pending = null;
		if (drag !== null) {
			drag = null;
			dropGroup = null;
		}
	}

	function stop(e: MouseEvent) {
		e.stopPropagation();
	}

	// ── context-menu helpers ───────────────────────────────────────────────────────
	function ensureSelectedLeaf(id: string) {
		if (!editor.isSelected(id)) editor.selectOne(id);
	}
	function ensureSelectedGroup(row: Extract<LayerRow, { kind: 'group' }>) {
		if (!groupSelected(row)) editor.selectGroup(row.id);
	}

	// ── inline rename (double-click the name) ───────────────────────────────────────
	let renaming = $state<{ kind: 'leaf' | 'group'; id: string } | null>(null);
	async function startRename(kind: 'leaf' | 'group', id: string) {
		renaming = { kind, id };
		await tick();
		const el = document.getElementById(`rn-${kind}-${id}`) as HTMLInputElement | null;
		el?.focus();
		el?.select();
	}
	function commitRename(value: string) {
		if (!renaming) return;
		if (renaming.kind === 'group') editor.renameGroup(renaming.id, value);
		else editor.rename(renaming.id, value);
		renaming = null;
	}
	function isRenaming(kind: 'leaf' | 'group', id: string): boolean {
		return renaming?.kind === kind && renaming.id === id;
	}

	const menuItem =
		'flex cursor-pointer items-center gap-2.5 rounded-md px-2 py-1.5 text-[13px] text-muted outline-none transition-colors data-[highlighted]:bg-ink-2 data-[highlighted]:text-ink data-[disabled]:pointer-events-none data-[disabled]:opacity-40';

	// Ghost icon button for the per-row eye / lock controls (shadcn-on-Dia ghost).
	const rowIcon =
		'inline-flex h-6 w-6 items-center justify-center rounded-md text-muted outline-none transition-colors hover:bg-ink-2 hover:text-ink focus-visible:ring-2 focus-visible:ring-line-strong';
</script>

<svelte:window onpointercancel={endDragGlobal} onblur={endDragGlobal} />

<div class="flex h-full flex-col">
	<header class="flex h-9 items-center justify-between gap-2 border-b border-line pr-1 pl-3">
		<span class="eyebrow text-faint">Layers</span>
		<div class="flex items-center gap-1.5">
			<span class="font-mono text-[11px] tabular-nums {editor.atLimit ? 'text-danger' : 'text-faint'}">
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
						{#each addItems as item (item.label)}
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
				? 'bg-white/[0.07] text-ink ring-1 ring-line-strong'
				: 'text-muted hover:bg-white/[0.04]'}"
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
		{#if rows.length === 0}
			<p class="px-3 py-6 text-center text-[12px] text-faint">No layers yet</p>
		{:else}
			<ul bind:this={listEl} class="relative select-none">
				{#each rows as row (row.kind === 'group' ? 'g:' + row.id : row.id)}
					<li animate:flip={{ duration: 200, easing: cubicOut }} transition:fade={{ duration: 110 }}>
					{#if row.kind === 'group'}
						{@const selected = groupSelected(row)}
						{@const GroupIcon = row.isMask ? Scissors : row.isBoolean ? Combine : FolderOpen}
							<ContextMenu.Root onOpenChange={(open) => open && ensureSelectedGroup(row)}>
								<ContextMenu.Trigger>
									{#snippet child({ props })}
										<div
											{...props}
											role="button"
											tabindex="0"
											onclick={() => clickGroup(row.id)}
											onkeydown={(e) => {
												if (e.key === 'Enter' || e.key === ' ') {
													e.preventDefault();
													clickGroup(row.id);
												}
											}}
											onpointerdown={(e) => {
												(props as Record<string, any>).onpointerdown?.(e);
												rowPointerDown(e, 'group', row.id);
											}}
											onpointermove={(e) => {
												(props as Record<string, any>).onpointermove?.(e);
												rowPointerMove(e);
											}}
											onpointerup={(e) => {
												(props as Record<string, any>).onpointerup?.(e);
												rowPointerUp(e);
											}}
											onpointercancel={(e) => {
												(props as Record<string, any>).onpointercancel?.(e);
												rowPointerCancel(e);
											}}
											class="group mx-1.5 flex h-8 cursor-pointer items-center gap-1 rounded-lg pr-1 pl-2 text-[13px] outline-none transition-all duration-100 {selected
												? 'bg-white/[0.07] text-ink ring-1 ring-line-strong'
												: 'text-muted hover:bg-white/[0.04]'} {drag?.id === row.id ? 'opacity-40' : ''}"
										>
											<!-- disclosure chevron -->
											<button
												type="button"
												onpointerdown={(e) => e.stopPropagation()}
												onclick={(e) => {
													stop(e);
													editor.toggleCollapse(row.id);
												}}
												aria-label={row.collapsed ? 'Expand group' : 'Collapse group'}
												class="flex h-5 w-5 shrink-0 items-center justify-center rounded text-faint transition-colors hover:bg-ink-2 hover:text-ink"
											>
												{#if row.collapsed}<ChevronRight size={14} />{:else}<ChevronDown size={14} />{/if}
											</button>

											<GroupIcon size={14} class="shrink-0 {row.isMask || row.isBoolean ? 'text-muted' : selected ? 'text-ink' : 'text-faint group-hover:text-muted'}" />

											{#if isRenaming('group', row.id)}
												<input
													id="rn-group-{row.id}"
													value={row.name}
													onpointerdown={(e) => e.stopPropagation()}
													onclick={(e) => e.stopPropagation()}
													onblur={(e) => commitRename(e.currentTarget.value)}
													onkeydown={(e) => {
														e.stopPropagation();
														if (e.key === 'Enter') e.currentTarget.blur();
														else if (e.key === 'Escape') (renaming = null);
													}}
													class="min-w-0 flex-1 select-text rounded border border-line-strong bg-ink-2 px-1 py-0.5 text-[13px] text-ink outline-none focus:border-faint"
												/>
											{:else}
												<span
													role="button"
													tabindex="-1"
													class="min-w-0 flex-1 truncate font-medium {row.hidden ? 'opacity-50' : ''}"
													ondblclick={(e) => {
														stop(e);
														startRename('group', row.id);
													}}>{row.name}</span
												>
												<span class="shrink-0 font-mono text-[10px] tabular-nums text-faint">{row.childIds.length}</span>
											{/if}

											<div
												class="flex items-center gap-0.5 transition-opacity {row.hidden || row.locked || selected
													? 'opacity-100'
													: 'opacity-0 group-hover:opacity-100'}"
											>
												<button
													type="button"
													onpointerdown={(e) => e.stopPropagation()}
													onclick={(e) => {
														stop(e);
														toggleGroupLock(row);
													}}
													aria-label={row.locked ? 'Unlock group' : 'Lock group'}
													class={rowIcon}
												>
													{#if row.locked}<Lock size={12} />{:else}<Unlock size={12} />{/if}
												</button>
												<button
													type="button"
													onpointerdown={(e) => e.stopPropagation()}
													onclick={(e) => {
														stop(e);
														toggleGroupHidden(row);
													}}
													aria-label={row.hidden ? 'Show group' : 'Hide group'}
													class={rowIcon}
												>
													{#if row.hidden}<EyeOff size={13} />{:else}<Eye size={13} />{/if}
												</button>
											</div>
										</div>
									{/snippet}
								</ContextMenu.Trigger>
								<ContextMenu.Portal>
									<ContextMenu.Content
										class="menu-pop z-50 min-w-[210px] rounded-xl border border-line-strong bg-surface p-1.5 shadow-2xl outline-none"
									>
										{#if row.isMask}
											<ContextMenu.Item class={menuItem} onSelect={() => editor.toggleMask()}>
												<Scissors size={14} class="text-faint" /> Release mask
											</ContextMenu.Item>
										{:else}
											<ContextMenu.Item class={menuItem} onSelect={() => editor.useAsMask()}>
												<Scissors size={14} class="text-faint" /> Use as mask
											</ContextMenu.Item>
										{/if}
										<ContextMenu.Item class={menuItem} onSelect={() => editor.toggleCollapse(row.id)}>
											{#if row.collapsed}<ChevronDown size={14} class="text-faint" /> Expand{:else}<ChevronRight
													size={14}
													class="text-faint"
												/> Collapse{/if}
										</ContextMenu.Item>

										<ContextMenu.Separator class="my-1 h-px bg-line" />

										<ContextMenu.Item class={menuItem} onSelect={() => editor.bringToFront(row.childIds[0])}>
											<ArrowUpToLine size={14} class="text-faint" /> Bring to front
										</ContextMenu.Item>
										<ContextMenu.Item class={menuItem} onSelect={() => editor.sendToBack(row.childIds[0])}>
											<ArrowDownToLine size={14} class="text-faint" /> Send to back
										</ContextMenu.Item>

										<ContextMenu.Separator class="my-1 h-px bg-line" />

										<ContextMenu.Item class={menuItem} onSelect={() => startRename('group', row.id)}>
											<PencilLine size={14} class="text-faint" /> Rename
										</ContextMenu.Item>
										<ContextMenu.Item
											class={menuItem}
											onSelect={() => {
												editor.selectGroup(row.id);
												editor.ungroup();
											}}
										>
											<Ungroup size={14} class="text-faint" /> Ungroup
										</ContextMenu.Item>

										<ContextMenu.Separator class="my-1 h-px bg-line" />

										<ContextMenu.Item
											class="{menuItem} data-[highlighted]:!bg-danger/15 data-[highlighted]:!text-danger"
											onSelect={() => deleteGroup(row.id)}
										>
											<Trash2 size={14} /> Delete group
										</ContextMenu.Item>
									</ContextMenu.Content>
								</ContextMenu.Portal>
							</ContextMenu.Root>
					{:else}
						{@const layer = row.layer}
						{@const Icon = icons[layer.type]}
						{@const selected = editor.isSelected(layer.id)}
							<ContextMenu.Root onOpenChange={(open) => open && ensureSelectedLeaf(layer.id)}>
								<ContextMenu.Trigger>
									{#snippet child({ props })}
										<div
											{...props}
											role="button"
											tabindex="0"
											onclick={(e) => clickLeaf(e, layer.id)}
											onkeydown={(e) => {
												if (e.key === 'Enter' || e.key === ' ') {
													e.preventDefault();
													editor.selectOne(layer.id);
												}
											}}
											onpointerdown={(e) => {
												(props as Record<string, any>).onpointerdown?.(e);
												rowPointerDown(e, 'leaf', layer.id);
											}}
											onpointermove={(e) => {
												(props as Record<string, any>).onpointermove?.(e);
												rowPointerMove(e);
											}}
											onpointerup={(e) => {
												(props as Record<string, any>).onpointerup?.(e);
												rowPointerUp(e);
											}}
											onpointercancel={(e) => {
												(props as Record<string, any>).onpointercancel?.(e);
												rowPointerCancel(e);
											}}
											class="group mx-1.5 flex h-8 cursor-pointer items-center gap-1.5 rounded-lg pr-1 pl-2 text-[13px] outline-none transition-all duration-100 {selected
												? 'bg-white/[0.07] text-ink ring-1 ring-line-strong'
												: 'text-muted hover:bg-white/[0.04]'} {drag?.id === layer.id ? 'opacity-40' : ''} {layer.locked
												? 'opacity-70'
												: ''}"
										>
											<!-- nesting indent + (masked sibling) into-mask arrow -->
											{#if row.depth === 1}
												<span class="flex w-4 shrink-0 items-center justify-center">
													{#if row.masked}<CornerDownRight size={12} class="text-faint" />{/if}
												</span>
											{/if}

											<Icon
												size={14}
												class="shrink-0 {row.isStencil
													? 'text-muted'
													: selected
														? 'text-ink'
														: 'text-faint group-hover:text-muted'}"
											/>

											{#if isRenaming('leaf', layer.id)}
												<input
													id="rn-leaf-{layer.id}"
													value={layer.name}
													onpointerdown={(e) => e.stopPropagation()}
													onclick={(e) => e.stopPropagation()}
													onblur={(e) => commitRename(e.currentTarget.value)}
													onkeydown={(e) => {
														e.stopPropagation();
														if (e.key === 'Enter') e.currentTarget.blur();
														else if (e.key === 'Escape') (renaming = null);
													}}
													class="min-w-0 flex-1 select-text rounded border border-line-strong bg-ink-2 px-1 py-0.5 text-[13px] text-ink outline-none focus:border-faint"
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
														startRename('leaf', layer.id);
													}}>{layer.name}</span
												>
											{/if}

											{#if row.isStencil}<Scissors size={11} class="shrink-0 text-muted" />{/if}

											<div
												class="flex items-center gap-0.5 transition-opacity {layer.hidden || layer.locked || selected
													? 'opacity-100'
													: 'opacity-0 group-hover:opacity-100'}"
											>
												<button
													type="button"
													onpointerdown={(e) => e.stopPropagation()}
													onclick={(e) => {
														stop(e);
														editor.toggleLock(layer.id);
													}}
													aria-label={layer.locked ? 'Unlock' : 'Lock'}
													class={rowIcon}
												>
													{#if layer.locked}<Lock size={12} />{:else}<Unlock size={12} />{/if}
												</button>
												<button
													type="button"
													onpointerdown={(e) => e.stopPropagation()}
													onclick={(e) => {
														stop(e);
														editor.patch(layer.id, { hidden: !layer.hidden });
													}}
													aria-label={layer.hidden ? 'Show layer' : 'Hide layer'}
													class={rowIcon}
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
										{#if row.isStencil}
											<ContextMenu.Item class={menuItem} onSelect={() => editor.releaseMask(layer.id)}>
												<Scissors size={14} class="text-faint" /> Release mask
											</ContextMenu.Item>
										{:else}
											<ContextMenu.Item
												class={menuItem}
												disabled={!editor.canMask}
												onSelect={() => {
													ensureSelectedLeaf(layer.id);
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

										<ContextMenu.Item class={menuItem} disabled={!editor.canGroup} onSelect={() => editor.group()}>
											<Group size={14} class="text-faint" /> Group selection
										</ContextMenu.Item>
										<ContextMenu.Item class={menuItem} disabled={!editor.canUngroup} onSelect={() => editor.ungroup()}>
											<Ungroup size={14} class="text-faint" /> Ungroup
										</ContextMenu.Item>

										<ContextMenu.Separator class="my-1 h-px bg-line" />

										<ContextMenu.Item class={menuItem} onSelect={() => startRename('leaf', layer.id)}>
											<PencilLine size={14} class="text-faint" /> Rename
										</ContextMenu.Item>
										<ContextMenu.Item class={menuItem} disabled={editor.atLimit} onSelect={() => editor.duplicateLayer(layer.id)}>
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
										<ContextMenu.Item class={menuItem} onSelect={() => editor.patch(layer.id, { hidden: !layer.hidden })}>
											{#if layer.hidden}<Eye size={14} class="text-faint" /> Show{:else}<EyeOff
													size={14}
													class="text-faint"
												/> Hide{/if}
										</ContextMenu.Item>

										<ContextMenu.Separator class="my-1 h-px bg-line" />

										<ContextMenu.Item
											class="{menuItem} data-[highlighted]:!bg-danger/15 data-[highlighted]:!text-danger"
											onSelect={() => {
												ensureSelectedLeaf(layer.id);
												editor.removeSelected();
											}}
										>
											<Trash2 size={14} /> Delete
										</ContextMenu.Item>
									</ContextMenu.Content>
								</ContextMenu.Portal>
							</ContextMenu.Root>
					{/if}
					</li>
				{/each}

				{#if drag !== null}
					<div
						class="pointer-events-none absolute h-0.5 -translate-y-1/2 rounded-full bg-ink transition-[left] {dropGroup
							? 'left-7 right-2'
							: 'inset-x-2'}"
						style="top:{dropGap * ROW}px"
					></div>
				{/if}
			</ul>
		{/if}
	</div>
</div>
