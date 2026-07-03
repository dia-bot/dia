// The declarative Layout schema — the single source of truth shared by the
// browser editor (live DOM preview), the Go renderer (the real PNG), and, later,
// the community Layout Browser. A Layout is a canvas + an ordered list of layers;
// every text/image binds to {variables} so one layout works for any member.
//
// Keep this in sync with internal/layout/schema.go (same JSON shape).

export type LayerType = 'text' | 'image' | 'rect' | 'ellipse' | 'path';
export type Align = 'left' | 'center' | 'right';
// Vertical alignment of text within its layer box (Figma's text vertical-align).
export type VAlign = 'top' | 'middle' | 'bottom';
// Case transform applied at render (Figma's "Type details" case control).
export type TextCase = 'none' | 'upper' | 'lower' | 'title';
// Underline / strikethrough (Figma's text decoration).
export type TextDecoration = 'none' | 'underline' | 'strike';

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
// Where a stroke sits relative to the shape's edge (Figma's stroke Position): 'center'
// straddles the edge, 'inside'/'outside' shift it fully in/out. Unset = 'center'.
export type StrokeAlign = 'inside' | 'center' | 'outside';
// Figma's advanced stroke controls. style 'dashed' draws dash/gap-length dashes; cap is
// the end shape of open paths + dashes ('butt' = Figma's "None"); join is how corners meet.
export type StrokeStyle = 'solid' | 'dashed';
export type StrokeCap = 'butt' | 'round' | 'square';
export type StrokeJoin = 'miter' | 'bevel' | 'round';
export type StrokeSide = 'top' | 'right' | 'bottom' | 'left';
// Figma's Stroke-settings "Basic" extras. Width profile tapers a path's weight along
// its length (uniform = constant); a profile only affects open/closed vector paths.
export type WidthProfile = 'uniform' | 'taper_start' | 'taper_end' | 'taper' | 'lens';
// Arrowhead / decoration drawn at a path endpoint (Figma's Start point / End point).
// 'none' = a plain cap; the rest paint a marker at the path's first/last node.
export type ArrowCap = 'none' | 'line' | 'arrow' | 'triangle' | 'circle' | 'diamond';
// Figma's "Brush" tab — a named brush from the catalog (see brushes.ts). Each brush is a
// Stretch (one shape stretched along the line) or Scatter (a mark repeated along it) kind.
// Brushes are center-only and replace the plain stroke rendering.
export type BrushDirection = 'forward' | 'backward';
// How a mask layer clips the layers above it (Figma's three mask types):
//   'alpha'     — the stencil's opacity sets the reveal (soft edges from a PNG/gradient)
//   'vector'    — any covered pixel is fully revealed (hard-edged shape crop; the default for cards)
//   'luminance' — the stencil's brightness × opacity sets the reveal (white reveals, black hides)
export type ClipMode = 'alpha' | 'vector' | 'luminance';

// Boolean shape operations (Figma's Union/Subtract/Intersect/Exclude). Set on a
// group's metadata to combine its (vector) members into one composited silhouette.
// Non-destructive: the member shapes stay editable; the op can change anytime.
export type BoolOp = 'union' | 'subtract' | 'intersect' | 'exclude';

// Layer effects — Figma's "Effects" panel. Each layer carries an ordered list of
// effects; the renderer applies them per layer in a fixed order regardless of list
// position: background blur (frost what's behind) → layer blur (soften the layer) →
// drop shadows (painted under) → the layer's own content → inner shadows (over).
//   'drop_shadow'      — blurred, offset, tinted copy of the layer's silhouette behind it
//   'inner_shadow'     — the same, painted inside the silhouette (edge shading)
//   'layer_blur'       — gaussian-blur the layer's own pixels
//   'background_blur'  — gaussian-blur whatever sits behind the (translucent) layer
// Mirrored in schema.go (same JSON shape).
export type EffectType = 'drop_shadow' | 'inner_shadow' | 'layer_blur' | 'background_blur';
export interface Effect {
	type: EffectType;
	x?: number; // shadow offset, canvas px (shadows only)
	y?: number;
	radius?: number; // blur radius, canvas px — shadow softness OR blur strength
	spread?: number; // shadow grow(+)/shrink(−), canvas px (shadows only)
	color?: string; // shadow colour hex (shadows only)
	opacity?: number; // shadow colour alpha 0..1 (shadows only; default 0.25)
	hidden?: boolean; // skip this effect without removing it (default visible)
}

