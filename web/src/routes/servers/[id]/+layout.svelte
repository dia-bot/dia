<script lang="ts">
	import { page } from '$app/stores';
	import { onDestroy, setContext, type Snippet } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import type { User } from '$lib/types';
	import Logo from '$lib/components/Logo.svelte';
	import ServerSwitcher from '$lib/components/dashboard/ServerSwitcher.svelte';
	import UserMenu from '$lib/components/dashboard/UserMenu.svelte';
	import CommandPalette from '$lib/components/dashboard/CommandPalette.svelte';
	import {
		LayoutDashboard,
		ImageIcon,
		TrendingUp,
		ToggleRight,
		UserPlus,
		ShieldCheck,
		ShieldAlert,
		Wand2,
		ChevronRight,
		Search,
		Menu,
		X
	} from 'lucide-svelte';

	let { data, children }: { data: { user: User }; children: Snippet } = $props();

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

	// Sidebar nav, grouped into labelled sections (the technical motif).
	const nav = [
		{ section: null, items: [{ label: 'Overview', path: '', icon: LayoutDashboard }] },
		{
			section: 'Engagement',
			items: [
				{ label: 'Welcome', path: 'welcome', icon: ImageIcon },
				{ label: 'Leveling', path: 'leveling', icon: TrendingUp },
				{ label: 'Reaction Roles', path: 'reaction-roles', icon: ToggleRight },
				{ label: 'Auto Roles', path: 'auto-roles', icon: UserPlus }
			]
		},
		{
			section: 'Moderation',
			items: [
				{ label: 'Moderation', path: 'moderation', icon: ShieldCheck },
				{ label: 'Automod', path: 'automod', icon: ShieldAlert }
			]
		},
		{ section: 'Advanced', items: [{ label: 'Custom Commands', path: 'commands', icon: Wand2 }] }
	];
	const flatPages = nav.flatMap((s) => s.items).map((i) => ({ label: i.label, path: i.path }));

	const base = $derived(`/servers/${$page.params.id}`);
	function isActive(p: string) {
		const path = $page.url.pathname;
		return p ? path.startsWith(`${base}/${p}`) : path === base;
	}

	// Breadcrumb tail: the current page's label.
	const currentSeg = $derived($page.url.pathname.replace(base, '').replace(/^\//, '').split('/')[0]);
	const pageTitle = $derived(flatPages.find((p) => p.path === currentSeg)?.label ?? 'Overview');

	let paletteOpen = $state(false);
	let navOpen = $state(false); // mobile drawer

	// Close the mobile drawer on navigation.
	$effect(() => {
		$page.url.pathname;
		navOpen = false;
	});

	function onKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
			e.preventDefault();
			paletteOpen = true;
		}
	}
</script>

<svelte:window onkeydown={onKeydown} />

<!-- Chrome: the dark frame behind header + sidebar. The content panel sits in a
     lighter, rounded-cornered work surface — the framed-content motif. -->
