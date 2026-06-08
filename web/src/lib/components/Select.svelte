<script lang="ts">
	// Custom dropdown on Bits UI Select — keyboard + typeahead + portalled, fully
	// styled to Dia. Same {value, options, placeholder} API as the old native
	// <select>, so every page that imports it upgrades automatically.
	import { Select } from 'bits-ui';
	import { Check, ChevronDown } from 'lucide-svelte';

	type Opt = { value: string; label: string };
	let {
		value = $bindable(''),
		options = [],
		placeholder = 'Select…',
		dense = false
	}: { value?: string; options?: Opt[]; placeholder?: string; dense?: boolean } = $props();

	const selectedLabel = $derived(options.find((o) => o.value === value)?.label ?? '');
</script>

<Select.Root type="single" bind:value items={options}>
	<Select.Trigger
		class="group flex w-full items-center justify-between gap-2 rounded-md border border-line bg-ink-2 outline-none transition-all hover:border-faint focus-visible:border-faint focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/20 data-[state=open]:border-faint data-[state=open]:ring-2 data-[state=open]:ring-accent/20 {dense
				? 'h-7 px-2.5 text-xs'
				: 'h-9 px-3 text-sm'}"
	>
		<span class="truncate {selectedLabel ? 'text-ink' : 'text-faint'}">
			{selectedLabel || placeholder}
		</span>
		<ChevronDown size={15} class="shrink-0 text-faint transition-transform duration-150 group-data-[state=open]:rotate-180" />
	</Select.Trigger>
	<Select.Portal>
		<Select.Content
			sideOffset={6}
			class="menu-pop z-50 max-h-72 w-[var(--bits-select-anchor-width)] min-w-[8rem] overflow-y-auto rounded-xl border border-line-strong bg-surface p-1.5 shadow-2xl outline-none"
		>
			<Select.Viewport>
				{#each options as o (o.value)}
					<Select.Item
						value={o.value}
						label={o.label}
						class="flex cursor-pointer items-center justify-between gap-2 rounded-md px-2.5 py-1.5 text-sm text-muted outline-none transition-colors data-[highlighted]:bg-ink-2 data-[highlighted]:text-ink data-[selected]:text-ink"
					>
						{#snippet children({ selected })}
							<span class="truncate">{o.label}</span>
							{#if selected}<Check size={14} class="shrink-0 text-ink" />{/if}
						{/snippet}
					</Select.Item>
				{/each}
			</Select.Viewport>
		</Select.Content>
	</Select.Portal>
</Select.Root>
