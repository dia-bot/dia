package imaging

// Figma Draw's brush catalog, recreated parametrically — mirrors web/src/lib/layout/brushes.ts
// (same params, same RNG, same noise, same geometry) so the live editor preview and the PNG
// render match. Figma ships 15 STRETCH brushes (movie-genre names; a textured line stretched
// along the stroke, with a Direction) and 10 SCATTER brushes (music-genre names; particle
// clusters stamped along the stroke). Each recipe below was tuned against the official brush
// preview images from developers.figma.com (ComplexStrokeProperties → available brushes).
//
// The RNG is a Park–Miller LCG (s*48271 % 2^31-1): every product stays under 2^53 so the
// JavaScript port produces bit-identical sequences (the previous 1103515245 LCG overflowed
// JS doubles and silently diverged).

import (
	"image/color"
	"math"

	"github.com/fogleman/gg"

	"github.com/dia-bot/dia/internal/layout"
)

// ── deterministic randomness (mirrored in brushes.ts) ──────────────────────

type brushRng struct{ s int64 }

func newBrushRng(seed int64) *brushRng {
	seed %= 2147483646
	if seed <= 0 {
		seed += 2147483646
	}
	return &brushRng{seed}
}

func (r *brushRng) next() float64 {
	r.s = r.s * 48271 % 2147483647
	return float64(r.s) / 2147483647
}

// hash01 maps an integer lattice coordinate to a stable 0..1 value.
func hash01(n int64) float64 {
	x := (n*48271 + 11) % 2147483647
	if x < 0 {
		x += 2147483647
	}
	x = x * 48271 % 2147483647
	return float64(x) / 2147483647
}

// vnoise is smooth 1-D value noise in 0..1 over x (salt selects the channel).
func vnoise(salt int64, x float64) float64 {
	i := math.Floor(x)
	f := x - i
	a := hash01(salt*7919 + int64(i))
	b := hash01(salt*7919 + int64(i) + 1)
	t := f * f * (3 - 2*f)
	return a + (b-a)*t
}

// pnoise is vnoise made periodic over [0,period) so closed outlines have no seam.
func pnoise(salt int64, x, period float64, closed bool) float64 {
	if !closed || period <= 0 {
		return vnoise(salt, x)
	}
	w := x / period
	return (1-w)*vnoise(salt, x) + w*vnoise(salt, x-period)
}

// ── brush definitions ───────────────────────────────────────────────────────

// brushStrand is one sub-ribbon of a stretch brush (rom-com's double line,
// slasher's crossing strands, spaghetti western's broken hairlines).
type brushStrand struct {
	off        float64 // perpendicular offset × stroke weight (+n points right of travel)
	w          float64 // ribbon width as a fraction of stroke weight
	wander     float64 // noise offset amplitude × stroke weight
	wanderF    float64 // noise frequency (cycles per stroke-weight of arc)
	dashOn     float64 // dash length × stroke weight (0 = solid)
	dashOff    float64 // gap length × stroke weight
	fringe     float64 // strand-local edge speckle density
	fringeSide int     // 0 = both edges, ±1 = only the ±n edge
	jag        float64 // strand-local edge jitter override (-1 = brush default)
}

type brushDef struct {
	kind string // "stretch" | "scatter"
	// stretch — width & envelope
	weight   float64 // base width multiplier
	contrast float64 // 0..1 calligraphic thick/thin with direction
	nib      float64 // nib angle, degrees
	bodyAmp  float64 // slow organic width swell 0..1 (biopic)
	bodyF    float64 // swell frequency
	taperIn  float64 // taper length at the start × stroke weight
	taperOut float64 // taper length at the end × stroke weight
	tipIn    float64 // width fraction at the very start (0 = point)
	tipOut   float64 // width fraction at the very end
	// stretch — texture
	jagAmp      float64 // high-frequency edge grit 0..1
	jagF        float64 // grit frequency (cycles per stroke-weight)
	lumpAmp     float64 // smooth rounded edge lumps 0..1 (screwball)
	lumpF       float64
	fringe      float64 // edge speckle density 0..1+
	holes       float64 // interior fleck-hole density 0..1
	holeW       float64 // base hole size × stroke weight
	holeCluster bool    // holes come in tight clusters (noir) instead of spread out
	streaks     float64 // interior long streak holes 0..1 (screwball, epic)
	streakBias  float64 // >0 concentrates streaks toward the stroke start (direction-aware)
	grain       float64 // charcoal mode: stipple the body from flecks instead of a solid ribbon
	endFray     float64 // extra fray + specks at the trailing end (heist, new wave)
	strands     []brushStrand
	rake        int // bristle filament count (new wave)
	// scatter
	form    string  // "dot" | "fleck" | "disc" | "blob"
	step    float64 // stamp spacing × stroke weight
	count   int     // particles per stamp
	prad    float64 // particle radius × stroke weight
	spread  float64 // perpendicular spread × stroke weight
	alpha   float64 // per-particle opacity
	uniform bool    // uniform band (vaporwave) instead of centre-weighted
	coreR   float64 // solid core disc radius × stroke weight (oi)
	coreA   float64 // solid core opacity
}

func stretchBase() brushDef {
	return brushDef{
		kind: "stretch", weight: 1, contrast: 0.08, nib: 25,
		taperIn: 0.4, taperOut: 0.4, tipIn: 0.95, tipOut: 0.95,
		jagF: 6, lumpF: 0.9, bodyF: 0.4, holeW: 0.07,
	}
}

