<script lang="ts">
	// Channel picker that mirrors Discord's layout: top-level channels first, then
	// each category as a heading with its channels nested under it. Reads the guild
	// store from context, so callers just bind a value.
	import { getContext } from 'svelte';
	import { Select } from 'bits-ui';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { Hash, ChevronDown, Check } from 'lucide-svelte';

	let {
		value = $bindable(''),
		placeholder = 'Select a channel…'
	}: { value?: string; placeholder?: string } = $props();

	const store = getContext<GuildStore>(GUILD_CTX);
	const groups = $derived(store.channelGroups());
	const items = $derived(groups.flatMap((g) => g.channels));
	const selectedLabel = $derived(items.find((c) => c.value === value)?.label ?? '');
</script>

<Select.Root type="single" bind:value {items}>
	<Select.Trigger
		class="flex h-9 w-full items-center gap-2 rounded-lg border border-line-strong bg-ink-2 px-3 text-sm outline-none transition-colors hover:border-faint focus:border-accent data-[state=open]:border-accent"
	>
		<Hash size={14} class="shrink-0 text-faint" />
		<span class="flex-1 truncate text-left {selectedLabel ? 'text-ink' : 'text-faint'}">
			{selectedLabel || placeholder}
		</span>
		<ChevronDown size={15} class="shrink-0 text-faint" />
	</Select.Trigger>
	<Select.Portal>
		<Select.Content
			sideOffset={6}
			class="menu-pop z-50 max-h-80 w-[var(--bits-select-anchor-width)] min-w-[12rem] overflow-y-auto rounded-xl border border-line-strong bg-surface p-1.5 shadow-2xl outline-none"
		>
			<Select.Viewport>
				{#each groups as g (g.id)}
					<Select.Group>
						{#if g.name}
							<Select.GroupHeading
								class="truncate px-2 pb-1 pt-2 font-mono text-[10px] font-medium uppercase tracking-[0.1em] text-faint"
							>
								{g.name}
							</Select.GroupHeading>
						{/if}
						{#each g.channels as ch (ch.value)}
							<Select.Item
								value={ch.value}
								label={ch.label}
								class="flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm text-ink outline-none data-[highlighted]:bg-ink-2"
							>
								{#snippet children({ selected })}
									<Hash size={14} class="shrink-0 text-faint" />
									<span class="flex-1 truncate">{ch.label}</span>
									{#if selected}<Check size={14} class="shrink-0 text-accent-ink" />{/if}
								{/snippet}
							</Select.Item>
						{/each}
					</Select.Group>
				{:else}
					<div class="px-2 py-6 text-center text-xs text-faint">No channels.</div>
				{/each}
			</Select.Viewport>
		</Select.Content>
	</Select.Portal>
</Select.Root>
