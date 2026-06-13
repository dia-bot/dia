<script lang="ts">
	// THE numeric input for the command editor — native spinners are killed
	// globally (app.css); this renders its own steppers: hover/focus reveals a
	// stacked ± column with press-and-hold repeat, arrow keys still work, and
	// values clamp to min/max. Type directly, click, or hold.
	import ChevronUp from 'lucide-svelte/icons/chevron-up';
	import ChevronDown from 'lucide-svelte/icons/chevron-down';

	let {
		value,
		onChange,
		min,
		max,
		step = 1,
		placeholder = '',
		suffix = '',
		disabled = false,
		class: cls = ''
	}: {
		value: number | undefined;
		// Emptying the field emits undefined so optional bounds can be cleared.
		onChange: (n: number | undefined) => void;
		min?: number;
		max?: number;
		step?: number;
		placeholder?: string;
		suffix?: string;
		disabled?: boolean;
		class?: string;
	} = $props();

	function clamp(n: number): number {
		if (min !== undefined && n < min) n = min;
		if (max !== undefined && n > max) n = max;
		// Snap away float noise from repeated fractional steps.
		const dec = (String(step).split('.')[1] ?? '').length;
		return Number(n.toFixed(dec));
	}

	function bump(dir: 1 | -1) {
		onChange(clamp((value ?? 0) + dir * step));
	}

	// Press-and-hold repeat: one step immediately, then accelerate.
	let holdTimer: ReturnType<typeof setTimeout> | null = null;
	let holdInterval: ReturnType<typeof setInterval> | null = null;

	function holdStart(dir: 1 | -1) {
		bump(dir);
		holdTimer = setTimeout(() => {
			holdInterval = setInterval(() => bump(dir), 60);
		}, 380);
	}
	function holdEnd() {
		if (holdTimer) clearTimeout(holdTimer);
		if (holdInterval) clearInterval(holdInterval);
		holdTimer = holdInterval = null;
	}
</script>

<div
	class="group/num flex h-7 items-stretch overflow-hidden rounded-md border border-line bg-bg transition-colors focus-within:border-line-strong hover:border-line-strong {disabled
		? 'pointer-events-none opacity-40'
		: ''} {cls}"
>
	<input
		type="number"
		{disabled}
		class="w-full min-w-0 bg-transparent px-2 text-[12px] tabular-nums text-ink outline-none placeholder:text-faint"
		value={value ?? ''}
		{placeholder}
		{min}
		{max}
		{step}
		oninput={(e) => {
			const el = e.currentTarget as HTMLInputElement;
			if (el.value === '') onChange(undefined);
			else if (!Number.isNaN(el.valueAsNumber)) onChange(clamp(el.valueAsNumber));
		}}
		onblur={(e) => {
			// Re-render the clamped value once focus leaves (typing stays free).
			const el = e.currentTarget as HTMLInputElement;
			if (value !== undefined) el.value = String(value);
		}}
	/>
	{#if suffix}
		<span class="self-center pr-1.5 font-mono text-[10px] text-faint">{suffix}</span>
	{/if}
	<div
		class="flex w-5 shrink-0 flex-col border-l border-line/60 opacity-0 transition-opacity duration-150 group-focus-within/num:opacity-100 group-hover/num:opacity-100"
	>
		<button
			type="button"
			tabindex="-1"
			class="grid flex-1 place-items-center text-faint transition-colors hover:bg-surface hover:text-ink"
			aria-label="Increase"
			onpointerdown={() => holdStart(1)}
			onpointerup={holdEnd}
			onpointerleave={holdEnd}
		>
			<ChevronUp size={9} strokeWidth={2.5} />
		</button>
		<button
			type="button"
			tabindex="-1"
			class="grid flex-1 place-items-center border-t border-line/60 text-faint transition-colors hover:bg-surface hover:text-ink"
			aria-label="Decrease"
			onpointerdown={() => holdStart(-1)}
			onpointerup={holdEnd}
			onpointerleave={holdEnd}
		>
			<ChevronDown size={9} strokeWidth={2.5} />
		</button>
	</div>
</div>
