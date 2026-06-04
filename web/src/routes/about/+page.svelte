<script lang="ts">
	import SiteNav from '$lib/components/marketing/SiteNav.svelte';
	import SiteFooter from '$lib/components/marketing/SiteFooter.svelte';
	import PageHero from '$lib/components/marketing/PageHero.svelte';
	import CtaSection from '$lib/components/marketing/CtaSection.svelte';
	import Reveal from '$lib/components/marketing/Reveal.svelte';
	import { loginURL } from '$lib/api';

	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Github from 'lucide-svelte/icons/github';
	import Check from 'lucide-svelte/icons/check';
	import Code from 'lucide-svelte/icons/code';
	import Server from 'lucide-svelte/icons/server';
	import LockOpen from 'lucide-svelte/icons/lock-open';
	import Zap from 'lucide-svelte/icons/zap';
	import Building2 from 'lucide-svelte/icons/building-2';

	let { data }: { data: { user?: { username: string } | null } } = $props();
	const cta = $derived(data.user ? '/servers' : loginURL);

	// Four principles — the convictions the project is built on. Each maps to a
	// concrete, honest property of the product (no invented claims).
	const principles = [
		{
			icon: Code,
			title: 'Open source',
			body: 'Every line is MIT-licensed and public on GitHub. Read it, audit it, fork it, send a patch — the bot you run is the code you can see.'
		},
		{
			icon: Server,
			title: 'Self-hostable',
			body: 'The whole stack ships in the repository and comes up with one command. Run it on your own infrastructure and your members’ data never leaves your machines.'
		},
		{
			icon: LockOpen,
			title: 'Fair pricing',
			body: 'The core is free for every server — we never gate the basics. Only the genuinely resource-heavy extras are Pro, at $3.99 per server, and self-hosting unlocks everything.'
		},
		{
			icon: Zap,
			title: 'Realtime & serious',
			body: 'Built on Elixir, Go and SvelteKit around live guild state. Change something in Discord and the dashboard reflects it without a refresh.'
		}
	];

	// The problem statement, as discrete pain points → what Dia replaces them with.
	const sprawl = [
		'A welcome bot for the join card',
		'A leveling bot for XP and rank cards',
		'A reaction-role bot for self-serve roles',
		'A moderation bot with its own case log',
		'An automod bot for spam, invites and links'
	];

	const unified = [
		'Welcome cards with a live, layer-based editor',
		'Leveling, role rewards and themeable rank cards',
		'Button and select role menus — no reactions',
		'Moderation with a numbered, searchable case log',
		'AutoMod for spam, invites, links and banned words'
	];

	const stack = ['Elixir', 'Go', 'SvelteKit', 'PostgreSQL', 'Redis', 'NATS', 'Docker'];
</script>

