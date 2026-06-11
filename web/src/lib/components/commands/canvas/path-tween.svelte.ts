// Tweened edge geometry. SvelteFlow recreates edge DOM on every graph
// rebuild, so CSS transitions can never animate a line. Instead the edge
// components tween their endpoint COORDS, and a module-level memory keyed by
// edge id survives the recreation: the new element starts from the old
// element's geometry and animates into place (re-anchoring a click path,
// Tidy, tuck-into-branch, …).
//
// Rapid successive updates (node drags stream a new position every frame)
// snap instead of tween, so dragging stays 1:1 with the pointer.
import { Tween } from 'svelte/motion';
import { cubicOut } from 'svelte/easing';

export type EdgeCoords = { sx: number; sy: number; tx: number; ty: number };

const memory = new Map<string, EdgeCoords>();
const lastSet = new Map<string, number>();

export function tweenedCoords(id: () => string, target: () => EdgeCoords): Tween<EdgeCoords> {
	const t = new Tween<EdgeCoords>(memory.get(id()) ?? target(), {
		duration: 360,
		easing: cubicOut
	});
	$effect(() => {
		const key = id();
		const tgt = target();
		const now = performance.now();
		const rapid = now - (lastSet.get(key) ?? 0) < 120;
		lastSet.set(key, now);
		memory.set(key, tgt);
		void t.set(tgt, { duration: rapid ? 0 : 360 });
	});
	return t;
}
