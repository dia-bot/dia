<script lang="ts" module>
	import type { LucideIcon } from '$lib/commands/icons';
	// One sub-section of a safety page. These are the page's OWN sections (Rules,
	// Escalation, Events, …) — deliberately NOT the four moderation modules, which
	// already live in the sidebar. badge = a small mono count; dot = a status pip.
	export type ModTab = {
		key: string;
		label: string;
		icon?: LucideIcon;
		badge?: string | number;
		dot?: boolean;
	};
</script>

<script lang="ts">
	// Shared chrome for the safety pages (Overview, Automod, Verification, Server
	// Logs). Flat and near-monochrome, matching the welcome / commands / automations
	// surfaces: a slim page-header row, a per-page subtab strip, and a full-bleed
	// body the page fills with hairline-separated sections. The shell owns the
	// loading skeleton and the error/retry state so no page can hang on a blank
	// skeleton, and owns the floating save dock.
	import { type Snippet } from 'svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ReleaseDock from '$lib/components/page/ReleaseDock.svelte';
	import { RotateCw, CircleAlert } from 'lucide-svelte';

	let {
		icon,
		title,
		blurb,
		enabled = $bindable(false),
		ready = true,
		error = '',
		onretry,
		toggleLabel,
		tabs = [],
		active = $bindable(''),
		dirty = false,
		saving = false,
		onsave,
		onreset,
		actions,
		children,
		skeleton
	}: {
		icon?: LucideIcon;
		title: string;
		blurb?: string;
		enabled?: boolean;
		ready?: boolean;
		error?: string;
		onretry?: () => void;
		toggleLabel?: string;
		tabs?: ModTab[];
		active?: string;
		dirty?: boolean;
		saving?: boolean;
		onsave?: () => void;
		onreset?: () => void;
		actions?: Snippet;
		children: Snippet;
		skeleton?: Snippet;
	} = $props();

	const HeaderIcon = $derived(icon);

	// Keep `active` valid: if a page never sets it (or its tab set changes), fall
	// back to the first tab so the strip always has a selection.
	$effect(() => {
		if (tabs.length && !tabs.some((t) => t.key === active)) active = tabs[0].key;
	});

	function onKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
			if (!dirty || !onsave) return;
			e.preventDefault();
			onsave();
		}
	}
</script>

<svelte:window onkeydown={onKeydown} />

<div class="flex h-full flex-col bg-bg text-ink">
	<!-- ── Page header ──────────────────────────────────────────────────── -->
	<header class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line bg-bg px-4 sm:px-5">
		{#if HeaderIcon}
			<span class="grid size-6 shrink-0 place-items-center rounded border border-line bg-surface text-muted">
				<HeaderIcon size={13} />
			</span>
		{/if}
		<span class="text-[13px] font-semibold tracking-tight text-ink">{title}</span>
		{#if blurb}
			<div class="hidden h-3.5 w-px bg-line sm:block"></div>
			<span class="hidden min-w-0 truncate text-[12px] text-muted sm:block">{blurb}</span>
		{/if}
		<div class="ml-auto flex shrink-0 items-center gap-1.5">
			{#if actions}
				{@render actions()}
			{/if}
			{#if ready && !error}
				<label class="ml-0.5 flex items-center gap-2 text-[12px]">
					<span class="hidden text-muted sm:inline">{enabled ? 'On' : 'Off'}</span>
					<Toggle bind:checked={enabled} label={toggleLabel ?? title} />
				</label>
			{/if}
		</div>
	</header>

	<!-- ── Subtab strip: the page's own sections (NOT the sidebar's modules) ── -->
	{#if tabs.length && ready && !error}
		<nav class="flex h-9 shrink-0 items-center gap-0.5 overflow-x-auto border-b border-line bg-bg px-2 sm:px-3">
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
						<span class="size-1.5 rounded-full bg-success" title="On"></span>
					{/if}
				</button>
			{/each}
		</nav>
	{/if}

	<!-- ── Body ─────────────────────────────────────────────────────────── -->
	<div class="min-h-0 flex-1 overflow-y-auto">
		{#if error}
			<!-- Load failed: a calm retry panel instead of a forever-spinning skeleton. -->
			<div class="flex min-h-full items-center justify-center px-6 py-16">
				<div class="flex max-w-md flex-col items-center gap-3 text-center">
					<span class="grid size-10 place-items-center rounded-full border border-line bg-surface text-danger">
						<CircleAlert size={18} />
					</span>
					<div>
						<p class="text-[13px] font-semibold text-ink">Couldn't load {title}</p>
						<p class="mt-1 max-w-sm text-[12px] text-muted">{error}</p>
					</div>
					{#if onretry}
						<button
							type="button"
							onclick={onretry}
							class="mt-1 inline-flex h-8 items-center gap-1.5 rounded-md border border-line-strong px-3 text-[12px] font-medium text-ink transition-colors hover:bg-ink-2"
						>
							<RotateCw size={13} /> Try again
						</button>
					{/if}
				</div>
			</div>
		{:else if !ready}
			{#if skeleton}
				{@render skeleton()}
			{:else}
				<!-- Default skeleton: a few hairline-separated section placeholders. -->
				<div>
					{#each Array(3) as _, i (i)}
						<div class="border-b border-line px-4 py-5 sm:px-5">
							<div class="skeleton h-3 w-24 rounded"></div>
							<div class="skeleton mt-4 h-9 w-full max-w-xl rounded-lg"></div>
							<div class="skeleton mt-3 h-9 w-full max-w-md rounded-lg"></div>
						</div>
					{/each}
				</div>
			{/if}
		{:else}
			<div class="pb-20">
				{@render children()}
			</div>
		{/if}
	</div>

	<!-- ── Save dock: the shared floating pill, centered on the viewport ── -->
	{#if ready && !error}
		<ReleaseDock {dirty} phase={saving ? 'saving' : 'idle'} {onsave} {onreset} />
	{/if}
</div>