<svelte:head>
	<title>About · Dia</title>
	<meta
		name="description"
		content="Dia replaces a pile of single-purpose Discord bots with one open-source operational stack: welcome cards, leveling, moderation, roles and custom commands, from one realtime dashboard. MIT licensed, self-hostable, with a free core tier."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} />

	<PageHero
		eyebrow="[ about ]"
		title="One serious bot, instead of five you have to babysit."
		lede="Dia exists to give communities the essentials they actually run every day — welcome, leveling, moderation, roles and commands — as a single open, self-hostable stack with one realtime dashboard. No bot sprawl, no gating the basics."
	>
		<div class="flex flex-wrap items-center gap-3">
			<a href={cta} class="btn btn-accent h-12 px-6 text-base">Get started <ArrowRight size={18} /></a>
			<a href="/#features" class="btn btn-ghost h-12 px-5 text-base">All features</a>
		</div>
		<div class="mt-6 flex flex-wrap items-center gap-x-5 gap-y-2 font-mono text-xs text-muted">
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> MIT licensed</span>
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> Self-hostable</span>
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> Free core</span>
		</div>
	</PageHero>

	<!-- ───────────────────────── 01 · Why Dia exists ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">01</span>
					<span class="eyebrow">Why Dia exists</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						Most servers run five bots to do one job.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							Setting up a community usually means inviting a handful of single-purpose bots — one
							for welcomes, another for levels, more for roles and moderation. Each brings its own
							dashboard, its own login, its own settings to learn, and increasingly its own paywall in
							front of the parts you actually need.
						</p>
						<p class="mt-4 text-lg leading-relaxed text-muted">
							Dia unifies those essentials into one operational stack you configure from a single
							realtime dashboard — or self-host end to end. One bot, one place to look, nothing held
							back behind a tier.
						</p>
					</div>
				</div>
			</Reveal>

			<!-- before / after -->
			<Reveal delay={80}>
				<div class="mt-12 grid gap-5 lg:grid-cols-2">
					<div class="card h-full p-7">
						<div class="mb-4 font-mono text-xs uppercase tracking-wide text-muted">Five bots, five tools</div>
						<ul class="grid gap-2.5">
							{#each sprawl as s (s)}
								<li class="flex items-start gap-2.5 text-sm text-ink/70">
									<span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-line-strong"></span>{s}
								</li>
							{/each}
						</ul>
						<p class="mt-5 border-t border-line pt-4 text-sm leading-relaxed text-muted">
							Five dashboards to keep in sync, five sets of permissions to grant, and feature gates
							scattered across all of them.
						</p>
					</div>

					<div class="card h-full border-line-strong p-7">
						<div class="mb-4 flex items-center gap-2">
							<span class="font-mono text-xs uppercase tracking-wide text-accent-ink">One bot, one stack</span>
							<span class="rounded-full bg-blush px-2 py-0.5 font-mono text-[11px] font-semibold text-accent-ink">Dia</span>
						</div>
						<ul class="grid gap-2.5">
							{#each unified as u (u)}
								<li class="flex items-start gap-2 text-sm text-ink/80">
									<Check size={16} class="mt-0.5 shrink-0 text-accent" />{u}
								</li>
							{/each}
						</ul>
						<p class="mt-5 border-t border-line pt-4 text-sm leading-relaxed text-muted">
							One dashboard secured by Discord login, one set of permissions, and every feature
							available from the start.
						</p>
					</div>
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── 02 · Principles ───────────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">02</span>
					<span class="eyebrow">Principles</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						The convictions it’s built on.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-6">
						These aren’t marketing lines — they’re the constraints the project holds itself to. The bot
						is open source and self-hostable, so you can read it, run it, and own it.
					</p>
				</div>
			</Reveal>

			<div class="mt-12 grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-2">
				{#each principles as p, i (p.title)}
					<Reveal delay={(i % 2) * 90} class="bg-surface">
						<div class="h-full p-7">
							<div class="flex items-center gap-3">
								<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent">
									<p.icon size={19} />
								</span>
								<h3 class="text-lg font-semibold">{p.title}</h3>
							</div>
							<p class="mt-4 text-sm leading-relaxed text-muted">{p.body}</p>
						</div>
					</Reveal>
				{/each}
			</div>

			<Reveal delay={120}>
				<div class="mt-5 flex flex-wrap gap-2">
					{#each stack as t (t)}
						<span class="rounded-md border border-line-strong bg-bg px-2.5 py-1 font-mono text-xs text-muted">{t}</span>
					{/each}
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── 03 · The company ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">03</span>
					<span class="eyebrow">The company</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
			</Reveal>

			<div class="mt-4 grid items-start gap-10 lg:grid-cols-12">
				<Reveal class="lg:col-span-7">
					<h2 class="text-3xl font-extrabold tracking-tight sm:text-4xl">
						Open project, accountable operator.
					</h2>
					<p class="mt-5 text-lg leading-relaxed text-muted">
						Dia is an open-source project anyone can run. The hosted service — the bot and dashboard
						at <span class="font-mono text-sm text-ink">dia.xyz</span> — is operated by
						<strong class="font-semibold text-ink">Mindroot&nbsp;Ltd</strong>, a company registered in
						England&nbsp;&amp;&nbsp;Wales. The code that runs the hosted instance is the same code in
						the public repository.
					</p>
					<p class="mt-4 text-lg leading-relaxed text-muted">
						Questions, security reports or anything else — reach us at
						<a href="mailto:hello@dia.xyz" class="font-medium text-accent-ink hover:text-accent">hello@dia.xyz</a>.
					</p>
					<div class="mt-7 flex flex-wrap gap-3">
						<a href="https://github.com/dia-bot/dia" class="btn btn-primary h-11 px-5">
							<Github size={16} /> View the source
						</a>
						<a href="https://github.com/dia-bot/dia/tree/main/deploy" class="btn btn-ghost h-11 px-5">
							Self-hosting docs <ArrowRight size={16} />
						</a>
					</div>
				</Reveal>

				<Reveal delay={120} class="lg:col-span-5">
					<div class="card p-7">
						<div class="flex items-center gap-3">
							<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent">
								<Building2 size={19} />
							</span>
							<h3 class="text-[15px] font-semibold">Registered entity</h3>
						</div>
						<dl class="mt-6 divide-y divide-line text-sm">
							<div class="flex items-baseline justify-between gap-4 py-3">
								<dt class="font-mono text-xs uppercase tracking-wide text-muted">Operator</dt>
								<dd class="text-right font-medium text-ink">Mindroot Ltd</dd>
							</div>
							<div class="flex items-baseline justify-between gap-4 py-3">
								<dt class="font-mono text-xs uppercase tracking-wide text-muted">Company no.</dt>
								<dd class="text-right font-mono text-ink">16543299</dd>
							</div>
							<div class="flex items-baseline justify-between gap-4 py-3">
								<dt class="font-mono text-xs uppercase tracking-wide text-muted">Jurisdiction</dt>
								<dd class="text-right font-medium text-ink">England &amp; Wales</dd>
							</div>
							<div class="flex items-baseline justify-between gap-4 py-3">
								<dt class="shrink-0 font-mono text-xs uppercase tracking-wide text-muted">Address</dt>
								<dd class="text-right text-ink/80">71–75 Shelton Street,<br />London WC2H 9JQ,<br />United Kingdom</dd>
							</div>
							<div class="flex items-baseline justify-between gap-4 py-3">
								<dt class="font-mono text-xs uppercase tracking-wide text-muted">Contact</dt>
								<dd class="text-right">
									<a href="mailto:hello@dia.xyz" class="font-medium text-accent-ink hover:text-accent">hello@dia.xyz</a>
								</dd>
							</div>
						</dl>
					</div>
				</Reveal>
			</div>
		</div>
	</section>

	<CtaSection href={cta} heading="Built in the open. Run it your way." sub="Add the hosted bot, or self-host the whole stack. Both free." />

	<SiteFooter />
</div>
