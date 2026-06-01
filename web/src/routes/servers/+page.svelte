<script lang="ts">
	import { onMount } from 'svelte';
	import { api } from '$lib/api';
	import Logo from '$lib/components/Logo.svelte';
	import type { GuildListItem } from '$lib/types';
	import { Plus, Settings, LogOut } from 'lucide-svelte';

	let { data }: { data: { user: { username: string; avatar_url?: string } } } = $props();

	let guilds = $state<GuildListItem[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			const res = await api.guilds();
			guilds = res.guilds;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load servers';
		} finally {
			loading = false;
		}
	});

	const managed = $derived(guilds.filter((g) => g.bot_present));
	const addable = $derived(guilds.filter((g) => !g.bot_present));

	async function logout() {
		await api.logout();
		location.href = '/';
	}
</script>

<svelte:head><title>Your servers · Dia</title></svelte:head>

<div class="min-h-screen">
	<header class="border-b border-line bg-surface">
		<div class="mx-auto flex max-w-5xl items-center justify-between px-6 py-4">
			<a href="/" class="flex items-center"><Logo size={28} wordmark /></a>
			<button class="btn btn-ghost h-9" onclick={logout}><LogOut size={16} /> Log out</button>
		</div>
	</header>

	<main class="mx-auto max-w-5xl px-6 py-10">
		<h1 class="text-2xl font-bold tracking-tight">Your servers</h1>
		<p class="mt-1 text-muted">Pick a server to configure, or add Dia to a new one.</p>

		{#if loading}
			<div class="mt-10 text-muted">Loading…</div>
		{:else if error}
			<div class="mt-10 rounded-xl border border-line bg-surface p-6 text-danger">{error}</div>
		{:else}
			{#if managed.length}
				<h2 class="eyebrow mt-10">Manage</h2>
				<div class="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each managed as g (g.id)}
						<a
							href="/servers/{g.id}"
							class="card flex items-center gap-3 p-4 transition-colors hover:border-line-strong"
						>
							{@render GuildIcon(g.icon_url, g.name)}
							<div class="min-w-0 flex-1">
								<div class="truncate font-semibold">{g.name}</div>
								<div class="text-xs text-muted">Configure</div>
							</div>
							<Settings size={18} class="text-faint" />
						</a>
					{/each}
				</div>
			{/if}

			{#if addable.length}
				<h2 class="eyebrow mt-10">Add to a server</h2>
				<div class="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each addable as g (g.id)}
						<a
							href={g.invite_url}
							class="card flex items-center gap-3 p-4 opacity-80 transition-opacity hover:opacity-100"
						>
							{@render GuildIcon(g.icon_url, g.name)}
							<div class="min-w-0 flex-1">
								<div class="truncate font-semibold">{g.name}</div>
								<div class="text-xs text-accent">Add Dia</div>
							</div>
							<Plus size={18} class="text-accent" />
						</a>
					{/each}
				</div>
			{/if}

			{#if !managed.length && !addable.length}
				<div class="mt-10 rounded-xl border border-line bg-surface p-8 text-center text-muted">
					You don't manage any servers. You need the <strong>Manage Server</strong> permission.
				</div>
			{/if}
		{/if}
	</main>
</div>

{#snippet GuildIcon(icon: string | undefined, name: string)}
	{#if icon}
		<img src={icon} alt="" class="h-10 w-10 rounded-full" />
	{:else}
		<div class="grid h-10 w-10 place-items-center rounded-full bg-blush font-bold text-accent">
			{name.charAt(0).toUpperCase()}
		</div>
	{/if}
{/snippet}
