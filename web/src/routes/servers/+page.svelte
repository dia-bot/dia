<script lang="ts">
	import { onMount } from 'svelte';
	import { flip } from 'svelte/animate';
	import { goto } from '$app/navigation';
	import { api } from '$lib/api';
	import Logo from '$lib/components/Logo.svelte';
	import type { GuildListItem } from '$lib/types';
	import { Plus, LogOut, Search, RotateCw } from 'lucide-svelte';

	let { data }: { data: { user: { username: string; global_name?: string; avatar_url?: string } } } =
		$props();

	let guilds = $state<GuildListItem[]>([]);
	let loading = $state(true);
	let error = $state('');
	let query = $state('');
	let cursor = $state(0); // keyboard highlight index into `items`
	let pending = $state<Set<string>>(new Set());
	let justAdded = $state<Set<string>>(new Set());
	let searchEl = $state<HTMLInputElement | null>(null);
	let refreshing = $state(false);

	const cancelers = new Map<string, () => void>(); // guild id → stop waiting

	onMount(() => {
		searchEl?.focus();
		void (async () => {
			await refresh();
			loading = false;
		})();

		// Keep the list fresh without a manual reload: refetch when the tab/window
		// regains focus and on a light interval while visible — so the bot showing
		// up in (or leaving) a server reflects on its own.
		const tick = () => {
			if (!document.hidden) refresh();
		};
		document.addEventListener('visibilitychange', tick);
		window.addEventListener('focus', tick);
		const interval = setInterval(tick, 20000);
		return () => {
			document.removeEventListener('visibilitychange', tick);
			window.removeEventListener('focus', tick);
			clearInterval(interval);
		};
	});

	async function manualRefresh() {
		if (refreshing) return;
		refreshing = true;
		try {
			await refresh();
		} finally {
			refreshing = false;
		}
	}

	async function refresh() {
		try {
			guilds = (await api.guilds()).guilds;
			error = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load servers';
		}
	}

	const q = $derived(query.trim().toLowerCase());
	const filtered = $derived(q ? guilds.filter((g) => g.name.toLowerCase().includes(q)) : guilds);
	const managed = $derived(filtered.filter((g) => g.bot_present));
	const addable = $derived(filtered.filter((g) => !g.bot_present));
	const items = $derived([...managed, ...addable]); // flat order for keyboard nav
	const name = $derived(data.user.global_name || data.user.username);

	$effect(() => {
		if (cursor > items.length - 1) cursor = Math.max(0, items.length - 1);
	});

	function withFlag(set: Set<string>, id: string, on: boolean) {
		const next = new Set(set);
		if (on) next.add(id);
		else next.delete(id);
		return next;
	}

	function onSearchKey(e: KeyboardEvent) {
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			cursor = Math.min(cursor + 1, items.length - 1);
			scrollCursorIntoView();
		} else if (e.key === 'ArrowUp') {
			e.preventDefault();
			cursor = Math.max(cursor - 1, 0);
			scrollCursorIntoView();
		} else if (e.key === 'Enter') {
			e.preventDefault();
			activate(items[cursor]);
		} else if (e.key === 'Escape') {
			query = '';
		}
	}

	function scrollCursorIntoView() {
		requestAnimationFrame(() =>
			document.querySelector(`[data-idx="${cursor}"]`)?.scrollIntoView({ block: 'nearest' })
		);
	}

	function activate(g: GuildListItem | undefined) {
		if (!g) return;
		if (g.bot_present) goto(`/servers/${g.id}`);
		else addToServer(g);
	}

	function addToServer(g: GuildListItem) {
		if (!g.invite_url || pending.has(g.id)) return;
		const w = 500;
		const h = 820;
		const left = Math.max(0, window.screenX + (window.outerWidth - w) / 2);
		const top = Math.max(0, window.screenY + (window.outerHeight - h) / 2);
		const popup = window.open(
			g.invite_url,
			`dia-add-${g.id}`,
			`popup=1,width=${w},height=${h},left=${left},top=${top}`
		);
		if (!popup) return; // popup blocked
		pending = withFlag(pending, g.id, true);

		let done = false;
		let poll: ReturnType<typeof setInterval>;
		let settle: ReturnType<typeof setTimeout> | undefined;
		let backstop: ReturnType<typeof setTimeout>;

		const finish = (joined: boolean) => {
			if (done) return;
			done = true;
			clearInterval(poll);
			clearTimeout(settle);
			clearTimeout(backstop);
			window.removeEventListener('focus', onReturn);
			window.removeEventListener('blur', onLeave);
			cancelers.delete(g.id);
			pending = withFlag(pending, g.id, false);
			if (joined) {
				try {
					popup.close();
				} catch {
					/* already closed */
				}
				justAdded = withFlag(justAdded, g.id, true);
				setTimeout(() => (justAdded = withFlag(justAdded, g.id, false)), 2600);
			}
		};

		const check = async () => {
			await refresh();
			if (guilds.find((x) => x.id === g.id)?.bot_present) finish(true);
		};

		// `popup.closed` is unreliable once the popup navigates to Discord (COOP
		// severs the handle). Instead: poll for the join while the popup is open,
		// and when focus returns here (popup closed/left) do a final check and stop.
		const onReturn = () => {
			clearTimeout(settle);
			settle = setTimeout(async () => {
				await check();
				finish(false);
			}, 2500);
		};
		const onLeave = () => clearTimeout(settle);

		poll = setInterval(check, 1300);
		backstop = setTimeout(() => finish(false), 120_000);
		window.addEventListener('focus', onReturn);
		window.addEventListener('blur', onLeave);
		cancelers.set(g.id, () => finish(false));
	}

	async function logout() {
		await api.logout();
		location.href = '/';
	}
