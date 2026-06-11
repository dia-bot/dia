<script lang="ts">
	// A dashboard number input with a CUSTOM up/down stepper (the native browser spinner
	// is hidden globally in app.css). Styled as the standard `.input`; the stepper sits
	// inside the right edge and reveals on hover/focus. Drop-in for `<input class="input"
	// type="number" bind:value>` — pass min/max/step like the native input.
	import { ChevronUp, ChevronDown } from 'lucide-svelte';

	let {
		value = $bindable<number | undefined>(undefined),
		min,
		max,
		step = 1,
		placeholder,
		id,
		class: className = ''
	}: {
		value?: number;
		min?: number;
		max?: number;
		step?: number;
		placeholder?: string;
		id?: string;
		class?: string;
	} = $props();

	function bump(dir: 1 | -1) {
		let n = (Number(value) || 0) + dir * step;
		if (min != null) n = Math.max(min, n);
		if (max != null) n = Math.min(max, n);
		value = Math.round(n * 1e6) / 1e6; // de-float (e.g. repeated 0.1 steps)
	}
</script>

<div class="group relative {className}">
	<input {id} type="number" bind:value {min} {max} {step} {placeholder} class="input w-full pr-8" />
	<span
		class="pointer-events-none absolute inset-y-[3px] right-[3px] flex w-6 flex-col overflow-hidden rounded-[9px] opacity-0 transition-opacity group-hover:opacity-100 group-focus-within:opacity-100"
	>
		<button
			type="button"
			tabindex="-1"
			aria-label="Increment"
			onclick={() => bump(1)}
			class="pointer-events-auto grid flex-1 place-items-center text-faint transition-colors hover:bg-ink-2 hover:text-ink"
		>
			<ChevronUp size={12} strokeWidth={2.5} />
		</button>
		<button
			type="button"
			tabindex="-1"
			aria-label="Decrement"
			onclick={() => bump(-1)}
			class="pointer-events-auto grid flex-1 place-items-center border-t border-line-strong/50 text-faint transition-colors hover:bg-ink-2 hover:text-ink"
		>
			<ChevronDown size={12} strokeWidth={2.5} />
		</button>
	</span>
</div>
