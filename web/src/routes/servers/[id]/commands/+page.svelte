<script lang="ts">
	// The Flow Atlas: the commands overview as the antechamber of the editor.
	// The body is the canvas's own dotted floor holding a wall of miniature
	// canvases, one per command, each drawn from the program's real shape.
	// Hovering a tile runs its edges, the same dash-flow the editor uses.
	import { onMount, getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { fade, fly, scale } from 'svelte/transition';
	import { flip } from 'svelte/animate';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import type { CommandSummary } from '$lib/commands/types';
	import NewCommandWizard from '$lib/components/commands/NewCommandWizard.svelte';
	import FlowThumb from '$lib/components/commands/FlowThumb.svelte';

	import Page from '$lib/components/page/Page.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import StatStrip from '$lib/components/page/StatStrip.svelte';
	import Stat from '$lib/components/page/Stat.svelte';
	import TopbarAction from '$lib/components/page/TopbarAction.svelte';

	import Plus from 'lucide-svelte/icons/plus';
	import Copy from 'lucide-svelte/icons/copy';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import Search from 'lucide-svelte/icons/search';
	import Play from 'lucide-svelte/icons/play';

	const store = getContext<GuildStore>(GUILD_CTX);

	let commands = $state<CommandSummary[]>([]);
	let loaded = $state(false);
	let query = $state('');
	let filter = $state<'all' | 'published' | 'draft' | 'enabled' | 'disabled'>('all');
	let wizardOpen = $state(false);
	let searchEl = $state<HTMLInputElement | null>(null);
	// Entrance stagger plays once; later reflows are FLIP glides, never both.
	let booted = $state(false);

	const filtered = $derived(
		commands.filter((c) => {
			if (filter === 'published' && c.status !== 'published') return false;
			if (filter === 'draft' && c.status !== 'draft') return false;
			if (filter === 'enabled' && !c.enabled) return false;
			if (filter === 'disabled' && c.enabled) return false;
			if (query.trim()) {
				const q = query.trim().toLowerCase();
				if (!c.name.includes(q) && !c.description.toLowerCase().includes(q)) return false;
			}
			return true;
		})
	);

	const stats = $derived({
		total: commands.length,
		published: commands.filter((c) => c.status === 'published').length,
		drafts: commands.filter((c) => c.status === 'draft').length,
		enabled: commands.filter((c) => c.enabled).length
	});

	onMount(async () => {
		await reload();
		loaded = true;
		setTimeout(() => (booted = true), 600);
	});

	async function reload() {
		const res = await api.commands(store.id);
		commands = res.commands ?? [];
	}

	async function duplicate(cmd: CommandSummary) {
		const full = await api.command(store.id, cmd.id);
		let copyName = (cmd.name + '-copy').slice(0, 32);
		const existing = new Set(commands.map((c) => c.name));
		if (existing.has(copyName)) {
			for (let i = 2; i < 20; i++) {
				const candidate = (cmd.name + '-' + i).slice(0, 32);
				if (!existing.has(candidate)) {
					copyName = candidate;
					break;
				}
			}
		}
		const res = await api.upsertCommand(store.id, {
			name: copyName,
			description: cmd.description,
			enabled: false,
			definition: full.definition
		});
		await reload();
		await goto(`/servers/${store.id}/commands/${res.id}`);
	}

	async function remove(cmd: CommandSummary) {
		if (!confirm(`Delete /${cmd.name}?`)) return;
		await api.deleteCommand(store.id, cmd.id);
		await reload();
	}

	function relTime(iso: string): string {
		const d = new Date(iso).getTime();
		const diff = (Date.now() - d) / 1000;
		if (diff < 60) return `${Math.round(diff)}s`;
		if (diff < 3600) return `${Math.round(diff / 60)}m`;
		if (diff < 86400) return `${Math.round(diff / 3600)}h`;
		return `${Math.round(diff / 86400)}d`;
	}

	// Empty description: the program speaks for itself with a kind trace.
	function kindTrace(cmd: CommandSummary): string {
		const shape = cmd.flow_shape ?? [];
		if (shape.length === 0) return '';
		const kinds = shape.slice(0, 3).map((n) => n.k);
		const rest = (cmd.step_count ?? shape.length) - kinds.length;
		return kinds.join(' -> ') + (rest > 0 ? ` +${rest}` : '');
	}

	// Runs readout: recent activity, then last-seen, then truly never.
	function runsLabel(c: CommandSummary): string {
		if ((c.runs_24h ?? 0) > 0) return `${c.runs_24h} runs 24h`;
		if (c.last_run_at) return `last run ${relTime(c.last_run_at)}`;
		return 'no runs yet';
	}

	function isInTextField(target: EventTarget | null): boolean {
		const el = target as HTMLElement | null;
		if (!el) return false;
		const tag = el.tagName?.toLowerCase();
		return tag === 'input' || tag === 'textarea' || el.isContentEditable;
	}

	function onShortcut(e: KeyboardEvent) {
		if (e.key === '/' && !isInTextField(e.target)) {
			e.preventDefault();
			searchEl?.focus();
		}
	}

	const filterOptions: { id: typeof filter; label: string }[] = [
		{ id: 'all', label: 'All' },
		{ id: 'published', label: 'Live' },
		{ id: 'draft', label: 'Drafts' },
		{ id: 'enabled', label: 'On' },
		{ id: 'disabled', label: 'Off' }
	];
</script>

<svelte:head>
	<title>Custom Commands · {store.name} · Dia</title>
</svelte:head>

<svelte:window onkeydown={onShortcut} />

<Page>
	<PageTopbar eyebrow="Custom Commands" subtitle={loaded ? `${commands.length} total` : undefined}>
		{#snippet actions()}
			<div class="relative hidden sm:block">
				<Search size={12} class="absolute left-2 top-1/2 -translate-y-1/2 text-faint" />
				<input
					bind:this={searchEl}
					class="h-7 w-44 rounded-md border border-line bg-bg pl-7 pr-2 text-[12px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none md:w-56"
					placeholder="Search commands"
					bind:value={query}
				/>
			</div>
			<TopbarAction onclick={() => (wizardOpen = true)}>
				{#snippet icon()}<Plus size={12} />{/snippet}
				New command
			</TopbarAction>
		{/snippet}
	</PageTopbar>

	<!-- Quiet stat strip — each cell filters the wall. -->
	<StatStrip cols={4}>
		<Stat
			label="Commands"
			value={loaded ? stats.total : '—'}
			onclick={() => (filter = 'all')}
			active={filter === 'all'}
		/>
		<Stat
			label="Live"
			value={loaded ? stats.published : '—'}
			sub="published to Discord"
			onclick={() => (filter = 'published')}
			active={filter === 'published'}
		/>
		<Stat
			label="Drafts"
			value={loaded ? stats.drafts : '—'}
			sub="not published yet"
			onclick={() => (filter = 'draft')}
			active={filter === 'draft'}
		/>
		<Stat
			label="Enabled"
			value={loaded ? stats.enabled : '—'}
			sub="switched on"
			onclick={() => (filter = 'enabled')}
			active={filter === 'enabled'}
			last
		/>
	</StatStrip>

	<SectionBar label="Flows" count={loaded ? filtered.length : undefined}>
		<div class="flex items-center gap-0.5">
			{#each filterOptions as f (f.id)}
				<button
					type="button"
					class="rounded px-1.5 py-0.5 text-[11px] font-medium transition-colors {filter === f.id
						? 'bg-surface text-ink'
						: 'text-muted hover:text-ink'}"
					onclick={() => (filter = f.id)}
				>
					{f.label}
				</button>
			{/each}
		</div>
	</SectionBar>

	<!-- Mobile search -->
	<div class="border-b border-line px-4 py-2 sm:hidden">
		<div class="relative">
			<Search size={12} class="absolute left-2 top-1/2 -translate-y-1/2 text-faint" />
			<input
				class="h-9 w-full rounded-md border border-line bg-bg pl-7 pr-2 text-[13px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
				placeholder="Search commands"
				bind:value={query}
			/>
		</div>
	</div>

	<!-- ── The atlas: the canvas floor, tiled with miniature programs ── -->
	<div class="atlas relative flex-1 overflow-y-auto">
		<div class="grid grid-cols-1 gap-4 p-4 sm:grid-cols-2 md:p-6 lg:grid-cols-3 2xl:grid-cols-4">
			{#if !loaded}
				{#each [0, 1, 2, 3, 4, 5] as i (i)}
					<div class="overflow-hidden rounded-xl border border-line bg-surface">
						<div class="skeleton aspect-[5/3] rounded-none"></div>
						<div class="px-3.5 pb-3 pt-2.5">
							<div class="skeleton h-3.5 w-28 rounded"></div>
							<div class="skeleton mt-2 h-3 w-40 rounded"></div>
						</div>
					</div>
				{/each}
			{:else if commands.length === 0}
				<div class="col-span-full grid place-items-center py-16" in:fade={{ duration: dur(300) }}>
					<div
						class="max-w-sm rounded-xl border border-line bg-surface p-6 text-center"
						in:fly={{ y: 8, duration: dur(220), easing: cubicOut }}
					>
						<p class="text-[14px] font-medium text-ink">No commands yet</p>
						<p class="mt-1.5 text-[12.5px] leading-relaxed text-muted">
							Create your first slash command. Name it, add the properties members fill in,
							and pick its first reply.
						</p>
						<div class="mt-4 flex justify-center">
							<TopbarAction onclick={() => (wizardOpen = true)}>
								{#snippet icon()}<Plus size={12} />{/snippet}
								New command
							</TopbarAction>
						</div>
					</div>
				</div>
			{:else if filtered.length === 0}
				<div class="col-span-full grid place-items-center py-16" in:fade={{ duration: dur(200) }}>
					<div class="max-w-sm rounded-xl border border-line bg-surface p-6 text-center">
						<p class="text-[14px] font-medium text-ink">No commands match</p>
						<p class="mt-1.5 text-[12.5px] text-muted">
							{query.trim() ? `Nothing for "${query.trim()}".` : 'Nothing under this filter.'}
						</p>
						<div class="mt-4 flex justify-center">
							<TopbarAction
								variant="ghost"
								onclick={() => {
									query = '';
									filter = 'all';
								}}
							>
								Show all
							</TopbarAction>
						</div>
					</div>
				</div>
			{:else}
				{#each filtered as cmd, i (cmd.id)}
					<div
						class="tile group relative overflow-hidden rounded-xl border border-line bg-surface {booted
							? ''
							: 'enter'}"
						style="--i: {Math.min(i, 12)}"
						animate:flip={{ duration: dur(280), easing: cubicOut }}
						out:scale|local={{ start: 0.97, duration: dur(140), easing: cubicOut }}
						in:fade|local={{ duration: dur(180) }}
					>
						<!-- The whole tile navigates; a stretched link sits under the
						     content so the action buttons stay valid, focusable siblings. -->
						<a
							href={`/servers/${store.id}/commands/${cmd.id}`}
							class="absolute inset-0 z-0 rounded-xl focus-visible:outline focus-visible:outline-1 focus-visible:outline-line-strong"
							aria-label={`Open /${cmd.name}`}
						></a>

						<!-- Zone 1: the miniature canvas -->
						<div class="pointer-events-none relative aspect-[5/3]">
							<div class="thumb absolute inset-0 border-b border-line/60"></div>
							<div
								class="absolute inset-0 transition-[opacity,filter] duration-300 {cmd.enabled
									? ''
									: 'opacity-50 saturate-0'}"
							>
								<FlowThumb shape={cmd.flow_shape} name={cmd.name} more={cmd.shape_more ?? 0} />
							</div>

							<!-- Status chip -->
							<span
								class="absolute left-2 top-2 flex h-[18px] items-center gap-1 rounded-full border border-line bg-surface/90 px-1.5 font-mono text-[9px] uppercase tracking-[0.12em] text-muted backdrop-blur"
							>
								{#if cmd.enabled && cmd.status === 'published'}
									<span class="live-dot size-1.5 rounded-full bg-success"></span>
									Live v{cmd.version}
								{:else if !cmd.enabled}
									<span class="size-1.5 rounded-full bg-faint/50"></span>
									Off
								{:else}
									<span class="size-1.5 rounded-full border border-faint"></span>
									Draft
								{/if}
							</span>

							<!-- Hover actions: above the stretched link, clickable. -->
							<div class="acts pointer-events-auto absolute right-2 top-2 z-10 flex overflow-hidden rounded-md border border-line bg-surface/90 backdrop-blur">
								<button
									type="button"
									class="grid size-6 place-items-center text-muted transition-colors hover:bg-ink-2 hover:text-ink focus-visible:ring-1 focus-visible:ring-line-strong"
									aria-label="Duplicate"
									title="Duplicate"
									onclick={() => duplicate(cmd)}
								>
									<Copy size={12} />
								</button>
								<button
									type="button"
									class="grid size-6 place-items-center text-muted transition-colors hover:bg-ink-2 hover:text-danger focus-visible:ring-1 focus-visible:ring-line-strong"
									aria-label="Delete"
									title="Delete"
									onclick={() => remove(cmd)}
								>
									<Trash2 size={12} />
								</button>
							</div>
						</div>

						<!-- Zone 2: nameplate (decorative; clicks fall to the link) -->
						<div class="pointer-events-none px-3.5 pt-2.5">
							<div class="flex items-baseline gap-1.5">
								<Play size={9} class="shrink-0 -translate-y-px self-center text-accent-ink" />
								<span class="min-w-0 truncate font-mono text-[13px] font-medium text-ink">
									<span class="text-faint">/</span>{cmd.name}
								</span>
								<span class="ml-auto shrink-0 font-mono text-[10px] tabular-nums text-faint">
									v{cmd.version}
								</span>
							</div>
							{#if cmd.description}
								<p class="mt-0.5 truncate text-[11.5px] text-muted">{cmd.description}</p>
							{:else if kindTrace(cmd)}
								<p class="mt-0.5 truncate font-mono text-[10px] text-faint">{kindTrace(cmd)}</p>
							{:else}
								<p class="mt-0.5 truncate text-[11.5px] text-faint">No description</p>
							{/if}
						</div>

						<!-- Zone 3: readout rail -->
						<div class="pointer-events-none px-3.5 pb-2.5 pt-1.5 font-mono text-[10px] tabular-nums text-faint">
							{#if cmd.step_count !== undefined}
								<span>{cmd.step_count} {cmd.step_count === 1 ? 'step' : 'steps'}</span>
								<span class="mx-1 opacity-50">·</span>
							{/if}
							{#if cmd.option_count !== undefined && cmd.option_count > 0}
								<span>{cmd.option_count} {cmd.option_count === 1 ? 'prop' : 'props'}</span>
								<span class="mx-1 opacity-50">·</span>
							{/if}
							{#if cmd.runs_24h !== undefined}
								<span>{runsLabel(cmd)}</span>
								<span class="mx-1 opacity-50">·</span>
							{/if}
							<span>edited {relTime(cmd.updated_at)}</span>
							{#if cmd.requires_defer}
								<span class="mx-1 opacity-50">·</span>
								<span title="Auto-defers, worst-case path over 3 seconds">defers</span>
							{/if}
						</div>
					</div>
				{/each}

				<!-- The ghost slot: an empty canvas waiting to be switched on -->
				<button
					type="button"
					class="ghost relative grid min-h-[15rem] place-items-center rounded-xl border-[1.5px] border-dashed border-line bg-transparent"
					onclick={() => (wizardOpen = true)}
				>
					<span class="ghost-dots absolute inset-0 rounded-xl"></span>
					<span class="relative flex flex-col items-center gap-2">
						<span class="ghost-plus grid size-9 place-items-center rounded-lg border border-line text-muted">
							<Plus size={16} />
						</span>
						<span class="text-[12px] font-medium text-ink">New command</span>
						<span class="font-mono text-[10px] text-faint">/your-command</span>
					</span>
				</button>
			{/if}
		</div>
	</div>
</Page>

<NewCommandWizard
	bind:open={wizardOpen}
	guildId={store.id}
	existingNames={commands.map((c) => c.name)}
/>

<style>
	/* The canvas's own floor, holding the wall of programs. */
	.atlas {
		background-image: radial-gradient(rgba(255, 255, 255, 0.05) 1px, transparent 1px);
		background-size: 28px 28px;
	}
	/* Each thumbnail viewport gets a denser private dot field. */
	.thumb {
		background-color: var(--color-ink-2);
		background-image: radial-gradient(rgba(255, 255, 255, 0.05) 1px, transparent 1px);
		background-size: 14px 14px;
	}

	/* Entrance: the canvas-node-enter stagger, first paint only. */
	.enter {
		animation: canvas-node-enter 220ms var(--canvas-ease, cubic-bezier(0.22, 1, 0.36, 1)) both;
		animation-delay: calc(var(--i) * 30ms);
	}

	/* Hover physics: StepNode's grammar at poster size. */
	.tile {
		box-shadow:
			inset 0 1px 0 rgba(255, 255, 255, 0.04),
			0 1px 2px rgba(0, 0, 0, 0.4);
		transition:
			transform 200ms var(--canvas-ease, cubic-bezier(0.22, 1, 0.36, 1)),
			border-color 200ms var(--canvas-ease, cubic-bezier(0.22, 1, 0.36, 1)),
			box-shadow 200ms var(--canvas-ease, cubic-bezier(0.22, 1, 0.36, 1));
	}
	.tile:hover,
	.tile:focus-within {
		transform: translateY(-2px);
		border-color: rgba(255, 255, 255, 0.14);
		box-shadow:
			inset 0 1px 0 rgba(255, 255, 255, 0.04),
			0 12px 32px -12px rgba(0, 0, 0, 0.5);
	}
	.tile:active {
		transform: translateY(-2px) scale(0.99);
		transition-duration: 90ms;
	}

	/* The hero: hovering a tile runs its program. Hover-gated so a large
	   wall idles at zero animation cost; never runs on touch. */
	@media (hover: hover) {
		.tile:hover :global(.tedge) {
			stroke-dasharray: 4 2;
			animation: connect-dash 600ms linear infinite;
		}
		.tile:hover :global(.terr) {
			animation: connect-dash 600ms linear infinite;
		}
		.tile:hover :global(.tg) {
			transform: scale(1.03);
		}
	}

	/* Action cluster: hidden until hover/focus; always present on touch. */
	.acts {
		opacity: 0;
		transform: translateY(2px);
		transition:
			opacity 140ms ease-out,
			transform 140ms ease-out;
	}
	.tile:hover .acts,
	.tile:focus-within .acts {
		opacity: 1;
		transform: none;
	}
	@media (pointer: coarse) {
		.acts {
			opacity: 1;
			transform: none;
		}
	}

	/* LIVE dot breathes like the canvas drop indicator. */
	.live-dot {
		animation: drop-breathe 2.4s ease-in-out infinite;
	}

	/* Ghost slot: a fresh canvas switching on. */
	.ghost {
		transition: border-color 150ms ease-out;
	}
	.ghost:hover {
		border-color: var(--color-line-strong);
	}
	.ghost-dots {
		background-image: radial-gradient(rgba(255, 255, 255, 0.05) 1px, transparent 1px);
		background-size: 14px 14px;
		opacity: 0;
		transition: opacity 200ms ease-out;
	}
	.ghost:hover .ghost-dots {
		opacity: 1;
	}
	.ghost-plus {
		transition: transform 200ms var(--canvas-ease, cubic-bezier(0.22, 1, 0.36, 1));
	}
	.ghost:hover .ghost-plus {
		transform: rotate(90deg);
	}

	@media (prefers-reduced-motion: reduce) {
		.enter,
		.live-dot {
			animation: none;
		}
		.tile,
		.acts,
		.ghost,
		.ghost-dots,
		.ghost-plus {
			transition-duration: 0s;
		}
		.tile:hover :global(.tedge),
		.tile:hover :global(.terr) {
			animation: none;
		}
		.tile:hover :global(.tg) {
			transform: none;
		}
	}
</style>
