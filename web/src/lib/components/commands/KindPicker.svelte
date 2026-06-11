<script lang="ts">
	// Searchable, grouped list of step kinds. Pure picker — the host renders
	// the floating container (Add-step dropdown, drop-point popover) and gets
	// the picked kind back through onPick.
	import { STEP_CATEGORIES, STEP_KINDS } from '$lib/commands/types';
	import { iconFor } from '$lib/commands/icons';
	import Search from 'lucide-svelte/icons/search';

	let {
		onPick,
		autofocus = true
	}: {
		onPick: (kind: string) => void;
		autofocus?: boolean;
	} = $props();

	let query = $state('');
	const lc = $derived(query.trim().toLowerCase());

	const groups = $derived(
		STEP_CATEGORIES.map((c) => ({
			category: c,
			items: STEP_KINDS.filter(
				(k) =>
					!k.hidden &&
					k.category === c.id &&
					(!lc ||
						k.kind.includes(lc) ||
						k.label.toLowerCase().includes(lc) ||
						k.short.toLowerCase().includes(lc))
			)
		})).filter((g) => g.items.length > 0)
	);

	const flat = $derived(groups.flatMap((g) => g.items));

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' && flat.length > 0) {
			e.preventDefault();
			onPick(flat[0].kind);
		}
	}
</script>

<div class="flex max-h-80 w-64 flex-col">
	<div class="relative shrink-0 border-b border-line/60 p-1.5">
		<Search size={11} class="absolute left-3.5 top-1/2 -translate-y-1/2 text-faint" />
		<!-- svelte-ignore a11y_autofocus -->
		<input
			class="h-7 w-full rounded-md border border-line bg-bg pl-6 pr-2 text-[12px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
			placeholder="Search steps…"
			{autofocus}
			bind:value={query}
			onkeydown={onKeydown}
		/>
	</div>
	<div class="min-h-0 flex-1 overflow-y-auto p-1">
		{#if groups.length === 0}
			<p class="px-2 py-3 text-center font-mono text-[10.5px] text-faint">no match</p>
		{/if}
		{#each groups as g (g.category.id)}
			<div
				class="px-2 pb-0.5 pt-2 font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-faint"
			>
				{g.category.label}
			</div>
			{#each g.items as k (k.kind)}
				{@const Icon = iconFor(k.icon)}
				<button
					type="button"
					class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-surface"
					onclick={() => onPick(k.kind)}
				>
					<span
						class="grid size-5 shrink-0 place-items-center rounded border border-line bg-ink-2 text-muted"
					>
						<Icon size={10} />
					</span>
					<span class="shrink-0 text-[12px] font-medium text-ink">{k.label}</span>
					<span class="min-w-0 truncate text-[10.5px] text-faint">{k.short}</span>
				</button>
			{/each}
		{/each}
	</div>
</div>
