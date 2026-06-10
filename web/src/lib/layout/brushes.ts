// Figma Draw's brush catalog, recreated parametrically — mirrors internal/imaging/brushes.go
// (same params, same RNG, same noise, same geometry, same rng-consumption ORDER) so the live
// editor preview and the rendered PNG match. 15 STRETCH brushes (movie-genre names; a textured
// line stretched along the stroke, with a Direction) and 10 SCATTER brushes (music-genre names;
// particle clusters stamped along the stroke). Recipes tuned against the official brush preview
// images from developers.figma.com (ComplexStrokeProperties → available brushes).
//
// The RNG is a Park–Miller LCG (s*48271 % 2^31-1): every product stays under 2^53, so this
// port produces bit-identical sequences to the Go renderer.
export type BrushKind = 'stretch' | 'scatter';

interface Strand {
	off: number; // perpendicular offset × stroke weight (+n points right of travel)
	w: number; // ribbon width as a fraction of stroke weight
	wander: number; // noise offset amplitude × stroke weight
	wanderF: number; // noise frequency (cycles per stroke-weight of arc)
	dashOn: number; // dash length × stroke weight (0 = solid)
	dashOff: number; // gap length × stroke weight
	fringe: number; // strand-local edge speckle density
	fringeSide: number; // 0 = both edges, ±1 = only the ±n edge
	jag: number; // strand-local edge jitter override (-1 = brush default)
}

export interface BrushDef {
	id: string;
	name: string;
	kind: BrushKind;
	// stretch — width & envelope
	weight: number; // base width multiplier
	contrast: number; // 0..1 calligraphic thick/thin with direction
	nib: number; // nib angle, degrees
	bodyAmp: number; // slow organic width swell 0..1 (biopic)
	bodyF: number; // swell frequency
	taperIn: number; // taper length at the start × stroke weight
	taperOut: number; // taper length at the end × stroke weight
	tipIn: number; // width fraction at the very start (0 = point)
	tipOut: number; // width fraction at the very end
	// stretch — texture
	jagAmp: number; // high-frequency edge grit 0..1
	jagF: number; // grit frequency (cycles per stroke-weight)
	lumpAmp: number; // smooth rounded edge lumps 0..1 (screwball)
	lumpF: number;
	fringe: number; // edge speckle density 0..1+
	holes: number; // interior fleck-hole density 0..1
	holeW: number; // base hole size × stroke weight
	holeCluster: boolean; // holes come in tight clusters (noir) instead of spread out
	streaks: number; // interior long streak holes 0..1 (screwball, epic)
	streakBias: number; // >0 concentrates streaks toward the stroke start (direction-aware)
	grain: number; // charcoal mode: stipple the body from flecks instead of a solid ribbon
	endFray: number; // extra fray + specks at the trailing end (heist, new wave)
	strands: Strand[] | null;
	rake: number; // bristle filament count (new wave)
	// scatter
	form: 'dot' | 'fleck' | 'disc' | 'blob';
	step: number; // stamp spacing × stroke weight
	count: number; // particles per stamp
	prad: number; // particle radius × stroke weight
	spread: number; // perpendicular spread × stroke weight
	alpha: number; // per-particle opacity
	uniform: boolean; // uniform band (vaporwave) instead of centre-weighted
	coreR: number; // solid core disc radius × stroke weight (oi)
	coreA: number; // solid core opacity
	// editor slider defaults (gap mirrors step; wiggle/size are user-only extras)
	gap: number;
	jitter: number;
	size: number;
}

const strand = (p: Partial<Strand>): Strand => ({
	off: 0, w: 0, wander: 0, wanderF: 0, dashOn: 0, dashOff: 0, fringe: 0, fringeSide: 0, jag: -1, ...p
});

const ST = (id: string, name: string, p: Partial<BrushDef>): BrushDef => ({
	id, name, kind: 'stretch',
	weight: 1, contrast: 0.08, nib: 25, bodyAmp: 0, bodyF: 0.4,
	taperIn: 0.4, taperOut: 0.4, tipIn: 0.95, tipOut: 0.95,
	jagAmp: 0, jagF: 6, lumpAmp: 0, lumpF: 0.9,
	fringe: 0, holes: 0, holeW: 0.07, holeCluster: false, streaks: 0, streakBias: 0, grain: 0, endFray: 0,
	strands: null, rake: 0,
	form: 'dot', step: 0.15, count: 10, prad: 0.05, spread: 0.5, alpha: 0.6,
	uniform: false, coreR: 0, coreA: 0, gap: 0.15, jitter: 0, size: 0, ...p
});

const SC = (id: string, name: string, p: Partial<BrushDef>): BrushDef => {
	const d = ST(id, name, p);
	d.kind = 'scatter';
	d.gap = d.step;
	return d;
};

