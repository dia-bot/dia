// The declarative Layout schema — the single source of truth shared by the
// browser editor (live DOM preview), the Go renderer (the real PNG), and, later,
// the community Layout Browser. A Layout is a canvas + an ordered list of layers;
// every text/image binds to {variables} so one layout works for any member.
//
// Keep this in sync with internal/layout/schema.go (same JSON shape).

export type LayerType = 'text' | 'image' | 'avatar' | 'rect' | 'ellipse' | 'path';
export type Align = 'left' | 'center' | 'right';

// How a node's two bezier handles relate while you drag one (Figma's three
// point types). 'corner' = independent (a sharp corner, or one with two freely
// angled handles); 'mirror' = same angle AND length (a perfectly smooth point);
// 'asym' = same angle, independent lengths. Missing/undefined means 'corner'.
// Purely an editor affordance — the renderer only ever reads the handle coords.
export type HandleMode = 'corner' | 'mirror' | 'asym';

// A bezier path node: an anchor plus its two cubic control handles (absolute
// canvas px). For a corner node the handles equal the anchor, so renderers can
// always emit a cubic segment without special-casing straight vs curved.
export interface PathNode {
	x: number;
	y: number;
	h1x: number; // in-handle (curve arriving at this node)
	h1y: number;
	h2x: number; // out-handle (curve leaving this node)
	h2y: number;
	m?: HandleMode; // handle relationship (default 'corner')
}
export type Fit = 'cover' | 'contain';
export type Shape = 'circle' | 'rounded';
export type Mask = '' | 'circle' | 'ellipse'; // image clip; '' = rounded-rect via radius

// The card renders server-side on every member join, so we cap layer count to
// keep that cheap — masking + vector shapes mean you rarely need many layers.
export const MAX_LAYERS = 12;

// Canvas size limits. The card renders server-side on every join, so the canvas
// can be any aspect ratio but its resolution is capped to keep memory/CPU
// bounded — total pixels (not just a side) is what drives the allocation, so a
// long thin banner is fine while a huge square is not. Mirrored in schema.go.
export const MIN_CANVAS = 64;
export const MAX_CANVAS_DIM = 4096; // hard cap on either side
export const MAX_CANVAS_PIXELS = 4_000_000; // ~2000×2000 budget

// clampCanvas constrains a width/height to the limits, scaling both down
// proportionally if the pixel budget is exceeded (so the aspect ratio is kept).
export function clampCanvas(w: number, h: number): { width: number; height: number } {
	let width = Math.max(MIN_CANVAS, Math.min(MAX_CANVAS_DIM, Math.round(w || MIN_CANVAS)));
	let height = Math.max(MIN_CANVAS, Math.min(MAX_CANVAS_DIM, Math.round(h || MIN_CANVAS)));
	const px = width * height;
	if (px > MAX_CANVAS_PIXELS) {
		const s = Math.sqrt(MAX_CANVAS_PIXELS / px);
		// floor (not round) to match Go's int() truncation and stay within budget
		width = Math.max(MIN_CANVAS, Math.floor(width * s));
		height = Math.max(MIN_CANVAS, Math.floor(height * s));
	}
	return { width, height };
}

export interface SizePreset {
	label: string;
	width: number;
	height: number;
}
// Common card sizes the picker offers; users can still type any custom W/H.
export const SIZE_PRESETS: SizePreset[] = [
	{ label: 'Welcome (1024×450)', width: 1024, height: 450 },
	{ label: 'Rank (934×282)', width: 934, height: 282 },
	{ label: 'Banner 16:9 (1280×720)', width: 1280, height: 720 },
	{ label: 'Wide 3:1 (1500×500)', width: 1500, height: 500 },
	{ label: 'Square (1080×1080)', width: 1080, height: 1080 },
	{ label: 'Story 9:16 (720×1280)', width: 720, height: 1280 }
];

