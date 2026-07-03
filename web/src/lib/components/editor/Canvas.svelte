<script lang="ts">
	// The interactive editing canvas — the centrepiece of the layout designer.
	// Paints editor.layout into a DOM stage (background + ordered layers), handles
	// selection, drag-to-move and 8-handle resize via pointer capture. Geometry is
	// stored in canvas pixels; we render layers as percentages so positioning is
	// scale-independent, and only multiply size-like fields (font, ring, radius,
	// blur) by the live scale = renderedWidth / layout.width.
	import { getContext } from 'svelte';
	import { EditorStore, EDITOR_CTX } from '$lib/layout/editor.svelte';
	import { resolveSrc, newLayer, cornerNode, pathD, paintsOf, paintPrimary, strokePaintsOf, strokePrimary, textPaintsOf, bgPaintsOf, stackPrimary, type Layer, type PathNode, type ShapeKind, type Effect, type Paint } from '$lib/layout/schema';
	import { brushStrokeMarkup, dynamicWobble } from '$lib/layout/brushes';
	import { fontCss } from '$lib/layout/fonts';
	import { uploadImage } from '$lib/api';
	import { ImageIcon } from 'lucide-svelte';

	const editor = getContext<EditorStore>(EDITOR_CTX);
	const layout = $derived(editor.layout);
	// Only pay the per-layer CSS-mask cost when the document actually uses masks.
	const hasMasks = $derived(layout.layers.some((l) => l.clip));
	// Likewise for effect (shadow/blur) CSS — only compute it when some layer has any.
	const hasFx = $derived(layout.layers.some((l) => (l.effects?.length ?? 0) > 0));

	// Boolean groups: the visible members (bottom→top) of each group whose metadata
	// carries a bool op, so the canvas can render the whole group as ONE composited
	// shape instead of separate layers.
	const boolGroups = $derived.by(() => {
		const m = new Map<string, Layer[]>();
		for (const l of layout.layers) {
			if (l.hidden) continue;
			const g = l.group;
			if (g && layout.groups?.[g]?.bool_op) {
				const a = m.get(g);
				if (a) a.push(l);
				else m.set(g, [l]);
			}
		}
		// A group with <2 visible members can't composite — its lone survivor draws as
		// a normal layer (mirrors the Go renderer's renderBoolean fallback).
		for (const [g, arr] of m) if (arr.length < 2) m.delete(g);
		return m;
	});
	const boolMemberIds = $derived(new Set([...boolGroups.values()].flat().map((l) => l.id)));
	const boolMembers = (gid: string): Layer[] => boolGroups.get(gid) ?? [];

	// Display helpers: card text/src are pure Go templates, resolved on the server
	// (editor.resolved) so the live canvas matches the bot. Plain (non-template)
	// strings are shown as-is; a template not resolved yet shows its raw form.
	function dtext(l: Layer): string {
		if (l.text && l.text.includes('{{')) return editor.resolved[l.text] ?? l.text;
		return l.text ?? '';
	}
	function dsrc(l: Layer): string {
		if (l.src && l.src.includes('{{')) return editor.resolved[l.src] ?? '';
		return resolveSrc(l.src);
	}

	// Live XP-progress fraction (0..1) from the host's sample vars, so a rect flagged
	// `progress` previews the fill width exactly like the server. Welcome cards inject
	// no {progress}, so the fraction is 1 (full width). Accepts '45%', '45' or '0.45'.
	const progressFrac = $derived.by(() => {
		const raw = editor.extraVars?.['{progress}'];
		if (raw == null || raw === '') return 1;
		let n = parseFloat(String(raw).replace(/[^0-9.\-]/g, ''));
		if (!isFinite(n)) return 1;
		if (n > 1) n /= 100;
		return Math.max(0, Math.min(1, n));
	});

	// Rendered pixel width of the stage; scale maps canvas px → screen px.
	let clientWidth = $state(0);
	const scale = $derived(clientWidth > 0 ? clientWidth / layout.width : 0);

	// Background paint stack for the stage — the SAME model as a layer's fill
	// (bgPaintsOf maps legacy solid/gradient/image documents to one paint). The
	// stage itself shows the renderer's brand-ink base; the stack composites in a
	// dedicated layer below everything, with the optional whole-background blur.
	const bgVisible = $derived(
		bgPaintsOf(layout.background).filter((p) => !p.hidden && (p.opacity ?? 1) > 0)
	);
	const bgCss = $derived(cssBackgroundsOf(bgVisible));

	// Percentage geometry for a layer box — scale-independent.
	function box(l: Layer) {
		return {
			left: `${(l.x / layout.width) * 100}%`,
			top: `${(l.y / layout.height) * 100}%`,
			width: `${(l.w / layout.width) * 100}%`,
			height: `${(l.h / layout.height) * 100}%`
		};
	}

	// ── masking preview: a white stencil shape (in canvas coords) for a mask layer.
	// The exact alpha/luminance compositing happens in the Go renderer; this CSS
	// mask reflects shape masks (rect/ellipse/circle/path) faithfully — with the
	// stencil's own rotation/corner-radius baked via shapeD — and falls back to the
	// mask's bounding box for image/text masks. ────────────────────────────────────
	// ── text-stencil masks ─────────────────────────────────────────────────────
	// Browsers DON'T render <text> inside an SVG used as a CSS mask (the masking
	// pipeline drops SVG text), so a text mask drawn that way falls back to the whole
	// box. Instead a text stencil is rasterised to a PNG on a 2D canvas — which DOES
	// use the document's real card fonts — and that PNG is the mask image, so the mask
	// follows the glyph layout. The Go PNG renderer stays authoritative for the card.
	let maskCanvas: HTMLCanvasElement | null = null;
	// Cache the last rasterised mask CSS per masked-layer id, keyed by a signature of
	// the inputs that actually change the bitmap — so dragging the whole mask group (or
	// zoom/pan) reuses the PNG instead of re-encoding via toDataURL every frame. The
	// signature uses the stencil's offset RELATIVE to the masked element (m.x - cov.x),
	// not absolutes, so a group move (stencil + content shift together) is a cache hit
	// while moving one of them alone correctly re-rasters.
	const textMaskCache = new Map<string, { sig: string; css: string }>();
	// Bumped when web fonts finish loading so a text mask rasterised before its card
	// font was ready (e.g. opening a saved layout) re-renders with the real glyphs.
	let fontTick = $state(0);
	$effect(() => {
		if (typeof document === 'undefined' || !document.fonts) return;
		const bump = () => fontTick++;
		document.fonts.addEventListener('loadingdone', bump);
		document.fonts.ready.then(bump).catch(() => {});
		return () => document.fonts.removeEventListener('loadingdone', bump);
	});
	function stencilCase(s: string, c: Layer['text_case']): string {
		if (c === 'upper') return s.toUpperCase();
		if (c === 'lower') return s.toLowerCase();
		// 'title' = capitalize each word's first letter, leaving the rest untouched —
		// matches CSS text-transform:capitalize and the Go renderer's applyTextCase
		// (not toLowerCase'ing the remainder, so 'iPhone' stays 'IPhone').
		if (c === 'title') return s.replace(/(^|\s)(\S)/g, (_, p, ch) => p + ch.toUpperCase());
		return s;
	}
	// lineW measures a line incl. tracking, using the ctx's already-set font. Count
	// code POINTS (not UTF-16 units) so tracking matches the per-glyph draw loop below
	// and the Go renderer's rune-based lineWidth (astral chars + letter-spacing).
	function lineW(ctx: CanvasRenderingContext2D, t: string, ls: number): number {
		return ctx.measureText(t).width + (ls ? Math.max(0, [...t].length - 1) * ls : 0);
	}
	// stencilPlace wraps a text layer to its box and returns each line positioned at its
	// baseline (canvas coords), honouring align/valign/line-height/letter-spacing/case —
	// the SAME layout as the visible preview text and the Go renderer. Shared by the mask
	// rasteriser (drawStencilText) and the boolean-op silhouette (textSvg) via a
	// measure(text)→width fn so both agree on wrapping.
	function stencilPlace(m: Layer, measure: (t: string) => number) {
		const size = m.font_size ?? 16;
		const ls = m.letter_spacing ?? 0;
		const raw = stencilCase(dtext(m), m.text_case);
		const lines: string[] = [];
		for (const para of raw.split('\n')) {
			let cur = '';
			for (const word of para.split(' ')) {
				const next = cur ? `${cur} ${word}` : word;
				if (cur && measure(next) > m.w) {
					lines.push(cur);
					cur = word;
				} else cur = next;
			}
			lines.push(cur);
		}
		const lh = (m.line_height && m.line_height > 0 ? m.line_height : 1.3) * size;
		const block = lh * lines.length;
		let top = m.y; // 'top' / unset
		if (m.valign === 'middle') top = m.y + (m.h - block) / 2;
		else if (m.valign === 'bottom') top = m.y + m.h - block;
		// Baseline within the line box: centre the em box (height = size) in the line box
		// (height = lh, the CSS half-leading), then drop ≈ 0.8em to the baseline.
		const asc = (lh - size) / 2 + size * 0.8;
		const placed: { text: string; x: number; y: number; w: number }[] = [];
		lines.forEach((line, i) => {
			if (!line) return;
			const w = measure(line);
			let x = m.x;
			if (m.align === 'center') x = m.x + (m.w - w) / 2;
			else if (m.align === 'right') x = m.x + m.w - w;
			placed.push({ text: line, x, y: top + i * lh + asc, w });
		});
		return { size, ls, placed };
	}
	// drawStencilText paints a text stencil's glyphs onto ctx in canvas coords, in `fill`
	// (white for alpha/vector; the stencil's real colour for luminance, so a dark stencil
	// hides). `erase` uses destination-out (inverted mask: punch the glyphs out of a box).
	function drawStencilText(ctx: CanvasRenderingContext2D, m: Layer, erase: boolean, fill = '#fff') {
		const size = m.font_size ?? 16;
		const weight = m.font_weight ?? 400;
		const ls = m.letter_spacing ?? 0;
		ctx.font = `${weight} ${size}px ${fontCss(m.font_family)}`;
		ctx.textBaseline = 'alphabetic';
		ctx.fillStyle = fill;
		ctx.globalCompositeOperation = erase ? 'destination-out' : 'source-over';
		const thick = Math.max(1, size * 0.06);
		const { placed } = stencilPlace(m, (t) => lineW(ctx, t, ls));
		for (const p of placed) {
			if (ls) {
				let cx = p.x;
				for (const ch of p.text) {
					ctx.fillText(ch, cx, p.y);
					cx += ctx.measureText(ch).width + ls;
				}
			} else {
				ctx.fillText(p.text, p.x, p.y);
			}
			if (m.text_decoration === 'underline' || m.text_decoration === 'strike') {
				const dy = m.text_decoration === 'strike' ? p.y - size * 0.28 : p.y + thick;
				ctx.fillRect(p.x, dy, p.w, thick);
			}
		}
	}
	// escapeXml escapes content for inline SVG markup. The boolean-op SVG is rendered
	// IN-DOCUMENT (not a data-URI mask), so <text> renders and uses the real card fonts.
	function escapeXml(s: string): string {
		return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
	}
	let measureCv: CanvasRenderingContext2D | null = null; // lazy ctx for SVG-text wrap
	// textSvg builds a text layer's silhouette as inline SVG <text> lines (canvas coords),
	// so a text member of a boolean op composites by its GLYPHS. Rotation goes on each
	// <text> (not a wrapping <g>) so it stays valid as a clipPath/<mask> child.
	function textSvg(m: Layer, fill: string): string {
		void fontTick; // re-wrap once fonts load (glyphs repaint on their own)
		const size = m.font_size ?? 16;
		const weight = m.font_weight ?? 400;
		const family = fontCss(m.font_family);
		const ls = m.letter_spacing ?? 0;
		if (typeof document !== 'undefined' && !measureCv) measureCv = document.createElement('canvas').getContext('2d');
		if (measureCv) measureCv.font = `${weight} ${size}px ${family}`;
		const measure = (t: string) => (measureCv ? lineW(measureCv, t, ls) : [...t].length * 0.55 * size);
		const { placed } = stencilPlace(m, measure);
		const fam = escapeXml(family);
		const lsAttr = ls ? ` letter-spacing='${ls}'` : '';
		const dec =
			m.text_decoration === 'underline'
				? " text-decoration='underline'"
				: m.text_decoration === 'strike'
					? " text-decoration='line-through'"
					: '';
		const rot = m.rotation ? ` transform='rotate(${m.rotation} ${m.x + m.w / 2} ${m.y + m.h / 2})'` : '';
		return placed
			.map(
				(p) =>
					`<text x='${p.x}' y='${p.y}' font-family="${fam}" font-size='${size}' font-weight='${weight}' fill='${fill}'${lsAttr}${dec}${rot} xml:space='preserve'>${escapeXml(p.text)}</text>`
			)
			.join('');
	}
	// silRaw returns a boolean member's silhouette as inline SVG markup — a <path> for
	// shapes, <text> lines for text — so a text member composites by its glyphs (Figma),
	// not its box. Valid as a clipPath/<mask> child and in normal flow.
	function silRaw(m: Layer, fill: string): string {
		if (m.type === 'text') return textSvg(m, fill);
		const d = shapeD(m);
		return d ? `<path d='${d}' fill='${fill}'/>` : '';
	}
	// textMaskCss rasterises a text stencil to a PNG and returns the mask CSS for the
	// masked layer `l`. `stage` picks the coord frame (whole canvas for a path-layer
	// SVG, else the layer's box), mirroring maskCss. The canvas is capped so a
	// full-stage mask can't allocate a huge bitmap (mask-size stretches it back).
	function textMaskCss(l: Layer, m: Layer, stage: boolean): string {
		if (typeof document === 'undefined') return '';
		const cov = stage
			? { x: 0, y: 0, w: layout.width, h: layout.height }
			: { x: l.x, y: l.y, w: l.w, h: l.h };
		if (cov.w <= 0 || cov.h <= 0) return '';
		// Reading every raster input here both (a) registers the reactive deps so the
		// style recomputes when any of them changes, and (b) forms the cache key. fontTick
		// re-rasters once fonts load; the relative offset makes a group move a cache hit.
		const sig = [
			stage ? 1 : 0,
			m.clip_invert ? 1 : 0,
			m.clip_mode ?? '',
			m.color ?? '',
			fontTick,
			dtext(m),
			m.text_case ?? '',
			m.text_decoration ?? '',
			m.align ?? '',
			m.valign ?? '',
			m.font_size ?? 16,
			m.font_weight ?? 400,
			m.font_family ?? '',
			m.letter_spacing ?? 0,
			m.line_height ?? 0,
			Math.round(m.w),
			Math.round(m.h),
			Math.round(m.x - cov.x),
			Math.round(m.y - cov.y),
			Math.round(cov.w),
			Math.round(cov.h),
			Math.round((m.rotation ?? 0) * 100),
			Math.round((!stage && l.rotation ? l.rotation : 0) * 100)
		].join('\u001f');
		const cached = textMaskCache.get(l.id);
		if (cached && cached.sig === sig) return cached.css;
		const r = Math.min(1, 1600 / Math.max(cov.w, cov.h));
		const cw = Math.max(1, Math.round(cov.w * r));
		const ch = Math.max(1, Math.round(cov.h * r));
		if (!maskCanvas) maskCanvas = document.createElement('canvas');
		const cv = maskCanvas;
		cv.width = cw;
		cv.height = ch;
		const ctx = cv.getContext('2d');
		if (!ctx) return '';
		ctx.clearRect(0, 0, cw, ch);
		const invert = !!m.clip_invert;
		// Luminance mode reads the glyphs' BRIGHTNESS, so paint them in the stencil's real
		// colour (a dark stencil reveals faintly/not at all) and emit mask-mode:luminance —
		// matching Go's applyMask. alpha/vector use white glyphs + alpha. (Invert paints a
		// white box then erases the glyphs, so its colour is irrelevant.)
		const lum = m.clip_mode === 'luminance';
		const glyphFill = !invert && lum ? m.color || '#fff' : '#fff';
		const mode = lum ? 'luminance' : 'alpha';
		if (invert) {
			ctx.fillStyle = '#fff'; // white box; the glyphs get punched out below
			ctx.fillRect(0, 0, cw, ch);
		}
		ctx.save();
		ctx.scale(r, r);
		ctx.translate(-cov.x, -cov.y);
		// A `.layer` div carries transform:rotate(); the mask rotates with it, so
		// counter-rotate the stencil to keep it fixed in canvas space (mirrors maskCss).
		if (!stage && l.rotation) {
			const cx = l.x + l.w / 2;
			const cy = l.y + l.h / 2;
			ctx.translate(cx, cy);
			ctx.rotate((-l.rotation * Math.PI) / 180);
			ctx.translate(-cx, -cy);
		}
		if (m.rotation) {
			const cx = m.x + m.w / 2;
			const cy = m.y + m.h / 2;
			ctx.translate(cx, cy);
			ctx.rotate((m.rotation * Math.PI) / 180);
			ctx.translate(-cx, -cy);
		}
		drawStencilText(ctx, m, invert, glyphFill);
		ctx.restore();
		let url: string;
		try {
			url = cv.toDataURL();
		} catch {
			return '';
		}
		const u = `url("${url}")`;
		const css = `-webkit-mask:${u} center/100% 100% no-repeat; mask:${u} center/100% 100% no-repeat; -webkit-mask-mode:${mode}; mask-mode:${mode};`;
		if (textMaskCache.size > 64) textMaskCache.clear(); // bound: ids churn across edits
		textMaskCache.set(l.id, { sig, css });
		return css;
	}

	// stencilRadii mirrors the Go renderer's cornerRadii: independent [tl,tr,br,bl]
	// when corners[] is set, else the uniform radius on all four corners.
	function stencilRadii(m: Layer): [number, number, number, number] {
		const c = m.corners;
		if (Array.isArray(c) && c.length === 4) return [c[0], c[1], c[2], c[3]];
		const r = m.radius ?? 0;
		return [r, r, r, r];
	}
	// roundRectEl builds a rounded-rect stencil path (canvas coords) with independent
	// corner radii AND the stencil's own rotation baked in — matching drawRoundRect in
	// the Go renderer (shapeD's rect path only handles a single uniform radius).
	function roundRectEl(m: Layer, fill: string, radii: [number, number, number, number]): string {
		const { x, y, w, h } = m;
		const cx = x + w / 2;
		const cy = y + h / 2;
		const deg = m.rotation ?? 0;
		const a = (deg * Math.PI) / 180;
		const sin = Math.sin(a);
		const cos = Math.cos(a);
		const P = (px: number, py: number) => rotPair(px, py, cx, cy, sin, cos);
		const lim = Math.min(w, h) / 2;
		const tl = Math.max(0, Math.min(radii[0], lim));
		const tr = Math.max(0, Math.min(radii[1], lim));
		const br = Math.max(0, Math.min(radii[2], lim));
		const bl = Math.max(0, Math.min(radii[3], lim));
		if (tl <= 0 && tr <= 0 && br <= 0 && bl <= 0) {
			return `<path d='M ${P(x, y)} L ${P(x + w, y)} L ${P(x + w, y + h)} L ${P(x, y + h)} Z' fill='${fill}'/>`;
		}
		const d =
			`M ${P(x + tl, y)} L ${P(x + w - tr, y)} A ${tr} ${tr} ${deg} 0 1 ${P(x + w, y + tr)}` +
			` L ${P(x + w, y + h - br)} A ${br} ${br} ${deg} 0 1 ${P(x + w - br, y + h)}` +
			` L ${P(x + bl, y + h)} A ${bl} ${bl} ${deg} 0 1 ${P(x, y + h - bl)}` +
			` L ${P(x, y + tl)} A ${tl} ${tl} ${deg} 0 1 ${P(x + tl, y)} Z`;
		return `<path d='${d}' fill='${fill}'/>`;
	}

	// strokeGrow is how far a stencil's silhouette extends past its shape edge: the full
	// ring width PLUS half the stroke (Go paints both into the stencil coverage, and the
	// stroke is centered on the edge so half sits outside), so the mask reveal reaches the
	// same outer edge as the Go renderer.
	function strokeGrow(m: Layer): number {
		// How far the stroke extends past the shape edge (mirrors Go's strokeInset): inside
		// adds nothing, center adds half, outside adds the full width. Plus the legacy ring
		// (always fully outside).
		const sw = Math.max(0, m.stroke_width ?? 0);
		const align = m.stroke_align ?? 'center';
		return align === 'inside' ? 0 : align === 'outside' ? sw : sw / 2;
	}
	// ringRoundRect builds a rounded-rect stencil, GROWN by strokeGrow (radii too), so a
	// ringed/stroked avatar/image masks to its OUTER edge like the Go renderer.
	function ringRoundRect(m: Layer, fill: string, radii: [number, number, number, number]): string {
		const grow = strokeGrow(m);
		if (grow <= 0) return roundRectEl(m, fill, radii);
		const em = { ...m, x: m.x - grow, y: m.y - grow, w: m.w + 2 * grow, h: m.h + 2 * grow } as Layer;
		const grown = radii.map((c) => (c > 0 ? c + grow : 0)) as [number, number, number, number];
		return roundRectEl(em, fill, grown);
	}

	function shapeEl(m: Layer, fill: string): string {
		// (A text stencil never reaches here — maskCss rasterises it via textMaskCss,
		// because browsers don't render SVG <text> in the CSS masking pipeline.)
		// A stroked stencil masks to its OUTER edge (ringRoundRect grows the image box below).
		// rect / ellipse / path stencils → one path with rotation + corner radius baked
		// in (shapeD), matching the Go renderer's drawSilhouette / clip shape.
		if (m.type === 'rect' || m.type === 'ellipse' || m.type === 'path') {
			const d = shapeD(m);
			return d ? `<path d='${d}' fill='${fill}'/>` : '';
		}
		// image stencil with no explicit mask → its (rounded) bounding box, with the
		// stencil's corners + rotation (and ring) baked in (parity with the rect/path stencils).
		return ringRoundRect(m, fill, stencilRadii(m));
	}
	// maskCss builds the per-layer CSS mask. `stage` picks the coordinate frame of the
	// masked ELEMENT: a `.layer` div is positioned at the layer's box (viewBox = that
	// box), but a `.path-layer` SVG spans the whole stage (viewBox = the canvas) — using
	// the wrong one stretches the stencil across the stage (the bug that broke shape
	// masks). clip_mode maps to mask-mode; a rotated div's mask is counter-rotated so
	// the stencil stays fixed in canvas space like the renderer.
	function maskCss(l: Layer, stage = false): string {
		const m = editor.maskFor(l);
		if (!m) return '';
		// A text stencil is rasterised (browsers don't mask with SVG <text>).
		if (m.type === 'text') return textMaskCss(l, m, stage);
		// Luminance mode reads the mask's BRIGHTNESS, so a (non-inverted) shape stencil must
		// carry its real fill colour — a dark fill hides, white reveals — to match Go's
		// applyMask. alpha/vector ignore colour (always white). Images have no solid fill,
		// so they fall back to white (their per-pixel brightness can't be a solid shape).
		const lumFill = !m.clip_invert && m.clip_mode === 'luminance' ? paintPrimary(m) || '#fff' : '#fff';
		const shape = shapeEl(m, lumFill);
		if (!shape) return '';
		const vb = stage ? `0 0 ${layout.width} ${layout.height}` : `${l.x} ${l.y} ${l.w} ${l.h}`;
		const cov = stage
			? { x: 0, y: 0, w: layout.width, h: layout.height }
			: { x: l.x, y: l.y, w: l.w, h: l.h };
		// Inverted: white everywhere except the shape (a stencil with a hole).
		const body = m.clip_invert
			? `<defs><mask id='im'><rect x='${cov.x}' y='${cov.y}' width='${cov.w}' height='${cov.h}' fill='#fff'/>${shapeEl(m, '#000')}</mask></defs><rect x='${cov.x}' y='${cov.y}' width='${cov.w}' height='${cov.h}' fill='#fff' mask='url(#im)'/>`
			: shape;
		// A `.layer` div carries transform:rotate(); undo it on the stencil so the mask
		// doesn't spin with the content. Full-stage path SVGs aren't rotated as a whole.
		const inner =
			!stage && l.rotation
				? `<g transform='rotate(${-l.rotation} ${l.x + l.w / 2} ${l.y + l.h / 2})'>${body}</g>`
				: body;
		const svg = `<svg xmlns='http://www.w3.org/2000/svg' viewBox='${vb}' preserveAspectRatio='none'>${inner}</svg>`;
		const url = `url("data:image/svg+xml,${encodeURIComponent(svg)}")`;
		const mode = m.clip_mode === 'luminance' ? 'luminance' : 'alpha';
		return `-webkit-mask:${url} center/100% 100% no-repeat; mask:${url} center/100% 100% no-repeat; -webkit-mask-mode:${mode}; mask-mode:${mode};`;
	}

	// ── fill paints preview (Figma's paint stack) ───────────────────────────────
	// Box layers composite their paints as CSS multi-backgrounds; vector paths get
	// SVG paint servers (defs + one <path> per paint). Diamond gradients (and
	// angular on paths) preview as radial — the PNG renderer is authoritative.
	function visiblePaints(l: Layer): Paint[] {
		return paintsOf(l).filter((p) => !p.hidden && (p.opacity ?? 1) > 0);
	}
	function cssStops(p: Paint): string {
		const op = p.opacity ?? 1;
		return (p.stops ?? [])
			.map((st) => `${rgba(st.color, (st.alpha ?? 1) * op)} ${(st.pos * 100).toFixed(1)}%`)
			.join(', ');
	}
	// cssPaintLayer: one CSS background layer (top paint listed FIRST in CSS).
	function cssPaintLayer(p: Paint): { img: string; size: string; rep: string } | null {
		const op = p.opacity ?? 1;
		switch (p.type) {
			case 'solid': {
				if (!p.color) return null;
				const c = rgba(p.color, op);
				return { img: `linear-gradient(${c}, ${c})`, size: '100% 100%', rep: 'no-repeat' };
			}
			case 'linear':
				return { img: `linear-gradient(${p.angle ?? 180}deg, ${cssStops(p)})`, size: '100% 100%', rep: 'no-repeat' };
			case 'radial':
			case 'diamond': // diamond previews as radial; the PNG renders it exactly
				return { img: `radial-gradient(closest-side, ${cssStops(p)})`, size: '100% 100%', rep: 'no-repeat' };
			case 'angular':
				return { img: `conic-gradient(from ${p.angle ?? 0}deg at 50% 50%, ${cssStops(p)})`, size: '100% 100%', rep: 'no-repeat' };
			case 'image': {
				const src = resolveSrc(p.src);
				if (!src) return null;
				// per-paint opacity on a CSS background image isn't expressible — preview at
				// full opacity; the PNG honours it.
				if (p.fit === 'tile') return { img: `url("${src}")`, size: `${256 * scale}px auto`, rep: 'repeat' };
				return { img: `url("${src}")`, size: p.fit === 'contain' ? 'contain' : 'cover', rep: 'no-repeat' };
			}
		}
		return null;
	}
	// cssBackgroundsOf: the full CSS background declaration for a paint stack
	// ('' = nothing visible). Shared by box layers, gradient text and the canvas
	// background.
	function cssBackgroundsOf(ps: Paint[]): string {
		const layers = ps.map(cssPaintLayer).filter(Boolean) as { img: string; size: string; rep: string }[];
		if (!layers.length) return '';
		layers.reverse(); // CSS lists the TOP background layer first
		return `background-image:${layers.map((x) => x.img).join(', ')}; background-size:${layers.map((x) => x.size).join(', ')}; background-repeat:${layers.map((x) => x.rep).join(', ')}; background-position:center;`;
	}
	// cssBackgrounds: a box layer's fill stack as CSS backgrounds ('' = no fill).
	function cssBackgrounds(l: Layer): string {
		return cssBackgroundsOf(visiblePaints(l));
	}

	// ── stroke paints preview (Figma's stroke stack) ────────────────────────────
	// A single visible SOLID stroke paint keeps the cheap previews (box-shadow,
	// plain SVG stroke attrs). Anything richer — a gradient/image paint, or
	// multiple paints on one stroke — renders through SVG paint servers instead.
	function visibleStrokePaints(l: Layer): Paint[] {
		return strokePaintsOf(l).filter((p) => !p.hidden && (p.opacity ?? 1) > 0);
	}
	// flatStroke: the single CSS colour of a flat stroke stack; '' = nothing
	// visible; null = the stack needs the SVG paint-server overlay.
	function flatStroke(l: Layer): string | null {
		const ps = visibleStrokePaints(l);
		if (!ps.length) return '';
		if (ps.length > 1 || ps[0].type !== 'solid') return null;
		const op = ps[0].opacity ?? 1;
		const c = ps[0].color ?? '#fff';
		return op >= 1 ? c : rgba(c, op);
	}
	// strokeCss: ONE representative stroke colour for single-colour consumers
	// (text outlines, brush tints, arrowhead markers) — mirrors the Go renderer's
	// strokePrimary approximation.
	function strokeCss(l: Layer): string {
		return strokePrimary(l) || '#fff';
	}
	// svgStrokePaint: paint-server defs + the stroke value for ONE paint on an
	// SVG shape. Gradients use the shape's object bounding box; `bbox` (in the
	// SVG's own coordinates) anchors image patterns. Angular/diamond preview as
	// radial (no SVG conic) — the PNG renderer is exact.
	function svgStrokePaint(
		id: string,
		p: Paint,
		bbox: { x: number; y: number; w: number; h: number }
	): { defs: string; stroke: string; opacity: number } | null {
		const op = p.opacity ?? 1;
		switch (p.type) {
			case 'solid':
				return p.color ? { defs: '', stroke: rgba(p.color, op), opacity: 1 } : null;
			case 'linear': {
				const a = ((p.angle ?? 180) * Math.PI) / 180;
				const dx = Math.sin(a) / 2;
				const dy = -Math.cos(a) / 2;
				return {
					defs: `<linearGradient id='${id}' x1='${0.5 - dx}' y1='${0.5 - dy}' x2='${0.5 + dx}' y2='${0.5 + dy}'>${svgStops(p)}</linearGradient>`,
					stroke: `url(#${id})`,
					opacity: 1
				};
			}
			case 'radial':
			case 'angular':
			case 'diamond':
				return {
					defs: `<radialGradient id='${id}' cx='0.5' cy='0.5' r='0.5'>${svgStops(p)}</radialGradient>`,
					stroke: `url(#${id})`,
					opacity: 1
				};
			case 'image': {
				const src = resolveSrc(p.src);
				if (!src) return null;
				const tile = p.fit === 'tile';
				const pw = tile ? 256 : bbox.w;
				const ph = tile ? 256 : bbox.h;
				const par = p.fit === 'contain' ? 'xMidYMid meet' : 'xMidYMid slice';
				return {
					defs: `<pattern id='${id}' x='${bbox.x}' y='${bbox.y}' width='${pw}' height='${ph}' patternUnits='userSpaceOnUse'><image href='${src}' width='${pw}' height='${ph}' preserveAspectRatio='${par}'/></pattern>`,
					stroke: `url(#${id})`,
					opacity: op
				};
			}
		}
		return null;
	}
	// strokePaintLayers: the per-paint <g stroke=…> bodies + defs for an SVG
	// shape string — the shared core of the box/sides/wobble stroke overlays.
	function strokePaintLayers(l: Layer, idBase: string, shape: string, bbox: { x: number; y: number; w: number; h: number }): { defs: string; body: string } {
		let defs = '';
		let body = '';
		visibleStrokePaints(l).forEach((p, i) => {
			const sp = svgStrokePaint(`${idBase}_${i}`, p, bbox);
			if (!sp) return;
			defs += sp.defs;
			body += `<g stroke='${sp.stroke}'${sp.opacity < 1 ? ` opacity='${sp.opacity}'` : ''}>${shape}</g>`;
		});
		return { defs, body };
	}
	// pathFillMarkup: SVG defs + stacked fill paths for a vector path's paint stack.
	// Drawn bottom→top before the stroke path; the stroke path keeps the pointer hits.
	function pathFillMarkup(l: Layer, d: string): string {
		const ps = visiblePaints(l);
		if (!ps.length || (l.nodes?.length ?? 0) < 3) return '';
		let defs = '';
		let paths = '';
		ps.forEach((p, i) => {
			const id = `pf_${l.id}_${i}`;
			const op = p.opacity ?? 1;
			let fill = '';
			if (p.type === 'solid') {
				if (!p.color) return;
				fill = rgba(p.color, op);
			} else if (p.type === 'linear') {
				const a = ((p.angle ?? 180) * Math.PI) / 180;
				const dx = Math.sin(a) / 2;
				const dy = -Math.cos(a) / 2;
				defs += `<linearGradient id='${id}' x1='${0.5 - dx}' y1='${0.5 - dy}' x2='${0.5 + dx}' y2='${0.5 + dy}'>${svgStops(p)}</linearGradient>`;
				fill = `url(#${id})`;
			} else if (p.type === 'radial' || p.type === 'angular' || p.type === 'diamond') {
				// angular/diamond preview as radial on paths (no SVG conic) — PNG is exact
				defs += `<radialGradient id='${id}' cx='0.5' cy='0.5' r='0.5'>${svgStops(p)}</radialGradient>`;
				fill = `url(#${id})`;
			} else if (p.type === 'image') {
				const src = resolveSrc(p.src);
				if (!src) return;
				const tile = p.fit === 'tile';
				const pw = tile ? 256 : l.w;
				const ph = tile ? 256 : l.h;
				const par = p.fit === 'contain' ? 'xMidYMid meet' : 'xMidYMid slice';
				defs += `<pattern id='${id}' x='${l.x}' y='${l.y}' width='${pw}' height='${ph}' patternUnits='userSpaceOnUse'><image href='${src}' width='${pw}' height='${ph}' preserveAspectRatio='${par}'/></pattern>`;
				fill = `url(#${id})`;
				paths += `<path d='${d}' fill='${fill}' opacity='${op}' pointer-events='none'/>`;
				return;
			}
			if (fill) paths += `<path d='${d}' fill='${fill}' pointer-events='none'/>`;
		});
		if (!paths) return '';
		return `${defs ? `<defs>${defs}</defs>` : ''}${paths}`;
	}
	// pathStrokeMarkup: SVG defs + stacked stroke paths for a vector path's stroke
	// stack — used when the stack isn't one flat colour (the interactive <path>
	// then strokes transparent and only keeps the pointer hits / markers).
	function pathStrokeMarkup(l: Layer, d: string): string {
		const sw = l.stroke_width ?? 0;
		if (l.clip || sw <= 0 || l.brush_name) return '';
		if (flatStroke(l) !== null) return '';
		const cap = l.stroke_cap ?? 'round';
		const join = l.stroke_join === 'miter' ? 'bevel' : (l.stroke_join ?? 'round');
		const dash = strokeDash(l);
		let defs = '';
		let body = '';
		visibleStrokePaints(l).forEach((p, i) => {
			const sp = svgStrokePaint(`pstk_${l.id}_${i}`, p, { x: l.x, y: l.y, w: l.w, h: l.h });
			if (!sp) return;
			defs += sp.defs;
			body += `<path d='${d}' fill='none' stroke='${sp.stroke}' stroke-width='${sw}' stroke-linecap='${cap}' stroke-linejoin='${join}'${dash ? ` stroke-dasharray='${dash}'` : ''}${sp.opacity < 1 ? ` opacity='${sp.opacity}'` : ''} pointer-events='none'/>`;
		});
		if (!body) return '';
		return `${defs ? `<defs>${defs}</defs>` : ''}${body}`;
	}
	function svgStops(p: Paint): string {
		const op = p.opacity ?? 1;
		return (p.stops ?? [])
			.map(
				(st) =>
					`<stop offset='${(st.pos * 100).toFixed(1)}%' stop-color='${st.color}' stop-opacity='${((st.alpha ?? 1) * op).toFixed(3)}'/>`
			)
			.join('');
	}

	// ── effect (shadow / blur) preview ──────────────────────────────────────────
	// Mirror the Go renderer's effects with CSS. Blur radii are kept 1:1 with the
	// renderer (× the live scale so on-screen px match the rendered px), exactly like
	// the background-image blur. The PNG is authoritative where CSS can't match (e.g.
	// CSS drop-shadow has no spread, and inset shadows only show on box-like layers).
	function fxList(l: Layer): Effect[] {
		return (l.effects ?? []).filter((e) => !e.hidden);
	}
	function rgba(hex: string | undefined, op: number | undefined): string {
		let h = (hex ?? '#000000').replace('#', '');
		if (h.length === 3) h = h.split('').map((c) => c + c).join('');
		let a = op ?? 0.25;
		// an #RRGGBBAA hex (the colour picker's alpha) folds into the opacity
		if (h.length === 8) a *= (parseInt(h.slice(6, 8), 16) || 0) / 255;
		h = h.padEnd(6, '0').slice(0, 6);
		const r = parseInt(h.slice(0, 2), 16) || 0;
		const g = parseInt(h.slice(2, 4), 16) || 0;
		const b = parseInt(h.slice(4, 6), 16) || 0;
		return `rgba(${r},${g},${b},${a})`;
	}
	// filter: layer blur + (for text/paths) drop shadows that follow the layer's alpha.
	// Box layers take their drop shadows via fxDrops/box-shadow instead — withShadows=false
	// — because Chrome's GPU compositor mis-renders a filter that changes under the
	// overlapping selection chrome (rectangular shadows clipped to the layer box).
	// Spread is folded into the blur as a coarse approximation.
	function fxFilter(l: Layer, withShadows = true): string {
		const parts: string[] = [];
		for (const e of fxList(l)) {
			if (e.type === 'layer_blur' && (e.radius ?? 0) > 0) parts.push(`blur(${(e.radius ?? 0) * scale}px)`);
		}
		for (const e of fxList(l)) {
			if (e.type !== 'drop_shadow' || !withShadows) continue;
			const r = Math.max(0, (e.radius ?? 0) + Math.max(0, e.spread ?? 0)) * scale;
			parts.push(`drop-shadow(${(e.x ?? 0) * scale}px ${(e.y ?? 0) * scale}px ${r}px ${rgba(e.color, e.opacity)})`);
		}
		return parts.length ? `filter:${parts.join(' ')};` : '';
	}
	// backdrop-filter: a background-blur effect (frosted glass; needs a translucent fill).
	function fxBackdrop(l: Layer): string {
		const e = fxList(l).find((x) => x.type === 'background_blur' && (x.radius ?? 0) > 0);
		if (!e) return '';
		const r = (e.radius ?? 0) * scale;
		return `-webkit-backdrop-filter:blur(${r}px); backdrop-filter:blur(${r}px);`;
	}
	// inset box-shadow(s) for inner-shadow effects — merged into a box layer's existing
	// stroke/ring box-shadow. Box-like layers only (rect/ellipse/image); the PNG covers
	// text/paths, which CSS can't inset-shadow.
	function fxInset(l: Layer): string[] {
		return fxList(l)
			.filter((e) => e.type === 'inner_shadow')
			.map(
				(e) =>
					`inset ${(e.x ?? 0) * scale}px ${(e.y ?? 0) * scale}px ${(e.radius ?? 0) * scale}px ${(e.spread ?? 0) * scale}px ${rgba(e.color, e.opacity)}`
			);
	}
	// outer drop shadows as box-shadow entries — box layers (rect/ellipse/image) only.
	// box-shadow follows the border-radius (a circle gets an elliptical shadow), supports
	// real spread, and avoids CSS-filter compositing entirely (see fxFilter).
	function fxDrops(l: Layer): string[] {
		return fxList(l)
			.filter((e) => e.type === 'drop_shadow')
			.map(
				(e) =>
					`${(e.x ?? 0) * scale}px ${(e.y ?? 0) * scale}px ${(e.radius ?? 0) * scale}px ${(e.spread ?? 0) * scale}px ${rgba(e.color, e.opacity)}`
			);
	}
	// boxShadow joins a layer's stroke/ring shadow with its inner-shadow effects, and —
	// when `drops` (the layer renders an opaque box, so its drop shadows come through
	// box-shadow, not filter) — the outer drop shadows LAST so the ring paints above them.
	// Inner shadows are listed FIRST so they paint OVER the stroke/ring (CSS box-shadow
	// paints the first entry on top), matching the Go draw order (content+stroke, then
	// inner shadows). A stencil (l.clip) is never composited, so it carries no effects.
	function boxShadow(l: Layer, base: string, drops = false): string {
		const fx = hasFx && !l.clip;
		const parts = [...(fx ? fxInset(l) : []), base, ...(fx && drops ? fxDrops(l) : [])].filter(Boolean);
		return parts.length ? `box-shadow:${parts.join(', ')};` : '';
	}
	// strokeShadow renders a FLAT (single solid paint) stroke as a box-shadow
	// honouring its Position (Figma stroke align): inside = inset, outside =
	// outset, center = half each. Follows border-radius. `sw` is the already-
	// scaled width. '' for no stroke or a stack that boxStrokeSvg paints instead.
	function strokeShadow(l: Layer, sw: number): string {
		if (sw <= 0) return '';
		const c = flatStroke(l);
		if (!c) return '';
		const align = l.stroke_align ?? 'center';
		if (align === 'inside') return `inset 0 0 0 ${sw}px ${c}`;
		if (align === 'outside') return `0 0 0 ${sw}px ${c}`;
		const h = sw / 2;
		return `0 0 0 ${h}px ${c}, inset 0 0 0 ${h}px ${c}`;
	}
	// strokeDash returns the stroke-dasharray (the `sw`-scaled dash/gap lengths) for a
	// dashed stroke, else '' (solid). An all-zero pattern reads as solid.
	function strokeDash(l: Layer, k = 1): string {
		if (l.stroke_style !== 'dashed') return '';
		const d = Math.max(0, l.dash ?? 0) * k;
		const g = Math.max(0, l.gap ?? 0) * k;
		return d > 0 || g > 0 ? `${d} ${g}` : '';
	}
	// boxStrokeSvg renders a box shape's (rect/ellipse/image) FULL outline stroke as an
	// inline SVG overlay whenever CSS box-shadow can't express it: dashed strokes, and
	// any non-flat paint stack (gradients / image paints / multiple paints on one
	// stroke). The viewBox is in canvas units so stroke-width/dash are canvas px and
	// scale with the stage; the shape is inset/outset by the stroke Position. '' when
	// the cheap box-shadow path applies.
	function boxStrokeSvg(l: Layer): string {
		if (l.clip) return '';
		const sw = l.stroke_width ?? 0;
		if (sw <= 0) return '';
		const dash = strokeDash(l);
		const fancy = flatStroke(l) === null;
		if (!fancy && (l.stroke_style !== 'dashed' || !dash)) return '';
		const cap = l.stroke_cap ?? 'round';
		// gg (the card renderer) has no miter join, so mirror its bevel for WYSIWYG.
		const join = l.stroke_join === 'miter' ? 'bevel' : (l.stroke_join ?? 'round');
		const align = l.stroke_align ?? 'center';
		// inside: never inset past the box centre (mirrors Go's strokeInset clamp).
		const off =
			align === 'inside'
				? Math.min(sw / 2, Math.min(l.w, l.h) / 2)
				: align === 'outside'
					? -sw / 2
					: 0;
		let shape: string;
		if (l.type === 'ellipse') {
			shape = `<ellipse cx='${l.w / 2}' cy='${l.h / 2}' rx='${Math.max(0, l.w / 2 - off)}' ry='${Math.max(0, l.h / 2 - off)}'/>`;
		} else {
			// rounded rect with INDEPENDENT corners (matches radiusCss / the Go renderer);
			// outside keeps a sharp (0) corner sharp. A 0-radius arc collapses to a corner.
			const cs = Array.isArray(l.corners) && l.corners.length === 4 ? l.corners : null;
			const base = cs ?? [l.radius ?? 0, l.radius ?? 0, l.radius ?? 0, l.radius ?? 0];
			const adj = (cc: number) => (off < 0 && cc <= 0 ? 0 : Math.max(0, cc - off));
			const [tl, tr, br, bl] = base.map(adj);
			const x = off;
			const y = off;
			const w = Math.max(0, l.w - 2 * off);
			const h = Math.max(0, l.h - 2 * off);
			const lim = Math.min(w, h) / 2;
			const k = (v: number) => Math.max(0, Math.min(v, lim));
			const [a, b, e, g] = [k(tl), k(tr), k(br), k(bl)];
			const d =
				`M ${x + a} ${y} L ${x + w - b} ${y} A ${b} ${b} 0 0 1 ${x + w} ${y + b}` +
				` L ${x + w} ${y + h - e} A ${e} ${e} 0 0 1 ${x + w - e} ${y + h}` +
				` L ${x + g} ${y + h} A ${g} ${g} 0 0 1 ${x} ${y + h - g}` +
				` L ${x} ${y + a} A ${a} ${a} 0 0 1 ${x + a} ${y} Z`;
			shape = `<path d='${d}'/>`;
		}
		const { defs, body } = strokePaintLayers(l, `bs_${l.id}`, shape, { x: 0, y: 0, w: l.w, h: l.h });
		if (!body) return '';
		return `<svg class='dash-stroke' style='position:absolute;inset:0;width:100%;height:100%;overflow:visible;pointer-events:none' viewBox='0 0 ${l.w} ${l.h}' preserveAspectRatio='none' fill='none' stroke-width='${sw}'${dash ? ` stroke-dasharray='${dash}'` : ''} stroke-linecap='${cap}' stroke-linejoin='${join}'>${defs ? `<defs>${defs}</defs>` : ''}${body}</svg>`;
	}
	// sidesRestricted: a rect strokes only SOME sides (Figma's individual strokes).
	function sidesRestricted(l: Layer): boolean {
		const s = l.stroke_sides;
		return !!s && s.length > 0 && s.length < 4;
	}
	// strokeSidesSvg draws only the enabled rect sides as straight lines (an SVG overlay,
	// canvas-unit viewBox), each offset inward/outward by the stroke Position. Corner
	// radius is dropped for per-side strokes (matching Figma). '' when not applicable.
	function strokeSidesSvg(l: Layer): string {
		const sw = l.stroke_width ?? 0;
		if (sw <= 0 || l.clip) return '';
		const set = new Set(l.stroke_sides ?? []);
		const cap = l.stroke_cap ?? 'round';
		const join = l.stroke_join === 'miter' ? 'bevel' : (l.stroke_join ?? 'round');
		const dash = strokeDash(l);
		const align = l.stroke_align ?? 'center';
		const off = align === 'inside' ? sw / 2 : align === 'outside' ? -sw / 2 : 0;
		const w = l.w;
		const h = l.h;
		const lines: string[] = [];
		if (set.has('top')) lines.push(`<line x1='0' y1='${off}' x2='${w}' y2='${off}'/>`);
		if (set.has('bottom')) lines.push(`<line x1='0' y1='${h - off}' x2='${w}' y2='${h - off}'/>`);
		if (set.has('left')) lines.push(`<line x1='${off}' y1='0' x2='${off}' y2='${h}'/>`);
		if (set.has('right')) lines.push(`<line x1='${w - off}' y1='0' x2='${w - off}' y2='${h}'/>`);
		if (!lines.length) return '';
		const { defs, body } = strokePaintLayers(l, `ss_${l.id}`, lines.join(''), { x: 0, y: 0, w, h });
		if (!body) return '';
		return `<svg class='dash-stroke' style='position:absolute;inset:0;width:100%;height:100%;overflow:visible;pointer-events:none' viewBox='0 0 ${w} ${h}' preserveAspectRatio='none' fill='none' stroke-width='${sw}' stroke-linecap='${cap}' stroke-linejoin='${join}'${dash ? ` stroke-dasharray='${dash}'` : ''}>${defs ? `<defs>${defs}</defs>` : ''}${body}</svg>`;
	}

	// ── advanced path stroke preview (Figma's Dynamic wobble + arrowheads) ──────
	// The Go PNG renderer stays authoritative; these mirror the VISIBLE bits for WYSIWYG
	// (a width profile previews as a uniform stroke — only the PNG tapers it).
	type Pt = { x: number; y: number };
	function cubicPt(p0: Pt, p1: Pt, p2: Pt, p3: Pt, t: number): Pt {
		const u = 1 - t,
			a = u * u * u,
			b = 3 * u * u * t,
			c = 3 * u * t * t,
			d = t * t * t;
		return { x: a * p0.x + b * p1.x + c * p2.x + d * p3.x, y: a * p0.y + b * p1.y + c * p2.y + d * p3.y };
	}
	function samplePts(ns: PathNode[], closed: boolean, steps: number): Pt[] {
		const out: Pt[] = [{ x: ns[0].x, y: ns[0].y }];
		const seg = (a: PathNode, b: PathNode) => {
			const p0 = { x: a.x, y: a.y },
				p1 = { x: a.h2x, y: a.h2y },
				p2 = { x: b.h1x, y: b.h1y },
				p3 = { x: b.x, y: b.y };
			for (let i = 1; i <= steps; i++) out.push(cubicPt(p0, p1, p2, p3, i / steps));
		};
		for (let i = 1; i < ns.length; i++) seg(ns[i - 1], ns[i]);
		if (closed && ns.length >= 3) seg(ns[ns.length - 1], ns[0]);
		return out;
	}
	// wobblePts mirrors the Go renderer's wobblePath — Figma's "Dynamic" stroke. The
	// noise-based deformation lives in $lib/layout/brushes (dynamicWobble), shared with
	// the brush engine so the editor and the PNG renderer stay in sync.
	function wobblePts(pts: Pt[], l: Layer, closed: boolean): Pt[] {
		return dynamicWobble(pts, l.dynamic_frequency ?? 0, l.dynamic_wiggle ?? 0, l.dynamic_smoothen ?? 0, closed);
	}
	// smoothPathD builds a SMOOTH cubic path through pts (Catmull-Rom -> bezier) so a wobbled
	// outline reads as rounded bumps, not straight zig-zag segments.
	function smoothPathD(pts: Pt[], closed: boolean): string {
		const n = pts.length;
		if (n < 2) return '';
		const at = (i: number) => pts[closed ? ((i % n) + n) % n : Math.max(0, Math.min(n - 1, i))];
		const f = (v: number) => v.toFixed(2);
		let d = `M ${f(pts[0].x)} ${f(pts[0].y)}`;
		const segs = closed ? n : n - 1;
		for (let i = 0; i < segs; i++) {
			const p0 = at(i - 1),
				p1 = at(i),
				p2 = at(i + 1),
				p3 = at(i + 2);
			const c1x = p1.x + (p2.x - p0.x) / 6,
				c1y = p1.y + (p2.y - p0.y) / 6;
			const c2x = p2.x - (p3.x - p1.x) / 6,
				c2y = p2.y - (p3.y - p1.y) / 6;
			d += ` C ${f(c1x)} ${f(c1y)} ${f(c2x)} ${f(c2y)} ${f(p2.x)} ${f(p2.y)}`;
		}
		if (closed) d += ' Z';
		return d;
	}
	// strokePathD returns the visible path 'd' — wobbled when Dynamic is set, but kept as the
	// crisp bezier while the path is being drawn/edited so the node handles stay aligned.
	function strokePathD(l: Layer, plain: boolean): string {
		const ns = l.nodes ?? [];
		if (ns.length < 2) return '';
		if (plain || (l.dynamic_wiggle ?? 0) <= 0) return pathD(l.nodes, l.closed);
		const pts = wobblePts(samplePts(ns, !!l.closed, 24), l, !!l.closed);
		return smoothPathD(pts, !!l.closed);
	}
	// arrowMarker builds a <marker> for a path-end decoration (open paths). `orient` flips
	// for the start (auto-start-reverse) so the head points OUTWARD at both ends. Sized in
	// strokeWidth units so it scales with the stroke like the Go renderer's arrowheads.
	function arrowMarker(id: string, kind: string | undefined, orient: string, c: string): string {
		if (!kind || kind === 'none') return '';
		let body = '';
		let refX = 0;
		switch (kind) {
			case 'triangle':
				body = `<path d='M0,0.4 L4,2 L0,3.6 Z' fill='${c}'/>`;
				break;
			case 'arrow':
				body = `<path d='M0,0.4 L4,2 L0,3.6' fill='none' stroke='${c}' stroke-width='0.9' stroke-linecap='round' stroke-linejoin='round'/>`;
				break;
			case 'line':
				body = `<path d='M0.5,0 L0.5,4' stroke='${c}' stroke-width='0.9' stroke-linecap='round'/>`;
				refX = 0.5;
				break;
			case 'circle':
				body = `<circle cx='2' cy='2' r='1.7' fill='${c}'/>`;
				refX = 2;
				break;
			case 'diamond':
				body = `<path d='M0,2 L2,0.3 L4,2 L2,3.7 Z' fill='${c}'/>`;
				refX = 2;
				break;
			default:
				return '';
		}
		return `<marker id='${id}' markerUnits='strokeWidth' markerWidth='4' markerHeight='4' viewBox='0 0 4 4' refX='${refX}' refY='2' orient='${orient}'>${body}</marker>`;
	}

	// boxOutlinePts / ellipseOutlinePts sample a rect (rounded) or ellipse outline into a
	// polyline in LAYER-LOCAL canvas units (0..w, 0..h), inset for the stroke Position — the
	// input to the dynamic wobble so a rect/ellipse/image BORDER can be hand-drawn too.
	function boxOutlinePts(l: Layer): Pt[] {
		const sw = l.stroke_width ?? 0;
		const align = l.stroke_align ?? 'center';
		const off = align === 'inside' ? sw / 2 : align === 'outside' ? -sw / 2 : 0;
		const x = off,
			y = off,
			w = l.w - 2 * off,
			h = l.h - 2 * off;
		const cs = Array.isArray(l.corners) && l.corners.length === 4 ? l.corners : null;
		const base = cs ?? [l.radius ?? 0, l.radius ?? 0, l.radius ?? 0, l.radius ?? 0];
		const lim = Math.min(w, h) / 2;
		const adj = (cc: number) => Math.max(0, Math.min(off < 0 && cc <= 0 ? 0 : cc - off, lim));
		const [tl, tr, br, bl] = base.map(adj);
		const pts: Pt[] = [];
		const gap = 7;
		const line = (x0: number, y0: number, x1: number, y1: number) => {
			const n = Math.max(1, Math.round(Math.hypot(x1 - x0, y1 - y0) / gap));
			for (let i = 0; i < n; i++) pts.push({ x: x0 + ((x1 - x0) * i) / n, y: y0 + ((y1 - y0) * i) / n });
		};
		const arc = (cx: number, cy: number, r: number, a0: number, a1: number) => {
			if (r <= 0) return;
			const n = Math.max(2, Math.round((r * Math.abs(a1 - a0)) / gap));
			for (let i = 0; i < n; i++) {
				const a = a0 + ((a1 - a0) * i) / n;
				pts.push({ x: cx + r * Math.cos(a), y: cy + r * Math.sin(a) });
			}
		};
		line(x + tl, y, x + w - tr, y);
		arc(x + w - tr, y + tr, tr, -Math.PI / 2, 0);
		line(x + w, y + tr, x + w, y + h - br);
		arc(x + w - br, y + h - br, br, 0, Math.PI / 2);
		line(x + w - br, y + h, x + bl, y + h);
		arc(x + bl, y + h - bl, bl, Math.PI / 2, Math.PI);
		line(x, y + h - bl, x, y + tl);
		arc(x + tl, y + tl, tl, Math.PI, Math.PI * 1.5);
		return pts;
	}
	function ellipseOutlinePts(l: Layer): Pt[] {
		const sw = l.stroke_width ?? 0;
		const align = l.stroke_align ?? 'center';
		const off = align === 'inside' ? sw / 2 : align === 'outside' ? -sw / 2 : 0;
		const rx = Math.max(0, l.w / 2 - off),
			ry = Math.max(0, l.h / 2 - off);
		const n = Math.max(16, Math.round((Math.max(rx, ry) * 2 * Math.PI) / 7));
		const pts: Pt[] = [];
		for (let i = 0; i < n; i++) {
			const a = (2 * Math.PI * i) / n;
			pts.push({ x: l.w / 2 + rx * Math.cos(a), y: l.h / 2 + ry * Math.sin(a) });
		}
		return pts;
	}
	// wobbleStrokeSvg renders a rect/ellipse/image border as a hand-drawn (wobbled) outline —
	// the Dynamic effect on a CLOSED shape. '' when off. Mirrors the Go renderer.
	// wobbleSvg renders a CLOSED shape's dynamic (wobbled) outline. withFill = true fills + strokes
	// it as ONE shape (rect/ellipse: the whole shape wobbles, no clean fill edge underneath);
	// false strokes only (image: the picture stays put, just its border wobbles).
	function wobbleSvg(l: Layer, withFill: boolean): string {
		if (l.clip || (l.dynamic_wiggle ?? 0) <= 0) return '';
		const pts = wobblePts(l.type === 'ellipse' ? ellipseOutlinePts(l) : boxOutlinePts(l), l, true);
		if (pts.length < 3) return '';
		const d = smoothPathD(pts, true);
		const sw = l.stroke_width ?? 0;
		const cap = l.stroke_cap ?? 'round';
		const join = l.stroke_join === 'miter' ? 'bevel' : (l.stroke_join ?? 'round');
		const dash = strokeDash(l);
		// The wobbled shape fills as ONE region (its primary colour); the stroke
		// paints once per stroke paint, like every other stroke preview.
		const fill = withFill ? paintPrimary(l) || 'none' : 'none';
		const { defs, body } = sw > 0 ? strokePaintLayers(l, `ws_${l.id}`, `<path d='${d}'/>`, { x: 0, y: 0, w: l.w, h: l.h }) : { defs: '', body: '' };
		return `<svg class='dash-stroke' style='position:absolute;inset:0;width:100%;height:100%;overflow:visible;pointer-events:none' viewBox='0 0 ${l.w} ${l.h}' preserveAspectRatio='none' fill='none' stroke-width='${sw}' stroke-linecap='${cap}' stroke-linejoin='${join}'${dash ? ` stroke-dasharray='${dash}'` : ''}>${defs ? `<defs>${defs}</defs>` : ''}<path d='${d}' fill='${fill}' stroke='none'/>${body}</svg>`;
	}
	// ── brush strokes (Figma's named Stretch / Scatter brushes) ────────────────
	// The whole engine lives in $lib/layout/brushes (mirrored 1:1 with the Go renderer);
	// here we only collect the layer's brush options and the outline to stroke.
	function brushOpts(l: Layer) {
		return {
			brush: l.brush_name,
			width: l.stroke_width ?? 0,
			// brushes tint with ONE colour — the stack's primary (mirrors the PNG)
			color: strokeCss(l),
			direction: l.brush_direction,
			gap: l.scatter_gap,
			wiggle: l.scatter_wiggle,
			size: l.scatter_size,
			rotation: l.scatter_rotation,
			angular: l.scatter_angular
		};
	}
	// brushShapeSvg renders a rect/ellipse/image BORDER as the selected brush (overlay).
	function brushShapeSvg(l: Layer): string {
		if (l.clip || !l.brush_name || (l.stroke_width ?? 0) <= 0) return '';
		const pts = l.type === 'ellipse' ? ellipseOutlinePts(l) : boxOutlinePts(l);
		if (pts.length < 3) return '';
		const inner = brushStrokeMarkup(pts, brushOpts(l), true);
		return `<svg class='dash-stroke' style='position:absolute;inset:0;width:100%;height:100%;overflow:visible;pointer-events:none' viewBox='0 0 ${l.w} ${l.h}' preserveAspectRatio='none'>${inner}</svg>`;
	}
	// brushPathMarkup renders a vector path's stroke as the selected brush (into the path-layer).
	function brushPathMarkup(l: Layer): string {
		if (!l.brush_name || l.clip || (l.stroke_width ?? 0) <= 0) return '';
		const ns = l.nodes ?? [];
		if (ns.length < 2) return '';
		return brushStrokeMarkup(samplePts(ns, !!l.closed, 24), brushOpts(l), !!l.closed);
	}
	// ── corner radius + typography (mirror the Go renderer) ─────────────────────
	// radiusCss: independent per-corner radii (tl tr br bl) when set, else uniform.
	function radiusCss(l: Layer): string {
		const c = l.corners;
		if (Array.isArray(c) && c.length === 4) {
			return `${c[0] * scale}px ${c[1] * scale}px ${c[2] * scale}px ${c[3] * scale}px`;
		}
		return `${(l.radius ?? 0) * scale}px`;
	}
	// textCss: the shared text typography (font/size/weight/colour/align + line height,
	// letter spacing, case, decoration). Sizes scale with the preview like everything.
	function textCss(l: Layer): string {
		const lh = l.line_height && l.line_height > 0 ? l.line_height : 1.3;
		const ls = (l.letter_spacing ?? 0) * scale;
		const tc =
			l.text_case === 'upper'
				? 'uppercase'
				: l.text_case === 'lower'
					? 'lowercase'
					: l.text_case === 'title'
						? 'capitalize'
						: 'none';
		const td =
			l.text_decoration === 'underline'
				? 'underline'
				: l.text_decoration === 'strike'
					? 'line-through'
					: 'none';
		// Text stroke = an outline of the glyphs (`-webkit-text-stroke`). paint-order:stroke
		// paints it BEHIND the fill so it reads as an outside outline (mirrors the Go pass).
		// A stroke stack previews with its primary colour — the PNG paints the full stack.
		const sw = (l.stroke_width ?? 0) * scale;
		const stroke = sw > 0 ? ` -webkit-text-stroke:${sw}px ${strokeCss(l)}; paint-order:stroke;` : '';
		return (
			`white-space:pre-wrap; overflow-wrap:break-word; text-align:${l.align ?? 'left'};` +
			` font-family:${fontCss(l.font_family)}; font-size:${(l.font_size ?? 16) * scale}px;` +
			` font-weight:${l.font_weight ?? 400}; ${textFillCss(l)} line-height:${lh};` +
			` letter-spacing:${ls}px; text-transform:${tc}; text-decoration:${td};` +
			stroke
		);
	}
	// textFillCss: a text layer's fill from its paint stack — a single solid stays
	// a plain `color`; gradients / images / multi-paint stacks paint the glyphs
	// via background-clip:text (decorations + caret keep a solid via the primary).
	function textFillCss(l: Layer): string {
		const raw = textPaintsOf(l);
		if (!raw.length) return `color:${l.color ?? '#fff'};`;
		const ps = raw.filter((p) => !p.hidden && (p.opacity ?? 1) > 0);
		if (!ps.length) return 'color:transparent;';
		if (ps.length === 1 && ps[0].type === 'solid') {
			const op = ps[0].opacity ?? 1;
			const c = ps[0].color ?? '#fff';
			return `color:${op >= 1 ? c : rgba(c, op)};`;
		}
		const deco = stackPrimary(ps) || '#fff';
		return `${cssBackgroundsOf(ps)} -webkit-background-clip:text; background-clip:text; color:transparent; text-decoration-color:${deco}; caret-color:${deco};`;
	}
	// vertical alignment of the text block within its box.
	function vAlignCss(l: Layer): string {
		const j = l.valign === 'middle' ? 'center' : l.valign === 'bottom' ? 'flex-end' : 'flex-start';
		return `display:flex; flex-direction:column; justify-content:${j};`;
	}

	// ── boolean group preview ───────────────────────────────────────────────────
	// Composite a boolean group's vector members into ONE shape per the group's op,
	// mirroring the Go renderer (combineBoolean). Geometry is exact; anti-aliasing at
	// overlapping edges can diverge slightly from the PNG (which is the source of
	// truth). Each member becomes a filled path 'd' in canvas coords with rotation
	// baked into the coordinates, so every op composes in one frame and the paths stay
	// valid inside <clipPath>/<mask> (where <g transform> children are not). ─────────
	function rotPair(x: number, y: number, cx: number, cy: number, sin: number, cos: number): string {
		const dx = x - cx;
		const dy = y - cy;
		return `${cx + dx * cos - dy * sin} ${cy + dx * sin + dy * cos}`;
	}
	function shapeD(l: Layer): string {
		const cx = l.x + l.w / 2;
		const cy = l.y + l.h / 2;
		const deg = l.rotation ?? 0;
		const a = (deg * Math.PI) / 180;
		const sin = Math.sin(a);
		const cos = Math.cos(a);
		const P = (x: number, y: number) => rotPair(x, y, cx, cy, sin, cos);
		if (l.type === 'ellipse') {
			const rx = l.w / 2;
			const ry = l.h / 2;
			// two 180° arcs; the ellipse's axes rotate with the layer (x-axis-rotation = deg).
			return `M ${P(l.x, cy)} A ${rx} ${ry} ${deg} 1 0 ${P(l.x + l.w, cy)} A ${rx} ${ry} ${deg} 1 0 ${P(l.x, cy)} Z`;
		}
		if (l.type === 'path') {
			const ns = l.nodes ?? [];
			if (ns.length < 2) return '';
			let d = `M ${P(ns[0].x, ns[0].y)}`;
			for (let i = 1; i < ns.length; i++) {
				const p = ns[i - 1];
				const q = ns[i];
				d += ` C ${P(p.h2x, p.h2y)} ${P(q.h1x, q.h1y)} ${P(q.x, q.y)}`;
			}
			// Always close for a filled silhouette (matches drawSilhouette, which closes
			// any path with >=2 nodes — not pathD's >=3 open-shape rule).
			if (ns.length >= 2) {
				const p = ns[ns.length - 1];
				const q = ns[0];
				d += ` C ${P(p.h2x, p.h2y)} ${P(q.h1x, q.h1y)} ${P(q.x, q.y)} Z`;
			}
			return d;
		}
		// rect (optionally rounded) — clamp the corner radius like the renderer.
		const r = Math.max(0, Math.min(l.radius ?? 0, Math.min(l.w, l.h) / 2));
		const { x, y, w, h } = l;
		if (r <= 0) return `M ${P(x, y)} L ${P(x + w, y)} L ${P(x + w, y + h)} L ${P(x, y + h)} Z`;
		return (
			`M ${P(x + r, y)} L ${P(x + w - r, y)} A ${r} ${r} ${deg} 0 1 ${P(x + w, y + r)}` +
			` L ${P(x + w, y + h - r)} A ${r} ${r} ${deg} 0 1 ${P(x + w - r, y + h)}` +
			` L ${P(x + r, y + h)} A ${r} ${r} ${deg} 0 1 ${P(x, y + h - r)}` +
			` L ${P(x, y + r)} A ${r} ${r} ${deg} 0 1 ${P(x + r, y)} Z`
		);
	}
	// boolSvgInner: the composited inner markup for a boolean group, in canvas coords
	// (the wrapping <svg> uses viewBox "0 0 W H"). Mirrors combineBoolean per op:
	//   union     – every member painted solid in one opacity group (overlaps = max)
	//   intersect – the base clipped by every other member (nested clip-paths = min)
	//   subtract  – the base with the others knocked out via a mask (base·∏(1−other))
	//   exclude   – one even-odd path over all members (odd parity)
	// The result takes the top member's fill (bottom member's for subtract), per Figma.
	function boolSvgInner(gid: string): string {
		const members = boolMembers(gid);
		if (members.length < 2) return '';
		const op = layout.groups?.[gid]?.bool_op ?? 'union';
		const src = op === 'subtract' ? members[0] : members[members.length - 1];
		// Top member's appearance (bottom's for subtract). `||` not `??` so an unfilled
		// path (fill==='') falls back to white like the Go renderer; a text source has no
		// fill, so use its colour (Figma keeps the front-most member's appearance).
		const fill = paintPrimary(src) || (src.type === 'text' ? src.color : '') || '#FFFFFF';
		const opacity = Math.min(1, src.opacity ?? 1);
		if (opacity <= 0) return '';
		const sid = gid.replace(/[^a-zA-Z0-9_-]/g, ''); // safe <defs> id fragment
		// Each member's silhouette as inline SVG markup — a <path> for shapes, <text> for
		// text — so a text member composites by its GLYPHS (Figma), not its box. Valid as
		// a clipPath/<mask> child and in normal flow.
		const sil = (m: Layer, f: string) => silRaw(m, f);
		// An image/avatar source keeps its pixels: clip the real image to the boolean
		// coverage (mirrors the Go renderer). A non-image source fills with its colour.
		const imgSrc = src.type === 'image' ? dsrc(src) : '';
		if (imgSrc) {
			const href = imgSrc.replace(/&/g, '&amp;').replace(/'/g, '&#39;');
			const par = (src.fit ?? 'cover') === 'contain' ? 'xMidYMid meet' : 'xMidYMid slice';
			const img = `<image href='${href}' x='${src.x}' y='${src.y}' width='${src.w}' height='${src.h}' preserveAspectRatio='${par}' opacity='${opacity}'`;
			if (op === 'subtract') {
				const holes = members.slice(1).map((m) => sil(m, '#000')).join('');
				return `<defs><mask id='bm_${sid}'>${sil(members[0], '#fff')}${holes}</mask></defs>${img} mask='url(#bm_${sid})'/>`;
			}
			if (op === 'exclude') {
				if (members.some((m) => m.type === 'text')) {
					// Symmetric-difference coverage as ONE mask (each member where no other
					// covers it) so text members crop the image by their GLYPHS.
					const box = `<rect x='0' y='0' width='${layout.width}' height='${layout.height}' fill='#fff'/>`;
					let defs = '';
					let cover = '';
					members.forEach((m, k) => {
						const others = members.filter((_, j) => j !== k).map((o) => sil(o, '#000')).join('');
						defs += `<mask id='exn_${sid}_${k}'>${box}${others}</mask>`;
						cover += `<g mask='url(#exn_${sid}_${k})'>${sil(m, '#fff')}</g>`;
					});
					defs += `<mask id='bx_${sid}'>${cover}</mask>`;
					return `<defs>${defs}</defs>${img} mask='url(#bx_${sid})'/>`;
				}
				// even-odd over all members (shapes); exact for any N.
				const d = members.map(shapeD).join(' ');
				return `<defs><clipPath id='bx_${sid}'><path d='${d}' clip-rule='evenodd'/></clipPath></defs>${img} clip-path='url(#bx_${sid})'/>`;
			}
			if (op === 'intersect') {
				// nested clip-paths over EVERY member → image ∩ all members
				let defs = '';
				for (let k = 0; k < members.length; k++) {
					const ref = k > 0 ? ` clip-path='url(#bc_${sid}_${k - 1})'` : '';
					defs += `<clipPath id='bc_${sid}_${k}'${ref}>${sil(members[k], '#fff')}</clipPath>`;
				}
				return `<defs>${defs}</defs>${img} clip-path='url(#bc_${sid}_${members.length - 1})'/>`;
			}
			// union: clip the image to the union of every member silhouette
			const cp = members.map((m) => sil(m, '#fff')).join('');
			return `<defs><clipPath id='bu_${sid}'>${cp}</clipPath></defs>${img} clip-path='url(#bu_${sid})'/>`;
		}
		if (op === 'exclude') {
			// Symmetric difference. With a text member, show each member where NO OTHER
			// member covers it — exact for 2 members (≡ (A−B)∪(B−A)) and for ≥3 outside any
			// triple-overlap — using only shallow masks so text GLYPHS composite reliably.
			// Shapes-only uses one even-odd path (exact, any N). The PNG is always exact.
			if (members.some((m) => m.type === 'text')) {
				const box = `<rect x='0' y='0' width='${layout.width}' height='${layout.height}' fill='#fff'/>`;
				let defs = '';
				let body = '';
				members.forEach((m, k) => {
					const others = members.filter((_, j) => j !== k).map((o) => sil(o, '#000')).join('');
					defs += `<mask id='ex_${sid}_${k}'>${box}${others}</mask>`;
					body += `<g mask='url(#ex_${sid}_${k})'>${sil(m, fill)}</g>`;
				});
				return `<defs>${defs}</defs><g opacity='${opacity}'>${body}</g>`;
			}
			const d = members.map(shapeD).join(' ');
			return `<path d='${d}' fill='${fill}' fill-rule='evenodd' opacity='${opacity}'/>`;
		}
		if (op === 'subtract') {
			const holes = members.slice(1).map((m) => sil(m, '#000')).join('');
			return `<defs><mask id='bm_${sid}'>${sil(members[0], '#fff')}${holes}</mask></defs><g mask='url(#bm_${sid})' opacity='${opacity}'>${sil(members[0], fill)}</g>`;
		}
		if (op === 'intersect') {
			let defs = '';
			for (let k = 1; k < members.length; k++) {
				const ref = k > 1 ? ` clip-path='url(#bc_${sid}_${k - 1})'` : '';
				defs += `<clipPath id='bc_${sid}_${k}'${ref}>${sil(members[k], '#fff')}</clipPath>`;
			}
			const clip = members.length > 1 ? ` clip-path='url(#bc_${sid}_${members.length - 1})'` : '';
			return `<defs>${defs}</defs><g${clip} opacity='${opacity}'>${sil(members[0], fill)}</g>`;
		}
		// union
		return `<g opacity='${opacity}'>${members.map((m) => sil(m, fill)).join('')}</g>`;
	}

	// ── Pointer interaction ──────────────────────────────────────────────────
	// One gesture model for move + resize. `handle` is null for a body drag, or
	// one of the 8 directions for a resize. We capture the pointer on the moving
	// element and translate screen deltas to canvas px with the live scale.
	type Dir = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw';
	type Gesture = {
		id: string;
		handle: Dir | null;
		startX: number; // canvas px at gesture start
		startY: number;
		startW: number;
		startH: number;
		ptrX: number; // screen px at gesture start
		ptrY: number;
		pointerId: number;
		pointerType: string;
		moved: boolean; // crossed the click→drag slop yet?
		isolate: string | null; // on a click (no drag) collapse the multi-selection to this id
		target: HTMLElement;
		startNodes?: PathNode[]; // for moving a path: node positions at gesture start
		// intrinsic props at gesture start, for the Scale tool (scales them too)
		startProps?: { fontSize?: number; stroke?: number; radius?: number; ring?: number };
		// for a multi-selection body drag: every selected layer's start geometry
		multi?: { id: string; x: number; y: number; nodes: PathNode[] | null }[];
	};
	let g: Gesture | null = $state(null);

	// On-canvas rotation: a drag in a corner rotate-zone spins the single selection
	// about its centre. cx/cy are the layer centre (canvas px); a0 is the pointer's
	// start angle and start the layer's rotation at grab, so rotation follows the
	// pointer. screen/angle drive the live angle badge near the cursor.
	type Rotate = {
		id: string;
		cx: number;
		cy: number;
		a0: number; // pointer angle at grab, degrees
		start: number; // layer rotation at grab, degrees
		ptrId: number;
		target: HTMLElement;
		angle: number; // current rotation, for the badge
		sx: number; // cursor screen px, for the badge
		sy: number;
	};
	let rot: Rotate | null = $state(null);

	// ── path drawing state (pen = multi-click bezier, pencil = freehand) ────────
	let penId = $state<string | null>(null); // path being built with the pen
	let penDrag: number | null = null; // node index whose handle is being dragged this click
	let penPtr: number | null = null; // captured pointer during a pen click-drag
	let cursor = $state<{ x: number; y: number } | null>(null); // pen rubber-band cursor
	let pencilId: string | null = null;
	let pencilLast: { x: number; y: number } | null = null;
	let pencilPtr: number | null = null;

	// Path bbox fitting lives on the store (editor.fitPath) so the inspector's node
	// operations and the canvas share one implementation.

	function beginBody(e: PointerEvent, l: Layer) {
		if (spaceDown) return; // space-pan: let the viewport handle the drag
		if (busy()) return; // a gesture is already in progress (ignore a 2nd finger)
		if (l.locked) return; // locked layers aren't selectable/movable on the canvas
		if (editor.tool !== 'select') return; // a draw tool is active → let the stage draw
		if (editor.editId === l.id) return; // editing this layer (text inline / path nodes) → no body-drag
		if (e.button !== 0) return;
		e.stopPropagation();
		if (e.shiftKey || e.metaKey || e.ctrlKey) {
			editor.select(l.id, true); // toggle this layer in/out of the selection
			return; // a modifier-click toggles; it doesn't start a drag
		}
		// Group drill-in (Figma): the first click on a grouped object selects the whole
		// group so it drags as a unit; once you're "inside" a group (a single member of
		// it is selected) clicking its siblings — or clicking the same member again —
		// targets that one object directly, so you can control it alone right on the
		// canvas without going to the layers panel.
		const inSelection = editor.isSelected(l.id);
		const drilled =
			!!l.group && editor.selectedIds.length === 1 && editor.selected?.group === l.group;
		if (drilled) {
			if (!inSelection) editor.selectOne(l.id); // step sideways to a sibling
			start(e, l, null, false); // already a single object → no further drill on click-up
			return;
		}
		// Plain click on an unselected layer selects it (a grouped layer brings its whole
		// group); clicking one that's already selected keeps the set so it drags together,
		// and a click without a drag then drills to just this object (see start/endGesture).
		if (!inSelection) editor.select(l.id);
		start(e, l, null, inSelection);
	}

	function beginHandle(e: PointerEvent, l: Layer, handle: Dir) {
		if (l.locked || busy()) return;
		// Handles are live for the Select tool (resize) and the Scale tool (resize +
		// scale intrinsic properties).
		if (editor.tool !== 'select' && editor.tool !== 'scale') return;
		if (e.button !== 0) return;
		e.stopPropagation();
		start(e, l, handle);
	}

	function start(e: PointerEvent, l: Layer, handle: Dir | null, wasSelected = false) {
		const target = e.currentTarget as HTMLElement;
		target.setPointerCapture(e.pointerId);
		g = {
			id: l.id,
			handle,
			startX: l.x,
			startY: l.y,
			startW: l.w,
			startH: l.h,
			ptrX: e.clientX,
			ptrY: e.clientY,
			pointerId: e.pointerId,
			pointerType: e.pointerType,
			moved: false,
			// A plain click (no drag) on a member of an ALREADY-selected multi-selection
			// collapses to just that one (drilling into a group / picking one of a marquee).
			// Gated on wasSelected so the first click that forms the selection doesn't
			// immediately collapse it — you get the group, then drill with the next click.
			isolate: handle === null && wasSelected && editor.selectedIds.length > 1 ? l.id : null,
			target,
			startNodes: l.type === 'path' && l.nodes ? l.nodes.map((n) => ({ ...n })) : undefined,
			startProps: {
				fontSize: l.font_size,
				stroke: l.stroke_width,
				radius: l.radius
			},
			multi:
				handle === null && editor.selectedIds.length > 1
					? editor.selectedLayers
							.filter((s) => !s.locked) // locked layers don't move with the group
							.map((s) => ({
								id: s.id,
								x: s.x,
								y: s.y,
								nodes: s.nodes ? s.nodes.map((n) => ({ ...n })) : null
							}))
					: undefined
		};
		e.preventDefault();
	}

	function onMove(e: PointerEvent) {
		if (!g || scale <= 0) return;
		if (e.pointerId !== g.pointerId) return; // ignore other fingers mid-gesture
		// Click→drag slop: don't move until the pointer travels past a threshold, so a
		// click doesn't become a 1px move + undo step (bigger threshold on touch).
		if (!g.moved) {
			const slop = g.pointerType === 'touch' ? 10 : 4;
			if (Math.hypot(e.clientX - g.ptrX, e.clientY - g.ptrY) < slop) return;
			g.moved = true;
		}
		const dx = (e.clientX - g.ptrX) / scale; // canvas-px delta
		const dy = (e.clientY - g.ptrY) / scale;

		if (g.handle === null) {
			const t = snapTargets(g.id);
			const sx = snapAxis(g.startX + dx, g.startW, t.xs);
			const sy = snapAxis(g.startY + dy, g.startH, t.ys);
			guideX = sx.guide;
			guideY = sy.guide;
			const ddx = sx.v - g.startX;
			const ddy = sy.v - g.startY;

			// Multi-selection: move every selected layer by the snapped delta.
			if (g.multi) {
				for (const m of g.multi) {
					const layer = editor.layout.layers.find((l) => l.id === m.id);
					if (!layer) continue;
					if (m.nodes) {
						layer.nodes = m.nodes.map((n) => ({
							x: n.x + ddx,
							y: n.y + ddy,
							h1x: n.h1x + ddx,
							h1y: n.h1y + ddy,
							h2x: n.h2x + ddx,
							h2y: n.h2y + ddy
						}));
						editor.fitPath(layer);
					} else {
						editor.move(m.id, m.x + ddx, m.y + ddy);
					}
				}
				return;
			}

			// Single path: shift its nodes (and handles) by the snapped delta.
			if (g.startNodes) {
				const layer = editor.layout.layers.find((l) => l.id === g!.id);
				if (layer) {
					layer.nodes = g.startNodes.map((n) => ({
						x: n.x + ddx,
						y: n.y + ddy,
						h1x: n.h1x + ddx,
						h1y: n.h1y + ddy,
						h2x: n.h2x + ddx,
						h2y: n.h2y + ddy
					}));
					editor.fitPath(layer);
				}
				return;
			}

			editor.move(g.id, sx.v, sy.v);
			return;
		}

		const h = g.handle;
		const MIN = 8;
		const scaleMode = editor.tool === 'scale';
		const aspect = g.startH !== 0 ? g.startW / g.startH : 1;
		// Scale tool is always uniform; Shift constrains aspect; Alt resizes from the
		// centre. (Figma's resize modifiers.)
		const constrain = scaleMode || e.shiftKey;
		const fromCenter = e.altKey;
		const hasE = h.includes('e');
		const hasW = h.includes('w');
		const hasS = h.includes('s');
		const hasN = h.includes('n');

		// Raw new size from the dragged edge.
		let w = g.startW + (hasE ? dx : hasW ? -dx : 0);
		let hgt = g.startH + (hasS ? dy : hasN ? -dy : 0);

		// Constrain aspect by the DOMINANT pointer axis so the locked axis doesn't
		// flip-flop near the start of the drag.
		if (constrain) {
			const corner = (hasE || hasW) && (hasS || hasN);
			if (corner ? Math.abs(dx) >= Math.abs(dy) : hasE || hasW) hgt = w / aspect;
			else w = hgt * aspect;
		}

		// Scale-tool factor from the UNROUNDED size (so font/stroke/radius scale
		// smoothly, not in integer steps).
		const f = g.startW !== 0 ? Math.max(0.01, w) / g.startW : 1;

		w = Math.max(MIN, w);
		hgt = Math.max(MIN, hgt);

		// Position with the anchored (far) edge pinned EXACTLY — derive the moving
		// edge from a single rounded far edge so the pinned side never jitters.
		let x: number;
		let y: number;
		let fw: number;
		let fh: number;
		if (fromCenter) {
			const cx = g.startX + g.startW / 2;
			const cy = g.startY + g.startH / 2;
			fw = Math.round(w);
			fh = Math.round(hgt);
			x = Math.round(cx - fw / 2);
			y = Math.round(cy - fh / 2);
		} else {
			if (hasW) {
				const far = Math.round(g.startX + g.startW);
				x = Math.round(far - w);
				fw = far - x;
			} else {
				x = g.startX;
				fw = Math.round(w);
			}
			if (hasN) {
				const far = Math.round(g.startY + g.startH);
				y = Math.round(far - hgt);
				fh = far - y;
			} else {
				y = g.startY;
				fh = Math.round(hgt);
			}
		}

		if (scaleMode) {
			editor.scaleLayer(g.id, g.startProps ?? {}, g.startNodes, g.startX, g.startY, g.startW, g.startH, x, y, fw, fh, f);
		} else if (g.startNodes) {
			// A path/shape has no box of its own — scale its nodes to the new bbox.
			editor.scalePath(g.id, g.startNodes, g.startX, g.startY, g.startW, g.startH, x, y, fw, fh);
		} else {
			editor.resize(g.id, fw, fh);
			if (x !== g.startX || y !== g.startY) editor.move(g.id, x, y);
		}
	}

	function endGesture(e: PointerEvent) {
		if (!g) return;
		if (e.pointerId !== g.pointerId) return;
		try {
			g.target.releasePointerCapture(g.pointerId);
		} catch {
			/* capture may already be gone */
		}
		// A click (no drag) on a member of a multi-selection isolates it (Figma). Use
		// selectOne, not select, so a grouped member collapses to itself instead of
		// re-expanding to the whole group (which made canvas drill-in impossible).
		if (!g.moved && g.isolate) editor.selectOne(g.isolate);
		g = null;
		guideX = null;
		guideY = null;
	}

	// ── on-canvas rotation (a drag in a corner rotate-zone) ─────────────────────
	function beginRotate(e: PointerEvent, l: Layer) {
		if (l.locked || busy() || !stageEl) return;
		if (editor.tool !== 'select' && editor.tool !== 'scale') return;
		if (e.button !== 0) return;
		e.stopPropagation();
		const target = e.currentTarget as HTMLElement;
		target.setPointerCapture(e.pointerId);
		const cx = l.x + l.w / 2;
		const cy = l.y + l.h / 2;
		const p = canvasPoint(e, stageEl);
		const a0 = (Math.atan2(p.y - cy, p.x - cx) * 180) / Math.PI;
		rot = { id: l.id, cx, cy, a0, start: l.rotation ?? 0, ptrId: e.pointerId, target, angle: l.rotation ?? 0, sx: e.clientX, sy: e.clientY };
		e.preventDefault();
	}
	function rotateMove(e: PointerEvent) {
		if (!rot || !stageEl || e.pointerId !== rot.ptrId) return;
		const p = canvasPoint(e, stageEl);
		const a = (Math.atan2(p.y - rot.cy, p.x - rot.cx) * 180) / Math.PI;
		let next = rot.start + (a - rot.a0);
		if (e.shiftKey) next = Math.round(next / 15) * 15; // snap to 15° while Shift is held
		next = Math.round(((next % 360) + 360) % 360);
		editor.patch(rot.id, { rotation: next });
		rot = { ...rot, angle: next, sx: e.clientX, sy: e.clientY };
	}
	function rotateUp(e: PointerEvent) {
		if (!rot || e.pointerId !== rot.ptrId) return;
		try {
			rot.target.releasePointerCapture(rot.ptrId);
		} catch {
			/* capture may already be gone */
		}
		rot = null;
	}

	// The four rotate-zones sit just OUTSIDE the corner resize handles of the single
	// selection box; each is a small transparent grab target.
	const ROT_ZONES: { key: Dir; style: string }[] = [
		{ key: 'nw', style: 'left:-18px;top:-18px;' },
		{ key: 'ne', style: 'right:-18px;top:-18px;' },
		{ key: 'se', style: 'right:-18px;bottom:-18px;' },
		{ key: 'sw', style: 'left:-18px;bottom:-18px;' }
	];

	// A gesture is in progress — used to ignore a second finger / stray pointerdown.
	function busy(): boolean {
		return !!(g || rot || draw || marquee || pan || nodeDrag || segDrag || bend);
	}
	// Single hard reset for ALL canvas gestures — wired to window pointercancel/blur
	// so a drag can never get stuck (the browser stealing the pointer, tab blur, the
	// SVG implicit-capture drop on touch, etc.).
	function cancelAllGestures() {
		g = null;
		rot = null;
		draw = null;
		marquee = null;
		nodeDrag = null;
		segDrag = null;
		bend = null;
		pan = null;
		guideX = null;
		guideY = null;
		closeHint = false;
	}

	// ── draw-on-canvas: with a draw tool active, pointerdown creates a layer and
	// drag sizes it (a click → default size). Reverts to the select tool. ───────
	let draw = $state<{ id: string; x0: number; y0: number; ptrId: number; target: HTMLElement; shape?: ShapeKind } | null>(null);

	// marquee (rubber-band) selection over empty canvas
	let marquee = $state<{ x0: number; y0: number; x1: number; y1: number; ptrId: number } | null>(null);

	// zoom & pan — a pure view transform; all geometry stays in canvas px
	let zoom = $state(1);
	let panX = $state(0);
	let panY = $state(0);
	let spaceDown = $state(false);
	// A draw/insert tool is active → the canvas shows a crosshair (Figma); Select
	// shows the normal arrow, the hand tool (space) shows grab.
	const drawCursor = $derived(
		['rect', 'ellipse', 'text', 'image', 'shape', 'pen', 'pencil'].includes(editor.tool)
	);
	let viewportEl = $state<HTMLElement>();
	let pan: { ptrId: number; x0: number; y0: number; px: number; py: number } | null = null;

	function setZoom(z: number) {
		zoom = Math.min(4, Math.max(0.2, z));
	}
	function resetView() {
		zoom = 1;
		panX = 0;
		panY = 0;
	}
	function onWheel(e: WheelEvent) {
		if (e.ctrlKey || e.metaKey) {
			e.preventDefault();
			setZoom(zoom * Math.exp(-e.deltaY * 0.0015));
		} else {
			e.preventDefault();
			panX -= e.shiftKey ? e.deltaY : e.deltaX;
			panY -= e.shiftKey ? 0 : e.deltaY;
		}
	}
	function onViewportDown(e: PointerEvent) {
		if (!spaceDown || e.button !== 0 || busy()) return;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		pan = { ptrId: e.pointerId, x0: e.clientX, y0: e.clientY, px: panX, py: panY };
		e.preventDefault();
	}
	function onViewportMove(e: PointerEvent) {
		if (!pan) return;
		panX = pan.px + (e.clientX - pan.x0);
		panY = pan.py + (e.clientY - pan.y0);
	}
	function onViewportUp(e: PointerEvent) {
		if (!pan) return;
		try {
			(e.currentTarget as HTMLElement).releasePointerCapture(pan.ptrId);
		} catch {
			/* gone */
		}
		pan = null;
	}
	function onKeyup(e: KeyboardEvent) {
		if (e.key === ' ') spaceDown = false;
	}

	let marqueeBase: string[] = []; // selection to union with when a modifier is held
	function marqueeMove(e: PointerEvent) {
		if (!marquee || !stageEl) return;
		const p = canvasPoint(e, stageEl);
		marquee = { ...marquee, x1: p.x, y1: p.y };
		const x = Math.min(marquee.x0, marquee.x1);
		const y = Math.min(marquee.y0, marquee.y1);
		const w = Math.abs(marquee.x1 - marquee.x0);
		const h = Math.abs(marquee.y1 - marquee.y0);
		const hits = layout.layers
			.filter(
				(l) => !l.hidden && !l.locked && l.x < x + w && l.x + l.w > x && l.y < y + h && l.y + l.h > y
			)
			.map((l) => l.id);
		// Shift/Cmd+marquee adds to the prior selection; plain marquee replaces.
		editor.selectMany(marqueeBase.length ? [...new Set([...marqueeBase, ...hits])] : hits);
	}
	function marqueeUp(e: PointerEvent) {
		if (!marquee) return;
		try {
			(e.currentTarget as HTMLElement).releasePointerCapture(marquee.ptrId);
		} catch {
			/* gone */
		}
		marquee = null;
	}

	function canvasPoint(e: { clientX: number; clientY: number }, el: HTMLElement) {
		const r = el.getBoundingClientRect();
		return {
			x: ((e.clientX - r.left) / r.width) * layout.width,
			y: ((e.clientY - r.top) / r.height) * layout.height
		};
	}

	function stageDown(e: PointerEvent) {
		if (spaceDown) return; // space-pan: the viewport handles this drag
		if (busy()) return; // ignore a second pointer while a gesture is active
		const el = e.currentTarget as HTMLElement;
		const p = canvasPoint(e, el);
		switch (editor.tool) {
			case 'select':
				// Empty-canvas pointerdown starts a marquee (clearing the selection
				// unless a modifier is held). Layers/handles stopPropagation, so a
				// pointerdown only reaches here when it's on the empty stage.
				if (e.target === e.currentTarget || (e.target as HTMLElement).dataset.stage === 'bg') {
					if (e.button !== 0) return;
					const additive = e.shiftKey || e.metaKey || e.ctrlKey;
					marqueeBase = additive ? [...editor.selectedIds] : [];
					if (!additive) editor.select(null);
					el.setPointerCapture(e.pointerId);
					marquee = { x0: p.x, y0: p.y, x1: p.x, y1: p.y, ptrId: e.pointerId };
				}
				return;
			case 'pen':
				penDown(e, p, el);
				return;
			case 'pencil':
				pencilDown(e, p, el);
				return;
			case 'bend':
				return; // the Bend tool only acts on a path (handled in pathDown)
			case 'scale':
				return; // the Scale tool acts on the selection's handles (beginHandle)
			case 'shape': {
				if (e.button !== 0) return;
				const layer = editor.createShape(editor.shapeKind, p.x, p.y);
				if (!layer) return; // at the layer cap
				el.setPointerCapture(e.pointerId);
				draw = { id: layer.id, x0: Math.round(p.x), y0: Math.round(p.y), ptrId: e.pointerId, target: el, shape: editor.shapeKind };
				e.preventDefault();
				return;
			}
			default: {
				if (e.button !== 0) return;
				const layer = editor.createLayer(editor.tool, p.x, p.y);
				if (!layer) return; // at the layer cap
				el.setPointerCapture(e.pointerId);
				draw = { id: layer.id, x0: Math.round(p.x), y0: Math.round(p.y), ptrId: e.pointerId, target: el };
				e.preventDefault();
			}
		}
	}

	function stageMove(e: PointerEvent) {
		if (marquee) {
			marqueeMove(e);
			return;
		}
		if (draw) {
			const p = canvasPoint(e, draw.target);
			if (draw.shape === 'line') {
				// a line follows the drag direction (its two endpoints)
				editor.setShapeBox(draw.id, 'line', draw.x0, draw.y0, p.x - draw.x0, p.y - draw.y0);
			} else if (draw.shape) {
				editor.setShapeBox(
					draw.id,
					draw.shape,
					Math.min(draw.x0, p.x),
					Math.min(draw.y0, p.y),
					Math.max(1, Math.abs(p.x - draw.x0)),
					Math.max(1, Math.abs(p.y - draw.y0))
				);
			} else {
				editor.patch(draw.id, {
					x: Math.round(Math.min(draw.x0, p.x)),
					y: Math.round(Math.min(draw.y0, p.y)),
					w: Math.max(1, Math.round(Math.abs(p.x - draw.x0))),
					h: Math.max(1, Math.round(Math.abs(p.y - draw.y0)))
				});
			}
			return;
		}
		if (editor.tool === 'pen' && penId) penMove(e);
		else if (editor.tool === 'pencil' && pencilId) pencilMove(e);
	}

	function stageUp(e: PointerEvent) {
		if (marquee) {
			marqueeUp(e);
			return;
		}
		if (draw) {
			try {
				draw.target.releasePointerCapture(draw.ptrId);
			} catch {
				/* gone */
			}
			const l = editor.layout.layers.find((x) => x.id === draw!.id);
			if (l && (l.w < 8 || l.h < 8)) {
				// treated as a click — give it a sensible default size centred on the point
				if (draw.shape === 'line') {
					editor.setShapeBox(l.id, 'line', Math.round(draw.x0 - 90), draw.y0, 180, 0);
				} else if (draw.shape) {
					editor.setShapeBox(l.id, draw.shape, Math.round(draw.x0 - 90), Math.round(draw.y0 - 90), 180, 180);
				} else {
					const def = newLayer(l.type);
					editor.patch(l.id, {
						w: def.w,
						h: def.h,
						x: Math.round(draw.x0 - def.w / 2),
						y: Math.round(draw.y0 - def.h / 2)
					});
				}
			}
			// A fresh text layer drops into inline edit with its placeholder selected, so
			// typing replaces it; other layers just select. Either way, back to Select.
			if (l?.type === 'text') {
				selectTextOnFocus = true;
				editor.enterEdit(l.id);
			} else {
				editor.select(draw.id);
			}
			editor.setTool('select');
			draw = null;
			return;
		}
		if (penPtr !== null) penUp(e);
		else if (pencilPtr !== null) pencilUp(e);
	}

	// ── pen: click adds a corner; click-drag pulls bezier handles; clicking the
	// first node (or Enter/Esc) finishes; closing fills when 3+ nodes. ──────────
	function penDown(e: PointerEvent, p: { x: number; y: number }, el: HTMLElement) {
		if (e.button !== 0) return;
		const path = penId ? editor.layout.layers.find((l) => l.id === penId) : null;
		if (path?.nodes?.length) {
			const n0 = path.nodes[0];
			if (path.nodes.length >= 2 && Math.hypot(p.x - n0.x, p.y - n0.y) <= 10 / (scale || 1)) {
				path.closed = true;
				finishPen();
				editor.setTool('select');
				return;
			}
			path.nodes.push(cornerNode(Math.round(p.x), Math.round(p.y)));
			penDrag = path.nodes.length - 1;
			editor.fitPath(path);
		} else {
			const layer = editor.createPath(p.x, p.y);
			if (!layer) return; // at the cap
			penId = layer.id;
			penDrag = 0;
		}
		el.setPointerCapture(e.pointerId);
		penPtr = e.pointerId;
		e.preventDefault();
	}
	function penMove(e: PointerEvent) {
		const path = editor.layout.layers.find((l) => l.id === penId);
		if (!path?.nodes) return;
		const p = canvasPoint(e, e.currentTarget as HTMLElement);
		cursor = p;
		if (penDrag !== null && e.buttons & 1) {
			const n = path.nodes[penDrag];
			n.h2x = Math.round(p.x);
			n.h2y = Math.round(p.y);
			n.h1x = Math.round(2 * n.x - p.x); // mirror for a smooth anchor
			n.h1y = Math.round(2 * n.y - p.y);
			n.m = 'mirror'; // a click-dragged pen point is a smooth, mirrored node
			editor.fitPath(path);
		}
	}
	function penUp(e: PointerEvent) {
		if (penPtr !== null) {
			try {
				(e.currentTarget as HTMLElement).releasePointerCapture(penPtr);
			} catch {
				/* gone */
			}
		}
		penPtr = null;
		penDrag = null;
	}
	function finishPen() {
		const path = penId ? editor.layout.layers.find((l) => l.id === penId) : null;
		if (path && (path.nodes?.length ?? 0) < 2) editor.removeLayer(path.id);
		else if (path) editor.select(path.id);
		penId = null;
		penDrag = null;
		penPtr = null;
		cursor = null;
	}

	// ── pencil: freehand drag, sampled by distance into corner nodes ────────────
	function pencilDown(e: PointerEvent, p: { x: number; y: number }, el: HTMLElement) {
		if (e.button !== 0) return;
		const layer = editor.createPath(p.x, p.y);
		if (!layer) return;
		pencilId = layer.id;
		pencilLast = { x: p.x, y: p.y };
		el.setPointerCapture(e.pointerId);
		pencilPtr = e.pointerId;
		e.preventDefault();
	}
	function pencilMove(e: PointerEvent) {
		if (!(e.buttons & 1)) return;
		const path = editor.layout.layers.find((l) => l.id === pencilId);
		if (!path?.nodes || !pencilLast) return;
		const p = canvasPoint(e, e.currentTarget as HTMLElement);
		if (Math.hypot(p.x - pencilLast.x, p.y - pencilLast.y) < 4) return;
		path.nodes.push(cornerNode(Math.round(p.x), Math.round(p.y)));
		pencilLast = { x: p.x, y: p.y };
		editor.fitPath(path);
	}
	function pencilUp(e: PointerEvent) {
		if (pencilPtr !== null) {
			try {
				(e.currentTarget as HTMLElement).releasePointerCapture(pencilPtr);
			} catch {
				/* gone */
			}
		}
		const path = editor.layout.layers.find((l) => l.id === pencilId);
		if (path && (path.nodes?.length ?? 0) < 2) editor.removeLayer(path.id);
		else if (path) editor.select(path.id);
		pencilId = null;
		pencilLast = null;
		pencilPtr = null;
		editor.setTool('select');
	}

	// Auto-commit an in-progress pen path if the tool changes (e.g. via the palette).
	$effect(() => {
		if (editor.tool !== 'pen' && penId) finishPen();
	});

	// ── double-click edit mode (Figma-style vector editing) ─────────────────────
	// editId/activeNode live on the store (so the inspector can drive them). When a
	// path is in edit mode you can: drag anchors (snapping to other points & the
	// grid), drag bezier handles (constrained by the node's point type), Alt-drag a
	// handle to break it independent, Shift-drag a handle to snap its angle to 45°,
	// drag the line itself to bend a segment, double-click a point to toggle
	// smooth/corner, double-click the line to insert a point, and drag an open
	// endpoint onto the other end to close the loop.
	let stageEl = $state<HTMLElement>();
	let textEl = $state<HTMLTextAreaElement>();
	// When a text layer is created with the text tool we drop straight into edit
	// mode; this asks the focus effect to select the placeholder so the first
	// keystroke replaces it (Figma's new-text behaviour).
	let selectTextOnFocus = false;
	// $state so the closeNode $derived re-tracks when a node drag starts/ends.
	let nodeDrag = $state<{ node: number; kind: 'anchor' | 'h1' | 'h2'; ptrId: number; target: Element } | null>(null);
	// The bezier handle most recently grabbed (for Delete = remove that bend).
	let activeHandle = $state<{ node: number; kind: 'h1' | 'h2' } | null>(null);
	let segDrag: { ptrId: number; target: Element; seg: number; sx: number; sy: number; start: PathNode[] } | null = null;
	let closeHint = $state(false); // dragging an endpoint within range of the other end

	const NODE_SNAP = 6; // screen px; converted to canvas px with the live scale
	const nodeSnapPx = $derived(NODE_SNAP / (scale > 0 ? scale : 1));

	// The endpoint a dragged open end would merge into (drives the close highlight).
	const closeNode = $derived.by(() => {
		if (!closeHint || !nodeDrag || nodeDrag.kind !== 'anchor') return null;
		const path = editor.editPath;
		if (!path?.nodes) return null;
		const ct = closeTarget(path, nodeDrag.node);
		return ct >= 0 ? path.nodes[ct] : null;
	});

	function enterEdit(l: Layer) {
		if (editor.tool !== 'select') return;
		editor.enterEdit(l.id);
	}
	// Leaving the layer (selecting another / empty canvas) exits edit mode.
	$effect(() => {
		if (editor.editId && editor.selectedId !== editor.editId) editor.exitEdit();
	});
	// Focus the inline text editor as soon as it appears. A newly-created text layer
	// also gets its placeholder selected so the first keystroke replaces it.
	$effect(() => {
		if (editor.editId && textEl) {
			textEl.focus();
			if (selectTextOnFocus) {
				textEl.select();
				selectTextOnFocus = false;
			}
		}
	});

	// Convert a canvas-space point into a path's un-rotated local space so node
	// dragging stays correct even when the path is rotated.
	function toLocal(p: { x: number; y: number }, l: Layer) {
		const deg = l.rotation ?? 0;
		if (!deg) return p;
		const rad = (-deg * Math.PI) / 180;
		const cx = l.x + l.w / 2;
		const cy = l.y + l.h / 2;
		const dx = p.x - cx;
		const dy = p.y - cy;
		return {
			x: cx + dx * Math.cos(rad) - dy * Math.sin(rad),
			y: cy + dx * Math.sin(rad) + dy * Math.cos(rad)
		};
	}

	// snapNodePoint snaps a dragged anchor to the canvas grid (edges/centre), this
	// path's other nodes, and other layers' bbox edges/centres — independently per
	// axis, so you also get clean horizontal/vertical alignment. Returns the
	// snapped point plus the active guide lines (canvas px) to draw.
	function snapNodePoint(path: Layer, x: number, y: number, skip: number) {
		const th = nodeSnapPx;
		let bx = x;
		let by = y;
		let gx: number | null = null;
		let gy: number | null = null;
		let bdx = th;
		let bdy = th;
		const consider = (tx: number, ty: number) => {
			const ax = Math.abs(x - tx);
			if (ax < bdx) {
				bdx = ax;
				bx = tx;
				gx = tx;
			}
			const ay = Math.abs(y - ty);
			if (ay < bdy) {
				bdy = ay;
				by = ty;
				gy = ty;
			}
		};
		// This path's own nodes are in the same (local) space as x/y, so they snap
		// correctly even when the path is rotated.
		(path.nodes ?? []).forEach((n, i) => {
			if (i !== skip) consider(n.x, n.y);
		});
		// Canvas grid + other layers live in canvas space, which only coincides with
		// the path's local space when it isn't rotated; skip them (and their guides)
		// for a rotated path rather than snap/draw to the wrong place.
		if (!(path.rotation ?? 0)) {
			consider(0, 0);
			consider(layout.width / 2, layout.height / 2);
			consider(layout.width, layout.height);
			for (const l of layout.layers) {
				if (l.id === path.id || l.hidden) continue;
				consider(l.x, l.y);
				consider(l.x + l.w / 2, l.y + l.h / 2);
				consider(l.x + l.w, l.y + l.h);
			}
		}
		return { x: Math.round(bx), y: Math.round(by), gx, gy };
	}

	// snapAngle constrains a handle to the nearest 45° around its anchor (Shift).
	function snapAngle(ax: number, ay: number, hx: number, hy: number) {
		const dx = hx - ax;
		const dy = hy - ay;
		const len = Math.hypot(dx, dy);
		if (len < 0.001) return { x: hx, y: hy };
		const step = Math.PI / 4;
		const a = Math.round(Math.atan2(dy, dx) / step) * step;
		return { x: Math.round(ax + Math.cos(a) * len), y: Math.round(ay + Math.sin(a) * len) };
	}

	// closeTarget: the other endpoint an open path's dragged end would merge into.
	function closeTarget(path: Layer, idx: number): number {
		if (path.closed || !path.nodes || path.nodes.length < 3) return -1;
		const last = path.nodes.length - 1;
		if (idx === 0) return last;
		if (idx === last) return 0;
		return -1;
	}

	// constrainOpposite re-derives a node's other handle from the one just moved,
	// honouring its point type: 'mirror' = exact reflection, 'asym' = mirrored
	// angle keeping the opposite length, 'corner' = leave it alone. `movedOut` is
	// true when the out-handle (h2) was the one edited.
	function constrainOpposite(n: PathNode, movedOut: boolean) {
		const mode = n.m ?? 'corner';
		if (mode === 'mirror') {
			if (movedOut) {
				n.h1x = Math.round(2 * n.x - n.h2x);
				n.h1y = Math.round(2 * n.y - n.h2y);
			} else {
				n.h2x = Math.round(2 * n.x - n.h1x);
				n.h2y = Math.round(2 * n.y - n.h1y);
			}
		} else if (mode === 'asym') {
			const hx = movedOut ? n.h2x : n.h1x;
			const hy = movedOut ? n.h2y : n.h1y;
			const oppX = movedOut ? n.h1x : n.h2x;
			const oppY = movedOut ? n.h1y : n.h2y;
			const oppLen = Math.hypot(oppX - n.x, oppY - n.y);
			const ang = Math.atan2(hy - n.y, hx - n.x) + Math.PI;
			const ox = Math.round(n.x + Math.cos(ang) * oppLen);
			const oy = Math.round(n.y + Math.sin(ang) * oppLen);
			if (movedOut) {
				n.h1x = ox;
				n.h1y = oy;
			} else {
				n.h2x = ox;
				n.h2y = oy;
			}
		}
	}

	function startNodeDrag(e: PointerEvent, idx: number, kind: 'anchor' | 'h1' | 'h2') {
		if (e.button !== 0) return;
		e.stopPropagation();
		editor.setActiveNode(idx);
		// Track which bezier handle was grabbed so Delete removes THAT bend (not the
		// node or the whole layer). Grabbing the anchor clears it.
		activeHandle = kind === 'anchor' ? null : { node: idx, kind };
		(e.currentTarget as Element).setPointerCapture(e.pointerId);
		nodeDrag = { node: idx, kind, ptrId: e.pointerId, target: e.currentTarget as Element };
	}
	function moveNodeDrag(e: PointerEvent) {
		if (!nodeDrag || !stageEl) return;
		const path = editor.editPath;
		if (!path?.nodes) return;
		const p = toLocal(canvasPoint(e, stageEl), path);
		const n = path.nodes[nodeDrag.node];
		if (!n) return;

		if (nodeDrag.kind === 'anchor') {
			let tx = Math.round(p.x);
			let ty = Math.round(p.y);
			// Dragging an open endpoint near the other end → preview a close/merge.
			const ct = closeTarget(path, nodeDrag.node);
			const other = ct >= 0 ? path.nodes[ct] : null;
			if (other && Math.hypot(tx - other.x, ty - other.y) <= nodeSnapPx * 1.8) {
				tx = other.x;
				ty = other.y;
				guideX = null;
				guideY = null;
				closeHint = true;
			} else {
				closeHint = false;
				const s = snapNodePoint(path, tx, ty, nodeDrag.node);
				tx = s.x;
				ty = s.y;
				guideX = s.gx;
				guideY = s.gy;
			}
			const dx = tx - n.x;
			const dy = ty - n.y;
			n.x += dx;
			n.y += dy;
			n.h1x += dx;
			n.h1y += dy;
			n.h2x += dx;
			n.h2y += dy;
		} else {
			let hx = Math.round(p.x);
			let hy = Math.round(p.y);
			if (e.shiftKey) ({ x: hx, y: hy } = snapAngle(n.x, n.y, hx, hy));
			const out = nodeDrag.kind === 'h2';
			if (out) {
				n.h2x = hx;
				n.h2y = hy;
			} else {
				n.h1x = hx;
				n.h1y = hy;
			}
			// Alt breaks the link (independent corner handles); else honour the type.
			if (e.altKey) n.m = 'corner';
			constrainOpposite(n, out);
		}
		// NB: don't fitPath here — recomputing the bbox mid-drag moves the rotation
		// pivot (l.x+l.w/2) used by both toLocal and the <g rotate>, which makes a
		// rotated path's node chase away from the cursor. Refit on release instead.
	}
	function endNodeDrag() {
		if (!nodeDrag) return;
		try {
			nodeDrag.target.releasePointerCapture(nodeDrag.ptrId);
		} catch {
			/* gone */
		}
		const path = editor.editPath;
		if (path?.nodes) {
			// Merge a dragged endpoint into the other end → close the loop.
			if (closeHint && nodeDrag.kind === 'anchor') {
				const ct = closeTarget(path, nodeDrag.node);
				if (ct >= 0 && path.nodes.length >= 3) {
					path.nodes.splice(nodeDrag.node, 1);
					editor.setActiveNode(null);
					editor.setClosed(true);
				}
			}
			editor.fitPath(path);
		}
		closeHint = false;
		guideX = null;
		guideY = null;
		nodeDrag = null;
	}

	// ── segment bend: drag the curve between two anchors to bow it (Figma) ───────
	function segCubic(ns: PathNode[], seg: number) {
		const len = ns.length;
		const a = ns[(seg - 1 + len) % len];
		const b = ns[seg % len];
		return [
			{ x: a.x, y: a.y },
			{ x: a.h2x, y: a.h2y },
			{ x: b.h1x, y: b.h1y },
			{ x: b.x, y: b.y }
		];
	}
	function cubicAt(c: { x: number; y: number }[], t: number) {
		const u = 1 - t;
		const a = u * u * u;
		const b = 3 * u * u * t;
		const cc = 3 * u * t * t;
		const d = t * t * t;
		return {
			x: a * c[0].x + b * c[1].x + cc * c[2].x + d * c[3].x,
			y: a * c[0].y + b * c[1].y + cc * c[2].y + d * c[3].y
		};
	}
	// nearestOnPath finds the closest point on the whole path: which segment and the
	// parameter t along it. Segments are 1..n-1, plus the closing segment (= n) when
	// the path is closed.
	function nearestOnPath(l: Layer, p: { x: number; y: number }) {
		const ns = l.nodes ?? [];
		// Mirror pathD: the closing segment only exists for a closed path with 3+
		// nodes, so don't let the editor target a segment the renderer won't draw.
		const closes = l.closed && ns.length >= 3;
		const segs = closes ? ns.length : ns.length - 1;
		let best = { seg: 1, t: 0.5, d: Infinity };
		for (let seg = 1; seg <= segs; seg++) {
			const c = segCubic(ns, seg);
			for (let i = 0; i <= 16; i++) {
				const t = i / 16;
				const q = cubicAt(c, t);
				const d = (q.x - p.x) ** 2 + (q.y - p.y) ** 2;
				if (d < best.d) best = { seg, t, d };
			}
		}
		return best;
	}
	function moveSegDrag(e: PointerEvent) {
		if (!segDrag || !stageEl) return;
		const path = editor.editPath;
		if (!path?.nodes) return;
		const p = toLocal(canvasPoint(e, stageEl), path);
		// Move the segment's two control handles so the curve follows the cursor:
		// B(0.5) shifts by 0.75·Δhandle, so scale the delta by 4/3 to track the drag.
		const k = 4 / 3;
		const ddx = (p.x - segDrag.sx) * k;
		const ddy = (p.y - segDrag.sy) * k;
		const len = path.nodes.length;
		const ai = (segDrag.seg - 1 + len) % len;
		const bi = segDrag.seg % len;
		const a0 = segDrag.start[ai];
		const b0 = segDrag.start[bi];
		const a = path.nodes[ai];
		const b = path.nodes[bi];
		a.h2x = Math.round(a0.h2x + ddx);
		a.h2y = Math.round(a0.h2y + ddy);
		b.h1x = Math.round(b0.h1x + ddx);
		b.h1y = Math.round(b0.h1y + ddy);
		// Keep each endpoint's opposite handle consistent with its point type.
		constrainOpposite(a, true);
		constrainOpposite(b, false);
	}
	function endSegDrag() {
		if (!segDrag) return;
		try {
			segDrag.target.releasePointerCapture(segDrag.ptrId);
		} catch {
			/* gone */
		}
		const path = editor.editPath;
		if (path) editor.fitPath(path);
		segDrag = null;
	}

	// ── Bend tool: click anywhere on a path and drag — near a point it pulls that
	// point into a curve (symmetric handles); on a segment it bows the curve. An
	// explicit palette tool (Figma's Bend), not an implicit edit-mode drag. ───────
	let bend: { node: number; ptrId: number; target: Element } | null = null;

	function beginBend(e: PointerEvent, l: Layer) {
		if (e.button !== 0) return;
		e.stopPropagation();
		if (!stageEl || !l.nodes || l.nodes.length < 2) return;
		const p = toLocal(canvasPoint(e, stageEl), l);
		// nearest node — grab it to curve that point
		let ni = -1;
		let nd = Infinity;
		l.nodes.forEach((n, i) => {
			const dd = Math.hypot(n.x - p.x, n.y - p.y);
			if (dd < nd) {
				nd = dd;
				ni = i;
			}
		});
		if (ni >= 0 && nd <= 12 / (scale || 1)) {
			if (editor.editId !== l.id) editor.enterEdit(l.id);
			(e.currentTarget as Element).setPointerCapture(e.pointerId);
			bend = { node: ni, ptrId: e.pointerId, target: e.currentTarget as Element };
			editor.setActiveNode(ni);
			return;
		}
		// otherwise only bow a segment if the press is actually ON the line — clicking
		// the fill interior or off the outline does nothing (Figma's bend behaviour).
		const { seg, d } = nearestOnPath(l, p);
		if (Math.sqrt(d) > 8 / (scale || 1)) return;
		if (editor.editId !== l.id) editor.enterEdit(l.id);
		(e.currentTarget as Element).setPointerCapture(e.pointerId);
		segDrag = {
			ptrId: e.pointerId,
			target: e.currentTarget as Element,
			seg,
			sx: p.x,
			sy: p.y,
			start: l.nodes.map((n) => ({ ...n }))
		};
		editor.setActiveNode(null);
	}
	function moveBend(e: PointerEvent) {
		if (!bend || !stageEl) return;
		const path = editor.editPath;
		const n = path?.nodes?.[bend.node];
		if (!n) return;
		const p = toLocal(canvasPoint(e, stageEl), path!);
		// pull symmetric handles toward the cursor — a corner becomes a smooth curve
		n.h2x = Math.round(p.x);
		n.h2y = Math.round(p.y);
		n.h1x = Math.round(2 * n.x - p.x);
		n.h1y = Math.round(2 * n.y - p.y);
		n.m = 'mirror';
	}
	function endBend() {
		if (!bend) return;
		try {
			bend.target.releasePointerCapture(bend.ptrId);
		} catch {
			/* gone */
		}
		const path = editor.editPath;
		if (path) editor.fitPath(path);
		bend = null;
	}

	// Path stroke pointer routing. With the Bend tool, clicking the path bends it;
	// in edit mode the stroke is inert (edit via points / use Bend); otherwise it
	// moves the whole layer.
	function pathDown(e: PointerEvent, l: Layer) {
		if (editor.tool === 'bend') {
			beginBend(e, l);
			return;
		}
		if (l.id === editor.editId) {
			e.stopPropagation(); // don't deselect; points/handles handle editing
			return;
		}
		beginBody(e, l);
	}
	function pathMove(e: PointerEvent) {
		if (bend) moveBend(e);
		else if (segDrag) moveSegDrag(e);
		else onMove(e);
	}
	function pathUp(e: PointerEvent) {
		if (bend) endBend();
		else if (segDrag) endSegDrag();
		else endGesture(e);
	}

	// Double-clicking the stroke while editing inserts a point on the curve at the
	// click, splitting the segment via de Casteljau so the shape is preserved.
	function addNodeOnSegment(e: MouseEvent, l: Layer) {
		if (!stageEl || !l.nodes || l.nodes.length < 2) return;
		const p = toLocal(canvasPoint(e, stageEl), l);
		const { seg, t } = nearestOnPath(l, p);
		const ns = l.nodes;
		const len = ns.length;
		const ai = (seg - 1 + len) % len;
		const bi = seg % len;
		const a = ns[ai];
		const b = ns[bi];
		const lerp = (u: { x: number; y: number }, v: { x: number; y: number }) => ({
			x: u.x + (v.x - u.x) * t,
			y: u.y + (v.y - u.y) * t
		});
		const P0 = { x: a.x, y: a.y };
		const P1 = { x: a.h2x, y: a.h2y };
		const P2 = { x: b.h1x, y: b.h1y };
		const P3 = { x: b.x, y: b.y };
		const P01 = lerp(P0, P1);
		const P12 = lerp(P1, P2);
		const P23 = lerp(P2, P3);
		const P012 = lerp(P01, P12);
		const P123 = lerp(P12, P23);
		const P0123 = lerp(P012, P123);
		a.h2x = Math.round(P01.x);
		a.h2y = Math.round(P01.y);
		b.h1x = Math.round(P23.x);
		b.h1y = Math.round(P23.y);
		const mid: PathNode = {
			x: Math.round(P0123.x),
			y: Math.round(P0123.y),
			h1x: Math.round(P012.x),
			h1y: Math.round(P012.y),
			h2x: Math.round(P123.x),
			h2y: Math.round(P123.y),
			m: 'asym'
		};
		ns.splice(seg, 0, mid);
		editor.setActiveNode(seg);
		editor.fitPath(l);
	}

	// Resize handles: 4 corners + 4 edges. position + cursor per direction.
	const HANDLES: { dir: Dir; cls: string; cursor: string }[] = [
		{ dir: 'nw', cls: 'left-0 top-0', cursor: 'nwse-resize' },
		{ dir: 'n', cls: 'left-1/2 top-0 -translate-x-1/2', cursor: 'ns-resize' },
		{ dir: 'ne', cls: 'right-0 top-0', cursor: 'nesw-resize' },
		{ dir: 'e', cls: 'right-0 top-1/2 -translate-y-1/2', cursor: 'ew-resize' },
		{ dir: 'se', cls: 'right-0 bottom-0', cursor: 'nwse-resize' },
		{ dir: 's', cls: 'left-1/2 bottom-0 -translate-x-1/2', cursor: 'ns-resize' },
		{ dir: 'sw', cls: 'left-0 bottom-0', cursor: 'nesw-resize' },
		{ dir: 'w', cls: 'left-0 top-1/2 -translate-y-1/2', cursor: 'ew-resize' }
	];

	const visible = $derived(layout.layers.filter((l) => !l.hidden));

	// ── snapping (Figma-style): snap a dragged box's edges/center to the canvas
	// and to other layers, showing a guide line when a snap is active. ──────────
	const SNAP = 6; // canvas px threshold
	let guideX = $state<number | null>(null);
	let guideY = $state<number | null>(null);

	function snapAxis(left: number, size: number, targets: number[]): { v: number; guide: number | null } {
		const edges = [left, left + size / 2, left + size]; // near edge, center, far edge
		let bestD = SNAP + 1;
		let v = left;
		let guide: number | null = null;
		for (const t of targets) {
			for (let i = 0; i < 3; i++) {
				const d = Math.abs(edges[i] - t);
				if (d < bestD) {
					bestD = d;
					guide = t;
					v = i === 0 ? t : i === 1 ? t - size / 2 : t - size;
				}
			}
		}
		return bestD <= SNAP ? { v, guide } : { v: left, guide: null };
	}
	function snapTargets(excludeId: string) {
		const xs = [0, layout.width / 2, layout.width];
		const ys = [0, layout.height / 2, layout.height];
		for (const l of layout.layers) {
			if (l.id === excludeId || l.hidden) continue;
			xs.push(l.x, l.x + l.w / 2, l.x + l.w);
			ys.push(l.y, l.y + l.h / 2, l.y + l.h);
		}
		return { xs, ys };
	}

	// ── keyboard: tool shortcuts, then nudge / delete / duplicate on selection ──
	// ── OS-clipboard paste (Figma-style Ctrl+V) ────────────────────────────────
	// An image on the clipboard (screenshot, copied picture) uploads and lands as
	// a new image layer sized to its natural dimensions; objects copied in the
	// editor paste via the internal clipboard; plain text becomes a text layer.
	async function onPaste(e: ClipboardEvent) {
		const t = e.target as HTMLElement | null;
		if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;
		const dt = e.clipboardData;
		if (!dt) return;
		const item = Array.from(dt.items).find((it) => it.type.startsWith('image/'));
		if (item) {
			e.preventDefault();
			const file = item.getAsFile();
			if (!file) return;
			try {
				const url = await uploadImage(editor.guildId, file);
				const dims = await new Promise<{ w: number; h: number }>((res) => {
					const im = new Image();
					im.onload = () => res({ w: im.naturalWidth || 200, h: im.naturalHeight || 200 });
					im.onerror = () => res({ w: 200, h: 200 });
					im.src = url;
				});
				const sc = Math.min(1, (layout.width * 0.6) / dims.w, (layout.height * 0.6) / dims.h);
				const w = Math.max(8, Math.round(dims.w * sc));
				const h = Math.max(8, Math.round(dims.h * sc));
				const layer = editor.addLayer('image');
				if (!layer) return;
				layer.name = 'Pasted image';
				layer.src = url;
				layer.radius = 0;
				layer.w = w;
				layer.h = h;
				layer.x = Math.round((layout.width - w) / 2);
				layer.y = Math.round((layout.height - h) / 2);
			} catch {
				/* upload failed — nothing to paste */
			}
			return;
		}
		const text = dt.getData('text/plain');
		e.preventDefault();
		// objects copied in the editor win; otherwise plain text becomes a text layer
		if (editor.canPasteInternal) {
			editor.paste();
			return;
		}
		if (text.trim()) {
			const layer = editor.addLayer('text');
			if (!layer) return;
			layer.name = text.trim().slice(0, 24);
			layer.text = text;
			layer.x = Math.round(layout.width / 2 - layer.w / 2);
			layer.y = Math.round(layout.height / 2 - layer.h / 2);
		}
	}

	function onKeydown(e: KeyboardEvent) {
		const t = e.target as HTMLElement | null;
		if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return;

		// hold Space to pan the canvas
		if (e.key === ' ') {
			spaceDown = true;
			e.preventDefault();
			return;
		}

		// document-level shortcuts (undo/redo, clipboard, select-all, group, zoom)
		if (e.metaKey || e.ctrlKey) {
			switch (e.key.toLowerCase()) {
				case 'z':
					e.preventDefault();
					if (e.shiftKey) editor.redo();
					else editor.undo();
					return;
				case 'y':
					e.preventDefault();
					editor.redo();
					return;
				case 'c':
					e.preventDefault();
					editor.copy();
					return;
				case 'x':
					e.preventDefault();
					editor.cut();
					return;
					case 'a':
					e.preventDefault();
					editor.selectAll();
					return;
				case 'g':
					e.preventDefault();
					if (e.shiftKey) editor.ungroup();
					else editor.group();
					return;
				case 'd':
					e.preventDefault();
					if (editor.selectedId) editor.duplicateLayer(editor.selectedId);
					return;
				case '=':
				case '+':
					e.preventDefault();
					setZoom(zoom * 1.2);
					return;
				case '-':
					e.preventDefault();
					setZoom(zoom / 1.2);
					return;
				case '0':
					e.preventDefault();
					resetView();
					return;
			}
		}

		// Shift+/ ("?") opens the keyboard-shortcuts sheet (owned by the toolbar chrome).
		if (e.key === '?') {
			e.preventDefault();
			editor.shortcutsOpen = true;
			return;
		}
		if (e.key === 'Enter' && penId) {
			finishPen();
			editor.setTool('select');
			return;
		}
		if (e.key === 'Escape') {
			// Esc cascade: leave edit mode → drop back to Select → clear the selection.
			// preventDefault ONLY when we actually consume the key, so once the canvas is
			// idle the host (the studio modal) sees an unconsumed Esc and can close.
			if (editor.editId) {
				editor.exitEdit();
				e.preventDefault();
			} else if (editor.tool !== 'select') {
				editor.setTool('select');
				e.preventDefault();
			} else if (editor.selectedIds.length) {
				editor.select(null);
				e.preventDefault();
			}
			return;
		}
		if (!e.metaKey && !e.ctrlKey && !e.altKey) {
			switch (e.key.toLowerCase()) {
				case 'v':
					editor.setTool('select');
					return;
				case 'r':
					editor.setTool('rect');
					return;
				case 'o':
					editor.setTool('ellipse');
					return;
				case 't':
					editor.setTool('text');
					return;
				case 'p':
					editor.setTool('pen');
					return;
				case 'b':
					editor.setTool('bend');
					return;
				case 'k':
					editor.setTool('scale');
					return;
			}
		}

		const sel = editor.selected;
		if (!sel) return;
		const step = e.shiftKey ? 10 : 1;

		// In path edit mode with an active node, arrows nudge the node (+ handles)
		// and Delete removes the node; otherwise they act on the whole layer.
		const editPath = editor.editPath;
		const node = editor.activePathNode;
		const nudge = (dx: number, dy: number) => {
			e.preventDefault();
			if (editPath && node) {
				node.x += dx;
				node.y += dy;
				node.h1x += dx;
				node.h1y += dy;
				node.h2x += dx;
				node.h2y += dy;
				editor.fitPath(editPath);
			} else {
				editor.move(sel.id, sel.x + dx, sel.y + dy);
			}
		};

		switch (e.key) {
			case 'ArrowLeft':
				nudge(-step, 0);
				break;
			case 'ArrowRight':
				nudge(step, 0);
				break;
			case 'ArrowUp':
				nudge(0, -step);
				break;
			case 'ArrowDown':
				nudge(0, step);
				break;
			case 'Delete':
			case 'Backspace':
				e.preventDefault();
				if (editor.editId) {
					// In edit mode never remove the whole layer. If a bezier handle is the
					// active selection, delete just THAT bend; otherwise remove the point.
					if (activeHandle) {
						editor.deleteHandle(activeHandle.node, activeHandle.kind);
						activeHandle = null;
					} else {
						editor.deleteActiveNode();
					}
				} else {
					editor.removeSelected();
				}
				break;
			default:
				if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'd') {
					e.preventDefault();
					editor.duplicateLayer(sel.id);
				}
		}
	}
