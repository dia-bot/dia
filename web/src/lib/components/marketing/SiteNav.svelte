<script lang="ts">
	import { onMount } from 'svelte';
	import Logo from '$lib/components/Logo.svelte';
	import { loginURL } from '$lib/api';
	import Github from 'lucide-svelte/icons/github';
	import ChevronDown from 'lucide-svelte/icons/chevron-down';
	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Menu from 'lucide-svelte/icons/menu';
	import X from 'lucide-svelte/icons/x';
	import ImageIcon from 'lucide-svelte/icons/image';
	import TrendingUp from 'lucide-svelte/icons/trending-up';
	import Users from 'lucide-svelte/icons/users';
	import ShieldCheck from 'lucide-svelte/icons/shield-check';
	import TerminalIcon from 'lucide-svelte/icons/terminal';
	import LayoutDashboard from 'lucide-svelte/icons/layout-dashboard';

	// Sticky header with a warmbly-style "Features" mega dropdown, rendered in
	// the developer/technical theme. Opens on hover (desktop) or click/keyboard.
	// `overlay` lets it sit transparent with light text over a dark hero at the
	// top of the page, turning solid (and dark-text) once scrolled.
	let { user = null, overlay = false }: { user?: { username: string } | null; overlay?: boolean } = $props();

	const featureItems = [
		{ id: 'welcome', title: 'Welcome', tag: 'Onboard', desc: 'Custom welcome cards, posted on join.', icon: ImageIcon },
		{ id: 'leveling', title: 'Leveling', tag: 'Engage', desc: 'XP, rank cards and role rewards.', icon: TrendingUp },
		{ id: 'roles', title: 'Roles', tag: 'Self-serve', desc: 'Button & select role menus.', icon: Users },
		{ id: 'moderation', title: 'Moderation', tag: 'Protect', desc: 'Case log plus rule-based AutoMod.', icon: ShieldCheck },
		{ id: 'commands', title: 'Commands', tag: 'Extend', desc: 'Build your own slash commands.', icon: TerminalIcon },
		{ id: 'dashboard', title: 'Dashboard', tag: 'Control', desc: 'Realtime, Discord-secured config.', icon: LayoutDashboard }
	];
	const links = [
		{ label: 'Pricing', href: '/pricing' },
		{ label: 'Compare', href: '/compare' },
		{ label: 'About', href: '/about' }
	];

	let scrolled = $state(false);
	let open = $state(false);
	let mobileOpen = $state(false);
	let t: ReturnType<typeof setTimeout>;

	const openNow = () => {
		clearTimeout(t);
		open = true;
	};
	const scheduleClose = () => {
		clearTimeout(t);
		t = setTimeout(() => (open = false), 180);
	};
	const cancelClose = () => clearTimeout(t);

	// Light (transparent-over-dark-hero) treatment only at the very top of an
	// overlay page; once scrolled or a menu is open the nav is solid + dark-text.
	const light = $derived(overlay && !scrolled && !open && !mobileOpen);

	onMount(() => {
		const onScroll = () => (scrolled = window.scrollY > 8);
		onScroll();
		const onKey = (e: KeyboardEvent) => {
			if (e.key === 'Escape') {
				open = false;
				mobileOpen = false;
			}
		};
		window.addEventListener('scroll', onScroll, { passive: true });
		window.addEventListener('keydown', onKey);
		return () => {
			window.removeEventListener('scroll', onScroll);
			window.removeEventListener('keydown', onKey);
		};
	});
</script>

<header
	class="sticky top-0 z-50 transition-colors duration-300 {scrolled || open || mobileOpen
		? 'border-b border-line bg-bg/85 backdrop-blur-md'
		: 'border-b border-transparent'}"
