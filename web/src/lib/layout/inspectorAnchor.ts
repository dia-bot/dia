// inspectorAnchor is a Bits UI `customAnchor` (a virtual "Measurable") that positions an
// editor popover to the LEFT of the inspector panel (the [data-inspector] aside), at the
// VERTICAL level of whichever trigger is currently open — Figma's behaviour. Paired with
// side="left" + align="start", the popover's right edge sits just left of the panel and its
// top lines up with the trigger, instead of floating centred on the whole sidebar.
//
// It finds the open trigger generically (the one panel control with data-state="open"), so
// every inspector popover can share this single anchor with no per-trigger ref plumbing —
// which also reinforces "one popover open at a time".
function openTrigger(): HTMLElement | null {
	if (typeof document === 'undefined') return null;
	return document.querySelector<HTMLElement>('[data-inspector] [data-state="open"]');
}

export const inspectorAnchor = {
	get contextElement(): Element | undefined {
		return openTrigger() ?? undefined;
	},
	getBoundingClientRect(): DOMRect {
		const panel = typeof document !== 'undefined' ? document.querySelector('[data-inspector]') : null;
		const pr = panel?.getBoundingClientRect();
		const tr = openTrigger()?.getBoundingClientRect();
		const left = pr?.left ?? tr?.left ?? 0;
		const top = tr?.top ?? pr?.top ?? 0;
		const height = tr?.height ?? 0;
		// a zero-width vertical strip at the panel's left edge, at the trigger's height.
		return new DOMRect(left, top, 0, height);
	}
};