// The card renders server-side on every member join, so we cap layer count to
// keep that cheap — masking + vector shapes mean you rarely need many layers.
// Mirrored (and enforced) server-side in schema.go.
export const MAX_LAYERS = 48;

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
// ── Fill paints — Figma's paint stack ─────────────────────────────────────────
// A layer's fill is a list of paints composited bottom→top, each independently
// hideable with its own opacity: solid colour, four gradient types or an image.
export type PaintType = 'solid' | 'linear' | 'radial' | 'angular' | 'diamond' | 'image';
export type PaintFit = 'cover' | 'contain' | 'tile';
export interface GradientStop {
	pos: number; // 0..1 along the gradient line
	color: string; // hex
	alpha?: number; // 0..1 (default 1)
}
export interface Paint {
	type: PaintType;
	hidden?: boolean;
	opacity?: number; // 0..1 (default 1)
	color?: string; // solid
	stops?: GradientStop[]; // gradients (2+)
	angle?: number; // linear: CSS angle in degrees (0 = up, clockwise); angular: rotation
	src?: string; // image fill URL (or {{template}})
	fit?: PaintFit; // image: cover (Figma Fill) | contain (Fit) | tile (default cover)
}

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
	// soft-group id: layers sharing it select/move/delete together AND scope a mask
	// group (the renderer reads it). Members of one group MUST be contiguous in
	// `layers`; a mask group's stencil (clip=true) sits at the run's bottom.
	group?: string;
	locked?: boolean; // editor-only: can't be selected/moved on the canvas
	// text
	text?: string; // supports {variables}
	font_size?: number;
	font_weight?: number; // 400 | 700
	font_family?: string; // card-font family name ('' = default); see layout/fonts.ts
	color?: string; // hex
	align?: Align; // horizontal alignment
	// typography (Figma's Type panel) — all renderable in both the DOM preview and Go:
	line_height?: number; // line-height multiplier (default 1.3)
	letter_spacing?: number; // tracking in canvas px (default 0; may be negative)
	valign?: VAlign; // vertical alignment within the layer box (default 'top')
	text_case?: TextCase; // case transform (default 'none')
	text_decoration?: TextDecoration; // underline / strikethrough (default 'none')
	// image
	src?: string; // url or {user.avatar}
	fit?: Fit;
	// rect / ellipse / common
	fill?: string; // LEGACY single hex fill; superseded by `fills` when set
	fills?: Paint[]; // Figma-style paint stack, BOTTOM → TOP (index 0 paints first)
	radius?: number; // corner radius (rect / image / rounded avatar)
	corners?: [number, number, number, number]; // independent corner radii [tl,tr,br,bl]; overrides `radius` when set (rect/image)
	// rect-only: bind the rect's painted width to the member's XP progress. When
	// true the renderer fills the rect to {{.Progress}} percent (an XP bar);
	// welcome cards (no progress value) render it full width. Mirrored in schema.go.
	progress?: boolean;
	stroke_color?: string; // LEGACY single outline hex; superseded by `strokes` when set
	strokes?: Paint[]; // Figma-style stroke paint stack, BOTTOM → TOP (like `fills`)
	stroke_width?: number; // outline width in canvas px
	stroke_align?: StrokeAlign; // stroke Position (inside/center/outside); default 'center'
	stroke_style?: StrokeStyle; // 'dashed' draws dash/gap dashes (default 'solid')
	dash?: number; // dash length, canvas px (dashed)
	gap?: number; // gap length, canvas px (dashed)
	stroke_cap?: StrokeCap; // end/dash cap, butt='None' (default 'round')
	stroke_join?: StrokeJoin; // corner join (default 'round')
	stroke_sides?: StrokeSide[]; // rect per-side strokes; unset OR all 4 = full outline
	// advanced stroke (Figma's Stroke-settings popover; mostly path-only) ────────────
	width_profile?: WidthProfile; // taper a path's weight along its length (default 'uniform')
	start_cap?: ArrowCap; // arrowhead at the path's first node (open paths; default 'none')
	end_cap?: ArrowCap; // arrowhead at the path's last node (open paths; default 'none')
	miter_angle?: number; // miter join cutoff in degrees (default ~28.96, Figma's default)
	brush_name?: string; // selected brush id from the catalog (brushes.ts); unset = no brush
	brush_direction?: BrushDirection; // stretch nib direction (default 'forward')
	scatter_gap?: number; // scatter: stamp spacing as a multiple of stroke weight (unset = brush preset)
	scatter_wiggle?: number; // scatter: perpendicular position jitter %, 0..100
	scatter_size?: number; // scatter: mark size jitter %, 0..100
	scatter_rotation?: number; // scatter: base mark rotation, degrees (-180..180)
	scatter_angular?: number; // scatter: random per-mark rotation jitter, degrees (0..180)
	dynamic_frequency?: number; // hand-drawn wobble density, 0..100 (paths; default 0 = off)
	dynamic_wiggle?: number; // hand-drawn wobble amplitude %, 0..200 (paths; default 0)
	dynamic_smoothen?: number; // smooth the wobble, 0..100 (paths; default 0)
	// path (pen / pencil)
	nodes?: PathNode[]; // absolute canvas-px anchors + handles
	closed?: boolean; // close + fill the path
	// masking (Figma "use as mask"): when clip is set, this layer is a stencil that
	// clips the layers ABOVE it (until the next mask). clip_mode picks how:
	//   'alpha'     — show masked content where the mask is opaque (shape + alpha)
	//   'luminance' — masked content alpha follows the mask's brightness
	clip?: boolean;
	clip_mode?: ClipMode;
	clip_invert?: boolean; // invert the mask (hide inside the shape / show outside)
	// effects (shadows / blur) — see Effect above. Applied in a fixed order, not list order.
	effects?: Effect[];
}

