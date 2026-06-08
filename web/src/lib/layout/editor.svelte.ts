// Editor state for the layout designer. One EditorStore holds the working
// Layout, the current (multi-)selection, undo history, clipboard, and all
// mutation actions. It's shared with the Canvas / LayersPanel / PropertiesPanel
// via Svelte context (EDITOR_CTX), so every panel reads and mutates the same
// reactive document.
import {
	defaultLayout,
	newLayer,
	newAvatarImage,
	cornerNode,
	smoothHandles,
	hasHandles,
	shapeInBox,
	newEffect,
	MAX_LAYERS,
	type Layout,
	type Layer,
	type LayerType,
	type PathNode,
	type HandleMode,
	type ShapeKind,
	type ClipMode,
	type BoolOp,
	type Effect,
	type EffectType
} from './schema';

export const EDITOR_CTX = Symbol('dia-layout-editor');

// Active canvas tool. 'select' is the arrow; rect..avatar are drag-to-create
// shapes; 'pen' places bezier nodes and 'pencil' draws freehand.
export type Tool = 'select' | 'scale' | 'rect' | 'ellipse' | 'text' | 'image' | 'avatar' | 'pen' | 'pencil' | 'shape' | 'bend';

export type AlignEdge = 'left' | 'hcenter' | 'right' | 'top' | 'vcenter' | 'bottom';

// A row in the layers panel's derived tree (one level of nesting). A 'group' row
// is a group / mask-group header; a 'leaf' row is a single layer at depth 0
// (loose) or depth 1 (inside a group). Built by EditorStore.tree from the flat
// layer list's contiguous group runs (front-most first, for display).
export type LayerRow =
	| {
			kind: 'group';
			id: string; // the group id
			depth: 0;
			isMask: boolean; // a mask group (bottom member is a stencil)
			isBoolean: boolean; // a boolean group (group meta carries a bool op)
			collapsed: boolean;
			name: string;
			childIds: string[]; // members, bottom → top
			hidden: boolean; // every member hidden
			locked: boolean; // every member locked
	  }
	| {
			kind: 'leaf';
			id: string; // the layer id
			layer: Layer;
			depth: 0 | 1; // 1 = inside a group
			group: string | null;
			isStencil: boolean; // this leaf is its group's mask stencil
			masked: boolean; // a masked sibling sitting above the stencil
	  };

export class EditorStore {
	layout = $state<Layout>(defaultLayout());
	selectedIds = $state<string[]>([]);
	// Editor-only collapse state for group containers in the layers panel, keyed by
	// group id. Deliberately NOT persisted (pure view state); group ids are stable
	// so a collapse survives edits, and a saved layout doesn't carry UI state.
	collapsed = $state<Record<string, boolean>>({});
	tool = $state<Tool>('select');
	// When tool === 'shape', this is the shape the canvas draws on drag.
	shapeKind = $state<ShapeKind>('triangle');
	// The guild this editor belongs to — set by the host so the inspector can
	// upload images to the right guild-scoped object-store path.
	guildId = $state('');
	// Guild custom (premium) fonts + entitlement, loaded by the host. The font
	// picker appends these and the preview loads them via the FontFace API.
	customFonts = $state<{ family: string; url: string }[]>([]);
	premium = $state(false);
	// Server-resolved card template strings (original → rendered), so the live
	// canvas shows {{.User.Username}}/{{.User.Avatar}} exactly like the bot output.
	resolved = $state<Record<string, string>>({});
	setResolved(map: Record<string, string>) {
		this.resolved = map;
	}

	setFonts(fonts: { family: string; url: string }[], premium: boolean) {
		this.customFonts = fonts;
		this.premium = premium;
	}
	addFont(f: { family: string; url: string }) {
		this.customFonts = [...this.customFonts.filter((x) => x.family !== f.family), f];
	}
	removeFont(family: string) {
		this.customFonts = this.customFonts.filter((x) => x.family !== family);
	}

	// Deep-edit state (Figma's "enter" mode): editId is the layer whose internals
	// are being edited — a path (drag anchors/handles, convert point types) or a
	// text layer (inline typing). activeNode is the focused path node, if any.
	// Lifted here (out of the Canvas) so the inspector can drive node operations.
	editId = $state<string | null>(null);
	activeNode = $state<number | null>(null);

	// Undo/redo: serialized layout snapshots. The editor calls record() (debounced)
	// so a burst of edits (a drag, a run of keystrokes) collapses into one step.
	past = $state<string[]>([]);
	future = $state<string[]>([]);
	#committed = '';
	#restoring = false;

	#clipboard: Layer[] = [];
	#seq = 0;

	constructor(initial?: Layout) {
		if (initial) this.layout = initial;
		this.#committed = JSON.stringify(this.layout);
	}

	setTool(t: Tool) {
		this.tool = t;
	}
	// setShape picks a shape and arms the 'shape' draw tool (drag on the canvas to
	// draw it, like the rect/ellipse tools).
	setShape(kind: ShapeKind) {
		this.shapeKind = kind;
		this.tool = 'shape';
	}

