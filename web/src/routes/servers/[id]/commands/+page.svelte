<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import type { CommandSummary } from '$lib/commands/types';
	import NewCommandWizard from '$lib/components/commands/NewCommandWizard.svelte';

	import Page from '$lib/components/page/Page.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import PageBody from '$lib/components/page/PageBody.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import StatStrip from '$lib/components/page/StatStrip.svelte';
	import Stat from '$lib/components/page/Stat.svelte';
	import Row from '$lib/components/page/Row.svelte';
	import EmptyBlock from '$lib/components/page/EmptyBlock.svelte';
	import TopbarAction from '$lib/components/page/TopbarAction.svelte';

	import Plus from 'lucide-svelte/icons/plus';
	import Copy from 'lucide-svelte/icons/copy';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import Search from 'lucide-svelte/icons/search';

	const store = getContext<GuildStore>(GUILD_CTX);

	let commands = $state<CommandSummary[]>([]);
	let loaded = $state(false);
	let query = $state('');
	let filter = $state<'all' | 'published' | 'draft' | 'enabled' | 'disabled'>('all');
	let wizardOpen = $state(false);

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

<Page>
	<PageTopbar eyebrow="Custom Commands" subtitle={loaded ? `${commands.length} total` : undefined}>
		{#snippet actions()}
			<div class="relative hidden sm:block">
				<Search size={12} class="absolute left-2 top-1/2 -translate-y-1/2 text-faint" />
				<input
					class="h-7 w-44 rounded-md border border-line bg-bg pl-7 pr-2 text-[12px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none md:w-56"
					placeholder="Search…"
					bind:value={query}
				/>
			</div>
			<TopbarAction onclick={() => (wizardOpen = true)}>
				{#snippet icon()}<Plus size={12} />{/snippet}
				New command
			</TopbarAction>
		{/snippet}
	</PageTopbar>

	<!-- Quiet stat strip — each cell filters the list. -->
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

	<SectionBar label="Library" count={loaded ? filtered.length : undefined}>
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

	<PageBody>
		{#if !loaded}
			{#each [0, 1, 2, 3, 4] as i (i)}
				<div class="flex h-11 items-center gap-3 border-b border-line/60 px-5">
					<div class="skeleton size-1.5 rounded-full"></div>
					<div class="skeleton h-3 w-28 rounded"></div>
					<div class="skeleton h-3 w-48 rounded"></div>
					<div class="skeleton ml-auto h-3 w-10 rounded"></div>
				</div>
			{/each}
		{:else if commands.length === 0}
			<EmptyBlock
				title="No commands yet"
				body="Create your first slash command — name it, define the properties members fill in, and pick its first reply."
			>
				{#snippet cta()}
					<TopbarAction onclick={() => (wizardOpen = true)}>
						{#snippet icon()}<Plus size={12} />{/snippet}
						New command
					</TopbarAction>
				{/snippet}
			</EmptyBlock>
		{:else if filtered.length === 0}
			<EmptyBlock
				title="No commands match"
				body={query.trim() ? `Nothing for "${query.trim()}".` : 'Nothing under this filter.'}
			>
				{#snippet cta()}
					<TopbarAction
						variant="ghost"
						onclick={() => {
							query = '';
							filter = 'all';
						}}
					>
						Show all
					</TopbarAction>
				{/snippet}
			</EmptyBlock>
		{:else}
			{#each filtered as cmd (cmd.id)}
				<div class="group/row relative">
					<Row href={`/servers/${store.id}/commands/${cmd.id}`}>
						<!-- Status pip — matches the chrome's live indicator -->
						<span
							class="size-1.5 shrink-0 rounded-full {cmd.enabled
								? cmd.status === 'published'
									? 'bg-success'
									: 'bg-ink/60'
								: 'bg-faint/50'}"
							title={cmd.enabled ? cmd.status : 'off'}
						></span>
						<span class="shrink-0 font-mono text-[13px] font-medium text-ink">
							<span class="text-faint">/</span>{cmd.name}
						</span>
						<span class="shrink-0 font-mono text-[10px] tabular-nums text-faint">
							v{cmd.version}
						</span>
						{#if cmd.description}
							<span class="hidden min-w-0 truncate text-[12.5px] text-muted md:inline">
								{cmd.description}
							</span>
						{/if}
						<span class="ml-auto"></span>
						{#if cmd.requires_defer}
							<span
								class="hidden shrink-0 font-mono text-[10px] uppercase tracking-[0.12em] text-faint sm:inline"
								title="Auto-defers (worst-case path > 3s)"
							>
								defers
							</span>
						{/if}
						<span
							class="shrink-0 font-mono text-[10px] tabular-nums text-faint transition-opacity group-hover/row:opacity-0"
						>
							{relTime(cmd.updated_at)}
						</span>
					</Row>
					<!-- Hover actions float over the row's right edge -->
					<div
						class="absolute right-4 top-1/2 hidden -translate-y-1/2 items-center gap-0.5 group-hover/row:flex"
					>
						<button
							type="button"
							class="grid size-6 place-items-center rounded text-muted transition-colors hover:bg-surface hover:text-ink"
							aria-label="Duplicate"
							title="Duplicate"
							onclick={(e) => {
								e.preventDefault();
								e.stopPropagation();
								duplicate(cmd);
							}}
						>
							<Copy size={12} />
						</button>
						<button
							type="button"
							class="grid size-6 place-items-center rounded text-muted transition-colors hover:bg-surface hover:text-danger"
							aria-label="Delete"
							title="Delete"
							onclick={(e) => {
								e.preventDefault();
								e.stopPropagation();
								remove(cmd);
							}}
						>
							<Trash2 size={12} />
						</button>
					</div>
				</div>
			{/each}
		{/if}
	</PageBody>
</Page>

<NewCommandWizard
	bind:open={wizardOpen}
	guildId={store.id}
	existingNames={commands.map((c) => c.name)}
/>