func scatterBase() brushDef {
	return brushDef{
		kind: "scatter", weight: 1, form: "dot",
		step: 0.15, count: 10, prad: 0.05, spread: 0.5, alpha: 0.6,
	}
}

// brushCatalog: recipes tuned against Figma's official brush preview images.
var brushCatalog = map[string]brushDef{}

func init() {
	st := func(id string, mod func(*brushDef)) {
		d := stretchBase()
		mod(&d)
		brushCatalog[id] = d
	}
	sc := func(id string, mod func(*brushDef)) {
		d := scatterBase()
		mod(&d)
		brushCatalog[id] = d
	}
	// ── Stretch (movie genres) ──
	// Heist: even felt marker, lightly inked edges, the tail frays into specks.
	st("heist", func(d *brushDef) {
		d.weight, d.jagAmp, d.fringe, d.endFray = 0.95, 0.14, 0.22, 0.8
		d.holes, d.holeW = 0.05, 0.05
		d.taperOut, d.tipOut = 8, 0.55
	})
	// Blockbuster: very fat soft marker, almost uniform, blunt ends.
	st("blockbuster", func(d *brushDef) {
		d.weight, d.jagAmp, d.jagF, d.fringe = 1.55, 0.09, 5, 0.12
		d.bodyAmp = 0.06
	})
	// Grindhouse: worn marker on rough paper — deep jagged grit everywhere.
	st("grindhouse", func(d *brushDef) {
		d.weight, d.jagAmp, d.jagF = 1.15, 0.26, 10
		d.fringe, d.holes, d.holeW = 0.4, 0.1, 0.05
	})
	// Biopic: wet ink brush — smooth liquid edges, width pools and swells.
	st("biopic", func(d *brushDef) {
		d.weight, d.bodyAmp, d.bodyF = 1.2, 0.6, 0.3
		d.contrast, d.nib = 0.15, 40
		d.taperIn, d.taperOut, d.tipIn, d.tipOut = 1.6, 0.8, 0.3, 0.55
		d.jagAmp = 0.02
	})
	// Spaghetti Western: three broken wandering hairlines.
	st("spaghetti_western", func(d *brushDef) {
		d.strands = []brushStrand{
			{off: -0.34, w: 0.085, wander: 0.45, wanderF: 0.09, dashOn: 4.5, dashOff: 0.9, jag: -1},
			{off: 0.02, w: 0.1, wander: 0.55, wanderF: 0.07, dashOn: 6, dashOff: 0.8, jag: -1},
			{off: 0.32, w: 0.075, wander: 0.5, wanderF: 0.11, dashOn: 4, dashOff: 1.2, jag: -1},
		}
		d.taperIn, d.taperOut, d.tipIn, d.tipOut = 0.4, 0.4, 0.7, 0.7
		d.jagAmp = 0
	})
	// Slasher: a bundle of quick crossing strands, pointed sweeping tips.
	st("slasher", func(d *brushDef) {
		d.strands = []brushStrand{
			{off: 0.06, w: 0.4, wander: 0.8, wanderF: 0.07, jag: -1},
			{off: -0.08, w: 0.32, wander: 0.95, wanderF: 0.055, jag: -1},
			{off: 0.02, w: 0.25, wander: 0.9, wanderF: 0.085, jag: -1},
			{off: -0.03, w: 0.18, wander: 1.1, wanderF: 0.1, jag: -1},
		}
		d.taperIn, d.taperOut, d.tipIn, d.tipOut = 4, 4, 0, 0
		d.jagAmp = 0.02
	})
	// Hardboiled: charcoal stick — thin solid core, heavy granular dust on both edges.
	st("hardboiled", func(d *brushDef) {
		d.weight, d.jagAmp, d.jagF, d.fringe = 0.6, 0.22, 5.5, 1.0
		d.bodyAmp = 0.1
		d.holes, d.holeW = 0.12, 0.045
		d.tipIn, d.tipOut = 0.8, 0.8
	})
	// Verite: dry charcoal — the body is loose stippled grain, no solid core.
	st("verite", func(d *brushDef) {
		d.weight, d.grain = 0.95, 1.0
		d.contrast, d.nib, d.bodyAmp = 0.14, 30, 0.15
		d.tipIn, d.tipOut = 0.6, 0.6
	})
	// Epic: pressed marker with torn chunky edges and the odd nick inside.
	st("epic", func(d *brushDef) {
		d.weight, d.jagAmp, d.jagF = 1.08, 0.2, 5
		d.lumpAmp, d.lumpF = 0.34, 1.0
		d.fringe = 0.2
		d.holes, d.holeW = 0.1, 0.06
		d.taperIn, d.tipIn = 5, 0.6
		d.streaks, d.streakBias = 0.5, 1.4
	})
	// Screwball: chewed lumpy edges with long dry streaks through the middle.
	st("screwball", func(d *brushDef) {
		d.weight = 0.95
		d.lumpAmp, d.lumpF = 0.36, 1.1
		d.jagAmp, d.fringe, d.streaks = 0.08, 0.3, 0.55
		d.holes, d.holeW = 0.1, 0.05
		d.tipIn, d.tipOut = 0.85, 0.85
	})
	// Rom-com: double pass — a thick chalky top line over a thin clean one.
	st("rom_com", func(d *brushDef) {
		d.strands = []brushStrand{
			{off: -0.33, w: 0.5, wander: 0.03, wanderF: 0.08, fringe: 0.5, fringeSide: -1, jag: 0.08},
			{off: 0.24, w: 0.18, wander: 0.02, wanderF: 0.06, jag: 0.02},
		}
	})
	// Noir: heavy ink slab — smooth lumpy silhouette, white pinhole clusters inside.
	st("noir", func(d *brushDef) {
		d.weight = 1.45
		d.lumpAmp, d.lumpF = 0.16, 0.9
		d.jagAmp = 0.03
		d.holes, d.holeW, d.holeCluster = 0.6, 0.06, true
		d.fringe = 0.08
		d.taperOut, d.tipOut = 1.3, 0.45
	})
	// Propaganda: flat poster nib — thin point swelling into a wide smooth wedge.
	st("propaganda", func(d *brushDef) {
		d.weight, d.contrast, d.nib = 1.2, 0.85, 28
		d.taperIn, d.taperOut, d.tipIn, d.tipOut = 3.2, 0.6, 0.06, 0.8
		d.bodyAmp, d.jagAmp = 0.05, 0
	})
	// Melodrama: confident round ink brush — clean, gently swelling, soft tips.
	st("melodrama", func(d *brushDef) {
		d.weight, d.contrast, d.nib = 1.1, 0.38, 50
		d.taperIn, d.taperOut, d.tipIn, d.tipOut = 2.0, 1.6, 0.15, 0.25
		d.bodyAmp, d.jagAmp = 0.07, 0.01
	})
	// New wave: dry rake — parallel bristle filaments fraying apart at the tail.
	st("new_wave", func(d *brushDef) {
		d.rake = 14
		d.weight, d.endFray, d.jagAmp = 1.15, 0.9, 0.05
		d.taperIn, d.taperOut, d.tipIn, d.tipOut = 1.0, 1.2, 0.5, 0.3
	})
	// ── Scatter (music genres) ──
	// Bubblegum: fine soft airbrush mist.
	sc("bubblegum", func(d *brushDef) {
		d.step, d.count, d.prad, d.spread, d.alpha = 0.09, 40, 0.025, 0.5, 0.24
	})
	// Witch house: the same mist but dense and dark to a near-solid core.
	sc("witch_house", func(d *brushDef) {
		d.step, d.count, d.prad, d.spread, d.alpha = 0.08, 44, 0.035, 0.5, 0.55
	})
	// Shoegaze: coarse scratchy grain, no solid core.
	sc("shoegaze", func(d *brushDef) {
		d.form = "fleck"
		d.step, d.count, d.prad, d.spread, d.alpha = 0.15, 16, 0.032, 0.45, 0.85
	})
	// Honky-tonk: chunky overlapping round dots, bumpy silhouette.
	sc("honky_tonk", func(d *brushDef) {
		d.form = "disc"
		d.step, d.count, d.prad, d.spread, d.alpha = 0.22, 2, 0.2, 0.34, 1
	})
	// Screamo: dark scratchy core with flecks flying off.
	sc("screamo", func(d *brushDef) {
		d.form = "fleck"
		d.step, d.count, d.prad, d.spread, d.alpha = 0.13, 12, 0.045, 0.34, 0.95
	})
	// Drone: huge soft ink blobs merging into a lumpy mass.
	sc("drone", func(d *brushDef) {
		d.form = "blob"
		d.step, d.count, d.prad, d.spread, d.alpha = 0.38, 1, 0.55, 0.14, 1
	})
	// Doo-wop: dense fine charcoal stipple, near-solid fuzzy band.
	sc("doo_wop", func(d *brushDef) {
		d.step, d.count, d.prad, d.spread, d.alpha = 0.08, 46, 0.032, 0.48, 0.45
	})
	// Spoken word: sparse faint pen stipple, wide and airy.
	sc("spoken_word", func(d *brushDef) {
		d.step, d.count, d.prad, d.spread, d.alpha = 0.22, 6, 0.032, 0.55, 0.6
	})
	// Vaporwave: light even grain in a wide flat band.
	sc("vaporwave", func(d *brushDef) {
		d.step, d.count, d.prad, d.spread, d.alpha = 0.14, 16, 0.032, 0.6, 0.33
		d.uniform = true
	})
	// Oi!: solid spray-paint line with a fine overspray halo.
	sc("oi", func(d *brushDef) {
		d.step, d.count, d.prad, d.spread, d.alpha = 0.13, 12, 0.038, 0.5, 0.5
		d.coreR, d.coreA = 0.26, 1
	})
}

