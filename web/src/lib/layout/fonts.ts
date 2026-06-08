// The card font roster — shared by the font picker and the DOM preview, and kept
// in sync with the Go renderer's registry (internal/imaging/fonts.go) and the
// download list (scripts/fetch-fonts.sh). Only families the server can actually
// render are offered, so preview and output match. The families are all on
// Google Fonts, which the editor loads for the live preview.

export interface CardFont {
	family: string; // value stored in the layout + sent to the renderer
	css: string; // font-family stack for the DOM preview
	google: string; // Google Fonts css2 `family=` query value
}

// First entry is the default (empty font_family resolves to it, matching the Go
// renderer's defaultFamilyPref which prefers Lato).
export const CARD_FONTS: CardFont[] = [
	{ family: 'Lato', css: "'Lato', sans-serif", google: 'Lato:wght@400;700' },
	{ family: 'Poppins', css: "'Poppins', sans-serif", google: 'Poppins:wght@400;700' },
	{ family: 'Kanit', css: "'Kanit', sans-serif", google: 'Kanit:wght@400;700' },
	{ family: 'Barlow', css: "'Barlow', sans-serif", google: 'Barlow:wght@400;700' },
	{ family: 'Rajdhani', css: "'Rajdhani', sans-serif", google: 'Rajdhani:wght@400;700' },
	{ family: 'Arvo', css: "'Arvo', serif", google: 'Arvo:wght@400;700' },
	{ family: 'Titillium Web', css: "'Titillium Web', sans-serif", google: 'Titillium+Web:wght@400;700' },
	{ family: 'Anton', css: "'Anton', sans-serif", google: 'Anton' },
	{ family: 'Bebas Neue', css: "'Bebas Neue', sans-serif", google: 'Bebas+Neue' },
	{ family: 'Lobster', css: "'Lobster', cursive", google: 'Lobster' },
	{ family: 'Pacifico', css: "'Pacifico', cursive", google: 'Pacifico' }
];

const DEFAULT_CSS = CARD_FONTS[0].css;

// fontCss maps a stored family to a DOM font-family stack. Empty/unknown →
// the default family, so a layer with no explicit font matches the render.
export function fontCss(family: string | undefined): string {
	if (!family) return DEFAULT_CSS;
	return CARD_FONTS.find((f) => f.family === family)?.css ?? `'${family}', sans-serif`;
}

// googleFontsHref builds one Google Fonts stylesheet URL covering the roster,
// loaded by the editor so the preview shows the real faces.
export function googleFontsHref(): string {
	const fams = CARD_FONTS.map((f) => `family=${f.google}`).join('&');
	return `https://fonts.googleapis.com/css2?${fams}&display=swap`;
}
