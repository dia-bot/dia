<script lang="ts">
	// Custom colour picker — built from scratch (no native <input type=color>), modelled on
	// Figma's: Custom/Libraries header, a Solid model row, a saturation/value square, an
	// eyedropper + hue + alpha sliders, a Hex field with an opacity %, and swatches. HSV +
	// alpha is the working model; `value` is a #RRGGBB hex (or #RRGGBBAA when opacity < 100,
	// which the Go renderer's parseHex and CSS both understand). Lives in a Bits UI popover.
	import { Popover } from 'bits-ui';
	import { Pipette, Check } from 'lucide-svelte';
	import { browser } from '$app/environment';
	import { inspectorAnchor } from '$lib/layout/inspectorAnchor';
	import { cn } from '$lib/utils';

	let {
		value = $bindable('#B244FC'),
		swatches = ['#FF6363', '#B244FC', '#34D399', '#F59E0B', '#3B82F6', '#FFFFFF', '#0A0A0C'],
		class: className = '',
		// dense = h-7 to line up with the editor inspector's compact fields; default h-9
		// matches the dashboard/marketing form inputs.
		dense = false,
		// inspector = open the picker to the LEFT of the editor inspector panel (floating
		// over the canvas, Figma-style) instead of below the trigger. Off everywhere else.
		inspector = false
	}: {
		value?: string;
		swatches?: string[];
		class?: string;
		dense?: boolean;
		inspector?: boolean;
	} = $props();

	let h = $state(280);
	let s = $state(0.7);
	let v = $state(0.9);
	let a = $state(1); // alpha 0..1
	let lastHex = $state('');

	// The eyedropper is a Chromium-only browser API; hide the button where unsupported.
	const hasEyedropper = browser && 'EyeDropper' in window;

	// solid = the current colour WITHOUT alpha (for previews / gradients); out = the value
	// we publish (8-digit only when not fully opaque, so opaque colours stay 6-digit).
	const solid = $derived(hsvToHex(h, s, v));
	const alphaPct = $derived(Math.round(a * 100));

	// Pull an externally-set value into HSV+alpha (skip our own writes to avoid loops).
	$effect(() => {
		const val = value;
		if (val && val.toLowerCase() !== lastHex.toLowerCase()) {
			const m = /^#?([0-9a-fA-F]{6})([0-9a-fA-F]{2})?$/.exec(val.trim());
			if (m) {
				const hsv = hexToHsv('#' + m[1]);
				if (hsv) {
					h = hsv.h;
					s = hsv.s;
					v = hsv.v;
				}
				a = m[2] ? parseInt(m[2], 16) / 255 : 1;
				lastHex = val;
			}
		}
	});

	function alphaHex(al: number): string {
		return Math.round(Math.min(1, Math.max(0, al)) * 255)
			.toString(16)
			.padStart(2, '0')
			.toUpperCase();
	}
	function commit() {
		const hex = a >= 1 ? hsvToHex(h, s, v) : hsvToHex(h, s, v) + alphaHex(a);
		lastHex = hex;
		value = hex;
	}

	// ── pointer dragging on the SV square, hue bar and alpha bar ──
	function drag(node: HTMLElement, onMove: (e: PointerEvent, r: DOMRect) => void) {
		function move(e: PointerEvent) {
			onMove(e, node.getBoundingClientRect());
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
	const dragSV = (node: HTMLElement) =>
		drag(node, (e, r) => {
			s = clamp((e.clientX - r.left) / r.width);
			v = 1 - clamp((e.clientY - r.top) / r.height);
		});
	const dragHue = (node: HTMLElement) =>
		drag(node, (e, r) => {
			h = clamp((e.clientX - r.left) / r.width) * 360;
		});
	const dragAlpha = (node: HTMLElement) =>
		drag(node, (e, r) => {
			a = clamp((e.clientX - r.left) / r.width);
		});

	async function eyedrop() {
		if (!hasEyedropper) return;
		try {
			// EyeDropper is not in the TS lib yet.
			const picked = await new (window as unknown as { EyeDropper: new () => { open(): Promise<{ sRGBHex: string }> } }).EyeDropper().open();
			if (picked?.sRGBHex) value = picked.sRGBHex.toUpperCase();
		} catch {
			/* user dismissed */
		}
	}

	function onHex(e: Event) {
		let t = (e.target as HTMLInputElement).value.trim();
		if (!t.startsWith('#')) t = '#' + t;
		if (/^#[0-9a-fA-F]{6}$/.test(t)) {
			// editing the RGB keeps the current opacity (Figma's hex field has no alpha)
			value = (a < 1 ? t + alphaHex(a) : t).toUpperCase();
		} else if (/^#[0-9a-fA-F]{8}$/.test(t)) {
			value = t.toUpperCase();
		}
	}
	function onOpacity(e: Event) {
		const n = (e.target as HTMLInputElement).valueAsNumber;
		if (!Number.isNaN(n)) {
			a = clamp(n / 100);
			commit();
		}
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

	// a tiny checkerboard so transparency reads on the swatch chip / alpha track.
	const checker =
		'background-image:linear-gradient(45deg,#5557 25%,transparent 25%,transparent 75%,#5557 75%),linear-gradient(45deg,#5557 25%,transparent 25%,transparent 75%,#5557 75%);background-size:8px 8px;background-position:0 0,4px 4px;';
</script>

<Popover.Root>
	<Popover.Trigger
		class={cn(
			'flex items-center gap-2 rounded-md border border-line bg-ink-2 px-2 outline-none transition-all hover:border-line-strong focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-line-strong data-[state=open]:border-faint data-[state=open]:ring-2 data-[state=open]:ring-line-strong',
			dense ? 'h-7' : 'h-9',
			className
		)}
	>
		<span class="relative shrink-0 overflow-hidden border border-line-strong {dense ? 'h-4 w-4 rounded' : 'h-5 w-5 rounded-md'}">
			<span class="absolute inset-0" style={checker}></span>
			<span class="absolute inset-0" style="background:{value}"></span>
		</span>
		<span class="font-mono text-xs uppercase text-muted">{value}</span>
	</Popover.Trigger>
	<Popover.Portal>
		<Popover.Content
			customAnchor={inspector ? inspectorAnchor : undefined}
			side={inspector ? 'left' : 'bottom'}
			align={inspector ? 'start' : 'center'}
			sideOffset={inspector ? 10 : 6}
			collisionPadding={inspector ? 12 : undefined}
			class="menu-pop z-50 w-64 rounded-xl border border-line-strong bg-surface shadow-2xl shadow-black/60 outline-none {inspector
				? 'ring-1 ring-black/40'
				: ''}"
		>
			<!-- header: Custom / Libraries tabs + close (Figma's picker header) -->
			<div class="flex items-center justify-between border-b border-line px-2.5 py-1.5">
				<div class="flex items-center gap-1 text-xs font-medium">
					<span class="rounded px-1.5 py-0.5 text-ink">Custom</span>
					<span class="cursor-default px-1.5 py-0.5 text-faint" title="Shared libraries — coming soon">Libraries</span>
				</div>
				<Popover.Close class="grid h-5 w-5 place-items-center rounded text-faint transition-colors hover:bg-ink-2 hover:text-ink" aria-label="Close">
					<svg width="11" height="11" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"><path d="M3 3l6 6M9 3l-6 6" /></svg>
				</Popover.Close>
			</div>

			<!-- model row: only Solid is supported for layer colours -->
			<div class="flex items-center gap-2 px-2.5 pb-1 pt-2">
				<span class="grid h-5 w-5 place-items-center rounded border border-line-strong" style="background:{solid}"></span>
				<span class="text-xs text-muted">Solid</span>
			</div>

			<div class="px-2.5 pb-2.5">
				<!-- saturation / value square -->
				<div
					use:dragSV
					class="relative mt-1 h-36 w-full cursor-crosshair touch-none overflow-hidden rounded-lg"
					style="background-color: hsl({h} 100% 50%)"
				>
					<div class="absolute inset-0" style="background:linear-gradient(to right,#fff,transparent)"></div>
					<div class="absolute inset-0" style="background:linear-gradient(to top,#000,transparent)"></div>
					<div
						class="pointer-events-none absolute h-3.5 w-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow"
						style="left:{s * 100}%; top:{(1 - v) * 100}%"
					></div>
				</div>

				<!-- eyedropper + hue + alpha -->
				<div class="mt-3 flex items-center gap-2.5">
					{#if hasEyedropper}
						<button
							type="button"
							onclick={eyedrop}
							title="Pick a colour from the screen"
							aria-label="Eyedropper"
							class="grid h-8 w-8 shrink-0 place-items-center rounded-md border border-line-strong text-muted transition-colors hover:bg-ink-2 hover:text-ink"
						>
							<Pipette size={15} />
						</button>
					{/if}
					<div class="min-w-0 flex-1 space-y-2.5">
						<!-- hue slider -->
						<div
							use:dragHue
							class="relative h-3 w-full cursor-pointer touch-none rounded-full"
							style="background:linear-gradient(to right,#f00,#ff0,#0f0,#0ff,#00f,#f0f,#f00)"
						>
							<div
								class="pointer-events-none absolute top-1/2 h-4 w-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow"
								style="left:{(h / 360) * 100}%"
							></div>
						</div>
						<!-- alpha slider -->
						<div use:dragAlpha class="relative h-3 w-full cursor-pointer touch-none overflow-hidden rounded-full">
							<div class="absolute inset-0" style={checker}></div>
							<div class="absolute inset-0" style="background:linear-gradient(to right, transparent, {solid})"></div>
							<div
								class="pointer-events-none absolute top-1/2 h-4 w-4 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-white shadow"
								style="left:{a * 100}%"
							></div>
						</div>
					</div>
				</div>

				<!-- format + hex + opacity -->
				<div class="mt-3 flex items-center gap-1.5">
					<span class="grid h-8 shrink-0 select-none place-items-center rounded-md border border-line-strong bg-ink-2 px-2 font-mono text-[11px] uppercase text-faint">Hex</span>
					<input
						value={value}
						oninput={onHex}
						spellcheck="false"
						class="h-8 min-w-0 flex-1 rounded-md border border-line-strong bg-ink-2 px-2 font-mono text-xs uppercase text-ink outline-none transition-all focus-visible:border-faint focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-line-strong"
					/>
					<label class="flex h-8 w-16 shrink-0 items-center rounded-md border border-line-strong bg-ink-2 pl-2 pr-1 focus-within:border-faint focus-within:ring-2 focus-within:ring-line-strong">
						<input
							type="number"
							min="0"
							max="100"
							value={alphaPct}
							oninput={onOpacity}
							class="w-full min-w-0 bg-transparent text-xs tabular-nums text-ink outline-none"
						/>
						<span class="select-none pr-1 text-[10px] text-faint">%</span>
					</label>
				</div>

				<!-- swatches -->
				<div class="mt-3">
					<span class="mb-1.5 block text-[10px] font-medium uppercase tracking-[0.08em] text-faint">Swatches</span>
					<div class="flex flex-wrap gap-1.5">
						{#each swatches as sw (sw)}
							<button
								type="button"
								aria-label={sw}
								onclick={() => (value = sw.toUpperCase())}
								class="relative grid h-5 w-5 place-items-center overflow-hidden rounded-md border border-line-strong transition-transform hover:scale-110"
							>
								<span class="absolute inset-0" style={checker}></span>
								<span class="absolute inset-0" style="background:{sw}"></span>
								{#if value.toUpperCase() === sw.toUpperCase()}
									<Check size={12} class="relative text-white mix-blend-difference" />
								{/if}
							</button>
						{/each}
					</div>
				</div>
			</div>
		</Popover.Content>
	</Popover.Portal>
</Popover.Root>
