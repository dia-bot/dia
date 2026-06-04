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
	class="relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors disabled:opacity-50 {checked
		? 'bg-accent'
		: 'bg-line-strong'}"
>
	<span
		class="inline-block h-5 w-5 transform rounded-full bg-white shadow-sm transition-transform {checked
			? 'translate-x-5'
			: 'translate-x-0.5'}"
	></span>
</button>