	#uid(): string {
		return `l${++this.#seq}_${this.layout.layers.length}`;
	}

	// ── selection ──────────────────────────────────────────────────────────────
	get selectedId(): string | null {
		return this.selectedIds.length ? this.selectedIds[this.selectedIds.length - 1] : null;
	}
	get selected(): Layer | null {
		const id = this.selectedId;
		return id ? (this.layout.layers.find((l) => l.id === id) ?? null) : null;
	}
	get selectedLayers(): Layer[] {
		return this.layout.layers.filter((l) => this.selectedIds.includes(l.id));
	}
	isSelected(id: string): boolean {
		return this.selectedIds.includes(id);
	}

	// select replaces the selection (or toggles when additive). Selecting one
	// member of a group selects the whole group.
	select(id: string | null, additive = false) {
		if (id === null) {
			this.selectedIds = [];
			return;
		}
		if (additive) {
			this.selectedIds = this.selectedIds.includes(id)
				? this.selectedIds.filter((x) => x !== id)
				: [...this.selectedIds, id];
			return;
		}
		const layer = this.layout.layers.find((l) => l.id === id);
		if (layer?.group) {
			// Select the group, but never pull a locked member into a movable selection.
			const ids = this.layout.layers
				.filter((l) => l.group === layer.group && !l.locked)
				.map((l) => l.id);
			this.selectedIds = ids.length ? ids : [id];
		} else {
			this.selectedIds = [id];
		}
	}
	selectMany(ids: string[]) {
		this.selectedIds = [...ids];
	}
	// selectOne selects exactly this layer, bypassing the group auto-expand in
	// select() — the panel's child-row click path (pick one member of a group).
	selectOne(id: string) {
		this.selectedIds = [id];
	}
	// selectGroup selects every (unlocked) member of a group as a unit — the panel's
	// container-header click path.
	selectGroup(gid: string) {
		const ids = this.layout.layers.filter((l) => l.group === gid && !l.locked).map((l) => l.id);
		if (ids.length) this.selectedIds = ids;
	}
	selectAll() {
		this.selectedIds = this.layout.layers.filter((l) => !l.hidden && !l.locked).map((l) => l.id);
	}

	// ── create / add / remove / duplicate ───────────────────────────────────────
	get atLimit(): boolean {
		return this.layout.layers.length >= MAX_LAYERS;
	}

	createLayer(type: LayerType, x: number, y: number): Layer | null {
		if (this.atLimit) return null;
		const layer = newLayer(type);
		layer.x = Math.round(x);
		layer.y = Math.round(y);
		layer.w = 1;
		layer.h = 1;
		this.layout.layers.push(layer);
		this.selectedIds = [layer.id];
		return layer;
	}

	createPath(x: number, y: number): Layer | null {
		if (this.atLimit) return null;
		const layer = newLayer('path');
		layer.nodes = [cornerNode(Math.round(x), Math.round(y))];
		layer.x = Math.round(x);
		layer.y = Math.round(y);
		layer.w = 1;
		layer.h = 1;
		this.layout.layers.push(layer);
		this.selectedIds = [layer.id];
		return layer;
	}

	addLayer(type: LayerType): Layer | null {
		if (this.atLimit) return null;
		const layer = newLayer(type);
		this.layout.layers.push(layer);
		this.selectedIds = [layer.id];
		return layer;
	}

	// addAvatar inserts a circular member-avatar image ({{.User.Avatar}}) — the
	// avatar is just an image preset, no separate layer type.
	addAvatar(): Layer | null {
		if (this.atLimit) return null;
		const layer = newAvatarImage();
		this.layout.layers.push(layer);
		this.selectedIds = [layer.id];
		return layer;
	}

	// convertToPath turns a rect/ellipse into an editable vector path — Figma flips
	// a primitive to vectors when you edit its points. Rect → 4 corners; ellipse →
	// 4 bezier nodes (kappa ≈ 0.5523). Other types are left unchanged.
	convertToPath(id: string) {
		const l = this.layout.layers.find((x) => x.id === id);
		if (!l || (l.type !== 'rect' && l.type !== 'ellipse')) return;
		const { x, y, w, h } = l;
		let nodes: PathNode[];
		if (l.type === 'ellipse') {
			const k = 0.5523;
			const rx = w / 2;
			const ry = h / 2;
			const cx = x + rx;
			const cy = y + ry;
			const node = (px: number, py: number, h1x: number, h1y: number, h2x: number, h2y: number): PathNode => ({
				x: Math.round(px),
				y: Math.round(py),
				h1x: Math.round(h1x),
				h1y: Math.round(h1y),
				h2x: Math.round(h2x),
				h2y: Math.round(h2y),
				m: 'mirror'
			});
			nodes = [
				node(cx + rx, cy, cx + rx, cy - ry * k, cx + rx, cy + ry * k), // right
				node(cx, cy + ry, cx + rx * k, cy + ry, cx - rx * k, cy + ry), // bottom
				node(cx - rx, cy, cx - rx, cy + ry * k, cx - rx, cy - ry * k), // left
				node(cx, cy - ry, cx - rx * k, cy - ry, cx + rx * k, cy - ry) // top
			];
		} else {
			nodes = [cornerNode(x, y), cornerNode(x + w, y), cornerNode(x + w, y + h), cornerNode(x, y + h)];
		}
		l.type = 'path';
		l.nodes = nodes;
		l.closed = true;
		if (!l.fill) l.fill = '#FFFFFF';
		this.fitPath(l);
	}

	// createShape starts a drawable shape (path) at (x,y); the canvas grows it via
	// setShapeBox while dragging — so shapes draw out like the rect/ellipse tools.
	createShape(kind: ShapeKind, x: number, y: number): Layer | null {
		if (this.atLimit) return null;
		const layer = newLayer('path');
		layer.name = kind[0].toUpperCase() + kind.slice(1);
		const { nodes, closed } = shapeInBox(kind, Math.round(x), Math.round(y), 1, 1);
		layer.nodes = nodes;
		layer.closed = closed;
		if (closed) {
			layer.fill = '#FFFFFF';
			layer.stroke_color = '#FFFFFF';
			layer.stroke_width = 0;
		} else {
			layer.fill = '';
			layer.stroke_color = '#FFFFFF';
			layer.stroke_width = 6;
		}
		this.layout.layers.push(layer);
		this.fitPath(layer);
		this.selectedIds = [layer.id];
		return layer;
	}
	// setShapeBox refits a shape's nodes to a bounding box (used while dragging it
	// out, and for the default size on a click).
	setShapeBox(id: string, kind: ShapeKind, x: number, y: number, w: number, h: number) {
		const l = this.layout.layers.find((ly) => ly.id === id);
		if (!l) return;
		l.nodes = shapeInBox(kind, x, y, w, h).nodes;
		this.fitPath(l);
	}
	// scaleLayer is the Scale tool (Figma's K): like resize, but it ALSO scales the
	// layer's intrinsic properties (font size, stroke width, corner radius, ring) by
	// the uniform factor f, so the whole object grows proportionally. props holds the
	// values captured at gesture start.
	scaleLayer(
		id: string,
		props: { fontSize?: number; stroke?: number; radius?: number; ring?: number },
		startNodes: PathNode[] | undefined,
		sx: number,
		sy: number,
		sw: number,
		sh: number,
		nx: number,
		ny: number,
		nw: number,
		nh: number,
		f: number
	) {
		const l = this.layout.layers.find((ly) => ly.id === id);
		if (!l) return;
		if (startNodes) {
			this.scalePath(id, startNodes, sx, sy, sw, sh, nx, ny, nw, nh);
		} else {
			l.x = nx;
			l.y = ny;
			l.w = nw;
			l.h = nh;
		}
		if (props.fontSize != null) l.font_size = Math.max(1, Math.round(props.fontSize * f));
		if (props.stroke != null) l.stroke_width = Math.max(0, Math.round(props.stroke * f * 10) / 10);
		if (props.radius != null) l.radius = Math.max(0, Math.round(props.radius * f));
		if (props.ring != null) l.ring_width = Math.max(0, Math.round(props.ring * f));
	}

	// scalePath maps a path's nodes (captured at gesture start) from the start bbox
	// to a new one — powers the resize handles on shape/path layers.
	scalePath(id: string, start: PathNode[], sx: number, sy: number, sw: number, sh: number, nx: number, ny: number, nw: number, nh: number) {
		const l = this.layout.layers.find((ly) => ly.id === id);
		if (!l) return;
		const fx = sw === 0 ? 1 : nw / sw;
		const fy = sh === 0 ? 1 : nh / sh;
		const mapX = (v: number) => Math.round(nx + (v - sx) * fx);
		const mapY = (v: number) => Math.round(ny + (v - sy) * fy);
		l.nodes = start.map((n) => ({
			x: mapX(n.x),
			y: mapY(n.y),
			h1x: mapX(n.h1x),
			h1y: mapY(n.h1y),
			h2x: mapX(n.h2x),
			h2y: mapY(n.h2y),
			m: n.m
		}));
		this.fitPath(l);
	}

	removeLayer(id: string) {
		this.layout.layers = this.layout.layers.filter((l) => l.id !== id);
		this.selectedIds = this.selectedIds.filter((x) => x !== id);
		if (this.editId === id) this.exitEdit();
		this.#pruneGroupMeta();
	}

	removeSelected() {
		if (!this.selectedIds.length) return;
		const ids = new Set(this.selectedIds);
		this.layout.layers = this.layout.layers.filter((l) => !ids.has(l.id));
		this.selectedIds = [];
		if (this.editId && ids.has(this.editId)) this.exitEdit();
		this.#pruneGroupMeta();
	}

	duplicateLayer(id: string) {
		if (this.atLimit) return;
		const arr = this.layout.layers;
		const i = arr.findIndex((l) => l.id === id);
		if (i < 0) return;
		const src = arr[i];
		const copy = this.#clone(src, 24, 24); // #clone strips the group → ungrouped copy
		copy.name = `${src.name} copy`;
		// Insert after the run end if the source is grouped, so the ungrouped copy
		// never lands inside a group's contiguous span (invariant C).
		const at = src.group ? this.#groupSpan(src.group)[1] : i + 1;
		arr.splice(at, 0, copy);
		this.selectedIds = [copy.id];
	}

	// #clone deep-copies a layer with a fresh id, offset by (dx,dy) including any
	// path nodes/handles. Pasted/duplicated copies are ungrouped.
	#clone(src: Layer, dx: number, dy: number): Layer {
		const copy = JSON.parse(JSON.stringify($state.snapshot(src))) as Layer;
		copy.id = this.#uid();
		copy.x = Math.round(src.x + dx);
		copy.y = Math.round(src.y + dy);
		delete copy.group;
		if (copy.nodes) {
			for (const n of copy.nodes) {
				n.x += dx;
				n.y += dy;
				n.h1x += dx;
				n.h1y += dy;
				n.h2x += dx;
				n.h2y += dy;
			}
		}
		return copy;
	}

	// ── clipboard ────────────────────────────────────────────────────────────────
	copy() {
		const sel = this.selectedLayers;
		if (sel.length) this.#clipboard = sel.map((l) => $state.snapshot(l) as Layer);
	}
	cut() {
		this.copy();
		this.removeSelected();
	}
	paste() {
		if (!this.#clipboard.length) return;
		const ids: string[] = [];
		for (const src of this.#clipboard) {
			if (this.atLimit) break;
			const copy = this.#clone(src, 24, 24);
			this.layout.layers.push(copy);
			ids.push(copy.id);
		}
		if (ids.length) this.selectedIds = ids;
	}

	// ── grouping (soft: a shared id; members are kept CONTIGUOUS in `layers` so the
	// panel renders them as one tree container and the mask loop stays correct) ────
	get canGroup(): boolean {
		return this.selectedIds.length >= 2;
	}
	get canUngroup(): boolean {
		return this.selectedLayers.some((l) => !!l.group);
	}
	// group gathers the selection into ONE contiguous run (preserving relative
	// order) under a fresh group id, and registers a default name — so a group is
	// always a clean block (the contiguity fix the tree + mask loop depend on).
	group() {
		const arr = this.layout.layers;
		const idxs = this.selectedIds
			.map((id) => arr.findIndex((l) => l.id === id))
			.filter((i) => i >= 0)
			.sort((a, b) => a - b);
		if (idxs.length < 2) return;
		const block = idxs.map((i) => arr[i]); // bottom → top
		const blockSet = new Set(block);
		const gid = `g${++this.#seq}`;
		for (const l of block) l.group = gid;
		const anchor = idxs[0];
		const below = arr.slice(0, anchor).filter((l) => !blockSet.has(l)).length;
		const rest = arr.filter((l) => !blockSet.has(l));
		rest.splice(below, 0, ...block);
		this.layout.layers = rest;
		this.layout.groups = { ...(this.layout.groups ?? {}), [gid]: { name: 'Group' } };
		this.selectedIds = block.map((l) => l.id);
	}
	// ungroup dissolves the group of every selected layer (un-clipping any stencil
	// so a released mask doesn't dangle), then prunes now-empty group metadata.
	ungroup() {
		for (const l of this.selectedLayers) {
			delete l.group;
			if (l.clip) {
				l.clip = false;
				delete l.clip_mode;
				l.clip_invert = false;
			}
		}
		this.#pruneGroupMeta();
	}

	// #groupSpan returns the [lo, hi) flat index range of a group's contiguous run.
	#groupSpan(gid: string): [number, number] {
		const arr = this.layout.layers;
		const lo = arr.findIndex((l) => l.group === gid);
		if (lo < 0) return [-1, -1];
		let hi = lo;
		while (hi < arr.length && arr[hi].group === gid) hi++;
		return [lo, hi];
	}
	// isMaskGroup: a group whose bottom-most (lowest-index) member is a stencil.
	isMaskGroup(gid: string): boolean {
		const bottom = this.layout.layers.find((l) => l.group === gid);
		return !!bottom?.clip;
	}
	// isBoolGroup: a group whose metadata carries a boolean operation.
	isBoolGroup(gid: string): boolean {
		return !!this.layout.groups?.[gid]?.bool_op;
	}
	groupName(gid: string): string {
		const stored = this.layout.groups?.[gid]?.name;
		if (stored) return stored;
		const op = this.layout.groups?.[gid]?.bool_op;
		if (op) return op[0].toUpperCase() + op.slice(1); // dynamic label tracks the op
		return this.isMaskGroup(gid) ? 'Mask group' : 'Group';
	}
	renameGroup(gid: string, name: string) {
		this.layout.groups = {
			...(this.layout.groups ?? {}),
			[gid]: { ...this.layout.groups?.[gid], name: name.trim() || 'Group' }
		};
	}
	toggleCollapse(gid: string) {
		this.collapsed = { ...this.collapsed, [gid]: !this.collapsed[gid] };
	}
	isCollapsed(gid: string): boolean {
		return !!this.collapsed[gid];
	}
	// #pruneGroupMeta dissolves any group left with fewer than 2 members (un-clipping
	// a lone stencil) and drops names/collapse for groups that no longer exist — so
	// the panel tree never shows a stray one-item container. Run after structural
	// mutations (delete, ungroup, move).
	#pruneGroupMeta() {
		const counts = new Map<string, number>();
		for (const l of this.layout.layers) if (l.group) counts.set(l.group, (counts.get(l.group) ?? 0) + 1);
		for (const l of this.layout.layers) {
			if (l.group && (counts.get(l.group) ?? 0) < 2) {
				delete l.group;
				if (l.clip) {
					l.clip = false;
					delete l.clip_mode;
					l.clip_invert = false;
				}
			}
		}
		const live = new Set(this.layout.layers.map((l) => l.group).filter((g): g is string => !!g));
		if (this.layout.groups) {
			for (const gid of Object.keys(this.layout.groups)) if (!live.has(gid)) delete this.layout.groups[gid];
		}
		for (const gid of Object.keys(this.collapsed)) if (!live.has(gid)) delete this.collapsed[gid];
	}

	// tree derives the layers-panel render list (display order, front-most first)
	// from the flat layer list: each contiguous group run becomes a container row
	// with its members nested one level under it; a mask group's stencil is shown
	// last (the run's bottom). Relies on invariant C (groups are contiguous).
	get tree(): LayerRow[] {
		const arr = this.layout.layers;
		const rows: LayerRow[] = [];
		let i = arr.length - 1; // walk top (front) → bottom (back)
		while (i >= 0) {
			const g = arr[i].group;
			if (!g) {
				rows.push({ kind: 'leaf', id: arr[i].id, layer: arr[i], depth: 0, group: null, isStencil: false, masked: false });
				i--;
				continue;
			}
			let lo = i;
			while (lo >= 0 && arr[lo].group === g) lo--;
			lo++; // lo..i is the whole run (invariant C)
			const stencil = arr[lo].clip ? arr[lo] : null;
			const isMask = !!stencil;
			const run = arr.slice(lo, i + 1);
			rows.push({
				kind: 'group',
				id: g,
				depth: 0,
				isMask,
				isBoolean: this.isBoolGroup(g),
				collapsed: this.isCollapsed(g),
				name: this.groupName(g),
				childIds: run.map((l) => l.id),
				hidden: run.every((l) => l.hidden),
				locked: run.every((l) => l.locked)
			});
			if (!this.isCollapsed(g)) {
				for (let k = i; k >= lo; k--) {
					const l = arr[k];
					rows.push({ kind: 'leaf', id: l.id, layer: l, depth: 1, group: g, isStencil: l === stencil, masked: isMask && l !== stencil });
				}
			}
			i = lo - 1;
		}
		return rows;
	}

	// ── geometry helpers ─────────────────────────────────────────────────────────
	patch(id: string, p: Partial<Layer>) {
		const layer = this.layout.layers.find((l) => l.id === id);
		if (layer) Object.assign(layer, p);
	}
	move(id: string, x: number, y: number) {
		this.patch(id, { x: Math.round(x), y: Math.round(y) });
	}
	resize(id: string, w: number, h: number) {
		this.patch(id, { w: Math.max(8, Math.round(w)), h: Math.max(8, Math.round(h)) });
	}

	// shift translates a layer (and its path nodes) by a delta, keeping the bbox
	// consistent — used by align/distribute and multi-drag.
	shift(layer: Layer, dx: number, dy: number) {
		layer.x = Math.round(layer.x + dx);
		layer.y = Math.round(layer.y + dy);
		if (layer.nodes) {
			for (const n of layer.nodes) {
				n.x += dx;
				n.y += dy;
				n.h1x += dx;
				n.h1y += dy;
				n.h2x += dx;
				n.h2y += dy;
			}
		}
	}

	// ── align / distribute (operate on the current multi-selection) ───────────────
	align(edge: AlignEdge) {
		const sel = this.selectedLayers;
		if (sel.length < 2) return;
		const minX = Math.min(...sel.map((l) => l.x));
		const maxX = Math.max(...sel.map((l) => l.x + l.w));
		const minY = Math.min(...sel.map((l) => l.y));
		const maxY = Math.max(...sel.map((l) => l.y + l.h));
		for (const l of sel) {
			let dx = 0;
			let dy = 0;
			switch (edge) {
				case 'left':
					dx = minX - l.x;
					break;
				case 'right':
					dx = maxX - (l.x + l.w);
					break;
				case 'hcenter':
					dx = (minX + maxX) / 2 - (l.x + l.w / 2);
					break;
				case 'top':
					dy = minY - l.y;
					break;
				case 'bottom':
					dy = maxY - (l.y + l.h);
					break;
				case 'vcenter':
					dy = (minY + maxY) / 2 - (l.y + l.h / 2);
					break;
			}
			if (dx || dy) this.shift(l, dx, dy);
		}
	}
	distribute(axis: 'h' | 'v') {
		const sel = [...this.selectedLayers];
		if (sel.length < 3) return;
		if (axis === 'h') {
			sel.sort((a, b) => a.x - b.x);
			const min = sel[0].x;
			const span = sel[sel.length - 1].x - min;
			const gap = span / (sel.length - 1);
			sel.forEach((l, i) => this.shift(l, Math.round(min + gap * i) - l.x, 0));
		} else {
			sel.sort((a, b) => a.y - b.y);
			const min = sel[0].y;
			const span = sel[sel.length - 1].y - min;
			const gap = span / (sel.length - 1);
			sel.forEach((l, i) => this.shift(l, 0, Math.round(min + gap * i) - l.y));
		}
	}

	// ── stacking order (group-aware: a grouped layer moves as its whole run so
	// groups stay contiguous; a loose layer hops over a neighbouring group's run) ──
	// reorder nudges a layer (or its whole group) one step toward the front (dir 1)
	// or back (-1), swapping with the adjacent unit (a loose layer or a whole group).
	reorder(id: string, dir: -1 | 1) {
		const arr = this.layout.layers;
		const i = arr.findIndex((l) => l.id === id);
		if (i < 0) return;
		const [lo, hi] = arr[i].group ? this.#groupSpan(arr[i].group!) : [i, i + 1];
		if (dir === 1) {
			if (hi >= arr.length) return;
			const nb = arr[hi];
			const [nlo, nhi] = nb.group ? this.#groupSpan(nb.group) : [hi, hi + 1];
			this.layout.layers = [...arr.slice(0, lo), ...arr.slice(nlo, nhi), ...arr.slice(lo, hi), ...arr.slice(nhi)];
		} else {
			if (lo <= 0) return;
			const pb = arr[lo - 1];
			const [plo, phi] = pb.group ? this.#groupSpan(pb.group) : [lo - 1, lo];
			this.layout.layers = [...arr.slice(0, plo), ...arr.slice(lo, hi), ...arr.slice(plo, phi), ...arr.slice(phi)];
		}
	}
	bringToFront(id: string) {
		const arr = this.layout.layers;
		const l = arr.find((x) => x.id === id);
		if (!l) return;
		const i = arr.indexOf(l);
		const [lo, hi] = l.group ? this.#groupSpan(l.group) : [i, i + 1];
		if (hi >= arr.length) return;
		this.layout.layers = [...arr.slice(0, lo), ...arr.slice(hi), ...arr.slice(lo, hi)];
	}
	sendToBack(id: string) {
		const arr = this.layout.layers;
		const l = arr.find((x) => x.id === id);
		if (!l) return;
		const i = arr.indexOf(l);
		const [lo, hi] = l.group ? this.#groupSpan(l.group) : [i, i + 1];
		if (lo <= 0) return;
		this.layout.layers = [...arr.slice(lo, hi), ...arr.slice(0, lo), ...arr.slice(hi)];
	}
	rename(id: string, name: string) {
		this.patch(id, { name: name.trim() || 'Layer' });
	}
	toggleLock(id: string) {
		const l = this.layout.layers.find((x) => x.id === id);
		if (l) l.locked = !l.locked;
	}
	// moveLayer relocates ONE layer to a new flat index (bottom→top order),
	// optionally re-parenting it into a group (intoGroup) or out (null). It upholds
	// invariant C: a layer joining a group is clamped inside that group's run (above
	// its stencil), a stencil dragged within its group stays pinned to the bottom,
	// and a loose layer never lands inside another group's run. Drives panel drag.
	moveLayer(id: string, flatIndex: number, intoGroup: string | null) {
		const arr = [...this.layout.layers];
		const from = arr.findIndex((l) => l.id === id);
		if (from < 0) return;
		const [l] = arr.splice(from, 1);
		let idx = from < flatIndex ? flatIndex - 1 : flatIndex;
		idx = Math.max(0, Math.min(arr.length, idx));
		const joiningNew = !!intoGroup && intoGroup !== l.group;
		if (intoGroup) {
			l.group = intoGroup;
			if (joiningNew && l.clip) {
				l.clip = false;
				delete l.clip_mode;
				l.clip_invert = false;
			}
		} else if (l.group) {
			delete l.group;
			if (l.clip) {
				l.clip = false;
				delete l.clip_mode;
				l.clip_invert = false;
			}
		}
		if (intoGroup) {
			const lo = arr.findIndex((x) => x.group === intoGroup);
			if (lo >= 0) {
				let hi = lo;
				while (hi < arr.length && arr[hi].group === intoGroup) hi++;
				if (l.clip) idx = lo; // a stencil stays at its run's bottom
				else idx = Math.max(arr[lo]?.clip ? lo + 1 : lo, Math.min(hi, idx));
			}
		} else if (idx > 0 && idx < arr.length && arr[idx - 1].group && arr[idx - 1].group === arr[idx].group) {
			// don't split a group: snap past the front edge of the run we'd land in
			const g = arr[idx - 1].group!;
			let hi = idx;
			while (hi < arr.length && arr[hi].group === g) hi++;
			idx = hi;
		}
		arr.splice(idx, 0, l);
		this.layout.layers = arr;
		this.#pruneGroupMeta();
		this.selectedIds = [id];
	}
	// moveGroup relocates a whole group's run to a new flat boundary, never landing
	// inside another group's run.
	moveGroup(gid: string, flatIndex: number) {
		const arr = [...this.layout.layers];
		const [lo, hi] = this.#groupSpan(gid);
		if (lo < 0) return;
		const block = arr.slice(lo, hi);
		const rest = [...arr.slice(0, lo), ...arr.slice(hi)];
		let idx = lo < flatIndex ? flatIndex - block.length : flatIndex;
		idx = Math.max(0, Math.min(rest.length, idx));
		if (idx > 0 && idx < rest.length && rest[idx - 1].group && rest[idx - 1].group === rest[idx].group) {
			const g = rest[idx - 1].group!;
			let h2 = idx;
			while (h2 < rest.length && rest[h2].group === g) h2++;
			idx = h2;
		}
		rest.splice(idx, 0, ...block);
		this.layout.layers = rest;
	}

	// ── path edit mode + node operations ────────────────────────────────────────
	get editLayer(): Layer | null {
		return this.editId ? (this.layout.layers.find((l) => l.id === this.editId) ?? null) : null;
	}
	get editPath(): Layer | null {
		const l = this.editLayer;
		return l?.type === 'path' ? l : null;
	}
	get activePathNode(): PathNode | null {
		const l = this.editPath;
		if (!l?.nodes || this.activeNode === null) return null;
		return l.nodes[this.activeNode] ?? null;
	}

	// enterEdit opens deep-edit on a path or text layer. A rect/ellipse is flipped
	// to an editable vector path first (Figma's "edit object" on a primitive).
	enterEdit(id: string) {
		const l = this.layout.layers.find((x) => x.id === id);
		if (!l) return;
		if (l.type === 'rect' || l.type === 'ellipse') this.convertToPath(id);
		if (l.type === 'path' || l.type === 'text') {
			// Single-select (not the whole group) so selectedId === editId and the
			// canvas's "selection changed → exit edit" guard doesn't immediately trip
			// for a grouped layer.
			this.selectOne(id);
			this.editId = id;
			this.activeNode = null;
		} else {
			this.select(id);
		}
	}
	exitEdit() {
		this.editId = null;
		this.activeNode = null;
	}
	setActiveNode(i: number | null) {
		this.activeNode = i;
	}

	// fitPath snaps a path's bbox (x/y/w/h) to its nodes + handles so selection,
	// rotation pivot, and move behave like any other layer.
	fitPath(l: Layer) {
		const ns = l.nodes ?? [];
		if (!ns.length) return;
		let minX = Infinity,
			minY = Infinity,
			maxX = -Infinity,
			maxY = -Infinity;
		for (const n of ns) {
			for (const pt of [
				[n.x, n.y],
				[n.h1x, n.h1y],
				[n.h2x, n.h2y]
			]) {
				minX = Math.min(minX, pt[0]);
				minY = Math.min(minY, pt[1]);
				maxX = Math.max(maxX, pt[0]);
				maxY = Math.max(maxY, pt[1]);
			}
		}
		l.x = Math.round(minX);
		l.y = Math.round(minY);
		l.w = Math.max(1, Math.round(maxX - minX));
		l.h = Math.max(1, Math.round(maxY - minY));
	}

	// neighbours returns the prev/next nodes of index i, wrapping when the path is
	// closed (so the first/last point gets smooth tangents from across the seam).
	#neighbours(l: Layer, i: number): { prev: PathNode | null; next: PathNode | null } {
		const ns = l.nodes ?? [];
		const prev = ns[i - 1] ?? (l.closed ? ns[ns.length - 1] : null) ?? null;
		const next = ns[i + 1] ?? (l.closed ? ns[0] : null) ?? null;
		return { prev: prev === ns[i] ? null : prev, next: next === ns[i] ? null : next };
	}

	// setNodeType converts a node between the three Figma point types. Converting a
	// (collapsed) corner to smooth/asym auto-pops tangent handles from neighbours —
	// the "make it curly" gesture. Converting to corner collapses handles flat.
	setNodeType(idx: number, mode: HandleMode) {
		const l = this.editPath;
		const n = l?.nodes?.[idx];
		if (!l || !n) return;
		if (mode === 'corner') {
			n.h1x = n.x;
			n.h1y = n.y;
			n.h2x = n.x;
			n.h2y = n.y;
		} else {
			if (!hasHandles(n)) {
				const { prev, next } = this.#neighbours(l, idx);
				const h = smoothHandles(n, prev, next);
				n.h1x = h.h1x;
				n.h1y = h.h1y;
				n.h2x = h.h2x;
				n.h2y = h.h2y;
			} else if (mode === 'mirror') {
				// Mirror from whichever handle is real so we never wipe the node's only
				// handle (e.g. a node with just an in-handle).
				const h2real = n.h2x !== n.x || n.h2y !== n.y;
				if (h2real) {
					n.h1x = Math.round(2 * n.x - n.h2x);
					n.h1y = Math.round(2 * n.y - n.h2y);
				} else {
					n.h2x = Math.round(2 * n.x - n.h1x);
					n.h2y = Math.round(2 * n.y - n.h1y);
				}
			}
		}
		n.m = mode;
		this.activeNode = idx;
		this.fitPath(l);
	}

	// toggleNodeType is the double-click action: a corner becomes smooth, a smooth/
	// asym point becomes a sharp corner. Keys off the stored type (falling back to
	// geometry) so it stays consistent even if a tangent came out degenerate.
	toggleNodeType(idx: number) {
		const n = this.editPath?.nodes?.[idx];
		if (!n) return;
		const isSmooth = n.m ? n.m !== 'corner' : hasHandles(n);
		this.setNodeType(idx, isSmooth ? 'corner' : 'mirror');
	}

	// setActiveNodeX/Y move the focused node (and its handles) to an absolute
	// coordinate — used by the inspector's numeric X/Y fields.
	setActiveNodeX(x: number) {
		const l = this.editPath;
		const n = this.activePathNode;
		if (!l || !n) return;
		const dx = Math.round(x) - n.x;
		n.x += dx;
		n.h1x += dx;
		n.h2x += dx;
		this.fitPath(l);
	}
	setActiveNodeY(y: number) {
		const l = this.editPath;
		const n = this.activePathNode;
		if (!l || !n) return;
		const dy = Math.round(y) - n.y;
		n.y += dy;
		n.h1y += dy;
		n.h2y += dy;
		this.fitPath(l);
	}

	deleteNodeAt(idx: number) {
		const l = this.editPath;
		if (!l?.nodes || l.nodes.length <= 2) return;
		l.nodes.splice(idx, 1);
		this.activeNode = null;
		this.fitPath(l);
	}
	deleteActiveNode() {
		if (this.activeNode !== null) this.deleteNodeAt(this.activeNode);
	}

	// deleteHandle collapses one bezier handle onto its anchor — "delete the bend
	// you clicked" — keeping the point (and the shape) intact.
	deleteHandle(idx: number, kind: 'h1' | 'h2') {
		const l = this.editPath;
		const n = l?.nodes?.[idx];
		if (!l || !n) return;
		if (kind === 'h1') {
			n.h1x = n.x;
			n.h1y = n.y;
		} else {
			n.h2x = n.x;
			n.h2y = n.y;
		}
		n.m = hasHandles(n) ? 'asym' : 'corner';
		this.fitPath(l);
	}

	// reversePath flips drawing direction, swapping each node's in/out handles so
	// the curve is geometrically identical, just wound the other way.
	reversePath() {
		const l = this.editPath ?? this.selected;
		if (l?.type !== 'path' || !l.nodes) return;
		l.nodes = l.nodes
			.slice()
			.reverse()
			.map((n) => ({ ...n, h1x: n.h2x, h1y: n.h2y, h2x: n.h1x, h2y: n.h1y }));
		this.activeNode = null;
		this.fitPath(l);
	}

	// setClosed opens/closes a path. Closing gives it a sensible default fill so a
	// freshly closed shape is actually filled (the old default '' rendered nothing).
	setClosed(v: boolean) {
		const l = this.editPath ?? this.selected;
		if (l?.type !== 'path') return;
		l.closed = v;
		if (v && !l.fill) l.fill = l.stroke_color || '#FFFFFF';
	}
	// setFillEnabled toggles whether a path paints a fill ('' = no fill).
	setFillEnabled(on: boolean) {
		const l = this.selected;
		if (l?.type !== 'path') return;
		l.fill = on ? l.fill || l.stroke_color || '#FFFFFF' : '';
	}
	get fillEnabled(): boolean {
		const l = this.selected;
		return l?.type === 'path' ? !!l.fill : false;
	}

	// ── masking ("use as mask") ──────────────────────────────────────────────────
	// isMask: the current selection is (or belongs to) a mask group — drives the
	// inspector's mask section + the Use-as-mask / Release-mask branch.
	get isMask(): boolean {
		const l = this.selected;
		if (!l) return false;
		if (l.clip) return true;
		return l.group ? this.isMaskGroup(l.group) : false;
	}
	// maskStencil: the actual stencil layer (the group's bottom member) of the
	// current selection's mask group, so the inspector can edit clip_mode / invert /
	// release even when the whole group is selected (selectedId is a top member).
	get maskStencil(): Layer | null {
		const l = this.selected;
		if (!l) return null;
		if (l.clip) return l;
		if (l.group && this.isMaskGroup(l.group)) {
			return this.layout.layers.find((x) => x.group === l.group && x.clip) ?? null;
		}
		return null;
	}
	// toggleMask routes to the two real mask operations — build a mask group from
	// the selection (Use as mask) or release the current one. Masks are always a
	// group (≥2 layers), so there's no raw single-layer clip flip anymore.
	toggleMask() {
		const stencil = this.maskStencil;
		if (stencil) this.releaseMask(stencil.id);
		else this.useAsMask();
	}
	get clipMode(): ClipMode {
		return (this.maskStencil?.clip_mode as ClipMode) ?? 'alpha';
	}
	setClipMode(mode: ClipMode) {
		const s = this.maskStencil;
		if (s) s.clip_mode = mode;
	}
	get clipInvert(): boolean {
		return !!this.maskStencil?.clip_invert;
	}
	setClipInvert(v: boolean) {
		const s = this.maskStencil;
		if (s) s.clip_invert = v;
	}
	// maskFor returns the stencil that clips the given layer, or null. Masks are
	// strictly group-scoped: a layer is clipped only by its OWN group's stencil
	// (the group's bottom member) and only when it sits above that stencil.
	maskFor(layer: Layer): Layer | null {
		if (layer.clip || !layer.group) return null;
		const arr = this.layout.layers;
		const [lo] = this.#groupSpan(layer.group);
		if (lo < 0 || !arr[lo].clip) return null;
		return arr.findIndex((l) => l.id === layer.id) > lo ? arr[lo] : null;
	}

	// useAsMask is Figma's "Use as mask": the bottom-most selected layer becomes a
	// stencil for the layers above it. The selection is gathered into one contiguous
	// mask group so the mask clips exactly those layers. With a single selection,
	// the contiguous layers above it are pulled into the group (so it's never a no-op).
	useAsMask() {
		if (this.isMask) return; // selection already forms a mask group → no-op
		const arr = this.layout.layers;
		let idxs = this.selectedIds
			.map((id) => arr.findIndex((l) => l.id === id))
			.filter((i) => i >= 0)
			.sort((a, b) => a - b);
		if (!idxs.length) return;
		// Single layer chosen as mask → gather the contiguous run above it (until the
		// next existing mask) so there's actually something to clip.
		if (idxs.length === 1) {
			const start = idxs[0];
			const extra: number[] = [];
			for (let k = start + 1; k < arr.length && !arr[k].clip; k++) extra.push(k);
			if (!extra.length) return; // nothing above to mask → no-op (guarded in UI too)
			idxs = [start, ...extra];
		}
		const block = idxs.map((i) => arr[i]); // bottom → top
		const blockSet = new Set(block);
		const gid = `mask${++this.#seq}`;
		for (const l of block) l.group = gid;
		block[0].clip = true; // the lowest layer is the mask
		if (!block[0].clip_mode) block[0].clip_mode = 'vector'; // hard-edged crop by default
		// Reinsert the block contiguously at the bottom-most selected position.
		const anchor = idxs[0];
		const below = arr.slice(0, anchor).filter((l) => !blockSet.has(l)).length;
		const rest = arr.filter((l) => !blockSet.has(l));
		rest.splice(below, 0, ...block);
		this.layout.layers = rest;
		this.layout.groups = { ...(this.layout.groups ?? {}), [gid]: { name: 'Mask group' } };
		this.#pruneGroupMeta(); // drop any group names orphaned by the re-grouping
		this.selectedIds = block.map((l) => l.id);
	}
	// canMask: "Use as mask" is meaningful only when there's a layer above the
	// prospective stencil within its run (else it would clip nothing).
	get canMask(): boolean {
		if (this.isMask) return false; // already a mask group → the menu shows Release
		const arr = this.layout.layers;
		const idxs = this.selectedIds
			.map((id) => arr.findIndex((l) => l.id === id))
			.filter((i) => i >= 0);
		if (!idxs.length) return false;
		if (idxs.length > 1) return true;
		const start = idxs[0];
		return start + 1 < arr.length && !arr[start + 1].clip;
	}

	// releaseMask un-masks the stencil but KEEPS the layers grouped (Figma's "remove
	// mask"): the run stays a plain group. Ungroup is a separate action.
	releaseMask(id: string) {
		const l = this.layout.layers.find((x) => x.id === id);
		if (!l?.clip) return;
		l.clip = false;
		l.clip_invert = false;
		// Reset the auto "Mask group" name so the now-plain group reads as "Group"
		// (a user-chosen name is left untouched).
		if (l.group && this.layout.groups?.[l.group]?.name === 'Mask group') {
			this.layout.groups = { ...this.layout.groups, [l.group]: { name: 'Group' } };
		}
	}

	// ── boolean ops (union / subtract / intersect / exclude) ─────────────────────
	// A boolean group is a normal contiguous group whose metadata carries a bool op;
	// the renderer composites its (vector) members' silhouettes with that op. It's
	// mutually exclusive with a mask group, and applies to vector members only.
	get isBoolean(): boolean {
		const l = this.selected;
		return !!(l?.group && this.isBoolGroup(l.group));
	}
	get boolOp(): BoolOp {
		const l = this.selected;
		return (l?.group && (this.layout.groups?.[l.group]?.bool_op as BoolOp)) || 'union';
	}
	// canBoolean: ≥2 selected layers, not already a mask. Any layer type works — a
	// shape source fills the boolean region with its colour, an image source keeps its
	// pixels clipped to the region (so e.g. intersect crops a photo to a shape).
	get canBoolean(): boolean {
		if (this.isMask) return false;
		return this.selectedLayers.length >= 2;
	}
	// applyBoolean gathers the selection into one contiguous group tagged with the op
	// (clearing any mask state — mutual exclusivity). On an existing boolean group it
	// just switches the op in place.
	applyBoolean(op: BoolOp) {
		const sel = this.selectedLayers;
		if (!sel.length) return;
		// If the selection already forms ONE existing group, convert it in place —
		// preserving the group's id + (user) name and just setting the op. Covers both
		// switching the op on a boolean group and promoting a named plain group, and
		// clears any mask state on the members (boolean ⇄ mask are mutually exclusive).
		const g0 = sel[0].group;
		if (g0 && sel.every((l) => l.group === g0)) {
			for (const l of sel) {
				if (l.clip) {
					l.clip = false;
					delete l.clip_mode;
					l.clip_invert = false;
				}
			}
			const meta = { ...this.layout.groups?.[g0], bool_op: op };
			if (meta.name === 'Mask group') delete meta.name; // the auto mask name shouldn't stick on a bool group
			this.layout.groups = { ...(this.layout.groups ?? {}), [g0]: meta };
			return;
		}
		const arr = this.layout.layers;
		const idxs = this.selectedIds
			.map((id) => arr.findIndex((l) => l.id === id))
			.filter((i) => i >= 0)
			.sort((a, b) => a - b);
		if (idxs.length < 2) return;
		const block = idxs.map((i) => arr[i]); // bottom → top
		const blockSet = new Set(block);
		const gid = `bool${++this.#seq}`;
		for (const l of block) {
			l.group = gid;
			if (l.clip) {
				l.clip = false;
				delete l.clip_mode;
				l.clip_invert = false;
			}
		}
		const anchor = idxs[0];
		const below = arr.slice(0, anchor).filter((l) => !blockSet.has(l)).length;
		const rest = arr.filter((l) => !blockSet.has(l));
		rest.splice(below, 0, ...block);
		this.layout.layers = rest;
		this.layout.groups = { ...(this.layout.groups ?? {}), [gid]: { bool_op: op } };
		this.#pruneGroupMeta(); // drop any group names orphaned by the re-grouping
		this.selectedIds = block.map((l) => l.id);
	}
	// clearBoolean removes the op but KEEPS the group (a plain group again).
	clearBoolean(gid?: string) {
		const id = gid ?? this.selected?.group;
		if (!id || !this.layout.groups?.[id]) return;
		const next = { ...this.layout.groups[id] };
		delete next.bool_op;
		this.layout.groups = { ...this.layout.groups, [id]: next };
	}

	// ── effects (shadows / blur) ─────────────────────────────────────────────────
	// Effects live on a single layer; the inspector edits the primary selection's
	// list. The renderer applies them in a fixed order, so list order is just for
	// display (newest on top of the panel list).
	get effects(): Effect[] {
		return this.selected?.effects ?? [];
	}
	addEffect(type: EffectType) {
		const l = this.selected;
		if (!l) return;
		// Figma allows only one layer-blur and one background-blur per layer.
		const solo = type === 'layer_blur' || type === 'background_blur';
		const base = (l.effects ?? []).filter((e) => !(solo && e.type === type));
		l.effects = [...base, newEffect(type)];
	}
	updateEffect(i: number, patch: Partial<Effect>) {
		const e = this.selected?.effects?.[i];
		if (e) Object.assign(e, patch);
	}
	removeEffect(i: number) {
		const l = this.selected;
		if (!l?.effects) return;
		l.effects = l.effects.filter((_, k) => k !== i);
		if (!l.effects.length) delete l.effects;
	}
	toggleEffectHidden(i: number) {
		const e = this.selected?.effects?.[i];
		if (e) e.hidden = !e.hidden;
	}

	// ── "edit object" (Figma's Enter) ────────────────────────────────────────────
	// canEditObject: deep-edit is meaningful for a single vector/text/primitive — a
	// path/text edits inline; a rect/ellipse flips to an editable vector first.
	get canEditObject(): boolean {
		const l = this.selected;
		return (
			this.selectedIds.length === 1 &&
			!!l &&
			(l.type === 'path' || l.type === 'text' || l.type === 'rect' || l.type === 'ellipse')
		);
	}
	editSelected() {
		const l = this.selected;
		if (l && this.selectedIds.length === 1) this.enterEdit(l.id);
	}

	// ── multi-selection editing (Figma: one inspector, "Mixed" when values differ) ──
	// common returns the value shared by EVERY selected layer, or undefined when they
	// differ (the inspector shows "Mixed") or nothing is selected. Primitives only.
	common<T>(read: (l: Layer) => T): T | undefined {
		const sel = this.selectedLayers;
		if (!sel.length) return undefined;
		const v = read(sel[0]);
		return sel.every((l) => read(l) === v) ? v : undefined;
	}
	// setAll applies a mutation to every selected layer (an edit applies to all).
	setAll(apply: (l: Layer) => void) {
		for (const l of this.selectedLayers) apply(l);
	}
	// selectionType is the layer type shared by the whole selection, or null when
	// mixed — gates the type-specific sections (Text/Image/Fill/…).
	get selectionType(): LayerType | null {
		const sel = this.selectedLayers;
		if (!sel.length) return null;
		const t = sel[0].type;
		return sel.every((l) => l.type === t) ? t : null;
	}
	// selectMatching selects every (visible, unlocked) layer of the primary layer's
	// type — Figma's "Select all with same …".
	selectMatching() {
		const t = this.selected?.type;
		if (!t) return;
		const ids = this.layout.layers
			.filter((l) => l.type === t && !l.hidden && !l.locked)
			.map((l) => l.id);
		if (ids.length) this.selectedIds = ids;
	}

	// ── flatten (Figma's flatten-to-vector for a primitive) ──────────────────────
	get canFlatten(): boolean {
		return this.selectedLayers.some((l) => l.type === 'rect' || l.type === 'ellipse');
	}
	flatten() {
		for (const l of [...this.selectedLayers]) {
			if (l.type === 'rect' || l.type === 'ellipse') this.convertToPath(l.id);
		}
	}

	// ── independent corner radii (Figma's "expand" on the corner-radius field) ────
	// cornersActive: the primary selection is in per-corner mode.
	get cornersActive(): boolean {
		const c = this.selected?.corners;
		return Array.isArray(c) && c.length === 4;
	}
	// expandCorners seeds the four corners from each layer's uniform radius.
	expandCorners() {
		this.setAll((l) => {
			if (!Array.isArray(l.corners) || l.corners.length !== 4) {
				const r = Math.max(0, Math.round(l.radius ?? 0));
				l.corners = [r, r, r, r];
			}
		});
	}
	// collapseCorners drops back to a single uniform radius (top-left wins).
	collapseCorners() {
		this.setAll((l) => {
			if (Array.isArray(l.corners) && l.corners.length === 4) {
				l.radius = l.corners[0];
				delete l.corners;
			}
		});
	}
	// setRadius edits the uniform corner radius across the selection, dropping any
	// per-corner array so the uniform value actually wins (this field is only shown in
	// uniform mode, and editing it should unify mixed siblings back to uniform).
	setRadius(v: number) {
		this.setAll((l) => {
			l.radius = Math.max(0, Math.round(v));
			if (Array.isArray(l.corners)) delete l.corners;
		});
	}
	// setCorner edits one corner (0=tl,1=tr,2=br,3=bl) across the selection, seeding
	// per-corner mode from the uniform radius if needed.
	setCorner(i: number, v: number) {
		this.setAll((l) => {
			if (!Array.isArray(l.corners) || l.corners.length !== 4) {
				const r = Math.max(0, Math.round(l.radius ?? 0));
				l.corners = [r, r, r, r];
			}
			l.corners[i] = Math.max(0, Math.round(v));
		});
	}

	// ── undo / redo ────────────────────────────────────────────────────────────────
	get canUndo(): boolean {
		return this.past.length > 0;
	}
	get canRedo(): boolean {
		return this.future.length > 0;
	}
	// record commits the current document as a new history checkpoint if it
	// changed since the last one. Called debounced by the editor chrome.
	record() {
		if (this.#restoring) return;
		const cur = JSON.stringify(this.layout);
		if (cur === this.#committed) return;
		this.past = [...this.past.slice(-99), this.#committed];
		this.future = [];
		this.#committed = cur;
	}
	undo() {
		if (!this.past.length) return;
		this.#restoring = true;
		this.future = [this.#committed, ...this.future.slice(0, 99)];
		const prev = this.past[this.past.length - 1];
		this.past = this.past.slice(0, -1);
		this.layout = JSON.parse(prev);
		this.#committed = prev;
		this.#pruneSelection();
		this.#restoring = false;
	}
	redo() {
		if (!this.future.length) return;
		this.#restoring = true;
		this.past = [...this.past.slice(-99), this.#committed];
		const next = this.future[0];
		this.future = this.future.slice(1);
		this.layout = JSON.parse(next);
		this.#committed = next;
		this.#pruneSelection();
		this.#restoring = false;
	}
	#pruneSelection() {
		const ids = new Set(this.layout.layers.map((l) => l.id));
		this.selectedIds = this.selectedIds.filter((id) => ids.has(id));
		// Drop collapse state for any group that vanished in the restored snapshot
		// (group names live in layout.groups, so they restore with the snapshot).
		const groups = new Set(this.layout.layers.map((l) => l.group).filter((g): g is string => !!g));
		for (const gid of Object.keys(this.collapsed)) if (!groups.has(gid)) delete this.collapsed[gid];
		// A restored snapshot is a different node array; drop into a clean view so
		// the editing overlay can't point at a stale node index.
		if (this.editId && !ids.has(this.editId)) this.exitEdit();
		else this.activeNode = null;
	}

	toJSON(): Layout {
		return $state.snapshot(this.layout);
	}
}