export const BRUSHES: BrushDef[] = [
	// ── Stretch (movie genres) ──
	// Heist: even felt marker, lightly inked edges, the tail frays into specks.
	ST('heist', 'Heist', {
		weight: 0.95, jagAmp: 0.14, fringe: 0.22, endFray: 0.8, holes: 0.05, holeW: 0.05,
		taperOut: 8, tipOut: 0.55
	}),
	// Blockbuster: very fat soft marker, almost uniform, blunt ends.
	ST('blockbuster', 'Blockbuster', { weight: 1.55, jagAmp: 0.09, jagF: 5, fringe: 0.12, bodyAmp: 0.06 }),
	// Grindhouse: worn marker on rough paper — deep jagged grit everywhere.
	ST('grindhouse', 'Grindhouse', { weight: 1.15, jagAmp: 0.26, jagF: 10, fringe: 0.4, holes: 0.1, holeW: 0.05 }),
	// Biopic: wet ink brush — smooth liquid edges, width pools and swells.
	ST('biopic', 'Biopic', {
		weight: 1.2, bodyAmp: 0.6, bodyF: 0.3, contrast: 0.15, nib: 40,
		taperIn: 1.6, taperOut: 0.8, tipIn: 0.3, tipOut: 0.55, jagAmp: 0.02
	}),
	// Spaghetti Western: three broken wandering hairlines.
	ST('spaghetti_western', 'Spaghetti Western', {
		strands: [
			strand({ off: -0.34, w: 0.085, wander: 0.45, wanderF: 0.09, dashOn: 4.5, dashOff: 0.9 }),
			strand({ off: 0.02, w: 0.1, wander: 0.55, wanderF: 0.07, dashOn: 6, dashOff: 0.8 }),
			strand({ off: 0.32, w: 0.075, wander: 0.5, wanderF: 0.11, dashOn: 4, dashOff: 1.2 })
		],
		taperIn: 0.4, taperOut: 0.4, tipIn: 0.7, tipOut: 0.7
	}),
	// Slasher: a bundle of quick crossing strands, pointed sweeping tips.
	ST('slasher', 'Slasher', {
		strands: [
			strand({ off: 0.06, w: 0.4, wander: 0.8, wanderF: 0.07 }),
			strand({ off: -0.08, w: 0.32, wander: 0.95, wanderF: 0.055 }),
			strand({ off: 0.02, w: 0.25, wander: 0.9, wanderF: 0.085 }),
			strand({ off: -0.03, w: 0.18, wander: 1.1, wanderF: 0.1 })
		],
		taperIn: 4, taperOut: 4, tipIn: 0, tipOut: 0, jagAmp: 0.02
	}),
	// Hardboiled: charcoal stick — thin solid core, heavy granular dust on both edges.
	ST('hardboiled', 'Hardboiled', {
		weight: 0.6, jagAmp: 0.22, jagF: 5.5, fringe: 1.0, bodyAmp: 0.1,
		holes: 0.12, holeW: 0.045, tipIn: 0.8, tipOut: 0.8
	}),
	// Verite: dry charcoal — the body is loose stippled grain, no solid core.
	ST('verite', 'Verite', {
		weight: 0.95, grain: 1.0, contrast: 0.14, nib: 30, bodyAmp: 0.15, tipIn: 0.6, tipOut: 0.6
	}),
	// Epic: pressed marker with torn chunky edges and the odd nick inside.
	ST('epic', 'Epic', {
		weight: 1.08, jagAmp: 0.2, jagF: 5, lumpAmp: 0.34, lumpF: 1.0, fringe: 0.2, holes: 0.1, holeW: 0.06,
		taperIn: 5, tipIn: 0.6, streaks: 0.5, streakBias: 1.4
	}),
	// Screwball: chewed lumpy edges with long dry streaks through the middle.
	ST('screwball', 'Screwball', {
		weight: 0.95, lumpAmp: 0.36, lumpF: 1.1, jagAmp: 0.08, fringe: 0.3, streaks: 0.55,
		holes: 0.1, holeW: 0.05, tipIn: 0.85, tipOut: 0.85
	}),
	// Rom-com: double pass — a thick chalky top line over a thin clean one.
	ST('rom_com', 'Rom-com', {
		strands: [
			strand({ off: -0.33, w: 0.5, wander: 0.03, wanderF: 0.08, fringe: 0.5, fringeSide: -1, jag: 0.08 }),
			strand({ off: 0.24, w: 0.18, wander: 0.02, wanderF: 0.06, jag: 0.02 })
		]
	}),
	// Noir: heavy ink slab — smooth lumpy silhouette, white pinhole clusters inside.
	ST('noir', 'Noir', {
		weight: 1.45, lumpAmp: 0.16, lumpF: 0.9, jagAmp: 0.03,
		holes: 0.6, holeW: 0.06, holeCluster: true, fringe: 0.08, taperOut: 1.3, tipOut: 0.45
	}),
	// Propaganda: flat poster nib — thin point swelling into a wide smooth wedge.
	ST('propaganda', 'Propaganda', {
		weight: 1.2, contrast: 0.85, nib: 28, taperIn: 3.2, taperOut: 0.6, tipIn: 0.06, tipOut: 0.8, bodyAmp: 0.05
	}),
	// Melodrama: confident round ink brush — clean, gently swelling, soft tips.
	ST('melodrama', 'Melodrama', {
		weight: 1.1, contrast: 0.38, nib: 50, taperIn: 2.0, taperOut: 1.6, tipIn: 0.15, tipOut: 0.25,
		bodyAmp: 0.07, jagAmp: 0.01
	}),
	// New wave: dry rake — parallel bristle filaments fraying apart at the tail.
	ST('new_wave', 'New wave', {
		rake: 14, weight: 1.15, endFray: 0.9, jagAmp: 0.05, taperIn: 1.0, taperOut: 1.2, tipIn: 0.5, tipOut: 0.3
	}),
	// ── Scatter (music genres) ──
	// Bubblegum: fine soft airbrush mist.
	SC('bubblegum', 'Bubblegum', { step: 0.09, count: 40, prad: 0.025, spread: 0.5, alpha: 0.24 }),
	// Witch house: the same mist but dense and dark to a near-solid core.
	SC('witch_house', 'Witch house', { step: 0.08, count: 44, prad: 0.035, spread: 0.5, alpha: 0.55 }),
	// Shoegaze: coarse scratchy grain, no solid core.
	SC('shoegaze', 'Shoegaze', { form: 'fleck', step: 0.15, count: 16, prad: 0.032, spread: 0.45, alpha: 0.85 }),
	// Honky-tonk: chunky overlapping round dots, bumpy silhouette.
	SC('honky_tonk', 'Honky-tonk', { form: 'disc', step: 0.22, count: 2, prad: 0.2, spread: 0.34, alpha: 1 }),
	// Screamo: dark scratchy core with flecks flying off.
	SC('screamo', 'Screamo', { form: 'fleck', step: 0.13, count: 12, prad: 0.045, spread: 0.34, alpha: 0.95 }),
	// Drone: huge soft ink blobs merging into a lumpy mass.
	SC('drone', 'Drone', { form: 'blob', step: 0.38, count: 1, prad: 0.55, spread: 0.14, alpha: 1 }),
	// Doo-wop: dense fine charcoal stipple, near-solid fuzzy band.
	SC('doo_wop', 'Doo-wop', { step: 0.08, count: 46, prad: 0.032, spread: 0.48, alpha: 0.45 }),
	// Spoken word: sparse faint pen stipple, wide and airy.
	SC('spoken_word', 'Spoken word', { step: 0.22, count: 6, prad: 0.032, spread: 0.55, alpha: 0.6 }),
	// Vaporwave: light even grain in a wide flat band.
	SC('vaporwave', 'Vaporwave', { step: 0.14, count: 16, prad: 0.032, spread: 0.6, alpha: 0.33, uniform: true }),
	// Oi!: solid spray-paint line with a fine overspray halo.
	SC('oi', 'Oi!', { step: 0.13, count: 12, prad: 0.038, spread: 0.5, alpha: 0.5, coreR: 0.26, coreA: 1 })
];

