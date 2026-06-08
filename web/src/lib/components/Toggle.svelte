<script lang="ts">
	let {
		checked = $bindable(false),
		disabled = false,
		label = 'Toggle',
		onchange
	}: { checked?: boolean; disabled?: boolean; label?: string; onchange?: (v: boolean) => void } =
		$props();

	function handle() {
		if (disabled) return;
		checked = !checked;
		onchange?.(checked);
	}
</script>

<button
	type="button"
	role="switch"
	aria-label={label}
	aria-checked={checked}
	{disabled}
	onclick={handle}
	class="relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-line-strong disabled:pointer-events-none disabled:opacity-40 {checked
		? 'bg-ink'
		: 'bg-line-strong'}"
>
	<span
		class="inline-block h-4 w-4 transform rounded-full shadow-sm transition-transform {checked
			? 'translate-x-[1.125rem] bg-bg'
			: 'translate-x-0.5 bg-white'}"
	></span>
</button>