</script>

<svelte:head><title>Select a server · Dia</title></svelte:head>

<div class="min-h-screen">
	<header class="sticky top-0 z-10 border-b border-line bg-bg/75 backdrop-blur-md">
		<div class="mx-auto flex max-w-4xl items-center justify-between px-6 py-3">
			<a href="/" class="flex items-center"><Logo size={24} wordmark /></a>
			<div class="flex items-center gap-3">
				{#if data.user.avatar_url}
					<img src={data.user.avatar_url} alt="" class="h-7 w-7 rounded-full ring-1 ring-line-strong" />
				{/if}
				<span class="hidden text-sm text-muted sm:block">{name}</span>
				<button class="iconbtn" onclick={manualRefresh} disabled={refreshing} aria-label="Refresh servers">
					<RotateCw size={15} class={refreshing ? 'animate-spin' : ''} />
				</button>
				<button class="iconbtn" onclick={logout} aria-label="Log out"><LogOut size={16} /></button>
			</div>
		</div>
	</header>

	<main class="mx-auto max-w-4xl px-6 pb-24 pt-10 sm:pt-14">
		<h1 class="text-2xl font-semibold tracking-tight text-ink">Select a server</h1>
		<p class="mt-1 text-sm text-muted">Open a server to configure Dia, or add it to a new one.</p>

		<div class="searchbar">
			<Search size={16} class="text-faint" />
			<input
				bind:this={searchEl}
				bind:value={query}
				onkeydown={onSearchKey}
				class="searchinput"
				placeholder="Search servers…"
				autocomplete="off"
				spellcheck="false"
			/>
			<kbd class="kbd">↑↓ ↵</kbd>
		</div>

		{#if loading}
			<div class="mt-7 grid gap-2.5 sm:grid-cols-2">
				{#each { length: 6 } as _, i (i)}
					<div class="tile">
						<div class="ico skeleton"></div>
						<div class="flex-1 space-y-2">
							<div class="skeleton h-3.5 w-1/2 rounded"></div>
							<div class="skeleton h-2.5 w-1/3 rounded"></div>
						</div>
					</div>
				{/each}
			</div>
		{:else if error}
			<div class="tile mt-7 justify-between">
				<span class="text-sm text-danger">{error}</span>
				<button class="addbtn" onclick={() => refresh()}>Retry</button>
			</div>
		{:else}
			{#if managed.length}
				<p class="grouplabel">Manage <span>{managed.length}</span></p>
				<div class="mt-3 grid gap-2.5 sm:grid-cols-2">
					{#each managed as g, i (g.id)}
						<a
							href="/servers/{g.id}"
							data-idx={i}
							animate:flip={{ duration: 200 }}
							class="tile"
							class:hl={cursor === i}
							class:added={justAdded.has(g.id)}
						>
							{@render Icon(g, true)}
							<div class="min-w-0 flex-1">
								<div class="name truncate">{g.name}</div>
								<div class="meta">{justAdded.has(g.id) ? 'just added' : 'configure'}</div>
							</div>
						</a>
					{/each}
				</div>
			{/if}

			{#if addable.length}
				<p class="grouplabel mt-8">Not added <span>{addable.length}</span></p>
				<div class="mt-3 grid gap-2.5 sm:grid-cols-2">
					{#each addable as g, j (g.id)}
						<div
							data-idx={managed.length + j}
							animate:flip={{ duration: 200 }}
							class="tile"
							class:hl={cursor === managed.length + j}
						>
							{@render Icon(g, false)}
							<div class="min-w-0 flex-1">
								<div class="name truncate">{g.name}</div>
								<div class="meta">{pending.has(g.id) ? 'waiting for Dia…' : 'not added'}</div>
							</div>
							<button
								class="addbtn"
								onclick={() => (pending.has(g.id) ? cancelers.get(g.id)?.() : addToServer(g))}
							>
								{#if pending.has(g.id)}
									<span class="spin" aria-hidden="true"></span> Cancel
								{:else}
									<Plus size={14} /> Add
								{/if}
							</button>
						</div>
					{/each}
				</div>
			{/if}

			{#if !items.length}
				<div class="empty mt-7">
					{#if q}
						No servers match “{query}”.
					{:else}
						<p class="font-medium text-ink">No manageable servers</p>
						<p class="mt-1 text-sm text-muted">
							You need the <strong class="text-ink">Administrator</strong> permission on a server to
							configure Dia there.
						</p>
					{/if}
				</div>
			{/if}
		{/if}
	</main>
</div>

{#snippet Icon(g: GuildListItem, active: boolean)}
	{#if g.icon_url}
		<img src={g.icon_url} alt="" class="ico" class:dim={!active} />
	{:else}
		<div class="ico ico-fallback" class:dim={!active}>
			{g.name.trim().charAt(0).toUpperCase() || '#'}
		</div>
	{/if}
{/snippet}

<style>
	.searchbar {
		display: flex;
		align-items: center;
		gap: 0.55rem;
		margin-top: 1.5rem;
		height: 2.85rem;
		padding: 0 0.85rem;
		border-radius: 12px;
		border: 1px solid var(--color-line);
		background: var(--color-ink-2);
		transition: border-color 0.15s;
	}
	.searchbar:focus-within {
		border-color: var(--color-line-strong);
	}
	.searchinput {
		flex: 1;
		min-width: 0;
		height: 100%;
		border: 0;
		background: transparent;
		font-size: 0.92rem;
		color: var(--color-ink);
	}
	.searchinput:focus {
		outline: none;
	}
	.searchinput::placeholder {
		color: var(--color-faint);
	}
	.kbd {
		flex: none;
		font-family: var(--font-mono);
		font-size: 0.66rem;
		letter-spacing: 0.05em;
		color: var(--color-faint);
		border: 1px solid var(--color-line);
		border-radius: 6px;
		padding: 0.12rem 0.4rem;
	}

	.grouplabel {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 1.9rem;
		font-family: var(--font-mono);
		font-size: 0.7rem;
		font-weight: 500;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--color-muted);
	}
	.grouplabel span {
		color: var(--color-faint);
	}

	.tile {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.75rem;
		border-radius: 12px;
		border: 1px solid var(--color-line);
		background: var(--color-surface);
		transition:
			border-color 0.14s ease,
			background-color 0.14s ease;
	}
	a.tile {
		cursor: pointer;
	}
	/* Highlight — keyboard cursor or hover. Neutral: brighter hairline + a hair
	   more surface. No glow, no colour. */
	.tile:hover,
	.tile.hl {
		border-color: var(--color-line-strong);
		background: #17171b;
	}
	/* Brief, quiet confirmation that Dia just joined. */
	.tile.added .meta {
		color: var(--color-muted);
	}

	.ico {
		height: 2.6rem;
		width: 2.6rem;
		flex: none;
		border-radius: 10px;
		object-fit: cover;
		background: #1c1c20;
	}
	.ico-fallback {
		display: grid;
		place-items: center;
		font-family: var(--font-mono);
		font-weight: 600;
		font-size: 0.95rem;
		color: var(--color-muted);
	}
	.ico.dim {
		opacity: 0.5;
	}

	.name {
		font-weight: 600;
		color: var(--color-ink);
	}
	.meta {
		margin-top: 0.1rem;
		font-family: var(--font-mono);
		font-size: 0.7rem;
		letter-spacing: 0.02em;
		color: var(--color-faint);
	}

	.addbtn {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		flex: none;
		height: 2rem;
		padding: 0 0.75rem;
		border-radius: 9px;
		border: 1px solid var(--color-line-strong);
		background: transparent;
		font-size: 0.8rem;
		font-weight: 600;
		color: var(--color-ink);
		white-space: nowrap;
		cursor: pointer;
		transition:
			background-color 0.14s,
			color 0.14s,
			border-color 0.14s;
	}
	.addbtn:hover {
		background: var(--color-ink);
		border-color: var(--color-ink);
		color: var(--color-bg);
	}
	.spin {
		height: 0.85rem;
		width: 0.85rem;
		border-radius: 50%;
		border: 2px solid var(--color-line-strong);
		border-top-color: var(--color-muted);
		animation: spin 0.7s linear infinite;
	}

	.iconbtn {
		display: grid;
		place-items: center;
		height: 2.2rem;
		width: 2.2rem;
		border-radius: 10px;
		color: var(--color-muted);
		transition:
			background-color 0.15s,
			color 0.15s;
	}
	.iconbtn:hover {
		background: var(--color-surface);
		color: var(--color-ink);
	}

	.empty {
		border: 1px dashed var(--color-line-strong);
		border-radius: 12px;
		padding: 2.25rem;
		text-align: center;
		color: var(--color-muted);
	}

	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
</style>