export const BRUSH_BY_ID: Record<string, BrushDef> = Object.fromEntries(BRUSHES.map((b) => [b.id, b]));
export const DEFAULT_BRUSH = 'heist';
export function brushDef(id: string | undefined): BrushDef {
	return (id && BRUSH_BY_ID[id]) || BRUSH_BY_ID[DEFAULT_BRUSH];
}

// ── deterministic randomness (mirrors brushes.go bit-for-bit) ───────────────

function newRng(seed: number): () => number {
	let s = seed % 2147483646;
	if (s <= 0) s += 2147483646;
	return () => {
		s = (s * 48271) % 2147483647;
		return s / 2147483647;
	};
}

function hash01(n: number): number {
	let x = (n * 48271 + 11) % 2147483647;
	if (x < 0) x += 2147483647;
	x = (x * 48271) % 2147483647;
	return x / 2147483647;
}

function vnoise(salt: number, x: number): number {
	const i = Math.floor(x);
	const f = x - i;
	const a = hash01(salt * 7919 + i);
	const b = hash01(salt * 7919 + i + 1);
	const t = f * f * (3 - 2 * f);
	return a + (b - a) * t;
}

function pnoise(salt: number, x: number, period: number, closed: boolean): number {
	if (!closed || period <= 0) return vnoise(salt, x);
	const w = x / period;
	return (1 - w) * vnoise(salt, x) + w * vnoise(salt, x - period);
}

// ── shared geometry (mirrors brushes.go) ────────────────────────────────────

export interface BrushPt {
	x: number;
	y: number;
}

interface Resampled {
	pts: BrushPt[];
	s: number[];
	length: number;
}

function resamplePath(pts: BrushPt[], closed: boolean, step: number): Resampled {
	const n = pts.length;
	const out: Resampled = { pts: [pts[0]], s: [0], length: 0 };
	const segs = closed ? n : n - 1;
	let dist = 0,
		next = step;
	for (let i = 0; i < segs; i++) {
		const a = pts[i],
			b = pts[(i + 1) % n];
		const segLen = Math.hypot(b.x - a.x, b.y - a.y);
		if (segLen < 1e-9) continue;
		while (next <= dist + segLen) {
			const t = (next - dist) / segLen;
			out.pts.push({ x: a.x + (b.x - a.x) * t, y: a.y + (b.y - a.y) * t });
			out.s.push(next);
			next += step;
		}
		dist += segLen;
	}
	out.length = dist;
	if (!closed) {
		const last = pts[n - 1];
		const end = out.pts[out.pts.length - 1];
		if (Math.hypot(last.x - end.x, last.y - end.y) > step * 0.25) {
			out.pts.push(last);
			out.s.push(dist);
		}
	}
	return out;
}

// normalAt returns [nx, ny, theta] at index i.
function normalAt(r: Resampled, i: number, closed: boolean): [number, number, number] {
	const n = r.pts.length;
	let a: BrushPt, b: BrushPt;
	if (closed) {
		a = r.pts[(i - 1 + n) % n];
		b = r.pts[(i + 1) % n];
	} else {
		a = r.pts[Math.max(0, i - 1)];
		b = r.pts[Math.min(n - 1, i + 1)];
	}
	const dx = b.x - a.x,
		dy = b.y - a.y;
	const l = Math.hypot(dx, dy);
	if (l === 0) return [0, 1, 0];
	return [-dy / l, dx / l, Math.atan2(dy, dx)];
}

function idxFor(r: Resampled, s: number): number {
	if (r.length <= 0) return 0;
	const i = Math.floor((s / r.length) * (r.pts.length - 1));
	return i < 0 ? 0 : i > r.pts.length - 1 ? r.pts.length - 1 : i;
}

const F = (v: number) => v.toFixed(2);

// ── stretch engine (mirrors brushStretch in brushes.go) ─────────────────────

interface StretchGeom {
	r: Resampled;
	closed: boolean;
	flip: boolean;
	sw: number;
	period: number;
	nibRad: number;
	lo: number;
	def: BrushDef;
}

function seAt(g: StretchGeom, i: number): number {
	return g.flip ? g.r.length - g.r.s[i] : g.r.s[i];
}