<div class="flex h-screen flex-col bg-ink-2 text-ink">
	<!-- ── Header ─────────────────────────────────────────────── -->
	<header class="flex h-14 shrink-0 items-center pr-3">
		<!-- Logo zone (sidebar width) -->
		<a
			href="/servers"
			class="flex h-full w-[260px] shrink-0 items-center gap-2 px-5 transition-opacity hover:opacity-80"
		>
			<Logo size={24} wordmark />
		</a>

		<!-- Mobile menu toggle -->
		<button
			type="button"
			onclick={() => (navOpen = true)}
			class="-ml-2 mr-1 grid h-9 w-9 place-items-center rounded-lg text-muted hover:bg-surface hover:text-ink md:hidden"
			aria-label="Open menu"
		>
			<Menu size={18} />
		</button>

		<!-- Breadcrumb: server switcher ▸ current page -->
		<div class="flex min-w-0 flex-1 items-center gap-1.5">
			<ServerSwitcher />
			<ChevronRight size={14} class="hidden shrink-0 text-line-strong sm:block" />
			<span class="hidden truncate text-[13px] font-medium text-muted sm:block">{pageTitle}</span>
		</div>

		<!-- Right utilities: live status + command palette -->
		<div class="flex shrink-0 items-center gap-2">
			{#if store.detail}
				<div
					class="hidden items-center gap-1.5 rounded-full border border-line bg-bg/40 px-2.5 py-1 sm:flex"
				>
					<span class="h-1.5 w-1.5 rounded-full bg-success" title="Live"></span>
					<span class="font-mono text-[11px] tabular-nums text-muted">
						{store.memberCount.toLocaleString()} <span class="text-faint">members</span>
					</span>
				</div>
			{/if}
			<button
				type="button"
				onclick={() => (paletteOpen = true)}
				class="flex items-center gap-2 rounded-lg border border-line px-2.5 py-1.5 text-muted transition-colors hover:border-line-strong hover:text-ink"
				aria-label="Open command palette"
			>
				<Search size={14} />
				<kbd class="hidden font-mono text-[11px] text-faint sm:block">⌘K</kbd>
			</button>
		</div>
	</header>

	<!-- ── Body: sidebar + content ────────────────────────────── -->
	<div class="flex min-h-0 flex-1">
		<!-- Mobile scrim -->
		{#if navOpen}
			<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
			<div class="fixed inset-0 z-40 bg-black/50 md:hidden" onclick={() => (navOpen = false)}></div>
		{/if}

		<!-- Sidebar -->
		<aside
			class="fixed inset-y-0 left-0 z-50 flex w-[260px] shrink-0 flex-col bg-ink-2 transition-transform duration-200 md:static md:z-auto md:translate-x-0 {navOpen
				? 'translate-x-0'
				: '-translate-x-full'} md:transition-none"
		>
			<!-- Mobile drawer header -->
			<div class="flex h-14 items-center justify-between px-5 md:hidden">
				<Logo size={24} wordmark />
				<button
					type="button"
					onclick={() => (navOpen = false)}
					class="grid h-8 w-8 place-items-center rounded-lg text-muted hover:bg-surface hover:text-ink"
					aria-label="Close menu"
				>
					<X size={16} />
				</button>
			</div>

			<nav class="flex-1 overflow-y-auto px-3 py-3">
				{#each nav as group, gi (group.section ?? gi)}
					{#if group.section}
						<div
							class="mb-1 mt-4 px-2.5 font-mono text-[10px] font-medium uppercase tracking-[0.12em] text-faint"
						>
							{group.section}
						</div>
					{/if}
					<div class="space-y-0.5">
						{#each group.items as item (item.path)}
							{@const active = isActive(item.path)}
							<a
								href={item.path ? `${base}/${item.path}` : base}
								aria-current={active ? 'page' : undefined}
								class="group flex h-8 items-center gap-2.5 rounded-md px-2.5 text-[13px] transition-colors duration-100 {active
									? 'bg-surface font-medium text-ink shadow-[inset_0_1px_0_rgba(255,255,255,0.04)]'
									: 'font-medium text-muted hover:bg-surface/50 hover:text-ink'}"
							>
								<item.icon
									size={15}
									strokeWidth={active ? 2 : 1.75}
									class="shrink-0 {active ? 'text-ink' : 'text-faint group-hover:text-muted'}"
								/>
								<span class="truncate">{item.label}</span>
							</a>
						{/each}
					</div>
				{/each}
			</nav>

			<!-- User identity, pinned bottom-left -->
			<div class="border-t border-line p-2">
				<UserMenu user={data.user} />
			</div>
		</aside>

		<!-- Content: the lighter framed work surface -->
		<main
			class="min-w-0 flex-1 overflow-auto border-line bg-bg md:rounded-tl-2xl md:border-l md:border-t"
		>
			{#if store.error}
				<div class="px-6 py-7">
					<div class="mx-auto max-w-3xl rounded-xl border border-line bg-surface p-6 text-danger">
						{store.error}
					</div>
				</div>
			{:else if store.loading && !store.detail}
				<div class="mx-auto max-w-3xl px-6 py-7">
					<div class="mb-6 space-y-2.5">
						<div class="skeleton h-6 w-44"></div>
						<div class="skeleton h-3.5 w-80 max-w-full"></div>
					</div>
					<div class="grid gap-3 sm:grid-cols-2">
						{#each { length: 6 } as _, i (i)}
							<div class="rounded-card border border-line bg-surface p-5">
								<div class="flex items-start justify-between">
									<div class="skeleton h-10 w-10 rounded-xl"></div>
									<div class="skeleton h-5 w-9 rounded-full"></div>
								</div>
								<div class="skeleton mt-4 h-4 w-28"></div>
								<div class="skeleton mt-2.5 h-3 w-full"></div>
								<div class="skeleton mt-1.5 h-3 w-2/3"></div>
								<div class="skeleton mt-4 h-3 w-20"></div>
							</div>
						{/each}
					</div>
				</div>
			{:else}
				<div class="mx-auto max-w-3xl px-6 py-7">{@render children()}</div>
			{/if}
		</main>
	</div>
</div>

<CommandPalette bind:open={paletteOpen} serverId={$page.params.id ?? ''} pages={flatPages} />
