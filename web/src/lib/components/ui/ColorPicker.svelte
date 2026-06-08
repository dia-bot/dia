<script lang="ts">
	// Custom color picker — built from scratch (no native <input type=color>):
	// a saturation/value square + hue slider + hex field + swatches, in a Bits UI
	// popover. HSV is the working model; `value` stays a #RRGGBB hex string.
	import { Popover } from 'bits-ui';
	import { cn } from '$lib/utils';

	let {
		value = $bindable('#B244FC'),
		swatches = ['#FF6363', '#B244FC', '#34D399', '#F59E0B', '#3B82F6', '#FFFFFF', '#0A0A0C'],
		class: className = ''
	}: { value?: string; swatches?: string[]; class?: string } = $props();

	let h = $state(280);
	let s = $state(0.7);
	let v = $state(0.9);
	let lastHex = $state('');

	// Pull an externally-set value into HSV (skip our own writes to avoid loops).
	$effect(() => {
		const val = value;
		if (val && val.toLowerCase() !== lastHex.toLowerCase()) {
			const hsv = hexToHsv(val);
			if (hsv) {
				h = hsv.h;
				s = hsv.s;
				v = hsv.v;
				lastHex = val;
			}
		}
	});

	function commit() {
		const hex = hsvToHex(h, s, v);
		lastHex = hex;
		value = hex;
	}

	// ── pointer dragging on the SV square and hue bar ──
	function dragSV(node: HTMLElement) {
		function move(e: PointerEvent) {
			const r = node.getBoundingClientRect();
			s = clamp((e.clientX - r.left) / r.width);
			v = 1 - clamp((e.clientY - r.top) / r.height);
			commit();
		}
		function down(e: PointerEvent) {
			node.setPointerCapture(e.pointerId);
			move(e);
			node.addEventListener('pointermove', move);
		}
		function up(e: PointerEvent) {
			node.releasePointerCapture?.(e.pointerId);
			node.removeEventListener('pointermove', move);
		}
		node.addEventListener('pointerdown', down);
		node.addEventListener('pointerup', up);
		return {
			destroy() {
				node.removeEventListener('pointerdown', down);
				node.removeEventListener('pointerup', up);
			}
		};
	}
	function dragHue(node: HTMLElement) {
		function move(e: PointerEvent) {
			const r = node.getBoundingClientRect();
			h = clamp((e.clientX - r.left) / r.width) * 360;
			commit();
		}
		function down(e: PointerEvent) {
			node.setPointerCapture(e.pointerId);
			move(e);
			node.addEventListener('pointermove', move);
		}
		function up(e: PointerEvent) {
			node.releasePointerCapture?.(e.pointerId);
			node.removeEventListener('pointermove', move);
		}
		node.addEventListener('pointerdown', down);
		node.addEventListener('pointerup', up);
		return {
			destroy() {
				node.removeEventListener('pointerdown', down);
				node.removeEventListener('pointerup', up);
			}
		};
	}

	function onHex(e: Event) {
		let t = (e.target as HTMLInputElement).value.trim();
		if (!t.startsWith('#')) t = '#' + t;
		if (/^#[0-9a-fA-F]{6}$/.test(t)) value = t;
	}

	// ── colour math ──
	function clamp(n: number) {
		return Math.min(1, Math.max(0, n));
	}
	function hsvToHex(h: number, s: number, v: number): string {
		const c = v * s;
		const x = c * (1 - Math.abs(((h / 60) % 2) - 1));
		const m = v - c;
		let r = 0,
			g = 0,
			b = 0;
		if (h < 60) [r, g, b] = [c, x, 0];
		else if (h < 120) [r, g, b] = [x, c, 0];
		else if (h < 180) [r, g, b] = [0, c, x];
		else if (h < 240) [r, g, b] = [0, x, c];
		else if (h < 300) [r, g, b] = [x, 0, c];
		else [r, g, b] = [c, 0, x];
		const to = (n: number) =>
			Math.round((n + m) * 255)
				.toString(16)
				.padStart(2, '0');
		return `#${to(r)}${to(g)}${to(b)}`.toUpperCase();
	}
	function hexToHsv(hex: string): { h: number; s: number; v: number } | null {
		const m = /^#?([0-9a-fA-F]{6})$/.exec(hex.trim());
		if (!m) return null;
		const int = parseInt(m[1], 16);
		const r = ((int >> 16) & 255) / 255,
			g = ((int >> 8) & 255) / 255,
			b = (int & 255) / 255;
		const max = Math.max(r, g, b),
			min = Math.min(r, g, b),
			d = max - min;
		let hh = 0;
		if (d) {
			if (max === r) hh = ((g - b) / d) % 6;
			else if (max === g) hh = (b - r) / d + 2;
			else hh = (r - g) / d + 4;
			hh *= 60;
			if (hh < 0) hh += 360;
		}
		return { h: hh, s: max ? d / max : 0, v: max };
	}
</script>

<Popover.Root>
	<Popover.Trigger
		class={cn(
			'flex h-9 items-center gap-2 rounded-md border border-line bg-ink-2 px-2 outline-none transition-all hover:border-line-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-line-strong data-[state=open]:border-faint data-[state=open]:ring-2 data-[state=open]:ring-line-strong',
			className
		)}
	>
		<span class="h-5 w-5 shrink-0 rounded-md border border-line-strong" style="background:{value}"></span>
		<span class="font-mono text-xs uppercase text-muted">{value}</span>
	</Popover.Trigger>
	<Popover.Portal>
		<Popover.Content
			sideOffset={6}
			class="menu-pop z-50 w-60 rounded-xl border border-line-strong bg-surface p-3 shadow-2xl outline-none"
		>
			<!-- saturation / value square -->
			<div
				use:dragSV
				class="relative h-36 w-full cursor-crosshair touch-none overflow-hidden rounded-lg"
				style="background-color: hsl({h} 100% 50%)"
			>
				<div class="absolute inset-0" style="background:linear-gradient(to right,#fff,transparent)"></div>
				<div class="absolute inset-0" style="background:linear-gradient(to top,#000,transparent)"></div>
				<div
					class="pointer-events-none absolute h-3.5 w-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow"
					style="left:{s * 100}%; top:{(1 - v) * 100}%"
				></div>
			</div>

			<!-- hue slider -->
			<div
				use:dragHue
				class="relative mt-3 h-3.5 w-full cursor-pointer touch-none rounded-full"
				style="background:linear-gradient(to right,#f00,#ff0,#0f0,#0ff,#00f,#f0f,#f00)"
			>
				<div
					class="pointer-events-none absolute top-1/2 h-4 w-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow"
					style="left:{(h / 360) * 100}%"
				></div>
			</div>

			<!-- hex + swatches -->
			<div class="mt-3 flex items-center gap-2">
				<span class="h-7 w-7 shrink-0 rounded-md border border-line-strong" style="background:{value}"></span>
				<input
					value={value}
					oninput={onHex}
					spellcheck="false"
					class="h-8 w-full rounded-md border border-line-strong bg-ink-2 px-2 font-mono text-xs uppercase text-ink outline-none transition-all focus-visible:border-faint focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-line-strong"
				/>
			</div>
			<div class="mt-2.5 flex flex-wrap gap-1.5">
				{#each swatches as sw (sw)}
					<button
						type="button"
						aria-label={sw}
						onclick={() => (value = sw.toUpperCase())}
						class="h-5 w-5 rounded-md border border-line-strong transition-transform hover:scale-110"
						style="background:{sw}"
					></button>
				{/each}
			</div>
		</Popover.Content>
	</Popover.Portal>
</Popover.Root>
