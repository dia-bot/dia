<script lang="ts">
	let { checked = $bindable(false), disabled = false }: { checked?: boolean; disabled?: boolean } =
		$props();
</script>

<button
	type="button"
	role="switch"
	aria-checked={checked}
	{disabled}
	onclick={() => (checked = !checked)}
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