func brushFor(id string) brushDef {
	if b, ok := brushCatalog[id]; ok {
		return b
	}
	return brushCatalog["heist"]
}

// ── shared geometry (mirrored in brushes.ts) ───────────────────────────────

// resampled is a polyline resampled to a fixed arc-length step.
type resampled struct {
	pts    []pathPt
	s      []float64 // cumulative arc length per point
	length float64   // total arc length
}

// resamplePath walks pts (closing the loop when closed) emitting a point every
// step px. The exact same walk happens in the TS engine.
func resamplePath(pts []pathPt, closed bool, step float64) resampled {
	n := len(pts)
	out := resampled{pts: []pathPt{pts[0]}, s: []float64{0}}
	segs := n - 1
	if closed {
		segs = n
	}
	dist, next := 0.0, step
	for i := 0; i < segs; i++ {
		a, b := pts[i], pts[(i+1)%n]
		segLen := math.Hypot(b.x-a.x, b.y-a.y)
		if segLen < 1e-9 {
			continue
		}
		for next <= dist+segLen {
			t := (next - dist) / segLen
			out.pts = append(out.pts, pathPt{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t})
			out.s = append(out.s, next)
			next += step
		}
		dist += segLen
	}
	out.length = dist
	if !closed {
		last := pts[n-1]
		end := out.pts[len(out.pts)-1]
		if math.Hypot(last.x-end.x, last.y-end.y) > step*0.25 {
			out.pts = append(out.pts, last)
			out.s = append(out.s, dist)
		}
	}
	return out
}

