<script lang="ts">
	let {
		value = $bindable('#B244FC'),
		label = ''
	}: { value?: string; label?: string } = $props();
</script>

<div>
	{#if label}<span class="label">{label}</span>{/if}
	<div class="flex items-center gap-2">
		<input
			type="color"
			aria-label={label || 'Colour'}
			value={value || '#000000'}
			oninput={(e) => (value = (e.target as HTMLInputElement).value)}
			class="h-9 w-10 cursor-pointer rounded-lg border border-[var(--color-line-strong)] bg-ink-2 p-1"
		/>
		<input class="input font-mono text-xs" aria-label={`${label || 'Colour'} hex value`} bind:value placeholder="#B244FC" />
	</div>
</div>