// A "fat" layer: common geometry + type-specific fields (only the ones relevant
// to `type` are used). Pragmatic over a strict union — the editor switches on
// `type` to decide which controls and drawing to apply.
export interface Layer {
	id: string;
	type: LayerType;
	name: string;
	// geometry, in canvas pixels (0..width / 0..height)
	x: number;
	y: number;
	w: number;
	h: number;
	opacity: number; // 0..1
	rotation?: number; // degrees, about the layer centre
	hidden: boolean;
	group?: string; // soft-group id: layers sharing one select/move/delete together
	// text
	text?: string; // supports {variables}
	font_size?: number;
	font_weight?: number; // 400 | 700
	font_family?: string; // card-font family name ('' = default); see layout/fonts.ts
	color?: string; // hex
	align?: Align;
	// image / avatar
	src?: string; // url or {user.avatar}
	fit?: Fit;
	shape?: Shape;
	mask?: Mask; // image: clip to circle/ellipse (else rounded-rect via radius)
	ring_color?: string;
	ring_width?: number;
	// rect / ellipse / common
	fill?: string; // hex (rect/ellipse/path fill)
	radius?: number; // corner radius (rect / image / rounded avatar)
	stroke_color?: string; // outline colour (rect / ellipse / path)
	stroke_width?: number; // outline width in canvas px
	// path (pen / pencil)
	nodes?: PathNode[]; // absolute canvas-px anchors + handles
	closed?: boolean; // close + fill the path
}

export type BackgroundType = 'solid' | 'gradient' | 'image';
export interface Background {
	type: BackgroundType;
	color?: string;
	from?: string;
	to?: string;
	angle?: number;
	image_url?: string;
	blur?: number; // px
}

export interface Layout {
	width: number;
	height: number;
	background: Background;
	layers: Layer[];
}

let counter = 0;
function uid(): string {
	counter += 1;
	return `l${counter}_${counter * 7919}`;
}

// newLayer returns a sensible default layer of the given type, placed near the
// top-left of the canvas.
export function newLayer(type: LayerType): Layer {
	const base: Layer = {
		id: uid(),
		type,
		name: type[0].toUpperCase() + type.slice(1),
		x: 80,
		y: 80,
		w: 240,
		h: 100,
		opacity: 1,
		hidden: false
	};
	if (type === 'text') {
		return { ...base, name: 'Text', text: 'Welcome, {user}!', font_size: 48, font_weight: 700, color: '#FFFFFF', align: 'left', w: 480, h: 70 };
	}
	if (type === 'image') {
		return { ...base, name: 'Image', src: '', fit: 'cover', radius: 12, w: 200, h: 200 };
	}
	if (type === 'avatar') {
		return { ...base, name: 'Avatar', src: '{user.avatar}', shape: 'circle', ring_color: '#FFFFFF', ring_width: 6, radius: 24, w: 180, h: 180 };
	}
	if (type === 'ellipse') {
		return { ...base, name: 'Ellipse', fill: '#B244FC', radius: 0, opacity: 0.3, w: 240, h: 240 };
	}
	if (type === 'path') {
		// Paths are built by the pen/pencil tools (see EditorStore.createPath); this
		// is just a valid default for type-completeness.
		return { ...base, name: 'Path', nodes: [], closed: false, fill: '', stroke_color: '#FFFFFF', stroke_width: 4, w: 1, h: 1 };
	}
	// rect
	return { ...base, name: 'Shape', fill: '#000000', radius: 16, opacity: 0.35, w: 400, h: 160 };
}

// defaultLayout is the starter canvas (welcome-card sized).
export function defaultLayout(): Layout {
	return {
		width: 1024,
		height: 450,
		background: { type: 'gradient', from: '#FF6363', to: '#B244FC', angle: 45 },
		layers: [
			{ ...newLayer('avatar'), x: 422, y: 50, w: 180, h: 180 },
			{ ...newLayer('text'), id: uid(), name: 'Title', text: 'Welcome, {user}!', x: 162, y: 250, w: 700, h: 64, font_size: 52, align: 'center' },
			{ ...newLayer('text'), id: uid(), name: 'Subtitle', text: "You're member #{count}", x: 162, y: 322, w: 700, h: 40, font_size: 28, font_weight: 400, color: '#F1DFDF', align: 'center' }
		]
	};
}