function frayRamp(g: StretchGeom, i: number): number {
	if (g.closed || g.def.endFray <= 0) return 0;
	const v = (seAt(g, i) - (g.r.length - 3 * g.sw)) / (3 * g.sw);
	return v < 0 ? 0 : v > 1 ? 1 : v;
}

function halfWidth(g: StretchGeom, st: Strand, salt: number, i: number, sA: number, sB: number): number {
	const theta = normalAt(g.r, i, g.closed)[2];
	const d = g.def;
	const nibF = g.lo + (1 - g.lo) * Math.abs(Math.cos(theta - g.nibRad));
	const u = g.r.s[i] / g.sw;
	// two octaves: a single low-frequency channel can come out near-monotonic
	const bodyN =
		0.62 * (pnoise(salt * 31 + 5, u * d.bodyF, g.period * d.bodyF, g.closed) * 2 - 1) +
		0.38 * (pnoise(salt * 31 + 6, u * d.bodyF * 2.3, g.period * d.bodyF * 2.3, g.closed) * 2 - 1);
	const body = 1 + d.bodyAmp * 1.25 * bodyN;
	let taper = 1;
	if (!g.closed) {
		let tin = d.taperIn,
			tout = d.taperOut,
			tipIn = d.tipIn,
			tipOut = d.tipOut;
		if (g.flip) {
			[tin, tout] = [tout, tin];
			[tipIn, tipOut] = [tipOut, tipIn];
		}
		const segIn = Math.min(tin * g.sw, Math.max((sB - sA) / 3, 1e-6));
		const segOut = Math.min(tout * g.sw, Math.max((sB - sA) / 3, 1e-6));
		const fIn = Math.min(1, (g.r.s[i] - sA) / segIn);
		const fOut = Math.min(1, (sB - g.r.s[i]) / segOut);
		taper = (tipIn + (1 - tipIn) * fIn) * (tipOut + (1 - tipOut) * fOut);
		if (taper < 0) taper = 0;
	}
	return ((g.sw * st.w) / 2) * nibF * body * taper;
}

// edgeBite returns [lf, rf], the edge jitter factors (mostly biting inward).
function edgeBite(g: StretchGeom, st: Strand, salt: number, i: number): [number, number] {
	const d = g.def;
	let jag = st.jag >= 0 ? st.jag : d.jagAmp;
	jag *= 1 + 2.2 * d.endFray * frayRamp(g, i);
	const u = g.r.s[i] / g.sw;
	let lf = 1,
		rf = 1;
	if (jag > 0) {
		// two octaves so the grit reads ragged, not wavy
		const el =
			0.62 * pnoise(salt * 31 + 11, u * d.jagF, g.period * d.jagF, g.closed) +
			0.38 * pnoise(salt * 31 + 15, u * d.jagF * 2.9, g.period * d.jagF * 2.9, g.closed);
		const er =
			0.62 * pnoise(salt * 31 + 12, u * d.jagF, g.period * d.jagF, g.closed) +
			0.38 * pnoise(salt * 31 + 16, u * d.jagF * 2.9, g.period * d.jagF * 2.9, g.closed);
		lf -= (el * 1.5 - 0.35) * jag;
		rf -= (er * 1.5 - 0.35) * jag;
	}
	if (d.lumpAmp > 0) {
		const ll = pnoise(salt * 31 + 13, u * d.lumpF, g.period * d.lumpF, g.closed);
		const lr = pnoise(salt * 31 + 14, u * d.lumpF, g.period * d.lumpF, g.closed);
		lf -= (ll * 1.6 - 0.5) * d.lumpAmp;
		rf -= (lr * 1.6 - 0.5) * d.lumpAmp;
	}
	return [lf, rf];
}

function strandOffset(g: StretchGeom, st: Strand, salt: number, i: number): number {
	let off = st.off * g.sw;
	if (st.wander > 0) {
		const u = g.r.s[i] / g.sw;
		const wn =
			0.68 * (pnoise(salt * 31 + 3, u * st.wanderF, g.period * st.wanderF, g.closed) * 2 - 1) +
			0.32 * (pnoise(salt * 31 + 4, u * st.wanderF * 2.6, g.period * st.wanderF * 2.6, g.closed) * 2 - 1);
		off += st.wander * g.sw * wn;
	}
	return off;
}

// capArc: semicircular end cap from edge point a to b, bulging toward (dx,dy).
function capArc(a: BrushPt, b: BrushPt, dx: number, dy: number): string {
	const mx = (a.x + b.x) / 2,
		my = (a.y + b.y) / 2;
	const hx = (a.x - b.x) / 2,
		hy = (a.y - b.y) / 2;
	const rad = Math.hypot(hx, hy);
	if (rad < 0.35) return '';
	let d = '';
	for (let k = 1; k <= 7; k++) {
		const al = (k / 8) * Math.PI;
		const c = Math.cos(al),
			s = Math.sin(al);
		d += ` L ${F(mx + hx * c + dx * rad * s)} ${F(my + hy * c + dy * rad * s)}`;
	}
	return d;
}

