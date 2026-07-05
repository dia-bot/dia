<script lang="ts" module>
	import type { LucideIcon } from '$lib/commands/icons';
	// One entry in a page's subtab strip. This is the single source of truth for
	// the underline-tab look shared by the safety pages (via ModerationShell) and
	// the engagement pages (welcome / leveling), so every dashboard subtab reads
	// the same. badge = a small mono count; dot = a green "on" pip; dotOff = a
	// faint "present but off" pip (welcome uses it to show which trigger fires).
	export type SubTab<K extends string = string> = {
		key: K;
		label: string;
		icon?: LucideIcon;
		badge?: string | number;
		dot?: boolean;
		dotOff?: boolean;
	};
</script>

<script lang="ts" generics="K extends string">
	import type { Snippet } from 'svelte';

	let {
		tabs,
		active = $bindable(),
		actions
	}: { tabs: SubTab<K>[]; active: K; actions?: Snippet } = $props();

	// Keep `active` valid if the tab set changes or a page never sets it.
	$effect(() => {
		if (tabs.length && !tabs.some((t) => t.key === active)) active = tabs[0].key;
	});
</script>

{#if tabs.length}
	<nav
		class="flex h-9 shrink-0 items-center gap-0.5 overflow-x-auto border-b border-line bg-bg px-2 sm:px-3"
	>
		{#each tabs as t (t.key)}
			{@const on = t.key === active}
			{@const TabIcon = t.icon}
			<button
				type="button"
				onclick={() => (active = t.key)}
				aria-current={on ? 'page' : undefined}
				class="group -mb-px inline-flex h-9 shrink-0 items-center gap-1.5 border-b-2 px-2.5 text-[12.5px] font-medium transition-colors {on
					? 'border-ink text-ink'
					: 'border-transparent text-muted hover:text-ink'}"
			>
				{#if TabIcon}
					<TabIcon size={14} class={on ? 'text-ink' : 'text-faint group-hover:text-muted'} />
				{/if}
				<span>{t.label}</span>
				{#if t.badge !== undefined && t.badge !== ''}
					<span
						class="rounded-full border border-line px-1.5 font-mono text-[10px] leading-[1.4] tabular-nums {on
							? 'text-muted'
							: 'text-faint'}"
					>
						{t.badge}
					</span>
				{/if}
				{#if t.dot}
					<span class="size-1.5 shrink-0 rounded-full bg-success" title="On"></span>
				{:else if t.dotOff}
					<span class="size-1.5 shrink-0 rounded-full bg-faint/40" title="Off"></span>
				{/if}
			</button>
		{/each}
		{#if actions}
			<div class="ml-auto flex shrink-0 items-center gap-2.5 pl-3">
				{@render actions()}
			</div>
		{/if}
	</nav>
{/if}