>
	<div class="mx-auto flex h-16 max-w-page items-center justify-between px-6">
		<a href="/" class="flex items-center" aria-label="Dia home"><Logo size={30} wordmark dark={light} /></a>

		<nav class="hidden items-center gap-1 md:flex">
			<div class="relative" role="none" onmouseenter={openNow} onmouseleave={scheduleClose}>
				<a
					href="/features"
					aria-expanded={open}
					onfocus={openNow}
					class="inline-flex h-9 items-center gap-1 rounded-md px-3 text-sm font-medium transition-colors {open
						? 'text-ink'
						: light
							? 'text-white/85 hover:text-white'
							: 'text-muted hover:text-ink'}"
				>
					Features
					<ChevronDown size={14} class="transition-transform duration-200 {open ? 'rotate-180' : ''}" />
				</a>

				{#if open}
					<div
						class="fixed inset-x-0 top-16 z-40"
						role="none"
						onmouseenter={cancelClose}
						onmouseleave={scheduleClose}
					>
						<div class="border-b border-line bg-surface shadow-[0_40px_90px_-30px_rgba(0,0,0,0.7)]">
							<div class="mx-auto grid max-w-page px-6 py-10 lg:grid-cols-[320px_1fr]">
								<div class="pr-10">
									<span class="eyebrow">[ features ]</span>
									<h3 class="mt-4 text-3xl font-bold leading-[1.05] tracking-[-0.02em]">
										Everything your server needs.
									</h3>
									<p class="mt-4 max-w-xs text-sm leading-relaxed text-muted">
										Six essentials in one open, self-hostable bot — configured from a single realtime
										dashboard.
									</p>
									<a
										href="/features"
										onclick={() => (open = false)}
										class="mt-6 inline-flex items-center gap-1.5 text-sm font-medium text-accent-ink hover:text-accent"
									>
										See all features <ArrowRight size={14} />
									</a>
								</div>
								<div
									class="mt-8 grid grid-cols-2 gap-2 lg:mt-0 lg:grid-cols-3 lg:border-l lg:border-line lg:pl-8"
								>
									{#each featureItems as f (f.id)}
										<a
											href="/features/{f.id}"
											onclick={() => (open = false)}
											class="group flex flex-col rounded-2xl p-5 transition-colors hover:bg-bg"
										>
											<span class="grid h-10 w-10 place-items-center rounded-xl bg-blush text-accent">
												<f.icon size={18} />
											</span>
											<div class="mt-4 font-mono text-[11px] uppercase tracking-wide text-accent-ink">
												{f.tag}
											</div>
											<div class="mt-1 text-[15px] font-semibold group-hover:text-accent-ink">{f.title}</div>
											<p class="mt-1 text-[13px] leading-snug text-muted">{f.desc}</p>
										</a>
									{/each}
								</div>
							</div>
						</div>
					</div>
				{/if}
			</div>

			{#each links as l (l.href)}
				<a
					href={l.href}
					class="inline-flex h-9 items-center rounded-md px-3 text-sm font-medium transition-colors {light
						? 'text-white/75 hover:text-white'
						: 'text-muted hover:text-ink'}">{l.label}</a
				>
			{/each}
			<a
				href="https://github.com/dia-bot/dia"
				class="inline-flex h-9 items-center gap-1.5 rounded-md px-3 text-sm font-medium transition-colors {light
					? 'text-white/75 hover:text-white'
					: 'text-muted hover:text-ink'}"
			>
				<Github size={15} /> GitHub
			</a>
		</nav>

		<div class="flex items-center gap-2">
			{#if user}
				<a
					href="/servers"
					class="brand-gradient inline-flex h-9 items-center gap-2 rounded-xl px-4 text-sm font-semibold text-white shadow-[0_6px_22px_-10px_rgba(178,68,252,0.55)] transition-[filter] duration-150 hover:brightness-110"
					>Open dashboard</a
				>
			{:else}
				<a
					href={loginURL}
					class="brand-gradient inline-flex h-9 items-center gap-2 rounded-xl px-4 text-sm font-semibold text-white shadow-[0_6px_22px_-10px_rgba(178,68,252,0.55)] transition-[filter] duration-150 hover:brightness-110"
					>Get started</a
				>
			{/if}
			<button
				type="button"
				class="inline-flex h-9 w-9 items-center justify-center rounded-md md:hidden {light
					? 'text-white/85 hover:text-white'
					: 'text-muted hover:text-ink'}"
				aria-label="Toggle menu"
				aria-expanded={mobileOpen}
				onclick={() => (mobileOpen = !mobileOpen)}
			>
				{#if mobileOpen}<X size={18} />{:else}<Menu size={18} />{/if}
			</button>
		</div>
	</div>

	{#if mobileOpen}
		<div class="border-t border-line bg-bg md:hidden">
			<div class="mx-auto max-w-page px-6 py-4">
				<span class="eyebrow">[ features ]</span>
				<div class="mt-2 grid grid-cols-2 gap-2">
					{#each featureItems as f (f.id)}
						<a
							href="/features/{f.id}"
							onclick={() => (mobileOpen = false)}
							class="rounded-lg border border-line p-3"
						>
							<span class="grid h-7 w-7 place-items-center rounded-md bg-blush text-accent">
								<f.icon size={15} />
							</span>
							<div class="mt-1.5 text-sm font-semibold">{f.title}</div>
						</a>
					{/each}
				</div>
				<div class="mt-3 grid gap-0.5 border-t border-line pt-3">
					{#each links as l (l.href)}
						<a
							href={l.href}
							onclick={() => (mobileOpen = false)}
							class="rounded-md px-2 py-2 text-sm font-medium text-ink hover:bg-surface">{l.label}</a
						>
					{/each}
					<a
						href="https://github.com/dia-bot/dia"
						class="rounded-md px-2 py-2 text-sm font-medium text-ink hover:bg-surface">GitHub</a
					>
				</div>
			</div>
		</div>
	{/if}
</header>