function ribbonRange(g: StretchGeom, st: Strand, salt: number, i0: number, i1: number, sA: number, sB: number): string {
	if (i1 - i0 < 1) return '';
	const lefts: BrushPt[] = [],
		rights: BrushPt[] = [];
	for (let i = i0; i <= i1; i++) {
		const [nx, ny] = normalAt(g.r, i, g.closed);
		const off = strandOffset(g, st, salt, i);
		const hw = halfWidth(g, st, salt, i, sA, sB);
		const [lf, rf] = edgeBite(g, st, salt, i);
		const p = g.r.pts[i];
		lefts.push({ x: p.x + nx * (off + hw * lf), y: p.y + ny * (off + hw * lf) });
		rights.push({ x: p.x + nx * (off - hw * rf), y: p.y + ny * (off - hw * rf) });
	}
	const n = lefts.length;
	let d = `M ${F(lefts[0].x)} ${F(lefts[0].y)}`;
	for (let i = 1; i < n; i++) d += ` L ${F(lefts[i].x)} ${F(lefts[i].y)}`;
	const [nx1, ny1] = normalAt(g.r, i1, g.closed);
	d += capArc(lefts[n - 1], rights[n - 1], ny1, -nx1); // bulge forward (+tangent)
	for (let i = n - 1; i >= 0; i--) d += ` L ${F(rights[i].x)} ${F(rights[i].y)}`;
	const [nx0, ny0] = normalAt(g.r, i0, g.closed);
	d += capArc(rights[0], lefts[0], -ny0, nx0); // bulge backward (-tangent)
	return d + ' Z ';
}

function ribbonClosed(g: StretchGeom, st: Strand, salt: number): string {
	const n = g.r.pts.length;
	const lefts: BrushPt[] = [],
		rights: BrushPt[] = [];
	for (let i = 0; i < n; i++) {
		const [nx, ny] = normalAt(g.r, i, true);
		const off = strandOffset(g, st, salt, i);
		const hw = halfWidth(g, st, salt, i, 0, g.r.length);
		const [lf, rf] = edgeBite(g, st, salt, i);
		const p = g.r.pts[i];
		lefts.push({ x: p.x + nx * (off + hw * lf), y: p.y + ny * (off + hw * lf) });
		rights.push({ x: p.x + nx * (off - hw * rf), y: p.y + ny * (off - hw * rf) });
	}
	let d = `M ${F(lefts[0].x)} ${F(lefts[0].y)}`;
	for (let i = 1; i < n; i++) d += ` L ${F(lefts[i].x)} ${F(lefts[i].y)}`;
	d += ` Z M ${F(rights[n - 1].x)} ${F(rights[n - 1].y)}`;
	for (let i = n - 2; i >= 0; i--) d += ` L ${F(rights[i].x)} ${F(rights[i].y)}`;
	return d + ' Z ';
}

// holePoly: one small rough fleck hole inside the ribbon. Always consumes 2 rng rolls.
function holePoly(g: StretchGeom, st: Strand, salt: number, rng: () => number, s: number, v: number): string {
	const i = idxFor(g.r, s);
	const rx = (0.6 + 0.9 * rng()) * g.def.holeW * g.sw;
	const ry = rx * (0.35 + 0.3 * rng());
	// keep the hole inside the WORST-CASE bitten edge — a hole poking past the
	// ribbon outline would get painted by the even-odd fill, not punched
	const jag = st.jag >= 0 ? st.jag : g.def.jagAmp;
	const bite = 1 - jag * 1.15 - g.def.lumpAmp * 1.1;
	const hw = halfWidth(g, st, salt, i, 0, g.r.length) * bite;
	if (hw < ry * 1.6) return '';
	const vv = v * (hw - ry * 1.3);
	const [nx, ny, theta] = normalAt(g.r, i, g.closed);
	const off = strandOffset(g, st, salt, i);
	const cx = g.r.pts[i].x + nx * (off + vv);
	const cy = g.r.pts[i].y + ny * (off + vv);
	let d = '';
	for (let k = 0; k <= 7; k++) {
		const a = (k / 7) * 2 * Math.PI;
		const wob = 0.75 + 0.5 * hash01(salt * 977 + i * 8 + k);
		const px = Math.cos(a) * rx * wob;
		const py = Math.sin(a) * ry * wob;
		const x = cx + px * Math.cos(theta) - py * Math.sin(theta);
		const y = cy + px * Math.sin(theta) + py * Math.cos(theta);
		d += `${k === 0 ? 'M' : ' L'} ${F(x)} ${F(y)}`;
	}
	return d + ' Z ';
}

// streakPoly: a long thin dry streak hole. Always consumes 4 rng rolls.
function streakPoly(g: StretchGeom, st: Strand, salt: number, rng: () => number): string {
	const n = g.r.pts.length;
	const lenS = (2.2 + 3.4 * rng()) * g.sw;
	let su = rng();
	if (g.def.streakBias > 0) {
		su = Math.pow(su, 1 + g.def.streakBias);
		if (g.flip) su = 1 - su;
	}
	const s0 = su * (g.r.length - lenS);
	const v = rng() * 2 - 1;
	const th = (0.04 + 0.06 * rng()) * g.sw;
	if (s0 < 0) return '';
	let d = '';
	const rights: BrushPt[] = [];
	for (let i = 0; i < n; i++) {
		if (g.r.s[i] < s0 || g.r.s[i] > s0 + lenS) continue;
		const jag = st.jag >= 0 ? st.jag : g.def.jagAmp;
		const hw = halfWidth(g, st, salt, i, 0, g.r.length) * (1 - jag * 1.15 - g.def.lumpAmp * 1.1);
		if (hw < th * 2) continue;
		// fade the streak thickness toward its ends
		const f = Math.sin(Math.min(1, Math.max(0, (g.r.s[i] - s0) / lenS)) * Math.PI);
		const t2 = th * (0.3 + 0.7 * f);
		const vv = v * 0.6 * (hw - t2 * 1.5);
		const [nx, ny] = normalAt(g.r, i, g.closed);
		const off = strandOffset(g, st, salt, i);
		const p = g.r.pts[i];
		d += `${d === '' ? 'M' : ' L'} ${F(p.x + nx * (off + vv + t2))} ${F(p.y + ny * (off + vv + t2))}`;
		rights.push({ x: p.x + nx * (off + vv - t2), y: p.y + ny * (off + vv - t2) });
	}
	for (let i = rights.length - 1; i >= 0; i--) d += ` L ${F(rights[i].x)} ${F(rights[i].y)}`;
	return d === '' ? '' : d + ' Z ';
}