// cornerNode makes a path node with its handles collapsed onto the anchor (a
// sharp corner). Curve nodes set h2 (and the mirrored h1) while dragging.
export function cornerNode(x: number, y: number): PathNode {
	return { x, y, h1x: x, h1y: y, h2x: x, h2y: y, m: 'corner' };
}

// hasHandles reports whether a node's handles are pulled off the anchor (i.e. it
// curves). A corner node's handles sit exactly on the anchor.
export function hasHandles(n: PathNode): boolean {
	return n.h1x !== n.x || n.h1y !== n.y || n.h2x !== n.x || n.h2y !== n.y;
}

// smoothHandles computes auto-tangent handles for a node from its neighbours
// (Catmull-Rom style: the tangent points along prev→next), so converting a
// corner to a smooth point "pops out" sensible curve handles — the core
// make-it-curly gesture. Endpoints fall back to the one neighbour they have.
export function smoothHandles(
	node: PathNode,
	prev: PathNode | null,
	next: PathNode | null
): { h1x: number; h1y: number; h2x: number; h2y: number } {
	const px = prev?.x ?? node.x;
	const py = prev?.y ?? node.y;
	const nx = next?.x ?? node.x;
	const ny = next?.y ?? node.y;
	let dx = nx - px;
	let dy = ny - py;
	const len = Math.hypot(dx, dy) || 1;
	dx /= len;
	dy /= len;
	const dPrev = prev ? Math.hypot(node.x - prev.x, node.y - prev.y) : len;
	const dNext = next ? Math.hypot(next.x - node.x, next.y - node.y) : len;
	const l1 = Math.max(12, dPrev / 3);
	const l2 = Math.max(12, dNext / 3);
	return {
		h1x: Math.round(node.x - dx * l1),
		h1y: Math.round(node.y - dy * l1),
		h2x: Math.round(node.x + dx * l2),
		h2y: Math.round(node.y + dy * l2)
	};
}

// pathD builds an SVG path `d` from a layer's nodes (always cubic segments, so a
// corner is just a cubic whose controls sit on the anchors). Mirrored by the Go
// renderer in internal/imaging/layout.go.
export function pathD(nodes: PathNode[] | undefined, closed = false): string {
	const ns = nodes ?? [];
	if (ns.length === 0) return '';
	if (ns.length === 1) return `M ${ns[0].x} ${ns[0].y}`;
	let d = `M ${ns[0].x} ${ns[0].y}`;
	for (let i = 1; i < ns.length; i++) {
		const a = ns[i - 1];
		const b = ns[i];
		d += ` C ${a.h2x} ${a.h2y} ${b.h1x} ${b.h1y} ${b.x} ${b.y}`;
	}
	if (closed && ns.length >= 3) {
		const a = ns[ns.length - 1];
		const b = ns[0];
		d += ` C ${a.h2x} ${a.h2y} ${b.h1x} ${b.h1y} ${b.x} ${b.y} Z`;
	}
	return d;
}

// Sample values used by the browser DOM preview (the Go renderer uses real data).
export const SAMPLE_VARS: Record<string, string> = {
	'{user}': 'Ada',
	'{user.mention}': '@Ada',
	'{user.name}': 'ada',
	'{username}': 'ada',
	'{user.id}': '123456789012345678',
	'{server}': 'Aurora SMP',
	'{count}': '1024',
	'{count.ordinal}': '1,024th',
	// rank-card tokens (so the studio canvas previews real-looking values)
	'{level}': '12',
	'{rank}': '1',
	'{xp}': '53,200',
	'{level.xp}': '450',
	'{level.needed}': '1,000',
	'{progress}': '45%'
};

export function resolveText(s: string | undefined, vars: Record<string, string> = SAMPLE_VARS): string {
	let out = s ?? '';
	for (const [k, v] of Object.entries(vars)) out = out.split(k).join(v);
	return out;
}

// resolveSrc maps {user.avatar} (and any future {tokens}) to a real URL for the
// DOM preview; a real avatar URL needs a user id we don't have here, so the
// preview shows a neutral placeholder for avatar bindings.
export function resolveSrc(src: string | undefined): string {
	const s = (src ?? '').trim();
	if (!s || s.startsWith('{')) return '';
	return s;
}
