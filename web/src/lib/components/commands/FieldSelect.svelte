<script lang="ts">
	// Compact themed dropdown — one-liner replacement for native <select> in the
	// command editor forms (bits-ui underneath: keyboard nav, smooth pop-in).
	import { Select } from '$lib/components/ui';

	let {
		value = $bindable(''),
		options,
		onChange,
		placeholder = 'Pick…',
		class: cls = ''
	}: {
		value?: string;
		// description (optional) renders as a muted second line in the dropdown
		// so each choice can explain itself in plain language.
		options: { value: string; label: string; description?: string }[];
		onChange?: (v: string) => void;
		placeholder?: string;
		class?: string;
	} = $props();

	const label = $derived(options.find((o) => o.value === value)?.label ?? '');
</script>

<Select.Root bind:value onValueChange={(v) => onChange?.(v)}>
	<Select.Trigger class={cls}>
		<span class="truncate {label ? '' : 'text-muted-foreground'}">{label || placeholder}</span>
	</Select.Trigger>
	<Select.Content>
		{#each options as o (o.value)}
			<Select.Item value={o.value} label={o.label} description={o.description} />
		{/each}
	</Select.Content>
</Select.Root>