interface BrushOpts {
	brush?: string;
	width: number;
	color: string;
	direction?: string;
	gap?: number;
	wiggle?: number;
	size?: number;
	// particle budget — leave unset for canvas strokes (12000, matching the Go
	// renderer exactly); the tiny picker previews pass a small cap instead
	cap?: number;
}

function stretchMarkup(pts: BrushPt[], o: BrushOpts, closed: boolean): string {
	const def = brushDef(o.brush);
	const sw = o.width;
	const step = Math.max(0.9, Math.min(4, sw * 0.16));
	const r = resamplePath(pts, closed, step);
	if (r.pts.length < 3 || r.length <= 0) return '';
	const g: StretchGeom = {
		r, closed, flip: o.direction === 'backward', sw,
		period: r.length / sw, nibRad: (def.nib * Math.PI) / 180,
		lo: 1 - def.contrast * 0.92, def
	};
	if (g.flip) g.nibRad = -g.nibRad;
	const rng = newRng(11);
	interface Job {
		st: Strand;
		salt: number;
		cut0: number;
		cut1: number;
	}
	const jobs: Job[] = (def.strands ?? []).map((st, k) => ({ st, salt: k + 1, cut0: 0, cut1: 0 }));
	if (def.rake > 0) {
		// filament widths grade from wide+overlapping at the -n edge (a merged solid
		// body) down to spaced hairlines at the +n edge, so the rake reads as one
		// stroke drying out — not separate ribbons.
		const band = def.weight * 0.82;
		for (let f = 0; f < def.rake; f++) {
			const fr = f / (def.rake - 1);
			const off = -band / 2 + band * Math.pow(fr, 1.55) + (rng() * 2 - 1) * 0.03;
			const w = 0.03 + 0.17 * Math.pow(1 - fr, 1.3) + 0.02 * rng();
			const cut = (0.15 + 0.85 * fr) * rng() * 3.4 * sw;
			jobs.push({
				st: strand({ off, w, wander: 0.035, wanderF: 0.3 + 0.04 * f }),
				salt: f + 1, cut0: rng() * (0.3 + fr) * sw, cut1: cut
			});
		}
	}
	if (jobs.length === 0) jobs.push({ st: strand({ off: 0, w: def.weight }), salt: 1, cut0: 0, cut1: 0 });
	const n = r.pts.length;
	let out = '';
	if (def.grain > 0) {
		// charcoal body: no solid ribbon at all — stipple elongated flecks across the
		// band (slightly past the edges), patchy along the stroke like dry media.
		const st = jobs[0].st;
		let cnt = Math.floor((def.grain * r.length) / sw * 110);
		if (cnt > (o.cap ?? 12000)) cnt = o.cap ?? 12000;
		let d = '';
		for (let h = 0; h < cnt; h++) {
			const i = Math.floor(rng() * (n - 1));
			const patch = rng();
			if (patch > 0.6 + 0.4 * vnoise(91, (r.s[i] / sw) * 0.7)) continue;
			const hw = halfWidth(g, st, jobs[0].salt, i, 0, r.length);
			if (hw <= 0) continue;
			const v = (rng() + rng() - 1) * 1.1 * hw;
			const ll = (0.07 + 0.12 * rng()) * sw;
			const lh = (0.02 + 0.032 * rng()) * sw;
			const [nx, ny, theta] = normalAt(r, i, closed);
			const rot = theta + (rng() * 2 - 1) * 0.25;
			const x = r.pts[i].x + nx * v;
			const y = r.pts[i].y + ny * v;
			const cs = Math.cos(rot),
				sn = Math.sin(rot);
			d += `M ${F(x - ll * cs + lh * sn)} ${F(y - ll * sn - lh * cs)} L ${F(x + ll * cs + lh * sn)} ${F(y + ll * sn - lh * cs)} L ${F(x + ll * cs - lh * sn)} ${F(y + ll * sn + lh * cs)} L ${F(x - ll * cs - lh * sn)} ${F(y - ll * sn + lh * cs)} Z `;
		}
		return `<path d='${d}' fill='${o.color}'/>`;
	}
	for (const j of jobs) {
		const st = j.st;
		let d = '';
		if (st.dashOn > 0 && (st.dashOn + st.dashOff) * sw >= step * 2) {
			// broken strand: walk dashes
			let s0 = rng() * st.dashOff * sw;
			while (s0 < r.length) {
				const on = st.dashOn * sw * (0.7 + 0.6 * rng());
				const off = st.dashOff * sw * (0.6 + 0.8 * rng());
				const s1 = Math.min(s0 + on, r.length);
				d += ribbonRange(g, st, j.salt, idxFor(r, s0), idxFor(r, s1), s0, s1);
				s0 = s1 + off;
			}
		} else if (closed) {
			d += ribbonClosed(g, st, j.salt);
		} else {
			const s0 = j.cut0,
				s1 = r.length - j.cut1;
			d += ribbonRange(g, st, j.salt, idxFor(r, s0), idxFor(r, s1), s0, s1);
		}
		// interior holes / streaks (single-strand brushes — texture lives in the body)
		if (!def.strands && def.rake === 0) {
			if (def.holes > 0) {
				if (def.holeCluster) {
					const nc = Math.floor((def.holes * r.length) / (6 * sw)) + 2;
					for (let c = 0; c < nc; c++) {
						const sc = rng() * r.length;
						const vc = (rng() * 2 - 1) * 0.55;
						const m = 8 + Math.floor(rng() * 9);
						for (let h = 0; h < m; h++) {
							const s = sc + (rng() * 2 - 1) * 1.3 * sw;
							const v = vc + (rng() * 2 - 1) * 0.4;
							if (s < 0 || s > r.length || v < -1 || v > 1) {
								rng();
								rng();
								continue;
							}
							d += holePoly(g, st, j.salt, rng, s, v);
						}
					}
				} else {
					const cnt = Math.floor((def.holes * r.length) / sw * 6);
					for (let h = 0; h < cnt; h++) d += holePoly(g, st, j.salt, rng, rng() * r.length, rng() * 2 - 1);
				}
			}
			if (def.streaks > 0) {
				const cnt = Math.floor((def.streaks * r.length) / (5 * sw)) + 1;
				for (let h = 0; h < cnt; h++) d += streakPoly(g, st, j.salt, rng);
			}
		}
		if (d !== '') out += `<path d='${d}' fill='${o.color}' fill-rule='evenodd'/>`;
		// edge fringe specks (drawn additively after the ribbon fill)
		const fr = st.fringe > 0 ? st.fringe : def.fringe;
		if (fr > 0) {
			let dots = '';
			const cnt = Math.floor((fr * r.length) / sw * 16);
			for (let h = 0; h < cnt; h++) {
				const i = Math.floor(rng() * (n - 1));
				let side = rng() < 0.5 ? -1 : 1;
				if (st.fringeSide !== 0) side = st.fringeSide;
				const hw = halfWidth(g, st, j.salt, i, 0, r.length);
				if (hw <= 0) {
					rng();
					rng();
					continue;
				}
				const spreadF = st.fringeSide !== 0 ? 0.16 : 0.3;
				const dd = hw + (rng() * rng() * 1.4 - 0.18) * spreadF * sw;
				const rad = (0.015 + 0.03 * rng()) * sw;
				const [nx, ny] = normalAt(r, i, closed);
				const offc = strandOffset(g, st, j.salt, i);
				dots += `<circle cx='${F(r.pts[i].x + nx * (offc + side * dd))}' cy='${F(r.pts[i].y + ny * (offc + side * dd))}' r='${F(rad)}'/>`;
			}
			if (dots !== '') out += `<g fill='${o.color}'>${dots}</g>`;
		}
	}
	// trailing-end fray specks
	if (def.endFray > 0 && !closed) {
		let dots = '';
		const cnt = Math.floor(14 * def.endFray);
		for (let h = 0; h < cnt; h++) {
			let s = r.length - rng() * 3 * sw;
			if (g.flip) s = r.length - s;
			const i = idxFor(r, s);
			const v = (rng() * 2 - 1) * sw * 0.6;
			const rad = (0.02 + 0.05 * rng()) * sw;
			const [nx, ny] = normalAt(r, i, closed);
			dots += `<circle cx='${F(r.pts[i].x + nx * v)}' cy='${F(r.pts[i].y + ny * v)}' r='${F(rad)}'/>`;
		}
		if (dots !== '') out += `<g fill='${o.color}'>${dots}</g>`;
	}
	return out;
}