// normalAt returns the unit normal, unit tangent and tangent angle at index i.
func normalAt(r resampled, i int, closed bool) (nx, ny, theta float64) {
	n := len(r.pts)
	var a, b pathPt
	if closed {
		a, b = r.pts[(i-1+n)%n], r.pts[(i+1)%n]
	} else {
		lo, hi := i-1, i+1
		if lo < 0 {
			lo = 0
		}
		if hi > n-1 {
			hi = n - 1
		}
		a, b = r.pts[lo], r.pts[hi]
	}
	dx, dy := b.x-a.x, b.y-a.y
	l := math.Hypot(dx, dy)
	if l == 0 {
		return 0, 1, 0
	}
	return -dy / l, dx / l, math.Atan2(dy, dx)
}

// idxFor returns the resampled index nearest to arc position s.
func idxFor(r resampled, s float64) int {
	if r.length <= 0 {
		return 0
	}
	i := int(s / r.length * float64(len(r.pts)-1))
	if i < 0 {
		return 0
	}
	if i > len(r.pts)-1 {
		return len(r.pts) - 1
	}
	return i
}

// ── stretch engine ──────────────────────────────────────────────────────────

// stretchGeom precomputes everything shared between strand ribbons.
type stretchGeom struct {
	r      resampled
	closed bool
	flip   bool // direction = backward
	sw     float64
	period float64 // texture period (length/sw) for seamless closed noise
	nibRad float64
	lo     float64
	def    brushDef
}

// se returns the direction-aware arc position (asymmetric features flip with direction).
func (g *stretchGeom) se(i int) float64 {
	if g.flip {
		return g.r.length - g.r.s[i]
	}
	return g.r.s[i]
}