</script>

<svelte:window
	onkeydown={onKeydown}
	onkeyup={onKeyup}
	onpaste={onPaste}
	onpointerup={cancelAllGestures}
	onpointercancel={cancelAllGestures}
	onblur={cancelAllGestures}
/>

<!-- Viewport: clips + centres the stage and owns zoom/pan (space-drag, ⌘-wheel). -->
<div
	bind:this={viewportEl}
	class="viewport"
	class:grab={spaceDown}
	class:draw={drawCursor && !spaceDown}
	role="presentation"
	onwheel={onWheel}
	onpointerdown={onViewportDown}
	onpointermove={onViewportMove}
	onpointerup={onViewportUp}
	onpointercancel={onViewportUp}
>
<div class="zoomwrap" style="transform: translate({panX}px, {panY}px) scale({zoom});">
<!-- The stage. aspect-ratio keeps the true canvas proportions; width:100%. -->
<div
	bind:clientWidth
	bind:this={stageEl}
	role="presentation"
	class="stage"
	style="aspect-ratio:{layout.width}/{layout.height}; background:#2B2233; cursor:{editor.tool === 'select' ? 'default' : 'crosshair'};"
	onpointerdown={stageDown}
	onpointermove={stageMove}
	onpointerup={stageUp}
	onpointercancel={stageUp}
