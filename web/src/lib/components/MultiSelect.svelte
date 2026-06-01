<script lang="ts">
	import { X } from 'lucide-svelte';
	type Opt = { value: string; label: string };
	let {
		value = $bindable<string[]>([]),
		options = [],
		placeholder = 'Add…'
	}: { value?: string[]; options?: Opt[]; placeholder?: string } = $props();

	const available = $derived(options.filter((o) => !value.includes(o.value)));
	const labelOf = (v: string) => options.find((o) => o.value === v)?.label ?? v;

	function add(e: Event) {
		const v = (e.target as HTMLSelectElement).value;
		if (v && !value.includes(v)) value = [...value, v];
		(e.target as HTMLSelectElement).value = '';
	}
	function remove(v: string) {
		value = value.filter((x) => x !== v);
	}
</script>

<div>
	{#if value.length}
		<div class="mb-2 flex flex-wrap gap-1.5">
			{#each value as v (v)}
				<span
					class="inline-flex items-center gap-1 rounded-full bg-blush px-2.5 py-1 text-xs font-medium text-accent-ink"
				>
					{labelOf(v)}
					<button type="button" onclick={() => remove(v)} aria-label="Remove"><X size={12} /></button>
				</span>
			{/each}
		</div>
	{/if}
	<select class="input" value="" onchange={add}>
		<option value="" disabled selected>{placeholder}</option>
		{#each available as o (o.value)}
			<option value={o.value}>{o.label}</option>
		{/each}
	</select>
</div>
