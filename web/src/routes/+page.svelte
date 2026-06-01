<script lang="ts">
	import Logo from '$lib/components/Logo.svelte';
	import { loginURL } from '$lib/api';
	import {
		ImageIcon,
		TrendingUp,
		ShieldCheck,
		Wand2,
		ToggleRight,
		Zap,
		ArrowRight,
		Github
	} from 'lucide-svelte';

	let { data }: { data: { user?: { username: string } | null } } = $props();

	const features = [
		{
			icon: ImageIcon,
			title: 'Welcome images',
			body: 'Greet members with custom-rendered welcome cards. Pick a preset or design your own — live preview as you edit.'
		},
		{
			icon: TrendingUp,
			title: 'Leveling & rank cards',
			body: 'Reward activity with XP, levels and role rewards, plus gorgeous generated rank cards and leaderboards.'
		},
		{
			icon: ToggleRight,
			title: 'Reaction & auto roles',
			body: 'Self-assignable roles with buttons and menus, and automatic roles on join — fully slash-native.'
		},
		{
			icon: ShieldCheck,
			title: 'Moderation & automod',
			body: 'Ban, kick, timeout and warn with a clean case log, plus rule-based automod for spam, links and words.'
		},
		{
			icon: Wand2,
			title: 'Custom commands',
			body: 'Design your own slash commands from the dashboard, with rich responses — no code required.'
		},
		{
			icon: Zap,
			title: 'Realtime dashboard',
			body: 'Create a channel in Discord and it appears in your dashboard instantly. Everything stays in sync, live.'
		}
	];
</script>

<svelte:head>
	<title>Dia — a beautifully simple Discord bot</title>
	<meta
		name="description"
		content="Dia is a modern, open-source Discord bot: welcome images, leveling, moderation, reaction roles and custom commands, all configured from a clean realtime dashboard."
	/>
</svelte:head>

<div class="min-h-screen">
	<!-- Nav -->
	<header class="mx-auto flex max-w-6xl items-center justify-between px-6 py-5">
		<a href="/" class="flex items-center"><Logo size={30} wordmark /></a>
		<nav class="flex items-center gap-6 text-sm font-medium text-muted">
			<a href="#features" class="hidden hover:text-ink sm:block">Features</a>
			<a
				href="https://github.com/dia-bot/dia"
				class="hidden items-center gap-1.5 hover:text-ink sm:flex"
			>
				<Github size={16} /> GitHub
			</a>
			{#if data.user}
				<a href="/servers" class="btn btn-primary h-9">Open dashboard</a>
			{:else}
				<a href={loginURL} class="btn btn-primary h-9">Log in</a>
			{/if}
		</nav>
	</header>

	<!-- Hero -->
	<section class="mx-auto max-w-4xl px-6 pb-10 pt-16 text-center sm:pt-24">
		<span class="eyebrow">Open-source Discord bot</span>
		<h1 class="mx-auto mt-4 max-w-3xl text-4xl font-extrabold leading-[1.05] tracking-tight sm:text-6xl">
			Everything your community needs,<br class="hidden sm:block" />
			beautifully run by <span class="brand-text">Dia</span>.
		</h1>
		<p class="mx-auto mt-6 max-w-xl text-lg leading-relaxed text-muted">
			Welcome images, leveling, moderation, roles and custom commands — all configured from one
			clean, realtime dashboard. Self-hostable, slash-native, yours.
		</p>
		<div class="mt-9 flex items-center justify-center gap-3">
			<a href={data.user ? '/servers' : loginURL} class="btn btn-accent h-11 px-6 text-base">
				Get started <ArrowRight size={18} />
			</a>
			<a
				href="https://github.com/dia-bot/dia"
				class="btn btn-ghost h-11 px-6 text-base"
			>
				<Github size={18} /> Star on GitHub
			</a>
		</div>

		<!-- Brand showcase tile -->
		<div class="mx-auto mt-16 max-w-2xl">
			<div class="card overflow-hidden p-2 shadow-sm">
				<div
					class="brand-gradient flex aspect-[1024/360] items-center justify-center rounded-[10px]"
				>
					<div class="text-center text-white">
						<div class="mx-auto mb-3 flex justify-center">
							<div class="grid h-20 w-20 place-items-center rounded-full bg-white/20 ring-4 ring-white/40">
								<Logo size={44} />
							</div>
						</div>
						<div class="text-2xl font-extrabold">Welcome, Ada!</div>
						<div class="text-sm text-white/85">You're our 1,024th member 🎉</div>
					</div>
				</div>
			</div>
			<p class="mt-3 text-center text-xs text-faint">A welcome card, rendered by Dia.</p>
		</div>
	</section>

	<!-- Features -->
	<section id="features" class="mx-auto max-w-6xl px-6 py-16">
		<div class="mb-10 text-center">
			<span class="eyebrow">Features</span>
			<h2 class="mt-3 text-3xl font-bold tracking-tight">One bot, every essential</h2>
		</div>
		<div class="grid gap-px overflow-hidden rounded-[14px] border border-line bg-line sm:grid-cols-2 lg:grid-cols-3">
			{#each features as f (f.title)}
				<div class="bg-surface p-6">
					<div class="mb-4 grid h-10 w-10 place-items-center rounded-xl bg-blush text-accent">
						<f.icon size={20} />
					</div>
					<h3 class="text-base font-semibold">{f.title}</h3>
					<p class="mt-2 text-sm leading-relaxed text-muted">{f.body}</p>
				</div>
			{/each}
		</div>
	</section>

	<!-- Self-host -->
	<section class="mx-auto max-w-6xl px-6 pb-24">
		<div class="card flex flex-col items-center gap-4 px-8 py-12 text-center">
			<h2 class="text-2xl font-bold tracking-tight">Self-host in minutes</h2>
			<p class="max-w-xl text-muted">
				Dia is open source and easily hostable — an Elixir gateway, a Go API and worker, Postgres,
				Redis and NATS. Scale shards across machines with a single config flag.
			</p>
			<div class="mt-2 flex gap-3">
				<a href="https://github.com/dia-bot/dia" class="btn btn-primary h-10">
					<Github size={16} /> View the source
				</a>
				<a href={data.user ? '/servers' : loginURL} class="btn btn-ghost h-10">Open dashboard</a>
			</div>
		</div>
	</section>

	<footer class="border-t border-line py-8">
		<div class="mx-auto flex max-w-6xl items-center justify-between px-6 text-sm text-muted">
			<div class="flex items-center gap-2"><Logo size={20} /> <span>Dia</span></div>
			<span>Open source · MIT</span>
		</div>
	</footer>
</div>
