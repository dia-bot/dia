<script lang="ts">
	// Custom colour picker — built from scratch (no native <input type=color>), modelled on
	// Figma's: Custom/Libraries header, a Solid model row, and the shared ColorArea controls
	// (saturation/value square, eyedropper + hue + alpha sliders, Hex + opacity, swatches).
	// `value` is a #RRGGBB hex (or #RRGGBBAA when opacity < 100, which the Go renderer's
	// parseHex and CSS both understand). Lives in a Bits UI popover.
	import { Popover } from 'bits-ui';
	import { inspectorAnchor } from '$lib/layout/inspectorAnchor';
	import { cn } from '$lib/utils';
	import ColorArea from './ColorArea.svelte';

	let {
		value = $bindable('#B244FC'),
		swatches = ['#FF6363', '#B244FC', '#34D399', '#3B82F6', '#FFFFFF', '#0A0A0C'],
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

	// solid preview without alpha for the model row.
	const solid = $derived(value.length === 9 ? value.slice(0, 7) : value);

	// a tiny checkerboard so transparency reads on the swatch chip.
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

			<!-- model row: only Solid is supported for plain colour fields -->
			<div class="flex items-center gap-2 px-2.5 pb-1 pt-2">
				<span class="grid h-5 w-5 place-items-center rounded border border-line-strong" style="background:{solid}"></span>
				<span class="text-xs text-muted">Solid</span>
			</div>

			<div class="px-2.5 pb-2.5 pt-1">
				<ColorArea bind:value {swatches} />
			</div>
		</Popover.Content>
	</Popover.Portal>
</Popover.Root>
