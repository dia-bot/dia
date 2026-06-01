<script lang="ts">
	type Opt = { value: string; label: string };
	let {
		value = $bindable(''),
		options = [],
		placeholder = 'Select…'
	}: { value?: string; options?: Opt[]; placeholder?: string } = $props();
</script>

<select class="input" bind:value>
	<option value="">{placeholder}</option>
	{#each options as o (o.value)}
		<option value={o.value}>{o.label}</option>
	{/each}
</select>