export type BackgroundType = 'solid' | 'gradient' | 'image';
export interface Background {
	type: BackgroundType; // LEGACY type switch; superseded by `fills` when set
	color?: string;
	from?: string;
	to?: string;
	angle?: number;
	image_url?: string;
	blur?: number; // px — blurs the whole composited background
	// Figma-style paint stack for the canvas background — the SAME model as a
	// layer's fill. Once set (even empty = no background) it supersedes the
	// legacy type/color/from/to/image_url fields; the editor migrates a legacy
	// background into `fills` on first edit.
	fills?: Paint[];
}

// Metadata for a soft group, keyed by the group id used on layers. Membership and
// z-order are NOT here — they live in the flat `layers` list (a contiguous run of
// layers sharing a group id = one group). Mask-ness is derived (the run's bottom
// layer has clip=true). `name` round-trips for display; `bool_op`, when set, makes
// the group a boolean group that the renderer DOES read (it composites the run's
// member shapes with that operation).
export interface LayoutGroup {
	name?: string;
	bool_op?: BoolOp; // present ⇒ a boolean group (mutually exclusive with a mask group)
}

export interface Layout {
	width: number;
	height: number;
	background: Background;
	layers: Layer[];
	groups?: Record<string, LayoutGroup>; // editor-only; keyed by Layer.group id
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
		return { ...base, name: 'Text', text: 'Welcome, {{.User.Name}}!', font_size: 48, font_weight: 700, color: '#FFFFFF', align: 'left', w: 480, h: 70 };
	}
	if (type === 'image') {
		return { ...base, name: 'Image', src: '', fit: 'cover', radius: 12, w: 200, h: 200 };
	}
	if (type === 'ellipse') {
		return { ...base, name: 'Ellipse', fill: '#FFFFFF', radius: 0, opacity: 1, w: 240, h: 240 };
	}
	if (type === 'path') {
		// Paths are built by the pen/pencil tools (see EditorStore.createPath); this
		// is just a valid default for type-completeness.
		return { ...base, name: 'Path', nodes: [], closed: false, fill: '', stroke_color: '#FFFFFF', stroke_width: 1, w: 1, h: 1 };
	}
	// rect — solid, fully visible, sharp corners, no border (Figma's default shape)
	return { ...base, name: 'Rectangle', fill: '#FFFFFF', radius: 0, opacity: 1, w: 400, h: 160 };
}

// newEffect returns a Figma-style default effect of the given type. Shadows default
// to a soft black X0 Y4 blur4 at 25% (Figma's "add shadow" default); blurs to 8px.
export function newEffect(type: EffectType): Effect {
	if (type === 'layer_blur' || type === 'background_blur') {
		return { type, radius: 8 };
	}
	return { type, x: 0, y: 4, radius: 4, spread: 0, color: '#000000', opacity: 0.25 };
}

// EFFECT_LABELS — display names for the inspector's effect rows / add menu.
export const EFFECT_LABELS: Record<EffectType, string> = {
	drop_shadow: 'Drop shadow',
	inner_shadow: 'Inner shadow',
	layer_blur: 'Layer blur',
	background_blur: 'Background blur'
};

