// scrub makes an element a Figma-style "scrubby slider": click and drag left/right
// to decrease/increase a bound number. Attach to a label or the field itself.
//   <span use:scrub={{ get: () => value, set, step: 1, min: 0 }}>X</span>
// Hold Shift while dragging for 10× steps, Alt for 0.1× (fine).
export interface ScrubParams {
	get: () => number;
	set: (n: number) => void;
	step?: number;
	min?: number;
	max?: number;
}

export function scrub(node: HTMLElement, params: ScrubParams) {
	let p = params;
	let dragging = false;
	let moved = false;
	let startX = 0;
	let startVal = 0;
	let ptr = -1;

	function down(e: PointerEvent) {
		if (e.button !== 0) return;
		dragging = true;
		moved = false;
		ptr = e.pointerId;
		startX = e.clientX;
		startVal = p.get() || 0;
		node.setPointerCapture(ptr);
		// Force the resize cursor everywhere for the whole drag (a global !important
		// rule beats inputs/canvas cursors as the pointer moves off the label).
		document.body.classList.add('is-scrubbing');
		e.preventDefault();
	}
	function move(e: PointerEvent) {
		if (!dragging) return;
		const dx = e.clientX - startX;
		if (Math.abs(dx) > 2) moved = true;
		// Apply a whole-pixel delta scaled by the step (and modifier) to the START
		// value — never snap the start value itself, so there's no jump on the first
		// micro-move. Shift = ×10, Alt = ×0.1 (fine).
		const step = (p.step ?? 1) * (e.shiftKey ? 10 : e.altKey ? 0.1 : 1);
		let v = startVal + Math.round(dx) * step;
		v = Math.round(v * 1000) / 1000; // tidy float drift on fractional steps
		if (p.min != null) v = Math.max(p.min, v);
		if (p.max != null) v = Math.min(p.max, v);
		p.set(v);
	}
	function up() {
		if (!dragging) return;
		dragging = false;
		document.body.classList.remove('is-scrubbing');
		try {
			node.releasePointerCapture(ptr);
		} catch {
			/* gone */
		}
	}
	// Don't let a scrub gesture also start a text selection / drag.
	function onClick(e: MouseEvent) {
		if (moved) {
			e.preventDefault();
			e.stopPropagation();
		}
	}

	node.style.cursor = 'ew-resize';
	node.style.touchAction = 'none';
	node.style.userSelect = 'none';
	node.addEventListener('pointerdown', down);
	node.addEventListener('pointermove', move);
	node.addEventListener('pointerup', up);
	node.addEventListener('pointercancel', up);
	node.addEventListener('click', onClick, true);

	return {
		update(np: ScrubParams) {
			p = np;
		},
		destroy() {
			if (dragging) document.body.classList.remove('is-scrubbing');
			node.removeEventListener('pointerdown', down);
			node.removeEventListener('pointermove', move);
			node.removeEventListener('pointerup', up);
			node.removeEventListener('pointercancel', up);
			node.removeEventListener('click', onClick, true);
		}
	};
}