>
	<!-- Background paint stack (+ its optional whole-background blur), painted as
	     an absolute layer under everything. The stage's own #2B2233 mirrors the Go
	     renderer's brand-ink base, visible through transparent/empty stacks. -->
	{#if bgCss}
		{@const blur = (layout.background.blur ?? 0) * scale}
		<div
			data-stage="bg"
			class="absolute inset-0"
			style="{bgCss} {blur > 0 ? `filter:blur(${blur}px);` : ''}"
		></div>
	{/if}

	<!-- snap guides -->
	{#if guideX !== null}<div class="guide guide-v" style="left:{(guideX / layout.width) * 100}%"></div>{/if}
	{#if guideY !== null}<div class="guide guide-h" style="top:{(guideY / layout.height) * 100}%"></div>{/if}

	<!-- marquee selection rectangle -->
	{#if marquee}
		{@const mx = Math.min(marquee.x0, marquee.x1)}
		{@const my = Math.min(marquee.y0, marquee.y1)}
		<div
			class="marquee"
			style="left:{(mx / layout.width) * 100}%; top:{(my / layout.height) * 100}%; width:{(Math.abs(
				marquee.x1 - marquee.x0
			) /
				layout.width) *
				100}%; height:{(Math.abs(marquee.y1 - marquee.y0) / layout.height) * 100}%;"
		></div>
	{/if}

	<!-- edit-mode hint -->
	{#if editor.editId}
		{@const el = layout.layers.find((l) => l.id === editor.editId)}
		<div class="edit-hint">
			{#if el?.type === 'path'}
				Drag points · double-click a point to curve/sharpen · drag the line to bend · Alt-drag a
				handle to break · double-click the line to add · Delete to remove · Esc to finish
			{:else}
				Editing text — type to change · Esc to finish
			{/if}
		</div>
	{/if}

	{#each visible as l (l.id)}
		{@const b = box(l)}
		{@const isSel = editor.isSelected(l.id)}
		{#if boolMemberIds.has(l.id)}
			{@const gid = l.group ?? ''}
			{@const members = boolMembers(gid)}
			<!-- Boolean group: the whole same-group run is composited as ONE shape,
			     rendered once at the bottom member's z-position (groups are contiguous,
			     so this matches the Go renderer's order). Other members render nothing. -->
			{#if members[0] && l.id === members[0].id}
				<!-- Per-member transparent hit paths so EACH source shape is selectable on the
				     canvas, not just the first: the first click selects the whole boolean group
				     (drag = move the composite); clicking a member again, or another member once
				     you're drilled in, targets that one shape so you can move it independently
				     (e.g. to adjust an intersect). The selected member is ordered last (on top) so
				     it stays grabbable across the overlap; the others stay reachable by their own
				     exposed area. -->
				{@const ordered = members
					.slice()
					.sort((a, b) => Number(editor.isSelected(a.id)) - Number(editor.isSelected(b.id)))}
				<svg class="path-layer" viewBox="0 0 {layout.width} {layout.height}" preserveAspectRatio="none">
					{@html boolSvgInner(gid)}
					{#each ordered as m (m.id)}
						<path
							d={shapeD(m)}
							class="bool-hit"
							class:sel={editor.isSelected(m.id)}
							fill-rule="nonzero"
							role="button"
							tabindex="-1"
							aria-label={m.name}
							onpointerdown={(e) => beginBody(e, m)}
							onpointermove={onMove}
							onpointerup={endGesture}
							onpointercancel={endGesture}
							ondblclick={() => {
								if (m.type === 'text') enterEdit(m);
							}}
						/>
					{/each}
				</svg>
				<!-- Inline text editor for a TEXT member of the boolean (members render only
				     as the composite, so the generic branch's textarea never exists for them). -->
				{#each members as m (m.id)}
					{#if m.id === editor.editId && m.type === 'text'}
						{@const mb = box(m)}
						<div
							class="layer"
							style="left:{mb.left}; top:{mb.top}; width:{mb.width}; height:{mb.height}; transform:rotate({m.rotation ??
								0}deg);"
						>
							<textarea
								bind:this={textEl}
								bind:value={m.text}
								onpointerdown={(e) => e.stopPropagation()}
								onkeydown={(e) => {
									if (e.key === 'Escape') {
										e.stopPropagation();
										editor.exitEdit();
									}
								}}
								class="h-full w-full resize-none bg-transparent p-0 outline-none"
								style="{textCss(m)} box-shadow:0 0 0 1px var(--vec);"
							></textarea>
						</div>
					{/if}
				{/each}
				<!-- Resize handles per selected member (members scale individually for now). -->
				{#each members as m (m.id)}
					{#if editor.isSelected(m.id)}
						{@const mb = box(m)}
						<div class="path-sel" style="left:{mb.left}; top:{mb.top}; width:{mb.width}; height:{mb.height};">
							{#each HANDLES as hd (hd.dir)}
								<button
									type="button"
									aria-label="Resize {hd.dir}"
									class="handle {hd.cls}"
									style="cursor:{hd.cursor};"
									onpointerdown={(e) => beginHandle(e, m, hd.dir)}
									onpointermove={onMove}
									onpointerup={endGesture}
									onpointercancel={endGesture}
								></button>
							{/each}
						</div>
					{/if}
				{/each}
			{/if}
		{:else if l.type === 'path'}
			{@const cx = l.x + l.w / 2}
			{@const cy = l.y + l.h / 2}
			{@const plain = l.id === penId}
			{@const d = strokePathD(l, plain) + (l.id === penId && cursor && (l.nodes?.length ?? 0) ? ` L ${cursor.x} ${cursor.y}` : '')}
			{@const sm = !l.clip && !l.closed ? arrowMarker(`as_${l.id}`, l.start_cap, 'auto-start-reverse', strokeCss(l)) : ''}
			{@const em = !l.clip && !l.closed ? arrowMarker(`ae_${l.id}`, l.end_cap, 'auto', strokeCss(l)) : ''}
			{@const brushed = !!l.brush_name && !l.clip && (l.stroke_width ?? 0) > 0 && !plain}
			{@const strokeMarkup = !l.clip && !brushed ? pathStrokeMarkup(l, d) : ''}
			{@const fxp = hasFx && !l.clip ? fxFilter(l) : ''}
			<svg
				class="path-layer"
				viewBox="0 0 {layout.width} {layout.height}"
				preserveAspectRatio="none"
				style="opacity:{l.opacity ?? 1}; {!hasMasks || l.id === editor.editId ? '' : maskCss(l, true)}"
			>
				<!-- The effect filter wraps only the painted content; node/handle overlays in
				     the outer group stay crisp while editing a path that has a shadow. The
				     {#key} recreates the group when the filter changes (see fx-wrap note). -->
				{#key fxp}
					<g transform="rotate({l.rotation ?? 0} {cx} {cy})" style={fxp}>
					{#if sm || em}<defs>{@html sm}{@html em}</defs>{/if}
					{#if d && !l.clip}{@html pathFillMarkup(l, d)}{/if}
					{#if d && strokeMarkup}{@html strokeMarkup}{/if}
					{#if d}
						<path
							{d}
							fill={l.clip || (visiblePaints(l).length && (l.nodes?.length ?? 0) >= 3) ? 'transparent' : 'none'}
							stroke={l.clip || brushed
								? 'none'
								: strokeMarkup
									? 'transparent'
									: flatStroke(l) || 'transparent'}
							stroke-width={l.clip || brushed ? 0 : (l.stroke_width ?? 0)}
							stroke-linecap={l.stroke_cap ?? 'round'}
							stroke-linejoin={(l.stroke_join === 'miter' ? 'bevel' : l.stroke_join) ?? 'round'}
							stroke-dasharray={strokeDash(l) || undefined}
							marker-start={sm ? `url(#as_${l.id})` : undefined}
							marker-end={em ? `url(#ae_${l.id})` : undefined}
							class="path-stroke"
							class:editing={l.id === editor.editId || editor.tool === 'bend'}
							role="button"
							tabindex="-1"
							aria-label={l.name}
							onpointerdown={(e) => pathDown(e, l)}
							onpointermove={pathMove}
							onpointerup={pathUp}
							onpointercancel={pathUp}
							ondblclick={(e) => (l.id === editor.editId ? addNodeOnSegment(e, l) : enterEdit(l))}
						/>
					{/if}
					{#if brushed}{@html brushPathMarkup(l)}{/if}
					</g>
				{/key}
				<!-- A second, UNFILTERED group with the same transform hosts the pen/edit
				     overlays so they never inherit the layer's shadow/blur. -->
				<g transform="rotate({l.rotation ?? 0} {cx} {cy})">
					{#if l.id === penId}
						{#each l.nodes ?? [] as n, ni (ni)}
							<circle cx={n.x} cy={n.y} r={4 / (scale || 1)} class="path-node" />
						{/each}
					{/if}
					{#if l.id === editor.editId}
						{@const ar = 5 / (scale || 1)}
						{@const hr = 4 / (scale || 1)}
						{#if closeNode}
							<circle cx={closeNode.x} cy={closeNode.y} r={ar * 2.1} class="path-close" />
						{/if}
						{#each l.nodes ?? [] as n, ni (ni)}
							{@const h1out = n.h1x !== n.x || n.h1y !== n.y}
							{@const h2out = n.h2x !== n.x || n.h2y !== n.y}
							{@const curved = h1out || h2out}
							{#if h1out}
								<line x1={n.x} y1={n.y} x2={n.h1x} y2={n.h1y} class="path-hline" />
								<circle
									cx={n.h1x}
									cy={n.h1y}
									r={hr}
									class="path-hdot"
									role="button"
									tabindex="-1"
									aria-label="Handle in"
									onpointerdown={(e) => startNodeDrag(e, ni, 'h1')}
									onpointermove={moveNodeDrag}
									onpointerup={endNodeDrag}
									onpointercancel={endNodeDrag}
								/>
							{/if}
							{#if h2out}
								<line x1={n.x} y1={n.y} x2={n.h2x} y2={n.h2y} class="path-hline" />
								<circle
									cx={n.h2x}
									cy={n.h2y}
									r={hr}
									class="path-hdot"
									role="button"
									tabindex="-1"
									aria-label="Handle out"
									onpointerdown={(e) => startNodeDrag(e, ni, 'h2')}
									onpointermove={moveNodeDrag}
									onpointerup={endNodeDrag}
									onpointercancel={endNodeDrag}
								/>
							{/if}
							<rect
								x={n.x - ar}
								y={n.y - ar}
								width={ar * 2}
								height={ar * 2}
								rx={curved ? ar : ar * 0.25}
								class="path-anchor"
								class:active={editor.activeNode === ni}
								role="button"
								tabindex="-1"
								aria-label="Node {ni + 1}"
								onpointerdown={(e) => startNodeDrag(e, ni, 'anchor')}
								onpointermove={moveNodeDrag}
								onpointerup={endNodeDrag}
								onpointercancel={endNodeDrag}
								ondblclick={(e) => {
									e.stopPropagation();
									editor.toggleNodeType(ni);
								}}
							/>
						{/each}
					{/if}
				</g>
			</svg>
			{#if isSel && l.id !== penId && l.id !== editor.editId}
				<div
					class="path-sel"
					style="left:{(l.x / layout.width) * 100}%; top:{(l.y / layout.height) * 100}%; width:{(l.w /
						layout.width) *
						100}%; height:{(l.h / layout.height) * 100}%;"
				>
					<!-- Resize handles: scaling a shape/path works just like a rectangle. -->
					{#each HANDLES as hd (hd.dir)}
						<button
							type="button"
							aria-label="Resize {hd.dir}"
							class="handle {hd.cls}"
							style="cursor:{hd.cursor};"
							onpointerdown={(e) => beginHandle(e, l, hd.dir)}
							onpointermove={onMove}
							onpointerup={endGesture}
							onpointercancel={endGesture}
						></button>
					{/each}
				</div>
			{/if}
		{:else}
		{@const wobAny = (l.dynamic_wiggle ?? 0) > 0 && (l.stroke_width ?? 0) > 0 && !l.clip}
		{@const shadowsViaBox =
			!l.clip &&
			(((l.type === 'rect' || l.type === 'ellipse') && !wobAny) ||
				(l.type === 'image' && (l.fit ?? 'cover') !== 'contain'))}
		{@const fxs = hasFx && !l.clip ? fxFilter(l, !shadowsViaBox) + fxBackdrop(l) : ''}
		<!-- Layer body. role/aria make it operable; pointer events drive move. -->
		<div
			role="button"
			tabindex="-1"
			aria-label={l.name}
			aria-pressed={isSel}
			class="layer"
			class:selected={isSel}
			style="left:{b.left}; top:{b.top}; width:{b.width}; height:{b.height}; opacity:{l.opacity ??
				1}; transform:rotate({l.rotation ?? 0}deg); {hasMasks ? maskCss(l) : ''}"
			onpointerdown={(e) => beginBody(e, l)}
			onpointermove={onMove}
			onpointerup={endGesture}
			onpointercancel={endGesture}
			ondblclick={() => enterEdit(l)}
		>
			<!-- Effects live on this inner wrapper, NOT the .layer div: the layer div also
			     carries the selection chrome, and a drop-shadow/blur filter there would
			     shadow the chrome itself. The {#key} recreates the wrapper whenever the
			     filter string changes: Chromium fails to invalidate a filtered element's
			     cached raster when only its filter value changes under overlapping
			     siblings (the selection ring), so live shadow edits looked stale until
			     the next unrelated repaint. A fresh element always rasterises fresh. -->
			{#key fxs}
			<div class="fx-wrap" style={fxs}>
			{#if l.type === 'text'}
				{#if l.id === editor.editId}
					<textarea
						bind:this={textEl}
						bind:value={l.text}
						onpointerdown={(e) => e.stopPropagation()}
						onkeydown={(e) => {
							if (e.key === 'Escape') {
								e.stopPropagation();
								editor.exitEdit();
							}
						}}
						class="h-full w-full resize-none bg-transparent p-0 outline-none"
						style="{textCss(l)} box-shadow:0 0 0 1px var(--vec);"
					></textarea>
				{:else}
					<!-- A text stencil's own glyphs aren't painted (only its shape masks the
						content above it); show a faint ghost while it's selected so it stays
						editable, mirroring the rect/image stencils. -->
					<div
						class="h-full w-full"
						class:stencil-ghost={l.clip && isSel}
						style="{vAlignCss(l)} {l.clip ? (isSel ? 'opacity:0.4;' : 'opacity:0;') : ''}"
					>
						<div style="width:100%; {textCss(l)}">
							{dtext(l)}
						</div>
					</div>
				{/if}
			{:else if l.type === 'image'}
				{@const src = dsrc(l)}
				{@const rad = radiusCss(l)}
				<!-- Border = the Figma-style stroke (inset, follows the corner radius). Shown
				     normally, and also on a SELECTED stencil so a masked image reads as itself. -->
				{@const sw = (l.stroke_width ?? 0) * scale}
				{@const wob = (l.dynamic_wiggle ?? 0) > 0 && (l.stroke_width ?? 0) > 0 && !l.clip}
					{@const brush = !!l.brush_name && (l.stroke_width ?? 0) > 0 && !l.clip}
					{@const svgStroke = !wob && !brush ? boxStrokeSvg(l) : ''}
					{@const strokeBase = sw > 0 && !svgStroke && l.stroke_style !== 'dashed' && !wob && !brush && (!l.clip || isSel) ? strokeShadow(l, sw) : ''}
				{@const ringCss = boxShadow(l, strokeBase, shadowsViaBox)}
				<!-- A stencil's own pixels aren't part of the card (only its SHAPE clips the
				     content): fade it to a faint ghost while selected and hide it otherwise. -->
				<div
					class="relative grid h-full w-full place-items-center"
					class:stencil-ghost={l.clip && isSel}
					class:opacity-40={l.clip && isSel}
					class:opacity-0={l.clip && !isSel}
				>
					{#if src}
						<img
							src={src}
							alt={l.name}
							draggable="false"
							class="block h-full w-full select-none"
							style="object-fit:{l.fit ?? 'cover'}; border-radius:{rad};"
						/>
						{#if ringCss}
							<!-- The box-shadows live on an overlay ABOVE the img: inset shadows
							     (inner-shadow effects, inside/center strokes) paint BEHIND a
							     replaced element's pixels, so on the <img> itself they would be
							     invisible. Outer entries only paint outside the box, so the
							     overlay never covers the picture. -->
							<div class="pointer-events-none absolute inset-0" style="border-radius:{rad}; {ringCss}"></div>
						{/if}
					{:else}
						<!-- Empty / bound source → neutral placeholder tile. -->
						<div class="grid h-full w-full place-items-center bg-line-strong/60 text-faint" style="border-radius:{rad}; {ringCss}">
							<ImageIcon size={Math.max(14, Math.min(l.w, l.h) * scale * 0.34)} strokeWidth={1.5} />
						</div>
					{/if}
				</div>
				{#if wob}{@html wobbleSvg(l, false)}{:else if brush}{@html brushShapeSvg(l)}{:else if svgStroke}{@html svgStroke}{/if}
			{:else if l.type === 'rect'}
				{@const sw = (l.stroke_width ?? 0) * scale}
				{@const wob = (l.dynamic_wiggle ?? 0) > 0 && (l.stroke_width ?? 0) > 0 && !l.clip}
				{@const brush = !!l.brush_name && (l.stroke_width ?? 0) > 0 && !l.clip}
				{@const perSide = sw > 0 && !l.clip && sidesRestricted(l) && !wob && !brush}
				{@const svgStroke = !l.clip && !perSide && !wob && !brush ? boxStrokeSvg(l) : ''}
				{@const strokeBase = sw > 0 && !l.clip && !svgStroke && !perSide && !wob && !brush ? strokeShadow(l, sw) : ''}
				<!-- A `progress` rect previews the XP fill: painted width = layer width ×
				     the sample progress fraction, anchored left, so the live canvas matches
				     the server (welcome cards, with no {progress}, render full width). -->
				{@const pw = l.progress ? progressFrac : 1}
				<div
					class="h-full"
					class:w-full={pw >= 1}
					class:stencil-ghost={l.clip && isSel}
					style="{pw < 1 ? `width:${pw * 100}%;` : ''} {wob || l.clip ? 'background:transparent;' : cssBackgrounds(l)} border-radius:{radiusCss(l)}; {wob ? '' : boxShadow(l, strokeBase, shadowsViaBox)}"
				></div>
				{#if wob}{@html wobbleSvg(l, true)}{:else if brush}{@html brushShapeSvg(l)}{:else if perSide}{@html strokeSidesSvg(l)}{:else if svgStroke}{@html svgStroke}{/if}
			{:else if l.type === 'ellipse'}
				{@const sw = (l.stroke_width ?? 0) * scale}
				{@const wob = (l.dynamic_wiggle ?? 0) > 0 && (l.stroke_width ?? 0) > 0 && !l.clip}
				{@const brush = !!l.brush_name && (l.stroke_width ?? 0) > 0 && !l.clip}
				{@const svgStroke = !l.clip && !wob && !brush ? boxStrokeSvg(l) : ''}
				{@const strokeBase = sw > 0 && !l.clip && !svgStroke && !wob && !brush ? strokeShadow(l, sw) : ''}
				<div
					class="h-full w-full"
					class:stencil-ghost={l.clip && isSel}
					style="{wob || l.clip ? 'background:transparent;' : cssBackgrounds(l)} border-radius:50%; {wob ? '' : boxShadow(l, strokeBase, shadowsViaBox)}"
				></div>
				{#if wob}{@html wobbleSvg(l, true)}{:else if brush}{@html brushShapeSvg(l)}{:else if svgStroke}{@html svgStroke}{/if}
			{/if}

			</div>
			{/key}
		</div>
		{/if}
	{/each}
	<!-- Selection chrome overlay: outline + resize handles for box layers render ABOVE
	     all layers (paths/booleans keep their own .path-sel chrome). The content itself
	     stays at its natural stacking position — lifting the selected layer (the old
	     z-index approach) painted it and its drop shadow over layers that should cover
	     it, so shadows looked wrong exactly while selected. -->
	{#each visible as l (`sel:${l.id}`)}
		{#if editor.isSelected(l.id) && !boolMemberIds.has(l.id) && l.type !== 'path'}
			{@const b = box(l)}
			<div
				class="sel-chrome"
				style="left:{b.left}; top:{b.top}; width:{b.width}; height:{b.height}; transform:rotate({l.rotation ??
					0}deg);"
			>
				<!-- Rotate zones just outside each corner (single selection only), reusing
				     the handle capture/endGesture plumbing. -->
				{#if editor.selectedIds.length === 1 && !l.locked}
					{#each ROT_ZONES as rz (rz.key)}
						<button
							type="button"
							aria-label="Rotate"
							class="rot-zone"
							style={rz.style}
							onpointerdown={(e) => beginRotate(e, l)}
							onpointermove={rotateMove}
							onpointerup={rotateUp}
							onpointercancel={rotateUp}
						></button>
					{/each}
				{/if}
				{#each HANDLES as hd (hd.dir)}
					<button
						type="button"
						aria-label="Resize {hd.dir}"
						class="handle {hd.cls}"
						style="cursor:{hd.cursor};"
						onpointerdown={(e) => beginHandle(e, l, hd.dir)}
						onpointermove={onMove}
						onpointerup={endGesture}
						onpointercancel={endGesture}
					></button>
				{/each}
			</div>
		{/if}
	{/each}
</div>
</div>

	<!-- Live rotation angle, pinned near the cursor while a rotate-zone is dragged. -->
	{#if rot}
		<div class="rot-badge" style="left:{rot.sx + 16}px; top:{rot.sy + 16}px;">{rot.angle}°</div>
	{/if}

	<!-- zoom control -->
	<div class="zoomctl">
		<button type="button" aria-label="Zoom out" onclick={() => setZoom(zoom / 1.2)}>−</button>
		<button type="button" onclick={resetView} title="Reset zoom">{Math.round(zoom * 100)}%</button>
		<button type="button" aria-label="Zoom in" onclick={() => setZoom(zoom * 1.2)}>+</button>
	</div>
</div>

<style>
	.viewport {
		position: absolute;
		inset: 0;
		overflow: hidden;
		display: grid;
		place-items: center;
		padding: 2rem;
		touch-action: none;
		/* Vector-editing overlay colours: a high-contrast cool blue + white that
		   reads on any card background (the rose accent vanishes on the gradient).
		   Scoped to the canvas editing UI only — never page chrome. */
		--vec: #0d99ff; /* Figma selection blue */
		--vec-ink: #0b3a63;
		--vec-close: #34d399;
		--vec-guide: #f24822; /* Figma's red snap/alignment guide */
	}
	.viewport.grab {
		cursor: grab;
	}
	.viewport.grab:active {
		cursor: grabbing;
	}
	/* A draw tool active → crosshair across the canvas, even over existing layers. */
	.viewport.draw,
	.viewport.draw :is(.layer, .path-stroke, .bool-hit) {
		cursor: crosshair;
	}
	.zoomwrap {
		width: 100%;
		transform-origin: center;
		will-change: transform;
	}

	.stage {
		position: relative;
		width: 100%;
		overflow: hidden;
		border-radius: 12px;
		isolation: isolate;
		/* float the card above the pit with a hairline ring + soft elevation */
		box-shadow:
			0 0 0 1px var(--color-line-strong),
			0 24px 60px -20px rgba(0, 0, 0, 0.7),
			0 8px 24px -12px rgba(0, 0, 0, 0.6);
		/* The editor owns all canvas gestures (draw/marquee/drag) — never let the
		   browser claim a touch drag for scrolling (that fires pointercancel and
		   kills the gesture). Pan/zoom is via space-drag + ctrl/⌘-wheel. */
		touch-action: none;
		user-select: none;
	}

	.marquee {
		position: absolute;
		z-index: 4;
		border: 1px solid var(--vec);
		background: color-mix(in srgb, var(--vec) 12%, transparent);
		pointer-events: none;
	}

	.zoomctl {
		position: absolute;
		right: 12px;
		bottom: 12px;
		z-index: 6;
		display: flex;
		align-items: center;
		gap: 2px;
		padding: 2px;
		border-radius: 9999px;
		border: 1px solid var(--color-line-strong);
		background: color-mix(in srgb, var(--color-surface) 92%, transparent);
		backdrop-filter: blur(6px);
	}
	.zoomctl button {
		min-width: 28px;
		height: 24px;
		padding: 0 6px;
		border-radius: 9999px;
		font-size: 12px;
		font-variant-numeric: tabular-nums;
		color: var(--color-muted);
		transition: color 0.12s, background-color 0.12s;
	}
	.zoomctl button:hover {
		color: var(--color-ink);
		background: var(--color-ink-2);
	}

	/* A mask stencil is shown in the editor as a dashed outline, not a solid fill —
	   so it never obscures the content it clips, even when selecting it lifts its
	   z-index above that content (Figma shows mask shapes the same way). The mask
	   itself reads the stencil's geometry, not this DOM fill, so it's unaffected. */
	.stencil-ghost {
		border: 1px dashed color-mix(in srgb, var(--vec) 60%, transparent);
	}
	.layer {
		position: absolute;
		cursor: default;
		touch-action: none;
		outline: none;
		/* NO will-change here: promoting every layer to its own composited surface
		   makes Chrome cache a stale raster when a child's filter (drop shadow/blur)
		   changes — the shadow then only updates on the next unrelated repaint. */
	}
	.layer.selected {
		cursor: move;
	}
	/* Full-size effects wrapper inside .layer — hosts filter/backdrop-filter so the
	   selection outline and handles on .layer never get shadowed or blurred. */
	.fx-wrap {
		position: absolute;
		inset: 0;
	}
	/* Dashed-stroke overlay for box shapes (rect/ellipse/image) — box-shadow can't dash.
	   Fills the layer box; the stroke is painted by the inline SVG, never takes pointers. */
	.dash-stroke {
		position: absolute;
		inset: 0;
		width: 100%;
		height: 100%;
		overflow: visible;
		pointer-events: none;
	}
	/* Selection chrome lives in .sel-chrome (an overlay above all layers) so the
	   layer itself — and its drop shadow — keep their natural stacking order. */
	.sel-chrome {
		position: absolute;
		outline: 1px solid var(--vec);
		outline-offset: 0;
		pointer-events: none;
		z-index: 3;
	}
	.sel-chrome .handle {
		pointer-events: all;
	}
	/* Rotate grab-zones: small transparent squares just outside the corner handles.
	   Sit below the resize handles' z so a corner drag still resizes. */
	.rot-zone {
		position: absolute;
		width: 16px;
		height: 16px;
		padding: 0;
		background: transparent;
		border: 0;
		pointer-events: all;
		cursor: grab;
		z-index: 2;
	}
	.rot-zone:active {
		cursor: grabbing;
	}
	/* Live angle readout that follows the cursor during a rotate drag. */
	.rot-badge {
		position: fixed;
		z-index: 50;
		padding: 2px 6px;
		border-radius: 6px;
		border: 1px solid var(--color-line-strong);
		background: color-mix(in srgb, var(--color-surface) 92%, transparent);
		font-family: var(--font-mono, monospace);
		font-size: 11px;
		font-variant-numeric: tabular-nums;
		color: var(--color-ink);
		pointer-events: none;
		white-space: nowrap;
	}

	/* Path layers render as a full-stage SVG; only the painted path takes pointers
	   so layers below stay clickable through the transparent areas. */
	.path-layer {
		position: absolute;
		inset: 0;
		overflow: visible;
		pointer-events: none;
	}
	.path-stroke {
		pointer-events: visiblePainted;
		cursor: default;
	}
	/* The transparent silhouette of a composited boolean group — its fill region is
	   the single hit target for selecting/dragging the whole group. */
	.bool-hit {
		fill: transparent;
		pointer-events: fill;
		cursor: default;
	}
	.bool-hit.sel {
		cursor: move;
	}
	.path-node {
		fill: var(--vec);
		stroke: #fff;
		stroke-width: 1;
		pointer-events: none;
	}
	.path-sel {
		position: absolute;
		outline: 1px dashed var(--vec);
		pointer-events: none;
		z-index: 2;
	}
	.path-stroke.editing {
		cursor: crosshair; /* drag to bend, double-click to add a point */
	}

	/* path edit mode: anchors, bezier handles and their connector lines. White
	   fills + a vector-blue stroke + a dark halo so points pop on any card. */
	.path-hline {
		stroke: var(--vec);
		stroke-width: 1.25;
		vector-effect: non-scaling-stroke;
		pointer-events: none;
	}
	.path-hdot {
		fill: #fff;
		stroke: var(--vec);
		stroke-width: 1.5;
		vector-effect: non-scaling-stroke;
		pointer-events: all;
		cursor: grab;
		filter: drop-shadow(0 0 1.5px rgba(0, 0, 0, 0.55));
	}
	.path-anchor {
		fill: #fff;
		stroke: var(--vec);
		stroke-width: 1.5;
		vector-effect: non-scaling-stroke;
		pointer-events: all;
		cursor: grab;
		filter: drop-shadow(0 0 1.5px rgba(0, 0, 0, 0.55));
	}
	.path-anchor.active {
		fill: var(--vec);
		stroke: #fff;
	}
	.path-close {
		fill: color-mix(in srgb, var(--vec-close) 30%, transparent);
		stroke: var(--vec-close);
		stroke-width: 1.5;
		vector-effect: non-scaling-stroke;
		pointer-events: none;
		filter: drop-shadow(0 0 2px rgba(0, 0, 0, 0.5));
	}

	.edit-hint {
		position: absolute;
		top: 8px;
		left: 50%;
		transform: translateX(-50%);
		z-index: 5;
		max-width: 92%;
		padding: 4px 10px;
		border-radius: 9999px;
		background: color-mix(in srgb, var(--color-ink-2) 88%, transparent);
		border: 1px solid var(--color-line-strong);
		color: var(--color-muted);
		font-size: 11px;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
		pointer-events: none;
		backdrop-filter: blur(4px);
	}

	.guide {
		position: absolute;
		z-index: 4;
		background: var(--vec-guide);
		pointer-events: none;
	}
	.guide-v {
		top: 0;
		bottom: 0;
		width: 1px;
	}
	.guide-h {
		left: 0;
		right: 0;
		height: 1px;
	}

	.handle {
		position: absolute;
		width: 8px;
		height: 8px;
		margin: -1px;
		padding: 0;
		background: #fff;
		border: 1px solid var(--vec);
		border-radius: 1px;
		box-shadow: 0 1px 2px rgba(0, 0, 0, 0.3);
		touch-action: none;
		z-index: 3;
		/* re-enable hit-testing inside the pointer-events:none .path-sel outline */
		pointer-events: auto;
	}
	/* Invisible hit-slop so the 8px handle is grabbable by mouse, and much bigger by
	   finger — resize is unusable on touch otherwise. */
	.handle::before {
		content: '';
		position: absolute;
		inset: -8px;
	}
	@media (pointer: coarse) {
		.handle::before {
			inset: -16px;
		}
	}
	.handle:focus-visible {
		outline: 2px solid var(--vec);
		outline-offset: 1px;
	}
</style>
