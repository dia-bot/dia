<script lang="ts">
	import type { El, Background } from './types';

	// Renders an element model onto a welcome-card canvas. When `interactive`,
	// elements can be selected, dragged (pointer), and nudged (arrow keys).
	let {
		elements,
		bg,
		username = 'maya',
		count = 1024,
		server = 'Aurora',
		level = 12,
		rank = 1,
		xp = '4,820',
		nextxp = '6,000',
		aspect = '1024 / 450',
		interactive = false,
		selectedId = $bindable<string | null>(null),
		onnudgedelete
	}: {
		elements: El[];
		bg: Background;
		username?: string;
		count?: number;
		server?: string;
		level?: number;
		rank?: number;
		xp?: string;
		nextxp?: string;
		aspect?: string;
		interactive?: boolean;
		selectedId?: string | null;
		onnudgedelete?: (id: string) => void;
	} = $props();

	let canvasEl: HTMLElement;
	let drag: { id: string; dx: number; dy: number } | null = null;

	const sub = (s: string) =>
		(s ?? '')
			.replaceAll('{user}', username)
			.replaceAll('{username}', username)
			.replaceAll('{count}', count.toLocaleString('en-US'))
			.replaceAll('{server}', server)
			.replaceAll('{level}', String(level))
			.replaceAll('{rank}', String(rank))
			.replaceAll('{nextxp}', nextxp)
			.replaceAll('{xp}', xp);

	const initials = $derived(
		username.replace(/[^a-zA-Z0-9]/g, '').slice(0, 2).toUpperCase() || '?'
	);

	const bgCss = $derived(
		bg.type === 'image' && bg.image
			? `center / cover no-repeat url(${bg.image})`
			: bg.type === 'gradient'
				? `linear-gradient(${bg.angle}deg, ${bg.from}, ${bg.to})`
				: bg.color || '#1F1B2E'
	);

	function anchorTf(a: string) {
		return a === 'left' ? 'translate(0,-50%)' : a === 'right' ? 'translate(-100%,-50%)' : 'translate(-50%,-50%)';
	}
	function posStyle(el: El) {
		return `left:${el.x}%; top:${el.y}%; opacity:${el.opacity}; transform:${anchorTf(el.anchor)} rotate(${el.rotation}deg);`;
	}

	function pct(e: PointerEvent) {
		const r = canvasEl.getBoundingClientRect();
		return {
			x: ((e.clientX - r.left) / r.width) * 100,
			y: ((e.clientY - r.top) / r.height) * 100
		};
	}
	function down(e: PointerEvent, el: El) {
		if (!interactive || el.locked) return;
		selectedId = el.id;
		const p = pct(e);
		drag = { id: el.id, dx: p.x - el.x, dy: p.y - el.y };
		(e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId);
		e.preventDefault();
	}
	function move(e: PointerEvent) {
		if (!drag) return;
		const el = elements.find((x) => x.id === drag!.id);
		if (!el) return;
		const p = pct(e);
		el.x = Math.max(0, Math.min(100, Math.round((p.x - drag.dx) * 10) / 10));
		el.y = Math.max(0, Math.min(100, Math.round((p.y - drag.dy) * 10) / 10));
	}
	function up() {
		drag = null;
	}
	function key(e: KeyboardEvent, el: El) {
		if (!interactive) return;
		const step = e.shiftKey ? 5 : 1;
		if (e.key === 'ArrowLeft') el.x = Math.max(0, el.x - step);
		else if (e.key === 'ArrowRight') el.x = Math.min(100, el.x + step);
		else if (e.key === 'ArrowUp') el.y = Math.max(0, el.y - step);
		else if (e.key === 'ArrowDown') el.y = Math.min(100, el.y + step);
		else if (e.key === 'Delete' || e.key === 'Backspace') onnudgedelete?.(el.id);
		else return;
		e.preventDefault();
	}

	const visible = $derived(elements.filter((e) => e.visible));
</script>

{#snippet body(el: El)}
	{#if el.kind === 'text'}
		<div
			style="font-size:{el.font}cqw; font-weight:{el.weight}; color:{el.color}; text-align:{el.align}; max-width:{el.maxw}cqw; letter-spacing:{el.letter}em; line-height:1.08; text-shadow:0 1px 8px rgba(0,0,0,0.16);"
		>
			{sub(el.text ?? '')}
		</div>
	{:else if el.kind === 'avatar'}
		<div
			style="width:{el.size}cqw; height:{el.size}cqw; border:0.8cqw solid {el.ring}; border-radius:{el.shape ===
			'circle'
				? '999px'
				: el.shape === 'rounded'
					? '22%'
					: '8%'}; background:rgba(255,255,255,0.16); display:grid; place-items:center; box-shadow:0 6px 20px rgba(0,0,0,0.18);"
		>
			<span style="font-size:{(el.size ?? 18) * 0.42}cqw; font-weight:800; color:{el.color};"
				>{initials}</span
			>
		</div>
	{:else if el.kind === 'badge'}
		<div
			style="display:inline-flex; align-items:center; background:{el.bg}; color:{el.color}; font-size:{el.font}cqw; font-weight:{el.weight}; padding:{(el.font ??
				2) * 0.55}cqw {(el.font ?? 2) * 1.1}cqw; border-radius:999px; letter-spacing:0.01em; white-space:nowrap;"
		>
			{sub(el.text ?? '')}
		</div>
	{:else if el.kind === 'divider'}
		<div style="width:{el.w}cqw; height:{el.thickness}px; background:{el.color}; border-radius:999px;"></div>
	{:else if el.kind === 'rect'}
		<div
			style="width:{el.w}cqw; height:{el.height}cqw; background:{el.color}; border-radius:{el.radius}px;"
		></div>
	{:else if el.kind === 'bar'}
		<div
			style="width:{el.w}cqw; height:{el.height}cqw; min-height:6px; background:{el.track}; border-radius:999px; overflow:hidden;"
		>
			<div style="width:{el.value}%; height:100%; background:{el.color}; border-radius:999px;"></div>
		</div>
	{/if}
{/snippet}

<div bind:this={canvasEl} class="wcx" style="background:{bgCss}; aspect-ratio:{aspect};">
	{#each visible as el (el.id)}
		{#if interactive}
			<div
				role="button"
				tabindex="0"
				aria-label={el.name}
				aria-pressed={selectedId === el.id}
				aria-keyshortcuts="ArrowUp ArrowDown ArrowLeft ArrowRight Delete"
				class="el {selectedId === el.id ? 'sel' : ''}"
				style={posStyle(el)}
				onpointerdown={(e) => down(e, el)}
				onpointermove={move}
				onpointerup={up}
				onpointercancel={up}
				onkeydown={(e) => key(e, el)}
			>
				{@render body(el)}
			</div>
		{:else}
			<div class="el static" style={posStyle(el)}>
				{@render body(el)}
			</div>
		{/if}
	{/each}
</div>

<style>
	.wcx {
		container-type: inline-size;
		position: relative;
		width: 100%;
		aspect-ratio: 1024 / 450;
		border-radius: 14px;
		overflow: hidden;
		isolation: isolate;
	}
	.el {
		position: absolute;
	}
	.el:not(.static) {
		cursor: grab;
		/* pan-y keeps vertical page scroll working on touch while still allowing
		   selection + horizontal repositioning of the element. */
		touch-action: pan-y;
		outline: none;
	}
	.el:not(.static):active {
		cursor: grabbing;
	}
	.el.sel {
		outline: 2px solid var(--color-accent);
		outline-offset: 3px;
		border-radius: 3px;
	}
	.el:not(.static):focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 3px;
	}
</style>
