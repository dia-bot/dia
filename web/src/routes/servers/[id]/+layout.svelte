<script lang="ts">
	import { page } from '$app/stores';
	import { onDestroy, setContext, type Snippet } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import {
		LayoutDashboard,
		ImageIcon,
		TrendingUp,
		ToggleRight,
		UserPlus,
		ShieldCheck,
		ShieldAlert,
		Wand2,
		ChevronLeft
	} from 'lucide-svelte';

	let { children }: { children: Snippet } = $props();

	const store = new GuildStore($page.params.id ?? '');
	setContext(GUILD_CTX, store);

	let loadedFor = '';
	$effect(() => {
		const gid = $page.params.id ?? '';
		if (gid !== loadedFor) {
			loadedFor = gid;
			store.id = gid;
			store.load();
			store.connect();
		}
	});
	onDestroy(() => store.destroy());

	const nav = [
		{ label: 'Overview', path: '', icon: LayoutDashboard },
		{ label: 'Welcome', path: 'welcome', icon: ImageIcon },
		{ label: 'Leveling', path: 'leveling', icon: TrendingUp },
		{ label: 'Reaction Roles', path: 'reaction-roles', icon: ToggleRight },
		{ label: 'Auto Roles', path: 'auto-roles', icon: UserPlus },
		{ label: 'Moderation', path: 'moderation', icon: ShieldCheck },
		{ label: 'Automod', path: 'automod', icon: ShieldAlert },
		{ label: 'Custom Commands', path: 'commands', icon: Wand2 }
	];

	const base = $derived(`/servers/${$page.params.id}`);
	function isActive(p: string) {
		const path = $page.url.pathname;
		return p ? path.startsWith(`${base}/${p}`) : path === base;
	}
</script>

<div class="flex min-h-screen">
	<!-- Sidebar -->
	<aside class="sticky top-0 hidden h-screen w-64 shrink-0 flex-col border-r border-line bg-bg md:flex">
		<div class="flex h-16 items-center px-5">
			<a href="/servers" class="flex items-center"><Logo size={26} wordmark /></a>
		</div>
		<a
			href="/servers"
			class="mx-3 mb-2 flex items-center gap-1.5 px-2 text-xs font-medium text-muted hover:text-ink"
		>
			<ChevronLeft size={14} /> All servers
		</a>
		<nav class="flex-1 space-y-0.5 px-3 py-2">
			{#each nav as item (item.path)}
				<a
					href={item.path ? `${base}/${item.path}` : base}
					class="flex h-9 items-center gap-2.5 rounded-lg px-2.5 text-sm transition-colors {isActive(
						item.path
					)
						? 'bg-blush font-medium text-accent-ink'
						: 'text-muted hover:bg-surface hover:text-ink'}"
				>
					<item.icon size={16} class="shrink-0" />
					<span class="truncate">{item.label}</span>
				</a>
			{/each}
		</nav>
	</aside>

	<!-- Main -->
	<div class="flex min-w-0 flex-1 flex-col">
		<header class="flex h-16 items-center gap-3 border-b border-line bg-surface px-6">
			{#if store.detail?.guild.icon}
				<img
					src="https://cdn.discordapp.com/icons/{store.id}/{store.detail.guild.icon}.png?size=64"
					alt=""
					class="h-8 w-8 rounded-full"
				/>
			{:else}
				<div class="grid h-8 w-8 place-items-center rounded-full bg-blush text-sm font-bold text-accent">
					{(store.name || '?').charAt(0).toUpperCase()}
				</div>
			{/if}
			<div class="min-w-0">
				<div class="truncate font-semibold leading-tight">{store.name || 'Loading…'}</div>
				<div class="flex items-center gap-1.5 text-xs text-muted">
					<span class="inline-block h-1.5 w-1.5 rounded-full bg-success"></span>
					{store.memberCount.toLocaleString()} members · live
				</div>
			</div>
		</header>

		<main class="flex-1 overflow-auto px-6 py-7">
			{#if store.error}
				<div class="rounded-xl border border-line bg-surface p-6 text-danger">{store.error}</div>
			{:else if store.loading && !store.detail}
				<div class="text-muted">Loading server…</div>
			{:else}
				<div class="mx-auto max-w-3xl">{@render children()}</div>
			{/if}
		</main>
	</div>
</div>
