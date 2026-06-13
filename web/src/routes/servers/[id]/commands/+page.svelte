<script lang="ts" module>
	import type { CommandSummary as _CS, CommandGroup as _CG } from '$lib/commands/types';
	// Per-guild cache so back-navigation shows the wall instantly (no skeleton
	// flash) for the return zoom to play against; refreshed in the background.
	const listCache = new Map<string, { commands: _CS[]; groups: _CG[] }>();
</script>

<script lang="ts">
	// The Flow Atlas: the commands overview as the antechamber of the editor.
	// Commands keep their living miniature-canvas tiles; the management layer
	// is organized into group bands you can create, rename, collapse and move
	// commands between. Entering a command zooms the wall into the clicked card;
	// returning flies back out of it.
	import { onMount, tick, getContext } from 'svelte';
	import { goto, afterNavigate } from '$app/navigation';
	import { fade, fly, scale, slide } from 'svelte/transition';
	import { flip } from 'svelte/animate';
	import { cubicOut } from 'svelte/easing';
	import { dur, motionOK } from '$lib/motion';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import type { CommandSummary, CommandGroup } from '$lib/commands/types';
	import NewCommandWizard from '$lib/components/commands/NewCommandWizard.svelte';
	import FlowThumb from '$lib/components/commands/FlowThumb.svelte';
	import { DropdownMenu } from '$lib/components/ui';

	import Page from '$lib/components/page/Page.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import TopbarAction from '$lib/components/page/TopbarAction.svelte';

	import Plus from 'lucide-svelte/icons/plus';
	import Copy from 'lucide-svelte/icons/copy';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import Search from 'lucide-svelte/icons/search';
	import Play from 'lucide-svelte/icons/play';
	import ChevronDown from 'lucide-svelte/icons/chevron-down';
	import FolderInput from 'lucide-svelte/icons/folder-input';
	import FolderPlus from 'lucide-svelte/icons/folder-plus';
	import Ellipsis from 'lucide-svelte/icons/ellipsis';
	import Check from 'lucide-svelte/icons/check';

	const store = getContext<GuildStore>(GUILD_CTX);

	let commands = $state<CommandSummary[]>([]);
	let groups = $state<CommandGroup[]>([]);
	let loaded = $state(false);
	let query = $state('');
	let filter = $state<'all' | 'published' | 'draft' | 'enabled' | 'disabled'>('all');
	let wizardOpen = $state(false);
	let searchEl = $state<HTMLInputElement | null>(null);
	let booted = $state(false);

	// Enter/return animation: the wall zooms into the clicked card on enter,
	// and flies back out of it when you return from that command.
	let launchingId = $state<string | null>(null);
	let launchTimer: ReturnType<typeof setTimeout> | null = null;
	let atlasEl = $state<HTMLDivElement | null>(null);
	let returnFromId = $state<string | null>(null);
	let didReturn = false;

	// Capture which command we came back FROM, so the wall can zoom out of it.
	afterNavigate((nav) => {
		const m = (nav.from?.url.pathname ?? '').match(/\/commands\/([^/?#]+)$/);
		returnFromId = m ? m[1] : null;
	});

	// Group management state.
	let collapsed = $state<Record<string, boolean>>({});
	let editingGroup = $state<string | null>(null);
	let editName = $state('');

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

	function countFor(id: typeof filter): number {
		if (id === 'all') return commands.length;
		if (id === 'published') return stats.published;
		if (id === 'draft') return stats.drafts;
		if (id === 'enabled') return stats.enabled;
		return commands.length - stats.enabled;
	}

	const activeQuery = $derived(query.trim() !== '' || filter !== 'all');
	const showBands = $derived(groups.length > 0);

	type Band = { key: string; group: CommandGroup | null; commands: CommandSummary[] };
	const bands = $derived.by<Band[]>(() => {
		const out: Band[] = [];
		for (const g of groups) {
			out.push({ key: `g${g.id}`, group: g, commands: filtered.filter((c) => c.group_id === g.id) });
		}
		out.push({ key: 'ungrouped', group: null, commands: filtered.filter((c) => c.group_id == null) });
		// While filtering, drop empty bands; otherwise show structure (empty groups too).
		return out.filter((b) => b.commands.length > 0 || !activeQuery);
	});

	onMount(async () => {
		// Cache-first: a returning view shows the wall immediately so the
		// zoom-out plays against real tiles, not a skeleton.
		const cached = listCache.get(store.id);
		if (cached) {
			commands = cached.commands;
			groups = cached.groups;
			loaded = true;
			booted = true;
		}
		await reload();
		loaded = true;
		if (!cached) setTimeout(() => (booted = true), 600);
	});

	async function reload() {
		const [cmdRes, grpRes] = await Promise.all([
			api.commands(store.id),
			api.commandGroups(store.id).catch(() => ({ groups: [] as CommandGroup[] }))
		]);
		commands = cmdRes.commands ?? [];
		groups = grpRes.groups ?? [];
		listCache.set(store.id, { commands, groups });
	}

	// Return zoom: when arriving back from a command's editor, start the wall
	// zoomed into that card and let it fly back out to rest.
	$effect(() => {
		if (didReturn || !loaded || returnFromId === null || !atlasEl) return;
		const id = returnFromId;
		didReturn = true;
		returnFromId = null;
		if (!motionOK()) return;
		const el = atlasEl;
		void tick().then(() => {
			const tileEl = el.querySelector<HTMLElement>(`[data-cmd="${id}"]`);
			if (tileEl) playReturn(el, tileEl);
		});
	});

	// Inverse of the launch zoom: snap the wall to scaled-into the card (no
	// transition), then ease it back out to its natural rest state.
	function playReturn(atlas: HTMLDivElement, tileEl: HTMLElement) {
		const a = atlas.getBoundingClientRect();
		const t = tileEl.getBoundingClientRect();
		atlas.style.setProperty('--zoom-x', `${t.left - a.left + t.width / 2}px`);
		atlas.style.setProperty('--zoom-y', `${t.top - a.top + t.height / 2}px`);
		atlas.style.transition = 'none';
		atlas.style.transform = 'scale(1.85)';
		atlas.style.opacity = '0';
		atlas.style.filter = 'blur(2px)';
		void atlas.offsetWidth; // commit the start frame
		requestAnimationFrame(() => {
			atlas.style.transition =
				'transform 440ms cubic-bezier(0.16, 1, 0.3, 1), opacity 320ms ease-out, filter 320ms ease-out';
			atlas.style.transform = '';
			atlas.style.opacity = '';
			atlas.style.filter = '';
			setTimeout(() => {
				atlas.style.transition = '';
			}, 480);
		});
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
		// Land the copy in the same group.
		if (cmd.group_id != null) await api.setCommandGroup(store.id, res.id, cmd.group_id).catch(() => {});
		await reload();
		await goto(`/servers/${store.id}/commands/${res.id}`);
	}

	async function remove(cmd: CommandSummary) {
		if (!confirm(`Delete /${cmd.name}?`)) return;
		await api.deleteCommand(store.id, cmd.id);
		await reload();
	}

	// Optimistic move; the bands re-flow via FLIP.
	async function moveTo(cmd: CommandSummary, groupId: string | null) {
		if ((cmd.group_id ?? null) === groupId) return;
		commands = commands.map((c) => (c.id === cmd.id ? { ...c, group_id: groupId } : c));
		try {
			await api.setCommandGroup(store.id, cmd.id, groupId);
		} catch {
			await reload();
		}
	}

	async function newGroup() {
		const res = await api.createCommandGroup(store.id, 'New group');
		groups = [...groups, { id: res.id, name: res.name, position: res.position }];
		startRename(res.id, res.name);
	}

	function startRename(id: string, name: string) {
		editingGroup = id;
		editName = name;
	}

	async function commitRename() {
		const id = editingGroup;
		editingGroup = null;
		if (id == null) return;
		const name = editName.trim();
		const g = groups.find((x) => x.id === id);
		if (!name || !g || g.name === name) return;
		groups = groups.map((x) => (x.id === id ? { ...x, name } : x));
		await api.renameCommandGroup(store.id, id, name).catch(() => reload());
	}

	async function deleteGroup(g: CommandGroup) {
		if (!confirm(`Delete group "${g.name}"? Its commands stay, just ungrouped.`)) return;
		groups = groups.filter((x) => x.id !== g.id);
		commands = commands.map((c) => (c.group_id === g.id ? { ...c, group_id: null } : c));
		await api.deleteCommandGroup(store.id, g.id).catch(() => reload());
	}

	// Focus + select a rename input the moment it mounts (more reliable than
	// the autofocus attribute for dynamically inserted fields).
	function focusSelect(el: HTMLInputElement) {
		// rAF so the field is fully laid out before we grab + select it;
		// without it the selection can collapse and typing appends.
		requestAnimationFrame(() => {
			el.focus();
			el.select();
		});
	}

	function toggleCollapse(key: string) {
		collapsed = { ...collapsed, [key]: !collapsed[key] };
	}

	function relTime(iso: string): string {
		const d = new Date(iso).getTime();
		const diff = (Date.now() - d) / 1000;
		if (diff < 60) return `${Math.round(diff)}s`;
		if (diff < 3600) return `${Math.round(diff / 60)}m`;
		if (diff < 86400) return `${Math.round(diff / 3600)}h`;
		return `${Math.round(diff / 86400)}d`;
	}

	function kindTrace(cmd: CommandSummary): string {
		const shape = cmd.flow_shape ?? [];
		if (shape.length === 0) return '';
		const kinds = shape.slice(0, 3).map((n) => n.k);
		const rest = (cmd.step_count ?? shape.length) - kinds.length;
		return kinds.join(' -> ') + (rest > 0 ? ` +${rest}` : '');
	}

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

	// Launch into the editor: fly the whole wall INTO the clicked card (the
	// viewport zooms toward that exact tile and fades) so it reads as diving
	// into the command, not a crossfade. Modified clicks keep native
	// behaviour (open in a new tab / window).
	function launch(cmd: CommandSummary, e: MouseEvent) {
		if (e.metaKey || e.ctrlKey || e.shiftKey || e.altKey || e.button !== 0) return;
		e.preventDefault();
		if (launchingId !== null) return;
		const href = `/servers/${store.id}/commands/${cmd.id}`;
		if (!motionOK() || !atlasEl) {
			launchingId = cmd.id;
			goto(href);
			return;
		}
		// Anchor the zoom on the clicked tile's centre, in the atlas's box.
		const tileEl = (e.currentTarget as HTMLElement).closest('.tile') as HTMLElement | null;
		const a = atlasEl.getBoundingClientRect();
		if (tileEl) {
			const t = tileEl.getBoundingClientRect();
			atlasEl.style.setProperty('--zoom-x', `${t.left - a.left + t.width / 2}px`);
			atlasEl.style.setProperty('--zoom-y', `${t.top - a.top + t.height / 2}px`);
		}
		launchingId = cmd.id;
		if (launchTimer) clearTimeout(launchTimer);
		launchTimer = setTimeout(() => goto(href), 360);
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
			<TopbarAction variant="ink" onclick={() => (wizardOpen = true)}>
				{#snippet icon()}<Plus size={12} />{/snippet}
				New command
			</TopbarAction>
		{/snippet}
	</PageTopbar>


	<SectionBar label="Flows">
		<button
			type="button"
			class="inline-flex h-7 items-center gap-1.5 rounded-md border border-line bg-bg px-2 text-[11.5px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink"
			onclick={newGroup}
			title="Create a group to organize commands"
		>
			<FolderPlus size={12} /> New group
		</button>
		<div class="mx-0.5 h-3.5 w-px bg-line"></div>
		<div class="flex items-center gap-0.5">
			{#each filterOptions as f (f.id)}
				<button
					type="button"
					class="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] font-medium transition-colors {filter === f.id
						? 'bg-surface text-ink'
						: 'text-muted hover:text-ink'}"
					onclick={() => (filter = f.id)}
				>
					{f.label}
					<span class="font-mono text-[10px] tabular-nums {filter === f.id ? 'text-ink' : 'text-faint'}">
						{loaded ? countFor(f.id) : '—'}
					</span>
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

	<div bind:this={atlasEl} class="atlas relative min-h-0 flex-1 overflow-y-auto {launchingId !== null ? 'launching' : ''}">
		{#if !loaded}
			<div class="grid grid-cols-1 gap-4 p-4 sm:grid-cols-2 md:p-6 lg:grid-cols-3 2xl:grid-cols-4">
				{#each [0, 1, 2, 3, 4, 5] as i (i)}
					<div class="overflow-hidden rounded-xl border border-line bg-surface">
						<div class="skeleton aspect-[5/3] rounded-none"></div>
						<div class="px-3.5 pb-3 pt-2.5">
							<div class="skeleton h-3.5 w-28 rounded"></div>
							<div class="skeleton mt-2 h-3 w-40 rounded"></div>
						</div>
					</div>
				{/each}
			</div>
		{:else if commands.length === 0}
			<div class="grid place-items-center py-16" in:fade={{ duration: dur(300) }}>
				<div class="max-w-sm rounded-xl border border-line bg-surface p-6 text-center" in:fly={{ y: 8, duration: dur(220), easing: cubicOut }}>
					<p class="text-[14px] font-medium text-ink">No commands yet</p>
					<p class="mt-1.5 text-[12.5px] leading-relaxed text-muted">
						Create your first slash command. Name it, add the properties members fill in, and pick its first reply.
					</p>
					<div class="mt-4 flex justify-center">
						<TopbarAction variant="ink" onclick={() => (wizardOpen = true)}>
							{#snippet icon()}<Plus size={12} />{/snippet}
							New command
						</TopbarAction>
					</div>
				</div>
			</div>
		{:else if filtered.length === 0}
			<div class="grid place-items-center py-16" in:fade={{ duration: dur(200) }}>
				<div class="max-w-sm rounded-xl border border-line bg-surface p-6 text-center">
					<p class="text-[14px] font-medium text-ink">No commands match</p>
					<p class="mt-1.5 text-[12.5px] text-muted">
						{query.trim() ? `Nothing for "${query.trim()}".` : 'Nothing under this filter.'}
					</p>
					<div class="mt-4 flex justify-center">
						<TopbarAction variant="ghost" onclick={() => { query = ''; filter = 'all'; }}>Show all</TopbarAction>
					</div>
				</div>
			</div>
		{:else if !showBands}
			<!-- No groups yet: the clean flat wall. -->
			<div class="grid grid-cols-1 gap-4 p-4 sm:grid-cols-2 md:p-6 lg:grid-cols-3 2xl:grid-cols-4">
				{#each filtered as cmd, i (cmd.id)}
					{@render tile(cmd, i)}
				{/each}
				{@render ghost()}
			</div>
		{:else}
			<!-- Grouped bands. -->
			<div class="flex flex-col gap-1 p-4 md:p-6">
				{#each bands as band (band.key)}
					<section animate:flip={{ duration: dur(280), easing: cubicOut }}>
						<!-- Band header -->
						<div class="group/band sticky top-0 z-[5] -mx-1 mb-2 flex items-center gap-2 bg-bg/80 px-1 py-1 backdrop-blur">
							<button
								type="button"
								class="grid size-5 place-items-center rounded text-faint transition-colors hover:text-ink"
								onclick={() => toggleCollapse(band.key)}
								aria-label={collapsed[band.key] ? 'Expand' : 'Collapse'}
							>
								<ChevronDown size={13} class="transition-transform duration-200 {collapsed[band.key] ? '-rotate-90' : ''}" />
							</button>

							{#if band.group && editingGroup === band.group.id}
								<input
									use:focusSelect
									class="h-6 w-40 rounded border border-line-strong bg-bg px-1.5 font-mono text-[11px] font-medium uppercase tracking-[0.1em] text-ink focus:outline-none"
									bind:value={editName}
									onblur={commitRename}
									onkeydown={(e) => {
										if (e.key === 'Enter') commitRename();
										if (e.key === 'Escape') editingGroup = null;
									}}
								/>
							{:else if band.group}
								<button
									type="button"
									class="font-mono text-[11px] font-medium uppercase tracking-[0.12em] text-ink transition-colors hover:text-ink"
									onclick={() => band.group && startRename(band.group.id, band.group.name)}
									title="Rename group"
								>
									{band.group.name}
								</button>
							{:else}
								<span class="font-mono text-[11px] font-medium uppercase tracking-[0.12em] text-faint">
									Ungrouped
								</span>
							{/if}

							<span class="font-mono text-[10px] tabular-nums text-faint">{band.commands.length}</span>
							<div class="h-px flex-1 bg-line/60"></div>

							{#if band.group}
								<DropdownMenu.Root>
									<DropdownMenu.Trigger
										class="grid size-6 place-items-center rounded text-faint opacity-0 transition-colors hover:bg-surface hover:text-ink focus-visible:opacity-100 group-hover/band:opacity-100"
										aria-label="Group actions"
									>
										<Ellipsis size={14} />
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="end">
										<DropdownMenu.Item onSelect={() => band.group && startRename(band.group.id, band.group.name)}>
											Rename
										</DropdownMenu.Item>
										<DropdownMenu.Separator />
										<DropdownMenu.Item class="text-danger data-[highlighted]:text-danger" onSelect={() => band.group && deleteGroup(band.group)}>
											<Trash2 size={13} /> Delete group
										</DropdownMenu.Item>
									</DropdownMenu.Content>
								</DropdownMenu.Root>
							{/if}
						</div>

						{#if !collapsed[band.key]}
							<div transition:slide={{ duration: dur(200), easing: cubicOut }}>
								{#if band.commands.length === 0}
									<p class="px-1 pb-3 font-mono text-[10.5px] text-faint">
										Empty group. Move a command in with its
										<FolderInput size={11} class="mb-px inline" /> button.
									</p>
								{:else}
									<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
										{#each band.commands as cmd, i (cmd.id)}
											{@render tile(cmd, i)}
										{/each}
										{#if band.group === null && !activeQuery}
											{@render ghost()}
										{/if}
									</div>
								{/if}
							</div>
						{/if}
					</section>
				{/each}
			</div>
		{/if}
	</div>
</Page>

{#snippet tile(cmd: CommandSummary, i: number)}
	<div
		data-cmd={cmd.id}
		class="tile group relative overflow-hidden rounded-xl border border-line bg-surface {booted ? '' : 'enter'}"
		style="--i: {Math.min(i, 12)}"
		out:scale|local={{ start: 0.97, duration: dur(140), easing: cubicOut }}
		in:fade|local={{ duration: dur(180) }}
	>
		<a
			href={`/servers/${store.id}/commands/${cmd.id}`}
			class="absolute inset-0 z-0 rounded-xl focus-visible:outline focus-visible:outline-1 focus-visible:outline-line-strong"
			aria-label={`Open /${cmd.name}`}
			onclick={(e) => launch(cmd, e)}
		></a>

		<div class="pointer-events-none relative aspect-[5/3]">
			<div class="thumb absolute inset-0 border-b border-line/60"></div>
			<div class="absolute inset-0 transition-[opacity,filter] duration-300 {cmd.enabled ? '' : 'opacity-50 saturate-0'}">
				<FlowThumb shape={cmd.flow_shape} name={cmd.name} more={cmd.shape_more ?? 0} />
			</div>

			<span class="absolute left-2 top-2 flex h-[18px] items-center gap-1 rounded-full border border-line bg-surface/90 px-1.5 font-mono text-[9px] uppercase tracking-[0.12em] text-muted backdrop-blur">
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

			<div class="acts pointer-events-auto absolute right-2 top-2 z-10 flex overflow-hidden rounded-md border border-line bg-surface/90 backdrop-blur">
				{#if showBands}
					<DropdownMenu.Root>
						<DropdownMenu.Trigger
							class="grid size-6 place-items-center text-muted transition-colors hover:bg-ink-2 hover:text-ink focus-visible:ring-1 focus-visible:ring-line-strong"
							aria-label="Move to group"
							title="Move to group"
						>
							<FolderInput size={12} />
						</DropdownMenu.Trigger>
						<DropdownMenu.Content align="end" class="min-w-[11rem]">
							<div class="px-2 py-1 font-mono text-[9.5px] uppercase tracking-[0.14em] text-muted-foreground">
								Move to
							</div>
							{#each groups as g (g.id)}
								<DropdownMenu.Item onSelect={() => moveTo(cmd, g.id)}>
									<span class="size-3.5">{#if cmd.group_id === g.id}<Check size={13} />{/if}</span>
									<span class="truncate">{g.name}</span>
								</DropdownMenu.Item>
							{/each}
							<DropdownMenu.Separator />
							<DropdownMenu.Item onSelect={() => moveTo(cmd, null)}>
								<span class="size-3.5">{#if cmd.group_id == null}<Check size={13} />{/if}</span>
								Ungrouped
							</DropdownMenu.Item>
						</DropdownMenu.Content>
					</DropdownMenu.Root>
				{/if}
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

		<div class="pointer-events-none px-3.5 pt-2.5">
			<div class="flex items-baseline gap-1.5">
				<Play size={9} class="shrink-0 -translate-y-px self-center text-faint" />
				<span class="min-w-0 truncate font-mono text-[13px] font-medium text-ink">
					<span class="text-faint">/</span>{cmd.name}
				</span>
				<span class="ml-auto shrink-0 font-mono text-[10px] tabular-nums text-faint">v{cmd.version}</span>
			</div>
			{#if cmd.description}
				<p class="mt-0.5 truncate text-[11.5px] text-muted">{cmd.description}</p>
			{:else if kindTrace(cmd)}
				<p class="mt-0.5 truncate font-mono text-[10px] text-faint">{kindTrace(cmd)}</p>
			{:else}
				<p class="mt-0.5 truncate text-[11.5px] text-faint">No description</p>
			{/if}
		</div>

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
{/snippet}

{#snippet ghost()}
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
{/snippet}

<NewCommandWizard bind:open={wizardOpen} guildId={store.id} existingNames={commands.map((c) => c.name)} />

<style>
	.atlas {
		background-image: radial-gradient(rgba(255, 255, 255, 0.05) 1px, transparent 1px);
		background-size: 28px 28px;
		transform-origin: var(--zoom-x, 50%) var(--zoom-y, 50%);
		transition:
			transform 360ms cubic-bezier(0.45, 0, 0.7, 0.3),
			opacity 380ms ease-in,
			filter 380ms ease-in;
		will-change: transform, opacity;
	}
	/* Fly the wall INTO the clicked card: scale up from that tile's centre
	   and fade, so entering reads as diving in, never a crossfade. */
	.atlas.launching {
		transform: scale(1.85);
		opacity: 0;
		filter: blur(2px);
		pointer-events: none;
	}
	.thumb {
		background-color: var(--color-ink-2);
		background-image: radial-gradient(rgba(255, 255, 255, 0.05) 1px, transparent 1px);
		background-size: 14px 14px;
	}

	.enter {
		animation: canvas-node-enter 220ms var(--canvas-ease, cubic-bezier(0.22, 1, 0.36, 1)) both;
		animation-delay: calc(var(--i) * 30ms);
	}

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

	.live-dot {
		animation: drop-breathe 2.4s ease-in-out infinite;
	}

	.ghost {
		transition:
			border-color 150ms ease-out,
			opacity 300ms ease-out,
			filter 300ms ease-out;
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
		.ghost-plus,
		.atlas {
			transition-duration: 0s;
		}
		.atlas.launching {
			transform: none;
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
