<script lang="ts">
	// A faithful mock of what members see in Discord while typing the command:
	// the autocomplete row (name + description) above the message bar, then the
	// command pill plus one pill per property — required first, optional dimmed.
	import type { CommandOption } from '$lib/commands/types';
	import { SLASH_OPTION_KIND_BY_ID } from '$lib/commands/types';
	import { iconFor } from '$lib/commands/icons';
	import Logo from '$lib/components/Logo.svelte';

	let {
		name,
		description = '',
		options = []
	}: {
		name: string;
		description?: string;
		options?: CommandOption[];
	} = $props();

	// Discord lists required properties before optional ones.
	const ordered = $derived(
		[...options].sort((a, b) => Number(!!b.required) - Number(!!a.required))
	);
	const shownName = $derived(name || 'command');
</script>

<div class="overflow-hidden rounded-xl border border-line bg-ink-2">
	<!-- The command picker row Discord shows above the box while typing "/" -->
	<div class="flex items-center gap-2.5 border-b border-line/60 px-3.5 py-2.5">
		<div class="grid size-6 shrink-0 place-items-center overflow-hidden rounded-full bg-surface">
			<Logo size={14} />
		</div>
		<span class="shrink-0 font-mono text-[12.5px] font-medium text-ink">/{shownName}</span>
		<span class="min-w-0 truncate text-[11.5px] text-muted">
			{description || 'Custom command'}
		</span>
		<span
			class="ml-auto shrink-0 font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-faint"
		>
			In Discord
		</span>
	</div>

	<!-- The message bar with the typed command + property pills -->
	<div class="px-3.5 py-3">
		<div
			class="flex min-h-9 flex-wrap items-center gap-1.5 rounded-lg border border-line bg-surface px-2.5 py-1.5"
		>
			<span
				class="inline-flex h-6 shrink-0 items-center rounded-md bg-accent/15 px-1.5 font-mono text-[11.5px] font-medium text-accent-ink"
			>
				/{shownName}
			</span>
			{#each ordered as o, i (`${o.name}-${i}`)}
				{@const meta = SLASH_OPTION_KIND_BY_ID.get(o.kind)}
				{@const Icon = iconFor(meta?.icon ?? 'Square')}
				<span
					class="inline-flex h-6 items-center gap-1 rounded-md px-1.5 font-mono text-[10.5px] {o.required
						? 'border border-line-strong bg-bg text-ink'
						: 'border border-dashed border-line bg-transparent text-muted'}"
					title="{meta?.label ?? o.kind}{o.required ? ' · required' : ' · optional'}"
				>
					<Icon size={10} class={o.required ? 'text-muted' : 'text-faint'} />
					{o.name || 'property'}{o.required ? '' : '?'}
				</span>
			{/each}
			{#if options.length === 0}
				<span class="text-[11.5px] italic text-faint">no properties — runs right away</span>
			{/if}
		</div>
		{#if options.length > 0}
			<p class="mt-1.5 font-mono text-[10px] text-faint">
				required fill in first · <span class="text-faint/80">name?</span> = optional, offered in a
				picker
			</p>
		{/if}
	</div>
</div>
