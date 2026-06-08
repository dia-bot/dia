<script lang="ts">
	// The interactive editing canvas — the centrepiece of the layout designer.
	// Paints editor.layout into a DOM stage (background + ordered layers), handles
	// selection, drag-to-move and 8-handle resize via pointer capture. Geometry is
	// stored in canvas pixels; we render layers as percentages so positioning is
	// scale-independent, and only multiply size-like fields (font, ring, radius,
	// blur) by the live scale = renderedWidth / layout.width.
	import { getContext } from 'svelte';
	import { EditorStore, EDITOR_CTX } from '$lib/layout/editor.svelte';
	import { resolveSrc, newLayer, cornerNode, pathD, type Layer, type PathNode, type ShapeKind, type Effect } from '$lib/layout/schema';
	import { fontCss } from '$lib/layout/fonts';
	import { ImageIcon, User } from 'lucide-svelte';

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

	// Rendered pixel width of the stage; scale maps canvas px → screen px.
	let clientWidth = $state(0);
	const scale = $derived(clientWidth > 0 ? clientWidth / layout.width : 0);

	// Background paint string for the stage element.
	const bgCss = $derived.by(() => {
		const b = layout.background;
		if (b.type === 'solid') return b.color || '#000000';
		if (b.type === 'gradient')
			return `linear-gradient(${b.angle ?? 0}deg, ${b.from ?? '#000'}, ${b.to ?? '#000'})`;
		return '#0b0b0e'; // image is painted by a dedicated layer below
	});

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
	// drawStencilText paints a text stencil's glyphs (white) in CANVAS coords onto ctx,
	// wrapping to the box width and honouring align/valign/line-height/letter-spacing/
	// case/decoration — the same layout as the live preview text. `erase` uses
	// destination-out (for an inverted mask: punch the glyphs out of a white box).
	function drawStencilText(ctx: CanvasRenderingContext2D, m: Layer, erase: boolean) {
		const size = m.font_size ?? 16;
		const weight = m.font_weight ?? 400;
		const ls = m.letter_spacing ?? 0;
		ctx.font = `${weight} ${size}px ${fontCss(m.font_family)}`;
		ctx.textBaseline = 'alphabetic';
		ctx.fillStyle = '#fff';
		ctx.globalCompositeOperation = erase ? 'destination-out' : 'source-over';
		const raw = stencilCase(dtext(m), m.text_case);
		// greedy word-wrap to the box width (canvas px), preserving explicit newlines.
		const lines: string[] = [];
		for (const para of raw.split('\n')) {
			let cur = '';
			for (const word of para.split(' ')) {
				const next = cur ? `${cur} ${word}` : word;
				if (cur && lineW(ctx, next, ls) > m.w) {
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
		const thick = Math.max(1, size * 0.06);
		lines.forEach((line, i) => {
			if (!line) return;
			const w = lineW(ctx, line, ls);
			let x0 = m.x;
			if (m.align === 'center') x0 = m.x + (m.w - w) / 2;
			else if (m.align === 'right') x0 = m.x + m.w - w;
			const y = top + i * lh + asc;
			if (ls) {
				let cx = x0;
				for (const ch of line) {
					ctx.fillText(ch, cx, y);
					cx += ctx.measureText(ch).width + ls;
				}
			} else {
				ctx.fillText(line, x0, y);
			}
			if (m.text_decoration === 'underline' || m.text_decoration === 'strike') {
				const dy = m.text_decoration === 'strike' ? y - size * 0.28 : y + thick;
				ctx.fillRect(x0, dy, w, thick);
			}
		});
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
		drawStencilText(ctx, m, invert);
		ctx.restore();
		let url: string;
		try {
			url = cv.toDataURL();
		} catch {
			return '';
		}
		const u = `url("${url}")`;
		const css = `-webkit-mask:${u} center/100% 100% no-repeat; mask:${u} center/100% 100% no-repeat; -webkit-mask-mode:alpha; mask-mode:alpha;`;
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

	function shapeEl(m: Layer, fill: string): string {
		// (A text stencil never reaches here — maskCss rasterises it via textMaskCss,
		// because browsers don't render SVG <text> in the CSS masking pipeline.)
		// An avatar stencil is a circle (default) or a rounded square (shape: rounded),
		// matching how the avatar itself draws — never a plain bounding box.
		if (m.type === 'avatar') {
			if (m.shape !== 'rounded') {
				const r = Math.min(m.w, m.h) / 2;
				return `<circle cx='${m.x + m.w / 2}' cy='${m.y + m.h / 2}' r='${r}' fill='${fill}'/>`;
			}
			// rounded avatar → per-corner rounded rect; when neither corners[] nor radius
			// is set, Go rounds by radius*0.3 = (min(w,h)/2)*0.3 (its gentle default).
			const hasCorners = Array.isArray(m.corners) && m.corners.length === 4;
			const def = (Math.min(m.w, m.h) / 2) * 0.3;
			const radii: [number, number, number, number] = hasCorners
				? stencilRadii(m)
				: m.radius && m.radius > 0
					? [m.radius, m.radius, m.radius, m.radius]
					: [def, def, def, def];
			return roundRectEl(m, fill, radii);
		}
		if (m.type === 'image' && m.mask === 'circle') {
			const r = Math.min(m.w, m.h) / 2;
			return `<circle cx='${m.x + m.w / 2}' cy='${m.y + m.h / 2}' r='${r}' fill='${fill}'/>`;
		}
		if (m.type === 'image' && m.mask === 'ellipse') {
			return `<ellipse cx='${m.x + m.w / 2}' cy='${m.y + m.h / 2}' rx='${m.w / 2}' ry='${m.h / 2}' fill='${fill}'/>`;
		}
		// rect / ellipse / path stencils → one path with rotation + corner radius baked
		// in (shapeD), matching the Go renderer's drawSilhouette / clip shape.
		if (m.type === 'rect' || m.type === 'ellipse' || m.type === 'path') {
			const d = shapeD(m);
			return d ? `<path d='${d}' fill='${fill}'/>` : '';
		}
		// image stencil with no explicit mask → its (rounded) bounding box, with the
		// stencil's corners + rotation baked in (parity with the rect/path stencils).
		return roundRectEl(m, fill, stencilRadii(m));
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
		const shape = shapeEl(m, '#fff');
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
		h = h.padEnd(6, '0').slice(0, 6);
		const r = parseInt(h.slice(0, 2), 16) || 0;
		const g = parseInt(h.slice(2, 4), 16) || 0;
		const b = parseInt(h.slice(4, 6), 16) || 0;
		return `rgba(${r},${g},${b},${op ?? 0.25})`;
	}
	// filter: layer blur + drop shadows (follows the layer's alpha — works for text,
	// images and vector paths). Spread is folded into the blur as a coarse approximation.
	function fxFilter(l: Layer): string {
		const parts: string[] = [];
		for (const e of fxList(l)) {
			if (e.type === 'layer_blur' && (e.radius ?? 0) > 0) parts.push(`blur(${(e.radius ?? 0) * scale}px)`);
		}
		for (const e of fxList(l)) {
			if (e.type !== 'drop_shadow') continue;
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
	// boxShadow joins a layer's stroke/ring shadow with its inner-shadow effects.
	// Inner shadows are listed FIRST so they paint OVER the stroke/ring (CSS box-shadow
	// paints the first entry on top), matching the Go draw order (content+stroke, then
	// inner shadows). A stencil (l.clip) is never composited, so it carries no effects.
	function boxShadow(l: Layer, base: string): string {
		const parts = [...(hasFx && !l.clip ? fxInset(l) : []), base].filter(Boolean);
		return parts.length ? `box-shadow:${parts.join(', ')};` : '';
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
		return (
			`white-space:pre-wrap; overflow-wrap:break-word; text-align:${l.align ?? 'left'};` +
			` font-family:${fontCss(l.font_family)}; font-size:${(l.font_size ?? 16) * scale}px;` +
			` font-weight:${l.font_weight ?? 400}; color:${l.color ?? '#fff'}; line-height:${lh};` +
			` letter-spacing:${ls}px; text-transform:${tc}; text-decoration:${td};`
		);
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
	// One transparent silhouette spanning the whole group — the click/drag hit area.
	function boolUnionD(members: Layer[]): string {
		return members.map(shapeD).filter(Boolean).join(' ');
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
		// `||` not `??`: an unfilled path has fill==='' and must fall back to white, to
		// match the Go renderer (parseHex('', white)). `??` would emit fill='' = black.
		const fill = src.fill || '#FFFFFF';
		const opacity = Math.min(1, src.opacity ?? 1);
		if (opacity <= 0) return '';
		const sid = gid.replace(/[^a-zA-Z0-9_-]/g, ''); // safe <defs> id fragment
		const ds = members.map(shapeD);
		// An image/avatar source keeps its pixels: clip the real image to the boolean
		// coverage (mirrors the Go renderer). A non-image source fills with its colour.
		const imgSrc = src.type === 'image' || src.type === 'avatar' ? dsrc(src) : '';
		if (imgSrc) {
			const href = imgSrc.replace(/&/g, '&amp;').replace(/'/g, '&#39;');
			const par = (src.fit ?? 'cover') === 'contain' ? 'xMidYMid meet' : 'xMidYMid slice';
			const img = `<image href='${href}' x='${src.x}' y='${src.y}' width='${src.w}' height='${src.h}' preserveAspectRatio='${par}' opacity='${opacity}'`;
			if (op === 'subtract') {
				const holes = ds
					.slice(1)
					.map((d) => `<path d='${d}' fill='#000'/>`)
					.join('');
				return `<defs><mask id='bm_${sid}'><path d='${ds[0]}' fill='#fff'/>${holes}</mask></defs>${img} mask='url(#bm_${sid})'/>`;
			}
			if (op === 'exclude') {
				return `<defs><clipPath id='bx_${sid}'><path d='${ds.join(' ')}' clip-rule='evenodd'/></clipPath></defs>${img} clip-path='url(#bx_${sid})'/>`;
			}
			if (op === 'intersect') {
				// nested clip-paths over EVERY member → image ∩ all members
				let defs = '';
				for (let k = 0; k < ds.length; k++) {
					const ref = k > 0 ? ` clip-path='url(#bc_${sid}_${k - 1})'` : '';
					defs += `<clipPath id='bc_${sid}_${k}'${ref}><path d='${ds[k]}'/></clipPath>`;
				}
				return `<defs>${defs}</defs>${img} clip-path='url(#bc_${sid}_${ds.length - 1})'/>`;
			}
			// union: clip the image to the union of every member shape
			const cp = ds.map((d) => `<path d='${d}'/>`).join('');
			return `<defs><clipPath id='bu_${sid}'>${cp}</clipPath></defs>${img} clip-path='url(#bu_${sid})'/>`;
		}
		if (op === 'exclude') {
			return `<path d='${ds.join(' ')}' fill='${fill}' fill-rule='evenodd' opacity='${opacity}'/>`;
		}
		if (op === 'subtract') {
			const holes = ds
				.slice(1)
				.map((d) => `<path d='${d}' fill='#000'/>`)
				.join('');
			return `<defs><mask id='bm_${sid}'><path d='${ds[0]}' fill='#fff'/>${holes}</mask></defs><path d='${ds[0]}' fill='${fill}' opacity='${opacity}' mask='url(#bm_${sid})'/>`;
		}
		if (op === 'intersect') {
			let defs = '';
			for (let k = 1; k < ds.length; k++) {
				const ref = k > 1 ? ` clip-path='url(#bc_${sid}_${k - 1})'` : '';
				defs += `<clipPath id='bc_${sid}_${k}'${ref}><path d='${ds[k]}'/></clipPath>`;
			}
			const clip = ds.length > 1 ? ` clip-path='url(#bc_${sid}_${ds.length - 1})'` : '';
			return `<defs>${defs}</defs><path d='${ds[0]}' fill='${fill}' opacity='${opacity}'${clip}/>`;
		}
		// union
		const paths = ds.map((d) => `<path d='${d}' fill='${fill}'/>`).join('');
		return `<g opacity='${opacity}'>${paths}</g>`;
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
				radius: l.radius,
				ring: l.ring_width
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

	// A gesture is in progress — used to ignore a second finger / stray pointerdown.
	function busy(): boolean {
		return !!(g || draw || marquee || pan || nodeDrag || segDrag || bend);
	}
	// Single hard reset for ALL canvas gestures — wired to window pointercancel/blur
	// so a drag can never get stuck (the browser stealing the pointer, tab blur, the
	// SVG implicit-capture drop on touch, etc.).
	function cancelAllGestures() {
		g = null;
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
			editor.select(draw.id);
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
				if (!path.fill) path.fill = path.stroke_color || '#FFFFFF';
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
	// Focus the inline text editor as soon as it appears.
	$effect(() => {
		if (editor.editId && textEl) textEl.focus();
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
				case 'v':
					e.preventDefault();
					editor.paste();
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

		if (e.key === 'Enter' && penId) {
			finishPen();
			editor.setTool('select');
			return;
		}
		if (e.key === 'Escape') {
			if (editor.editId) editor.exitEdit();
			else if (editor.tool !== 'select') editor.setTool('select');
			else editor.select(null);
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
	style="aspect-ratio:{layout.width}/{layout.height}; background:{bgCss}; cursor:{editor.tool === 'select' ? 'default' : 'crosshair'};"
	onpointerdown={stageDown}
	onpointermove={stageMove}
	onpointerup={stageUp}
	onpointercancel={stageUp}
>
	<!-- Image background + its blur, painted as absolute layers under everything. -->
	{#if layout.background.type === 'image' && resolveSrc(layout.background.image_url)}
		{@const blur = (layout.background.blur ?? 0) * scale}
		<div
			data-stage="bg"
			class="absolute inset-0"
			style="background-image:url({resolveSrc(layout.background.image_url)}); background-size:cover; background-position:center; {blur >
			0
				? `filter:blur(${blur}px);`
				: ''}"
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
				{@const grpSel = editor.isSelected(members[0].id)}
				<svg class="path-layer" viewBox="0 0 {layout.width} {layout.height}" preserveAspectRatio="none">
					{@html boolSvgInner(gid)}
					<!-- transparent silhouette over the whole group = one select/drag target -->
					<path
						d={boolUnionD(members)}
						class="bool-hit"
						class:sel={grpSel}
						fill-rule="nonzero"
						role="button"
						tabindex="-1"
						aria-label={editor.groupName(gid)}
						onpointerdown={(e) => beginBody(e, members[0])}
						onpointermove={onMove}
						onpointerup={endGesture}
						onpointercancel={endGesture}
					/>
				</svg>
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
			{@const d = pathD(l.nodes, l.closed) + (l.id === penId && cursor && (l.nodes?.length ?? 0) ? ` L ${cursor.x} ${cursor.y}` : '')}
			<svg
				class="path-layer"
				viewBox="0 0 {layout.width} {layout.height}"
				preserveAspectRatio="none"
				style="opacity:{l.opacity ?? 1}; {!hasMasks || l.id === editor.editId ? '' : maskCss(l, true)} {hasFx && !l.clip ? fxFilter(l) : ''}"
			>
				<g transform="rotate({l.rotation ?? 0} {cx} {cy})">
					{#if d}
						<path
							{d}
							fill={l.clip ? 'transparent' : l.closed && l.fill && (l.nodes?.length ?? 0) >= 3 ? l.fill : 'none'}
							stroke={l.clip ? 'none' : (l.stroke_color ?? '#fff')}
							stroke-width={l.clip ? 0 : (l.stroke_width ?? 4)}
							stroke-linecap="round"
							stroke-linejoin="round"
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
		<!-- Layer body. role/aria make it operable; pointer events drive move. -->
		<div
			role="button"
			tabindex="-1"
			aria-label={l.name}
			aria-pressed={isSel}
			class="layer"
			class:selected={isSel}
			style="left:{b.left}; top:{b.top}; width:{b.width}; height:{b.height}; opacity:{l.opacity ??
				1}; transform:rotate({l.rotation ?? 0}deg); {hasMasks ? maskCss(l) : ''} {hasFx && !l.clip
				? fxFilter(l) + fxBackdrop(l)
				: ''}"
			onpointerdown={(e) => beginBody(e, l)}
			onpointermove={onMove}
			onpointerup={endGesture}
			onpointercancel={endGesture}
			ondblclick={() => enterEdit(l)}
		>
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
					<div class="h-full w-full" style={vAlignCss(l)}>
						<div style="width:100%; {textCss(l)}">
							{dtext(l)}
						</div>
					</div>
				{/if}
			{:else if l.type === 'image' || l.type === 'avatar'}
				{@const src = dsrc(l)}
				<!-- circle = avatar (non-rounded) OR image masked to a circle; both render
				     as a centered SQUARE so a non-square box is a centered circle, exactly
				     like the Go renderer. ellipseMask fills the box with a 50% radius. -->
				{@const circle = (l.type === 'avatar' && l.shape !== 'rounded') || (l.type === 'image' && l.mask === 'circle')}
				{@const ellipseMask = l.type === 'image' && l.mask === 'ellipse'}
				{@const ring = (l.ring_width ?? 0) * scale}
				{@const rad = circle || ellipseMask ? '50%' : radiusCss(l)}
				{@const ringBase = ring > 0 && !l.clip ? `0 0 0 ${ring}px ${l.ring_color ?? '#fff'}` : ''}
				{@const ringCss = boxShadow(l, ringBase)}
				{@const sz = circle ? `width:${Math.min(l.w, l.h) * scale}px;height:${Math.min(l.w, l.h) * scale}px;` : 'width:100%;height:100%;'}
				<div class="grid h-full w-full place-items-center" class:stencil-ghost={l.clip && isSel}>
					{#if src}
						<img
							src={src}
							alt={l.name}
							draggable="false"
							class="block select-none"
							class:opacity-40={l.clip && isSel}
							class:opacity-0={l.clip && !isSel}
							style="{sz} object-fit:{l.fit ?? 'cover'}; border-radius:{rad}; {ringCss}"
						/>
					{:else}
						<!-- Empty / bound source → neutral placeholder tile. -->
						<div class="grid place-items-center bg-line-strong/60 text-faint" style="{sz} border-radius:{rad}; {ringCss}">
							{#if l.type === 'avatar'}
								<User size={Math.max(14, Math.min(l.w, l.h) * scale * 0.4)} strokeWidth={1.5} />
							{:else}
								<ImageIcon size={Math.max(14, Math.min(l.w, l.h) * scale * 0.34)} strokeWidth={1.5} />
							{/if}
						</div>
					{/if}
				</div>
			{:else if l.type === 'rect'}
				{@const sw = (l.stroke_width ?? 0) * scale}
				{@const strokeBase = sw > 0 && !l.clip ? `inset 0 0 0 ${sw}px ${l.stroke_color ?? '#fff'}` : ''}
				<div
					class="h-full w-full"
					class:stencil-ghost={l.clip && isSel}
					style="background:{l.clip ? 'transparent' : (l.fill ?? '#000')}; border-radius:{radiusCss(l)}; {boxShadow(l, strokeBase)}"
				></div>
			{:else if l.type === 'ellipse'}
				{@const sw = (l.stroke_width ?? 0) * scale}
				{@const strokeBase = sw > 0 && !l.clip ? `inset 0 0 0 ${sw}px ${l.stroke_color ?? '#fff'}` : ''}
				<div
					class="h-full w-full"
					class:stencil-ghost={l.clip && isSel}
					style="background:{l.clip ? 'transparent' : (l.fill ?? '#000')}; border-radius:50%; {boxShadow(l, strokeBase)}"
				></div>
			{/if}

			{#if isSel}
				<!-- Resize handles, only on the selected layer. -->
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
			{/if}
		</div>
		{/if}
	{/each}
</div>
</div>

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
		/* fractional % positions land mid-pixel; this keeps edges crisp */
		will-change: left, top, width, height;
	}
	.layer.selected {
		cursor: move;
	}
	.selected {
		outline: 1px solid var(--vec);
		outline-offset: 0;
		z-index: 2;
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
