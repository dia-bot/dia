<script lang="ts">
	// Friendly seconds control: a number paired with a unit select, stored as
	// raw seconds. Picks the largest exact unit when displaying an existing value.
	import Select from '$lib/components/Select.svelte';
	import NumberField from '$lib/components/ui/NumberField.svelte';

	let { value = $bindable<number | undefined>(undefined) }: { value?: number } = $props();

	const UNITS = [
		{ value: '1', label: 'seconds' },
		{ value: '60', label: 'minutes' },
		{ value: '3600', label: 'hours' },
		{ value: '86400', label: 'days' }
	];

	function pickUnit(secs: number): string {
		if (secs <= 0) return '60';
		for (const u of ['86400', '3600', '60']) if (secs % Number(u) === 0) return u;
		return '1';
	}

	// Local edit state, seeded once from the incoming value.
	const initialUnit = pickUnit(value ?? 0);
	let unit = $state(initialUnit);
	let amount = $state(value && value > 0 ? Math.round((value as number) / Number(initialUnit)) : 0);

	// Write back to the bound seconds value whenever the local controls change.
	$effect(() => {
		const next = Math.max(0, Math.round(Number(amount) || 0)) * Number(unit);
		if (next !== value) value = next;
	});
</script>

<div class="flex items-center gap-2">
	<div class="w-28">
		<NumberField bind:value={amount} min={0} />
	</div>
	<div class="w-32">
		<Select bind:value={unit} options={UNITS} />
	</div>
</div>
