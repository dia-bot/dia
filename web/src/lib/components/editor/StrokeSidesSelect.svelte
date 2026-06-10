<script lang="ts">
	// Figma's "Individual strokes" control: a square-icon trigger that opens a popover
	// with a clickable square — click an edge to toggle that side's stroke on/off.
	import { Popover } from 'bits-ui';
	import { inspectorAnchor } from '$lib/layout/inspectorAnchor';
	import type { StrokeSide } from '$lib/layout/schema';

	let {
		isSide,
		toggle,
		reset
	}: {
		isSide: (s: StrokeSide) => boolean;
		toggle: (s: StrokeSide) => void;
		reset: () => void;
	} = $props();

	const sides: StrokeSide[] = ['top', 'right', 'bottom', 'left'];
	const allOn = $derived(sides.every((s) => isSide(s)));
	// [side, absolute-position classes] for each clickable edge bar in the square.
	const edges: [StrokeSide, string][] = [
		['top', 'left-1.5 right-1.5 top-0.5 h-[5px]'],
		['bottom', 'left-1.5 right-1.5 bottom-0.5 h-[5px]'],
		['left', 'top-1.5 bottom-1.5 left-0.5 w-[5px]'],
		['right', 'top-1.5 bottom-1.5 right-0.5 w-[5px]']
	];
</script>

<Popover.Root>
	<Popover.Trigger
		title="Individual sides"
		aria-label="Individual sides"
		class="grid h-7 w-7 shrink-0 place-items-center rounded-md border border-line bg-ink-2 transition-colors hover:border-line-strong data-[state=open]:border-faint {allOn
			? 'text-muted'
			: 'text-accent-ink'}"
	>
		<svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
			<rect
				x="2.5"
				y="2.5"
				width="9"
				height="9"
				rx="1"
				stroke="currentColor"
				stroke-opacity="0.3"
				stroke-width="1"
			/>
			<line x1="2.5" y1="2.5" x2="11.5" y2="2.5" stroke="currentColor" stroke-width="1.7" stroke-opacity={isSide('top') ? 1 : 0} />
			<line x1="11.5" y1="2.5" x2="11.5" y2="11.5" stroke="currentColor" stroke-width="1.7" stroke-opacity={isSide('right') ? 1 : 0} />
			<line x1="2.5" y1="11.5" x2="11.5" y2="11.5" stroke="currentColor" stroke-width="1.7" stroke-opacity={isSide('bottom') ? 1 : 0} />
			<line x1="2.5" y1="2.5" x2="2.5" y2="11.5" stroke="currentColor" stroke-width="1.7" stroke-opacity={isSide('left') ? 1 : 0} />
		</svg>
	</Popover.Trigger>
	<Popover.Portal>
		<Popover.Content
			customAnchor={inspectorAnchor}
			side="left"
			align="start"
			sideOffset={10}
			collisionPadding={12}
			class="menu-pop z-[60] rounded-xl border border-line-strong bg-surface p-3 shadow-2xl shadow-black/60 ring-1 ring-black/40 outline-none"
		>
			<div class="flex flex-col items-center gap-2.5">
				<div class="relative h-16 w-16">
					<div class="absolute inset-1.5 rounded-sm border border-line"></div>
					{#each edges as [side, pos] (side)}
						<button
							type="button"
							aria-label={side}
							title={side}
							onclick={() => toggle(side)}
							class="absolute {pos} rounded-full transition-colors {isSide(side)
								? 'bg-accent'
								: 'bg-line-strong hover:bg-faint'}"
						></button>
					{/each}
				</div>
				<button
					type="button"
					onclick={() => reset()}
					disabled={allOn}
					class="w-full rounded-md border border-line-strong px-2 py-1 text-[11px] text-muted transition-colors hover:bg-ink-2 hover:text-ink disabled:opacity-40"
				>
					All sides
				</button>
			</div>
		</Popover.Content>
	</Popover.Portal>
</Popover.Root>
