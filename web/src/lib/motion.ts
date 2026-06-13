// Shared motion helpers for Svelte transitions. CSS-driven animations respect
// prefers-reduced-motion via media queries; Svelte transitions need it
// handled in JS — `dur(ms)` collapses to 0 for users who opted out.
export function motionOK(): boolean {
	return (
		typeof window === 'undefined' ||
		!window.matchMedia('(prefers-reduced-motion: reduce)').matches
	);
}

export function dur(ms: number): number {
	return motionOK() ? ms : 0;
}