// newAvatarImage returns an image layer pre-configured as a circular member avatar.
// It is JUST an image bound to {{.User.Avatar}}: rounded into a circle with a fully-rounded
// corner radius (clamped to min(w,h)/2 by both renderers) and bordered with an outside
// stroke. There are no special avatar/mask/ring fields — shape it with the corner radius
// and border it with the stroke, exactly like any other image.
export function newAvatarImage(): Layer {
	return {
		...newLayer('image'),
		name: 'Avatar',
		src: '{{.User.Avatar}}',
		fit: 'cover',
		radius: 9999, // fully rounded → circle (clamped to min/2 by both renderers)
		stroke_color: '#FFFFFF',
		stroke_width: 6,
		stroke_align: 'outside', // a border around the avatar, not eating into the image
		w: 180,
		h: 180
	};
}

// defaultLayout is the starter canvas (welcome-card sized).
export function defaultLayout(): Layout {
	return {
		width: 1024,
		height: 450,
		background: { type: 'gradient', from: '#FF6363', to: '#B244FC', angle: 45 },
		layers: [
			{ ...newAvatarImage(), x: 422, y: 50 },
			{ ...newLayer('text'), id: uid(), name: 'Title', text: 'Welcome, {{.User.Name}}!', x: 162, y: 250, w: 700, h: 64, font_size: 52, align: 'center' },
			{ ...newLayer('text'), id: uid(), name: 'Subtitle', text: "You're member #{{.Count}}", x: 162, y: 322, w: 700, h: 40, font_size: 28, font_weight: 400, color: '#F1DFDF', align: 'center' }
		]
	};
}

// cornerNode makes a path node with its handles collapsed onto the anchor (a
// sharp corner). Curve nodes set h2 (and the mirrored h1) while dragging.
export function cornerNode(x: number, y: number): PathNode {
	return { x, y, h1x: x, h1y: y, h2x: x, h2y: y, m: 'corner' };
}

// Parametric shapes the editor can insert — all built from corner path nodes, so
// they render in the DOM + Go renderer with no new layer type and stay fully
// editable with the path tools.
export type ShapeKind = 'triangle' | 'diamond' | 'pentagon' | 'hexagon' | 'star' | 'line';

function regularPolygon(cx: number, cy: number, r: number, n: number, rot = -Math.PI / 2): PathNode[] {
	const out: PathNode[] = [];
	for (let i = 0; i < n; i++) {
		const a = rot + (i * 2 * Math.PI) / n;
		out.push(cornerNode(Math.round(cx + r * Math.cos(a)), Math.round(cy + r * Math.sin(a))));
	}
	return out;
}

// shapePath returns the nodes (+ whether closed) for a parametric shape centred
// at (cx,cy) with radius r.
export function shapePath(kind: ShapeKind, cx: number, cy: number, r: number): { nodes: PathNode[]; closed: boolean } {
	switch (kind) {
		case 'triangle':
			return { nodes: regularPolygon(cx, cy, r, 3), closed: true };
		case 'diamond':
			return { nodes: regularPolygon(cx, cy, r, 4), closed: true };
		case 'pentagon':
			return { nodes: regularPolygon(cx, cy, r, 5), closed: true };
		case 'hexagon':
			return { nodes: regularPolygon(cx, cy, r, 6), closed: true };
		case 'star': {
			const out: PathNode[] = [];
			const inner = r * 0.45;
			for (let i = 0; i < 10; i++) {
				const rad = i % 2 === 0 ? r : inner;
				const a = -Math.PI / 2 + (i * Math.PI) / 5;
				out.push(cornerNode(Math.round(cx + rad * Math.cos(a)), Math.round(cy + rad * Math.sin(a))));
			}
			return { nodes: out, closed: true };
		}
		case 'line':
			return { nodes: [cornerNode(cx - r, cy), cornerNode(cx + r, cy)], closed: false };
	}
}

