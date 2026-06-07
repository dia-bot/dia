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
	MAX_LAYERS,
	type Layout,
	type Layer,
	type LayerType,
	type PathNode,
	type HandleMode,
	type ShapeKind,
	type ClipMode
} from './schema';

export const EDITOR_CTX = Symbol('dia-layout-editor');

// Active canvas tool. 'select' is the arrow; rect..avatar are drag-to-create
// shapes; 'pen' places bezier nodes and 'pencil' draws freehand.
export type Tool = 'select' | 'scale' | 'rect' | 'ellipse' | 'text' | 'image' | 'avatar' | 'pen' | 'pencil' | 'shape' | 'bend';

export type AlignEdge = 'left' | 'hcenter' | 'right' | 'top' | 'vcenter' | 'bottom';

export class EditorStore {
	layout = $state<Layout>(defaultLayout());
	selectedIds = $state<string[]>([]);
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
			this.selectedIds = this.layout.layers.filter((l) => l.group === layer.group).map((l) => l.id);
		} else {
			this.selectedIds = [id];
		}
	}
	selectMany(ids: string[]) {
		this.selectedIds = [...ids];
	}
	selectAll() {
		this.selectedIds = this.layout.layers.filter((l) => !l.hidden).map((l) => l.id);
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
	}

	removeSelected() {
		if (!this.selectedIds.length) return;
		const ids = new Set(this.selectedIds);
		this.layout.layers = this.layout.layers.filter((l) => !ids.has(l.id));
		this.selectedIds = [];
		if (this.editId && ids.has(this.editId)) this.exitEdit();
	}

	duplicateLayer(id: string) {
		if (this.atLimit) return;
		const i = this.layout.layers.findIndex((l) => l.id === id);
		if (i < 0) return;
		const copy = this.#clone(this.layout.layers[i], 24, 24);
		copy.name = `${this.layout.layers[i].name} copy`;
		this.layout.layers.splice(i + 1, 0, copy);
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

	// ── grouping (soft: a shared id; selecting one selects all, move/delete together) ─
	get canGroup(): boolean {
		return this.selectedIds.length >= 2;
	}
	get canUngroup(): boolean {
		return this.selectedLayers.some((l) => !!l.group);
	}
	group() {
		const sel = this.selectedLayers;
		if (sel.length < 2) return;
		const gid = `g${++this.#seq}`;
		for (const l of sel) l.group = gid;
	}
	ungroup() {
		for (const l of this.selectedLayers) delete l.group;
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

	// ── stacking order ────────────────────────────────────────────────────────────
	reorder(id: string, dir: -1 | 1) {
		const i = this.layout.layers.findIndex((l) => l.id === id);
		const j = i + dir;
		if (i < 0 || j < 0 || j >= this.layout.layers.length) return;
		const arr = this.layout.layers;
		[arr[i], arr[j]] = [arr[j], arr[i]];
	}
	bringToFront(id: string) {
		const arr = this.layout.layers;
		const i = arr.findIndex((l) => l.id === id);
		if (i < 0 || i === arr.length - 1) return;
		const [l] = arr.splice(i, 1);
		arr.push(l);
	}
	sendToBack(id: string) {
		const arr = this.layout.layers;
		const i = arr.findIndex((l) => l.id === id);
		if (i <= 0) return;
		const [l] = arr.splice(i, 1);
		arr.unshift(l);
	}
	rename(id: string, name: string) {
		this.patch(id, { name: name.trim() || 'Layer' });
	}
	toggleLock(id: string) {
		const l = this.layout.layers.find((x) => x.id === id);
		if (l) l.locked = !l.locked;
	}
	setOrder(frontToBackIds: string[]) {
		const byId = new Map(this.layout.layers.map((l) => [l.id, l]));
		const next = frontToBackIds.map((id) => byId.get(id)).filter((l): l is Layer => !!l);
		if (next.length !== this.layout.layers.length) return;
		next.reverse();
		this.layout.layers = next;
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
			this.selectedIds = [id];
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
	get isMask(): boolean {
		return !!this.selected?.clip;
	}
	// toggleMask flips the selected layer between a normal layer and a stencil that
	// clips the layers above it.
	toggleMask() {
		const l = this.selected;
		if (!l) return;
		l.clip = !l.clip;
		if (l.clip && !l.clip_mode) l.clip_mode = 'alpha';
	}
	setClipMode(mode: ClipMode) {
		const l = this.selected;
		if (l?.clip) l.clip_mode = mode;
	}
	// maskFor returns the mask layer that clips the given layer, or null. A mask
	// scopes to its group when it has one (a "mask group", created by useAsMask);
	// a groupless mask clips everything above it until the next mask.
	maskFor(layer: Layer): Layer | null {
		if (layer.clip) return null; // a mask isn't masked by itself / another mask
		const arr = this.layout.layers;
		const i = arr.findIndex((l) => l.id === layer.id);
		for (let k = i - 1; k >= 0; k--) {
			const c = arr[k];
			if (!c.clip) continue;
			if (!c.group) return c; // groupless mask → clips everything above it
			return c.group === layer.group ? c : null; // grouped mask → same group only
		}
		return null;
	}

	// useAsMask is Figma's "Use as mask": the bottom-most selected layer becomes a
	// stencil for the others. The selection is gathered into one contiguous mask
	// group so the mask clips exactly those layers (and nothing else).
	useAsMask() {
		const arr = this.layout.layers;
		const idxs = this.selectedIds
			.map((id) => arr.findIndex((l) => l.id === id))
			.filter((i) => i >= 0)
			.sort((a, b) => a - b);
		if (!idxs.length) return;
		const block = idxs.map((i) => arr[i]); // bottom → top
		const blockSet = new Set(block);
		const gid = `mask${++this.#seq}`;
		for (const l of block) l.group = gid;
		block[0].clip = true; // the lowest layer is the mask
		if (!block[0].clip_mode) block[0].clip_mode = 'alpha';
		// Reinsert the block contiguously at the bottom-most selected position.
		const anchor = idxs[0];
		const below = arr.slice(0, anchor).filter((l) => !blockSet.has(l)).length;
		const rest = arr.filter((l) => !blockSet.has(l));
		rest.splice(below, 0, ...block);
		this.layout.layers = rest;
		this.selectedIds = block.map((l) => l.id);
	}

	// releaseMask turns a stencil back into a normal layer and dissolves its group.
	releaseMask(id: string) {
		const l = this.layout.layers.find((x) => x.id === id);
		if (!l?.clip) return;
		const gid = l.group;
		l.clip = false;
		l.clip_invert = false;
		if (gid) for (const m of this.layout.layers) if (m.group === gid) delete m.group;
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
		// A restored snapshot is a different node array; drop into a clean view so
		// the editing overlay can't point at a stale node index.
		if (this.editId && !ids.has(this.editId)) this.exitEdit();
		else this.activeNode = null;
	}

	toJSON(): Layout {
		return $state.snapshot(this.layout);
	}
}
