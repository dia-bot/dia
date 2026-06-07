<script lang="ts">
	// Labelled colour control. Delegates to the custom ColorPicker so every page
	// that already uses <ColorField> gets the from-scratch picker for free.
	import ColorPicker from '$lib/components/ui/ColorPicker.svelte';

	let {
		value = $bindable('#B244FC'),
		label = ''
	}: { value?: string; label?: string } = $props();
</script>

<div>
	{#if label}<span class="label">{label}</span>{/if}
	<ColorPicker bind:value />
</div>
