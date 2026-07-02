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
					class="inline-flex items-center gap-1 rounded-full border border-line bg-surface py-1 pl-2 pr-1 text-xs font-medium text-ink"
				>
					{labelOf(v)}
					<button
						type="button"
						class="grid size-4 place-items-center rounded-full opacity-70 transition hover:bg-ink-2 hover:opacity-100"
						onclick={() => remove(v)}
						aria-label="Remove"><X size={12} /></button
					>
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