// ── scatter engine (mirrors brushScatter in brushes.go) ─────────────────────

function scatterMarkup(pts: BrushPt[], o: BrushOpts, closed: boolean): string {
	const def = brushDef(o.brush);
	const sw = o.width;
	const stepMul = (o.gap ?? 0) > 0 ? o.gap! : def.step;
	let step = Math.max(1, stepMul * sw);
	// cap total particle work on very long strokes — same deterministic rule as the
	// Go engine so both renders stay identical
	let approxLen = 0;
	for (let i = 1; i < pts.length; i++) approxLen += Math.hypot(pts[i].x - pts[i - 1].x, pts[i].y - pts[i - 1].y);
	if (closed && pts.length > 1)
		approxLen += Math.hypot(pts[0].x - pts[pts.length - 1].x, pts[0].y - pts[pts.length - 1].y);
	const cap = o.cap ?? 12000;
	const est = (approxLen / step) * def.count;
	if (est > cap) step *= est / cap;
	const r = resamplePath(pts, closed, step);
	if (r.pts.length < 2) return '';
	const wig = ((o.wiggle ?? 0) / 100) * sw * 1.2;
	const sizeJit = (o.size ?? 0) / 100;
	const rng = newRng(11);
	let cores = '',
		parts = '';
	for (let i = 0; i < r.pts.length; i++) {
		const [nx, ny, theta] = normalAt(r, i, closed);
		const tx = ny,
			ty = -nx; // unit tangent
		if (def.coreR > 0)
			cores += `<circle cx='${F(r.pts[i].x)}' cy='${F(r.pts[i].y)}' r='${F(def.coreR * sw * (1 - sizeJit * 0.7 * rng()))}'/>`;
		for (let p = 0; p < def.count; p++) {
			let v: number;
			if (def.uniform) v = (rng() * 2 - 1) * def.spread * sw;
			else v = (rng() + rng() - 1) * def.spread * sw;
			if (def.form === 'fleck' && p % 5 === 4) v *= 2.1; // the occasional fleck flies off the band
			v += (rng() * 2 - 1) * wig;
			const along = (rng() * 2 - 1) * step * 0.6;
			const x = r.pts[i].x + nx * v + tx * along;
			const y = r.pts[i].y + ny * v + ty * along;
			let rad = def.prad * sw * (0.6 + 0.8 * rng()) * (1 - sizeJit * 0.85 * rng());
			if (rad < 0.25) rad = 0.25;
			if (def.form === 'fleck') {
				const rot = theta + (rng() * 2 - 1) * 0.9;
				const lw = rad * 1.9,
					lh = rad * 0.7;
				const cs = Math.cos(rot),
					sn = Math.sin(rot);
				parts += `<path d='M ${F(x - lw * cs + lh * sn)} ${F(y - lw * sn - lh * cs)} L ${F(x + lw * cs + lh * sn)} ${F(y + lw * sn - lh * cs)} L ${F(x + lw * cs - lh * sn)} ${F(y + lw * sn + lh * cs)} L ${F(x - lw * cs - lh * sn)} ${F(y - lw * sn + lh * cs)} Z'/>`;
			} else if (def.form === 'blob') {
				const bn = 14;
				const ph = rng() * 12;
				let d = '';
				for (let k = 0; k <= bn; k++) {
					const a = (k / bn) * 2 * Math.PI;
					const wob = 1 + 0.26 * (vnoise(41, ph + k * 0.55) * 2 - 1);
					d += `${k === 0 ? 'M' : ' L'} ${F(x + Math.cos(a) * rad * wob)} ${F(y + Math.sin(a) * rad * wob)}`;
				}
				parts += `<path d='${d} Z'/>`;
			} else {
				parts += `<circle cx='${F(x)}' cy='${F(y)}' r='${F(rad)}'/>`;
			}
		}
	}
	let out = '';
	if (cores !== '') out += `<g fill='${o.color}' fill-opacity='${def.coreA}'>${cores}</g>`;
	if (parts !== '') out += `<g fill='${o.color}' fill-opacity='${def.alpha}'>${parts}</g>`;
	return out;
}

