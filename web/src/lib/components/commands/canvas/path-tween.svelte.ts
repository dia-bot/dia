// Tweened edge geometry. SvelteFlow recreates edge DOM on every graph
// rebuild, so CSS transitions can never animate a line. Instead the edge
// components tween their endpoint COORDS, and a module-level memory keyed by
// edge id survives the recreation: the new element starts from wherever the
// old element WAS (mid-flight included) and animates into place.
//
// Two situations must snap, never tween:
//  - while a node is being dragged (the line must track the pointer 1:1 —
//    FlowInner flips `dragState.active` around drags);
//  - rapid successive updates (streams from layout/measure churn).
import { Tween } from 'svelte/motion';
import { cubicOut } from 'svelte/easing';

export type EdgeCoords = { sx: number; sy: number; tx: number; ty: number };

// Flipped by FlowInner around node drags.
export const dragState = { active: false };

const memory = new Map<string, EdgeCoords>();
const lastSet = new Map<string, number>();

export function tweenedCoords(id: () => string, target: () => EdgeCoords): Tween<EdgeCoords> {
	const t = new Tween<EdgeCoords>(memory.get(id()) ?? target(), {
		duration: 360,
		easing: cubicOut
	});

	// Drive the tween towards the live target.
	$effect(() => {
		const key = id();
		const tgt = target();
		const now = performance.now();
		const rapid = now - (lastSet.get(key) ?? 0) < 120;
		lastSet.set(key, now);
		console.debug('[tween]', key.slice(-6), 'drag=', dragState.active, 'rapid=', rapid, 'tx=', Math.round(tgt.tx));
		void t.set(tgt, { duration: dragState.active || rapid ? 0 : 360 });
	});

	// Remember the CURRENT (possibly mid-flight) position continuously, so a
	// rebuild that lands mid-animation continues from where the line is, not
	// where it was headed.
	$effect(() => {
		const c = t.current;
		memory.set(id(), { sx: c.sx, sy: c.sy, tx: c.tx, ty: c.ty });
	});

	return t;
}
