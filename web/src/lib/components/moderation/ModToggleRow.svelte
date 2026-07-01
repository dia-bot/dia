<script lang="ts">
	// A flat "setting" row: a title (+ optional description) on the left, a Toggle
	// on the right. The repeated atom across the safety pages — no card, just a row
	// that sits inside a ModSection. Stack several with `divided` for a hairline-
	// separated list (the first row keeps its top edge open).
	import Toggle from '$lib/components/Toggle.svelte';

	let {
		title,
		desc,
		checked = $bindable(false),
		label,
		disabled = false,
		divided = false
	}: {
		title: string;
		desc?: string;
		checked?: boolean;
		label?: string;
		disabled?: boolean;
		divided?: boolean;
	} = $props();
</script>

<label
	class="flex items-center justify-between gap-4 py-3 {divided
		? 'border-t border-line first:border-t-0'
		: ''}"
>
	<span class="min-w-0">
		<span class="block text-[13px] font-medium text-ink">{title}</span>
		{#if desc}<span class="mt-0.5 block text-xs text-muted">{desc}</span>{/if}
	</span>
	<Toggle bind:checked {disabled} label={label ?? title} />
</label>