// dynamicWobble displaces each point along its normal by smooth deterministic value
// noise — Figma's "Dynamic" hand-drawn look (the editor-side twin of brushes.go's
// wobblePath in layout.go; same noise, same params). Frequency maps to the number of
// noise cycles along the path (Figma's API range is 0.01..20), wiggle is the bump
// amplitude (UI % that may exceed 100), smoothen blends out a high-frequency octave
// so bumps go from jagged to rounded. Open paths keep their endpoints anchored over
// a short ramp; closed outlines use periodic noise — no seam.
export function dynamicWobble(
	pts: BrushPt[],
	frequency: number,
	wiggle: number,
	smoothen: number,
	closed: boolean
): BrushPt[] {
	const n = pts.length;
	if (wiggle <= 0 || n < 3) return pts;
	let A = (wiggle / 100) * 38; // px at 100% (calibrated against Figma's help screenshot)
	let cycles = Math.max(0.01, (frequency / 100) * 20);
	if (closed) cycles = Math.max(1, Math.round(cycles)); // integer cycles: seamless loop
	const smooth = Math.min(1, Math.max(0, smoothen / 100));
	const rough = 1 - smooth;
	const s: number[] = [0];
	let plen = 0;
	for (let i = 1; i < n; i++) {
		plen += Math.hypot(pts[i].x - pts[i - 1].x, pts[i].y - pts[i - 1].y);
		s.push(plen);
	}
	if (closed) plen += Math.hypot(pts[0].x - pts[n - 1].x, pts[0].y - pts[n - 1].y);
	if (plen <= 0) return pts;
	// keep lobes rounded (no cusps/self-intersection) on small shapes
	A = Math.min(A, (plen / cycles) * 0.55);
	const ramp = Math.min(plen * 0.1, (plen / cycles) * 0.5);
	return pts.map((p, i) => {
		const pa = pts[(i - 1 + n) % n];
		const pb = pts[(i + 1) % n];
		const tx = pb.x - pa.x,
			ty = pb.y - pa.y;
		const tl = Math.hypot(tx, ty);
		if (!tl) return p;
		const nx = -ty / tl,
			ny = tx / tl;
		const u = (s[i] / plen) * cycles;
		// a bump per cycle (Figma: "frequency = the number of bumps") with noise-
		// jittered phase and per-region magnitude, so every bump is pronounced but
		// their sizes and spacing stay organic — not a uniform sine, not flat noise
		const ph = 0.65 * (pnoise(57, u, cycles, closed) - 0.5);
		const m = 0.3 + 0.75 * pnoise(58, u * 1.3, cycles * 1.3, closed);
		let w = Math.sin(2 * Math.PI * (u + ph)) * m;
		if (rough > 0) w += rough * 0.35 * (pnoise(59, u * 3.3, cycles * 3.3, closed) * 2 - 1);
		w = Math.max(-1, Math.min(1, w));
		const env = !closed && ramp > 0 ? Math.min(1, s[i] / ramp, (plen - s[i]) / ramp) : 1;
		const off = A * w * env;
		return { x: p.x + nx * off, y: p.y + ny * off };
	});
}

// brushStrokeMarkup renders a brush stroke along pts as SVG markup (the editor-side
// twin of brushes.go strokeBrush).
export function brushStrokeMarkup(pts: BrushPt[], o: BrushOpts, closed: boolean): string {
	if (pts.length < 2 || o.width <= 0) return '';
	return brushDef(o.brush).kind === 'scatter' ? scatterMarkup(pts, o, closed) : stretchMarkup(pts, o, closed);
}

// brushPreviewSvg renders a small sample stroke for the picker with the real engine,
// so what you pick is exactly what you get. Returns inner SVG markup (currentColor).
export function brushPreviewSvg(def: BrushDef, w: number, h: number): string {
	const n = 24;
	const pad = 4;
	const pts: BrushPt[] = [];
	for (let i = 0; i < n; i++) {
		const t = i / (n - 1);
		pts.push({ x: pad + (w - pad * 2) * t, y: h / 2 + Math.sin(t * Math.PI * 2) * h * 0.16 });
	}
	return brushStrokeMarkup(pts, { brush: def.id, width: h * 0.34, color: 'currentColor', cap: 400 }, false);
}