// shapeInBox returns a shape's nodes fit to fill the bounding box (x,y,w,h) — so
// a shape can be DRAWN by dragging (like the rect/ellipse tools) and resized to
// any aspect ratio. A line is the box's drag diagonal.
export function shapeInBox(kind: ShapeKind, x: number, y: number, w: number, h: number): { nodes: PathNode[]; closed: boolean } {
	if (kind === 'line') {
		return { nodes: [cornerNode(Math.round(x), Math.round(y)), cornerNode(Math.round(x + w), Math.round(y + h))], closed: false };
	}
	// Build the shape on a unit circle, then normalise its bounding box to fill the
	// target box exactly (regardless of aspect ratio).
	let raw: { x: number; y: number }[];
	if (kind === 'star') {
		raw = [];
		for (let i = 0; i < 10; i++) {
			const rad = i % 2 === 0 ? 1 : 0.45;
			const a = -Math.PI / 2 + (i * Math.PI) / 5;
			raw.push({ x: Math.cos(a) * rad, y: Math.sin(a) * rad });
		}
	} else {
		const n = kind === 'triangle' ? 3 : kind === 'diamond' ? 4 : kind === 'pentagon' ? 5 : 6;
		raw = [];
		for (let i = 0; i < n; i++) {
			const a = -Math.PI / 2 + (i * 2 * Math.PI) / n;
			raw.push({ x: Math.cos(a), y: Math.sin(a) });
		}
	}
	let minX = Infinity,
		minY = Infinity,
		maxX = -Infinity,
		maxY = -Infinity;
	for (const p of raw) {
		minX = Math.min(minX, p.x);
		minY = Math.min(minY, p.y);
		maxX = Math.max(maxX, p.x);
		maxY = Math.max(maxY, p.y);
	}
	const bw = maxX - minX || 1;
	const bh = maxY - minY || 1;
	const nodes = raw.map((p) =>
		cornerNode(Math.round(x + ((p.x - minX) / bw) * w), Math.round(y + ((p.y - minY) / bh) * h))
	);
	return { nodes, closed: true };
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

// paintsOf normalizes a layer's fill to its paint stack: `fills` when present,
// else the legacy single hex as one solid paint. [] = no fill.
export function paintsOf(l: Layer): Paint[] {
	if (l.fills && l.fills.length) return l.fills;
	return l.fill ? [{ type: 'solid', color: l.fill }] : [];
}

// stackPrimary returns a representative solid hex for a paint stack: the topmost
// visible paint's colour (a gradient contributes its first stop). '' = nothing.
export function stackPrimary(ps: Paint[]): string {
	for (let i = ps.length - 1; i >= 0; i--) {
		const p = ps[i];
		if (p.hidden) continue;
		if (p.type === 'solid' && p.color) return p.color;
		if (p.stops?.length) return p.stops[0].color;
	}
	return '';
}

// paintPrimary returns a representative solid hex for consumers that need ONE
// colour (boolean composites, luminance masks, wobbled outlines).
export function paintPrimary(l: Layer): string {
	return stackPrimary(paintsOf(l));
}

// strokePaintsOf normalizes a layer's stroke to its paint stack: `strokes` when
// present, else the legacy single stroke_color as one solid paint. An unset
// colour keeps the renderers' historic white default. Only meaningful when the
// layer actually has a stroke (stroke_width > 0).
export function strokePaintsOf(l: Layer): Paint[] {
	if (l.strokes && l.strokes.length) return l.strokes;
	return [{ type: 'solid', color: l.stroke_color || '#FFFFFF' }];
}

// strokePrimary returns ONE representative stroke colour for consumers that
// can't paint a stack (brush stamps, text outlines, arrowhead markers).
export function strokePrimary(l: Layer): string {
	return stackPrimary(strokePaintsOf(l));
}

// textPaintsOf normalizes a text layer's fill to a paint stack: `fills` when
// present, else the legacy `color` as one solid paint.
export function textPaintsOf(l: Layer): Paint[] {
	if (l.fills && l.fills.length) return l.fills;
	return l.color ? [{ type: 'solid', color: l.color }] : [];
}

// bgPaintsOf normalizes the canvas background to a paint stack: `fills` once set
// (even empty — that means "no background"), else the legacy type fields mapped
// to one equivalent paint. The Go renderer keeps a native legacy path, so this
// mapping only has to match it visually (CSS gradient conventions on both ends).
export function bgPaintsOf(b: Background): Paint[] {
	if (b.fills) return b.fills;
	if (b.type === 'image' || (!b.type && b.image_url)) {
		return b.image_url ? [{ type: 'image', src: b.image_url, fit: 'cover' }] : [];
	}
	if (b.type === 'gradient' || (!b.type && (b.from || b.to))) {
		return [
			{
				type: 'linear',
				angle: b.angle ?? 0,
				stops: [
					{ pos: 0, color: b.from ?? '#000000' },
					{ pos: 1, color: b.to ?? '#000000' }
				]
			}
		];
	}
	return [{ type: 'solid', color: b.color || '#141417' }];
}

// resolveSrc maps {user.avatar} (and any future {tokens}) to a real URL for the
// DOM preview; a real avatar URL needs a user id we don't have here, so the
// preview shows a neutral placeholder for avatar bindings.
export function resolveSrc(src: string | undefined): string {
	const s = (src ?? '').trim();
	if (!s || s.startsWith('{')) return '';
	return s;
}