// frayRamp is 0 along the stroke and ramps to 1 over the trailing 3sw of arc.
func (g *stretchGeom) frayRamp(i int) float64 {
	if g.closed || g.def.endFray <= 0 {
		return 0
	}
	v := (g.se(i) - (g.r.length - 3*g.sw)) / (3 * g.sw)
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// halfWidth computes the ribbon half-width for strand st at point i, with the
// taper measured inside the dash segment [sA,sB].
func (g *stretchGeom) halfWidth(st brushStrand, salt int64, i int, sA, sB float64) float64 {
	_, _, theta := normalAt(g.r, i, g.closed)
	d := g.def
	nibF := g.lo + (1-g.lo)*math.Abs(math.Cos(theta-g.nibRad))
	u := g.r.s[i] / g.sw
	// two octaves: a single low-frequency channel can come out near-monotonic
	bodyN := 0.62*(pnoise(salt*31+5, u*d.bodyF, g.period*d.bodyF, g.closed)*2-1) +
		0.38*(pnoise(salt*31+6, u*d.bodyF*2.3, g.period*d.bodyF*2.3, g.closed)*2-1)
	body := 1 + d.bodyAmp*1.25*bodyN
	taper := 1.0
	if !g.closed {
		tin, tout := d.taperIn, d.taperOut
		tipIn, tipOut := d.tipIn, d.tipOut
		if g.flip {
			tin, tout = tout, tin
			tipIn, tipOut = tipOut, tipIn
		}
		segIn := math.Min(tin*g.sw, math.Max((sB-sA)/3, 1e-6))
		segOut := math.Min(tout*g.sw, math.Max((sB-sA)/3, 1e-6))
		fIn := math.Min(1, (g.r.s[i]-sA)/segIn)
		fOut := math.Min(1, (sB-g.r.s[i])/segOut)
		taper = (tipIn + (1-tipIn)*fIn) * (tipOut + (1-tipOut)*fOut)
		if taper < 0 {
			taper = 0
		}
	}
	return g.sw * st.w / 2 * nibF * body * taper
}

// edgeBite returns the edge jitter factors (mostly biting inward) for both edges.
func (g *stretchGeom) edgeBite(st brushStrand, salt int64, i int) (lf, rf float64) {
	d := g.def
	jag := d.jagAmp
	if st.jag >= 0 {
		jag = st.jag
	}
	jag *= 1 + 2.2*d.endFray*g.frayRamp(i)
	u := g.r.s[i] / g.sw
	lf, rf = 1.0, 1.0
	if jag > 0 {
		// two octaves so the grit reads ragged, not wavy
		el := 0.62*pnoise(salt*31+11, u*d.jagF, g.period*d.jagF, g.closed) +
			0.38*pnoise(salt*31+15, u*d.jagF*2.9, g.period*d.jagF*2.9, g.closed)
		er := 0.62*pnoise(salt*31+12, u*d.jagF, g.period*d.jagF, g.closed) +
			0.38*pnoise(salt*31+16, u*d.jagF*2.9, g.period*d.jagF*2.9, g.closed)
		lf -= (el*1.5 - 0.35) * jag
		rf -= (er*1.5 - 0.35) * jag
	}
	if d.lumpAmp > 0 {
		ll := pnoise(salt*31+13, u*d.lumpF, g.period*d.lumpF, g.closed)
		lr := pnoise(salt*31+14, u*d.lumpF, g.period*d.lumpF, g.closed)
		lf -= (ll*1.6 - 0.5) * d.lumpAmp
		rf -= (lr*1.6 - 0.5) * d.lumpAmp
	}
	return lf, rf
}

// strandOffset is the centreline offset of strand st at point i.
func (g *stretchGeom) strandOffset(st brushStrand, salt int64, i int) float64 {
	off := st.off * g.sw
	if st.wander > 0 {
		u := g.r.s[i] / g.sw
		wn := 0.68*(pnoise(salt*31+3, u*st.wanderF, g.period*st.wanderF, g.closed)*2-1) +
			0.32*(pnoise(salt*31+4, u*st.wanderF*2.6, g.period*st.wanderF*2.6, g.closed)*2-1)
		off += st.wander * g.sw * wn
	}
	return off
}

// capArc appends a semicircular end cap from edge point a to edge point b,
// bulging in direction (dx,dy) (a unit vector). Pure lerp/trig — ports 1:1.
func capArc(dc *gg.Context, a, b pathPt, dx, dy float64) {
	mx, my := (a.x+b.x)/2, (a.y+b.y)/2
	hx, hy := (a.x-b.x)/2, (a.y-b.y)/2
	rad := math.Hypot(hx, hy)
	if rad < 0.35 {
		return
	}
	for k := 1; k <= 7; k++ {
		al := float64(k) / 8 * math.Pi
		c, s := math.Cos(al), math.Sin(al)
		dc.LineTo(mx+hx*c+dx*rad*s, my+hy*c+dy*rad*s)
	}
}

// ribbonRange emits one filled ribbon polygon (with round caps) over indices [i0,i1].
func ribbonRange(dc *gg.Context, g *stretchGeom, st brushStrand, salt int64, i0, i1 int, sA, sB float64) {
	if i1-i0 < 1 {
		return
	}
	var lefts, rights []pathPt
	for i := i0; i <= i1; i++ {
		nx, ny, _ := normalAt(g.r, i, g.closed)
		off := g.strandOffset(st, salt, i)
		hw := g.halfWidth(st, salt, i, sA, sB)
		lf, rf := g.edgeBite(st, salt, i)
		p := g.r.pts[i]
		lefts = append(lefts, pathPt{p.x + nx*(off+hw*lf), p.y + ny*(off+hw*lf)})
		rights = append(rights, pathPt{p.x + nx*(off-hw*rf), p.y + ny*(off-hw*rf)})
	}
	n := len(lefts)
	dc.MoveTo(lefts[0].x, lefts[0].y)
	for i := 1; i < n; i++ {
		dc.LineTo(lefts[i].x, lefts[i].y)
	}
	nx1, ny1, _ := normalAt(g.r, i1, g.closed)
	capArc(dc, lefts[n-1], rights[n-1], ny1, -nx1) // bulge forward (+tangent)
	for i := n - 1; i >= 0; i-- {
		dc.LineTo(rights[i].x, rights[i].y)
	}
	nx0, ny0, _ := normalAt(g.r, i0, g.closed)
	capArc(dc, rights[0], lefts[0], -ny0, nx0) // bulge backward (-tangent)
	dc.ClosePath()
}

// ribbonClosed emits a full closed ring ribbon (outer ring + inner ring).
func ribbonClosed(dc *gg.Context, g *stretchGeom, st brushStrand, salt int64) {
	n := len(g.r.pts)
	var lefts, rights []pathPt
	for i := 0; i < n; i++ {
		nx, ny, _ := normalAt(g.r, i, true)
		off := g.strandOffset(st, salt, i)
		hw := g.halfWidth(st, salt, i, 0, g.r.length)
		lf, rf := g.edgeBite(st, salt, i)
		p := g.r.pts[i]
		lefts = append(lefts, pathPt{p.x + nx*(off+hw*lf), p.y + ny*(off+hw*lf)})
		rights = append(rights, pathPt{p.x + nx*(off-hw*rf), p.y + ny*(off-hw*rf)})
	}
	dc.MoveTo(lefts[0].x, lefts[0].y)
	for i := 1; i < n; i++ {
		dc.LineTo(lefts[i].x, lefts[i].y)
	}
	dc.ClosePath()
	dc.MoveTo(rights[n-1].x, rights[n-1].y)
	for i := n - 2; i >= 0; i-- {
		dc.LineTo(rights[i].x, rights[i].y)
	}
	dc.ClosePath()
}

// holePoly punches one small rough fleck hole fully inside the strand ribbon at
// arc position s (px) and relative offset v (-1..1). Consumes 2 rng rolls.
func holePoly(dc *gg.Context, g *stretchGeom, st brushStrand, salt int64, rng *brushRng, s, v float64) {
	i := idxFor(g.r, s)
	rx := (0.6 + 0.9*rng.next()) * g.def.holeW * g.sw
	ry := rx * (0.35 + 0.3*rng.next())
	// keep the hole inside the WORST-CASE bitten edge — a hole poking past the
	// ribbon outline would get painted by the even-odd fill, not punched
	jag := g.def.jagAmp
	if st.jag >= 0 {
		jag = st.jag
	}
	bite := 1 - jag*1.15 - g.def.lumpAmp*1.1
	hw := g.halfWidth(st, salt, i, 0, g.r.length) * bite
	if hw < ry*1.6 {
		return
	}
	vv := v * (hw - ry*1.3)
	nx, ny, theta := normalAt(g.r, i, g.closed)
	off := g.strandOffset(st, salt, i)
	cx := g.r.pts[i].x + nx*(off+vv)
	cy := g.r.pts[i].y + ny*(off+vv)
	for k := 0; k <= 7; k++ {
		a := float64(k) / 7 * 2 * math.Pi
		wob := 0.75 + 0.5*hash01(salt*977+int64(i*8+k))
		px := math.Cos(a) * rx * wob
		py := math.Sin(a) * ry * wob
		x := cx + px*math.Cos(theta) - py*math.Sin(theta)
		y := cy + px*math.Sin(theta) + py*math.Cos(theta)
		if k == 0 {
			dc.MoveTo(x, y)
		} else {
			dc.LineTo(x, y)
		}
	}
	dc.ClosePath()
}

// streakPoly punches a long thin dry streak hole along the stroke direction.
func streakPoly(dc *gg.Context, g *stretchGeom, st brushStrand, salt int64, rng *brushRng) {
	n := len(g.r.pts)
	lenS := (2.2 + 3.4*rng.next()) * g.sw
	su := rng.next()
	if g.def.streakBias > 0 {
		su = math.Pow(su, 1+g.def.streakBias)
		if g.flip {
			su = 1 - su
		}
	}
	s0 := su * (g.r.length - lenS)
	v := rng.next()*2 - 1
	th := (0.04 + 0.06*rng.next()) * g.sw
	if s0 < 0 {
		return
	}
	first := true
	var rights []pathPt
	for i := 0; i < n; i++ {
		if g.r.s[i] < s0 || g.r.s[i] > s0+lenS {
			continue
		}
		jag := g.def.jagAmp
		if st.jag >= 0 {
			jag = st.jag
		}
		hw := g.halfWidth(st, salt, i, 0, g.r.length) * (1 - jag*1.15 - g.def.lumpAmp*1.1)
		if hw < th*2 {
			continue
		}
		// fade the streak thickness toward its ends
		f := math.Sin(math.Min(1, math.Max(0, (g.r.s[i]-s0)/lenS)) * math.Pi)
		t2 := th * (0.3 + 0.7*f)
		vv := v * 0.6 * (hw - t2*1.5)
		nx, ny, _ := normalAt(g.r, i, g.closed)
		off := g.strandOffset(st, salt, i)
		p := g.r.pts[i]
		if first {
			dc.MoveTo(p.x+nx*(off+vv+t2), p.y+ny*(off+vv+t2))
			first = false
		} else {
			dc.LineTo(p.x+nx*(off+vv+t2), p.y+ny*(off+vv+t2))
		}
		rights = append(rights, pathPt{p.x + nx*(off+vv-t2), p.y + ny*(off+vv-t2)})
	}
	for i := len(rights) - 1; i >= 0; i-- {
		dc.LineTo(rights[i].x, rights[i].y)
	}
	if !first {
		dc.ClosePath()
	}
}

// brushStretch renders a stretch brush stroke along pts.
func brushStretch(dc *gg.Context, pts []pathPt, l layout.Layer, col color.Color, closed bool) {
	def := brushFor(l.BrushName)
	sw := l.StrokeWidth
	step := math.Max(0.9, math.Min(4, sw*0.16))
	r := resamplePath(pts, closed, step)
	if len(r.pts) < 3 || r.length <= 0 {
		return
	}
	g := &stretchGeom{
		r: r, closed: closed, flip: l.BrushDirection == "backward", sw: sw,
		period: r.length / sw, nibRad: def.nib * math.Pi / 180,
		lo: 1 - def.contrast*0.92, def: def,
	}
	if g.flip {
		g.nibRad = -g.nibRad
	}
	rng := newBrushRng(11)
	type job struct {
		st   brushStrand
		salt int64
		cut0 float64 // arc trimmed off the start (rake fray)
		cut1 float64 // arc trimmed off the end
	}
	var jobs []job
	for k, st := range def.strands {
		jobs = append(jobs, job{st: st, salt: int64(k + 1)})
	}
	if def.rake > 0 {
		// bristle filaments spread across the band, denser/heavier toward the -n
		// (leading) edge, each trimmed to a different length so the tail frays.
		// filament widths grade from wide+overlapping at the -n edge (a merged solid
		// body) down to spaced hairlines at the +n edge, so the rake reads as one
		// stroke drying out — not separate ribbons.
		band := def.weight * 0.82
		for f := 0; f < def.rake; f++ {
			fr := float64(f) / float64(def.rake-1)
			off := -band/2 + band*math.Pow(fr, 1.55) + (rng.next()*2-1)*0.03
			w := 0.03 + 0.17*math.Pow(1-fr, 1.3) + 0.02*rng.next()
			cut := (0.15 + 0.85*fr) * rng.next() * 3.4 * sw
			jobs = append(jobs, job{
				st:   brushStrand{off: off, w: w, wander: 0.035, wanderF: 0.3 + 0.04*float64(f), jag: -1},
				salt: int64(f + 1), cut0: rng.next() * (0.3 + fr) * sw, cut1: cut,
			})
		}
	}
	if len(jobs) == 0 {
		jobs = []job{{st: brushStrand{off: 0, w: def.weight, jag: -1}, salt: 1}}
	}
	dc.SetColor(col)
	dc.SetFillRule(gg.FillRuleEvenOdd)
	n := len(r.pts)
	if def.grain > 0 {
		// charcoal body: no solid ribbon at all — stipple elongated flecks across the
		// band (slightly past the edges), patchy along the stroke like dry media.
		st := jobs[0].st
		cnt := int(def.grain * r.length / sw * 110)
		if cnt > 12000 {
			cnt = 12000
		}
		for h := 0; h < cnt; h++ {
			i := int(rng.next() * float64(n-1))
			patch := rng.next()
			if patch > 0.6+0.4*vnoise(91, r.s[i]/sw*0.7) {
				continue
			}
			hw := g.halfWidth(st, jobs[0].salt, i, 0, r.length)
			if hw <= 0 {
				continue
			}
			v := (rng.next() + rng.next() - 1) * 1.1 * hw
			ll := (0.07 + 0.12*rng.next()) * sw
			lh := (0.02 + 0.032*rng.next()) * sw
			nx, ny, theta := normalAt(r, i, closed)
			rot := theta + (rng.next()*2-1)*0.25
			x := r.pts[i].x + nx*v
			y := r.pts[i].y + ny*v
			cs, sn := math.Cos(rot), math.Sin(rot)
			dc.MoveTo(x-ll*cs+lh*sn, y-ll*sn-lh*cs)
			dc.LineTo(x+ll*cs+lh*sn, y+ll*sn-lh*cs)
			dc.LineTo(x+ll*cs-lh*sn, y+ll*sn+lh*cs)
			dc.LineTo(x-ll*cs-lh*sn, y-ll*sn+lh*cs)
			dc.ClosePath()
			dc.Fill()
		}
		dc.SetFillRule(gg.FillRuleWinding)
		return
	}
	for _, j := range jobs {
		st := j.st
		if st.dashOn > 0 && (st.dashOn+st.dashOff)*sw >= step*2 {
			// broken strand: walk dashes
			s0 := rng.next() * st.dashOff * sw
			for s0 < r.length {
				on := st.dashOn * sw * (0.7 + 0.6*rng.next())
				off := st.dashOff * sw * (0.6 + 0.8*rng.next())
				s1 := math.Min(s0+on, r.length)
				ribbonRange(dc, g, st, j.salt, idxFor(r, s0), idxFor(r, s1), s0, s1)
				s0 = s1 + off
			}
		} else if closed {
			ribbonClosed(dc, g, st, j.salt)
		} else {
			s0, s1 := j.cut0, r.length-j.cut1
			ribbonRange(dc, g, st, j.salt, idxFor(r, s0), idxFor(r, s1), s0, s1)
		}
		// interior holes / streaks (single-strand brushes — texture lives in the body)
		if len(def.strands) == 0 && def.rake == 0 {
			if def.holes > 0 {
				if def.holeCluster {
					nc := int(def.holes*r.length/(6*sw)) + 2
					for c := 0; c < nc; c++ {
						sc := rng.next() * r.length
						vc := (rng.next()*2 - 1) * 0.55
						m := 8 + int(rng.next()*9)
						for h := 0; h < m; h++ {
							s := sc + (rng.next()*2-1)*1.3*sw
							v := vc + (rng.next()*2-1)*0.4
							if s < 0 || s > r.length || v < -1 || v > 1 {
								rng.next()
								rng.next()
								continue
							}
							holePoly(dc, g, st, j.salt, rng, s, v)
						}
					}
				} else {
					cnt := int(def.holes * r.length / sw * 6)
					for h := 0; h < cnt; h++ {
						holePoly(dc, g, st, j.salt, rng, rng.next()*r.length, rng.next()*2-1)
					}
				}
			}
			if def.streaks > 0 {
				cnt := int(def.streaks*r.length/(5*sw)) + 1
				for h := 0; h < cnt; h++ {
					streakPoly(dc, g, st, j.salt, rng)
				}
			}
		}
		dc.Fill()
		// edge fringe specks (drawn additively after the ribbon fill)
		fr := def.fringe
		if st.fringe > 0 {
			fr = st.fringe
		}
		if fr > 0 {
			cnt := int(fr * r.length / sw * 16)
			for h := 0; h < cnt; h++ {
				i := int(rng.next() * float64(n-1))
				side := 1.0
				if rng.next() < 0.5 {
					side = -1
				}
				if st.fringeSide != 0 {
					side = float64(st.fringeSide)
				}
				hw := g.halfWidth(st, j.salt, i, 0, r.length)
				if hw <= 0 {
					rng.next()
					rng.next()
					continue
				}
				spreadF := 0.3
				if st.fringeSide != 0 {
					spreadF = 0.16
				}
				d := hw + (rng.next()*rng.next()*1.4-0.18)*spreadF*sw
				rad := (0.015 + 0.03*rng.next()) * sw
				nx, ny, _ := normalAt(r, i, closed)
				offc := g.strandOffset(st, j.salt, i)
				dc.DrawCircle(r.pts[i].x+nx*(offc+side*d), r.pts[i].y+ny*(offc+side*d), rad)
				dc.Fill()
			}
		}
	}
	// trailing-end fray specks
	if def.endFray > 0 && !closed {
		cnt := int(14 * def.endFray)
		for h := 0; h < cnt; h++ {
			s := r.length - rng.next()*3*sw
			if g.flip {
				s = r.length - s
			}
			i := idxFor(r, s)
			v := (rng.next()*2 - 1) * sw * 0.6
			rad := (0.02 + 0.05*rng.next()) * sw
			nx, ny, _ := normalAt(r, i, closed)
			dc.DrawCircle(r.pts[i].x+nx*v, r.pts[i].y+ny*v, rad)
			dc.Fill()
		}
	}
	dc.SetFillRule(gg.FillRuleWinding)
}

// ── scatter engine ──────────────────────────────────────────────────────────

// brushScatter renders a scatter brush stroke: particle clusters stamped along pts.
func brushScatter(dc *gg.Context, pts []pathPt, l layout.Layer, col color.Color, opacity float64, closed bool) {
	def := brushFor(l.BrushName)
	sw := l.StrokeWidth
	stepMul := def.step
	if l.ScatterGap > 0 {
		stepMul = l.ScatterGap
	}
	step := math.Max(1, stepMul*sw)
	// cap total particle work on very long strokes — same deterministic rule as the
	// TS engine so both renders stay identical
	approxLen := 0.0
	for i := 1; i < len(pts); i++ {
		approxLen += math.Hypot(pts[i].x-pts[i-1].x, pts[i].y-pts[i-1].y)
	}
	if closed && len(pts) > 1 {
		approxLen += math.Hypot(pts[0].x-pts[len(pts)-1].x, pts[0].y-pts[len(pts)-1].y)
	}
	est := approxLen / step * float64(def.count)
	if est > 12000 {
		step *= est / 12000
	}
	r := resamplePath(pts, closed, step)
	if len(r.pts) < 2 {
		return
	}
	wig := (l.ScatterWiggle / 100) * sw * 1.2
	sizeJit := l.ScatterSize / 100
	rotBase := l.ScatterRotation * math.Pi / 180
	angJit := l.ScatterAngular * math.Pi / 180
	cr, cg, cb, _ := col.RGBA()
	rng := newBrushRng(11)
	setA := func(a float64) {
		dc.SetRGBA(float64(cr)/65535, float64(cg)/65535, float64(cb)/65535, a*opacity)
	}
	for i := range r.pts {
		nx, ny, theta := normalAt(r, i, closed)
		tx, ty := ny, -nx // unit tangent
		if def.coreR > 0 {
			setA(def.coreA)
			dc.DrawCircle(r.pts[i].x, r.pts[i].y, def.coreR*sw*(1-sizeJit*0.7*rng.next()))
			dc.Fill()
		}
		for p := 0; p < def.count; p++ {
			var v float64
			if def.uniform {
				v = (rng.next()*2 - 1) * def.spread * sw
			} else {
				v = (rng.next() + rng.next() - 1) * def.spread * sw
			}
			if def.form == "fleck" && p%5 == 4 {
				v *= 2.1 // the occasional fleck flies off the band
			}
			v += (rng.next()*2 - 1) * wig
			along := (rng.next()*2 - 1) * step * 0.6
			x := r.pts[i].x + nx*v + tx*along
			y := r.pts[i].y + ny*v + ty*along
			rad := def.prad * sw * (0.6 + 0.8*rng.next()) * (1 - sizeJit*0.85*rng.next())
			if rad < 0.25 {
				rad = 0.25
			}
			setA(def.alpha)
			switch def.form {
			case "fleck":
				rot := theta + (rng.next()*2-1)*0.9 + rotBase
				if angJit > 0 {
					rot += (rng.next()*2 - 1) * angJit
				}
				lw, lh := rad*1.9, rad*0.7
				cs, sn := math.Cos(rot), math.Sin(rot)
				dc.MoveTo(x-lw*cs+lh*sn, y-lw*sn-lh*cs)
				dc.LineTo(x+lw*cs+lh*sn, y+lw*sn-lh*cs)
				dc.LineTo(x+lw*cs-lh*sn, y+lw*sn+lh*cs)
				dc.LineTo(x-lw*cs-lh*sn, y-lw*sn+lh*cs)
				dc.ClosePath()
				dc.Fill()
			case "blob":
				const bn = 14
				ph := rng.next() * 12
				rot := rotBase
				if angJit > 0 {
					rot += (rng.next()*2 - 1) * angJit
				}
				for k := 0; k <= bn; k++ {
					a := float64(k)/bn*2*math.Pi + rot
					wob := 1 + 0.26*(vnoise(41, ph+float64(k)*0.55)*2-1)
					px := x + math.Cos(a)*rad*wob
					py := y + math.Sin(a)*rad*wob
					if k == 0 {
						dc.MoveTo(px, py)
					} else {
						dc.LineTo(px, py)
					}
				}
				dc.ClosePath()
				dc.Fill()
			default: // dot, disc
				dc.DrawCircle(x, y, rad)
				dc.Fill()
			}
		}
	}
}

// strokeBrush dispatches a brush stroke (stretch or scatter) for a path or outline.
func strokeBrush(dc *gg.Context, pts []pathPt, l layout.Layer, opacity float64, closed bool) {
	if len(pts) < 2 || l.StrokeWidth <= 0 {
		return
	}
	// Brush stamps take ONE tint — the stroke stack's primary colour (gradient
	// brush strokes aren't rasterised; mirrors the web preview's brushOpts).
	base := parseHex(strokePrimary(l), color.White)
	if brushFor(l.BrushName).kind == "scatter" {
		brushScatter(dc, pts, l, base, opacity, closed)
	} else {
		brushStretch(dc, pts, l, withAlpha(base, opacity), closed)
	}
}
