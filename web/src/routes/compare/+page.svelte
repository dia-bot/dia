<script lang="ts">
	import SiteNav from '$lib/components/marketing/SiteNav.svelte';
	import SiteFooter from '$lib/components/marketing/SiteFooter.svelte';
	import PageHero from '$lib/components/marketing/PageHero.svelte';
	import CtaSection from '$lib/components/marketing/CtaSection.svelte';
	import Reveal from '$lib/components/marketing/Reveal.svelte';
	import CountUp from '$lib/components/marketing/CountUp.svelte';
	import { loginURL } from '$lib/api';

	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Check from 'lucide-svelte/icons/check';
	import X from 'lucide-svelte/icons/x';
	import Shield from 'lucide-svelte/icons/shield';
	import SlidersHorizontal from 'lucide-svelte/icons/sliders-horizontal';
	import KeyRound from 'lucide-svelte/icons/key-round';
	import Database from 'lucide-svelte/icons/database';
	import Github from 'lucide-svelte/icons/github';

	let { data }: { data: { user?: { username: string } | null } } = $props();
	const cta = $derived(data.user ? '/servers' : loginURL);

	// Comparison matrix. `dia` is always a clean yes; `usual` is `false` (a hard no)
	// or `'partial'` (technically possible, but it costs you elsewhere).
	type Cell = true | false | 'partial';
	const rows: { label: string; note: string; dia: Cell; usual: Cell }[] = [
		{
			label: 'Welcome, leveling, moderation, roles & commands in one bot',
			note: 'Every core feature ships in the same install.',
			dia: true,
			usual: false
		},
		{
			label: 'One dashboard to configure everything',
			note: 'A single Discord login, one place for every setting.',
			dia: true,
			usual: false
		},
		{
			label: 'Realtime sync — changes apply instantly',
			note: 'Create a channel or role and it appears with no refresh.',
			dia: true,
			usual: 'partial'
		},
		{
			label: 'Open source (MIT)',
			note: 'Read it, fork it, audit it on GitHub.',
			dia: true,
			usual: 'partial'
		},
		{
			label: 'Self-hostable end to end',
			note: 'Run the whole stack on your own infrastructure.',
			dia: true,
			usual: 'partial'
		},
		{
			label: 'Core features free',
			note: 'Every essential is free; only resource-heavy extras are Pro.',
			dia: true,
			usual: false
		},
		{
			label: 'You own your data',
			note: 'Your configuration lives in one place you control.',
			dia: true,
			usual: false
		},
		{
			label: 'One set of bot permissions to manage',
			note: 'Grant access once, not five separate times.',
			dia: true,
			usual: false
		}
	];

	const costs = [
		{
			icon: Shield,
			n: '01',
			title: 'More permissions to grant',
			body: 'Every extra bot is another OAuth grant and another role with access to your server. Each one widens the blast radius if something is compromised or abandoned.'
		},
		{
			icon: SlidersHorizontal,
			n: '02',
			title: 'More dashboards to learn',
			body: 'Five bots means five logins, five settings layouts and five places to remember where a toggle lives. Routine changes turn into a scavenger hunt.'
		},
		{
			icon: KeyRound,
			n: '03',
			title: 'Premium upsells everywhere',
			body: 'The feature you actually need is often gated behind a per-bot subscription. The bill grows with each tool, and so does the pressure to upgrade.'
		},
		{
			icon: Database,
			n: '04',
			title: 'Configuration drift',
			body: 'When settings are scattered, they fall out of sync. One bot logs to a channel another can not see, exemptions disagree, and nobody owns the whole picture.'
		}
	];

	const consolidation = [
		'One OAuth grant and one role to audit',
		'A single dashboard with consistent settings',
		'A free core — only heavy extras are Pro',
		'One source of truth, so nothing drifts',
		'Realtime state shared across every feature',
		'Self-host the entire stack, or let us run it'
	];
</script>

<svelte:head>
	<title>Why Dia — one bot, not five · Dia</title>
	<meta
		name="description"
		content="Replace a stack of single-purpose Discord bots with one open-source bot. Welcome, leveling, moderation, roles and custom commands — one dashboard, one set of permissions, a free core tier, and your data stays yours."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} />

	<PageHero
		eyebrow="[ compare ]"
		title="One bot, not five."
		lede="Most servers bolt together a stack of single-purpose bots — one for welcomes, one for leveling, one for moderation, and on it goes. Dia is all of it in a single open-source bot, configured from one realtime dashboard."
	>
		<div class="flex flex-wrap items-center gap-3">
			<a href={cta} class="btn btn-accent h-12 px-6 text-base">
				Get started <ArrowRight size={18} />
			</a>
			<a href="/#features" class="btn btn-ghost h-12 px-5 text-base">All features</a>
		</div>
		<div class="mt-6 flex flex-wrap items-center gap-x-5 gap-y-2 font-mono text-xs text-muted">
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> One install</span>
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> One dashboard</span>
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> One set of permissions</span>
		</div>
	</PageHero>

	<!-- ───────────────────────── Stats ───────────────────────── -->
	<section class="border-b border-line bg-surface">
		<div class="mx-auto grid max-w-page grid-cols-1 gap-px overflow-hidden px-6 py-0 sm:grid-cols-3">
			<Reveal class="px-2 py-12 text-center">
				<div class="text-4xl font-extrabold tracking-tight sm:text-5xl"><CountUp to={1} /></div>
				<div class="mt-1.5 font-mono text-xs uppercase tracking-wide text-muted">bot · not a stack</div>
			</Reveal>
			<Reveal delay={80} class="border-y border-line px-2 py-12 text-center sm:border-x sm:border-y-0">
				<div class="text-4xl font-extrabold tracking-tight sm:text-5xl"><CountUp to={6} /></div>
				<div class="mt-1.5 font-mono text-xs uppercase tracking-wide text-muted">features · included</div>
			</Reveal>
			<Reveal delay={160} class="px-2 py-12 text-center">
				<div class="text-4xl font-extrabold tracking-tight sm:text-5xl"><CountUp to={3.99} decimals={2} prefix="$" /></div>
				<div class="mt-1.5 font-mono text-xs uppercase tracking-wide text-muted">/ server · Pro</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── Comparison matrix ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">01</span>
					<span class="eyebrow">Side by side</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						The same job, with a lot less to manage.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-6">
						Here is what you get from Dia versus the usual approach of stitching together a stack of
						single-purpose bots. Everything below is part of one install — nothing is reserved for a
						paid tier.
					</p>
				</div>
			</Reveal>

			<Reveal delay={80}>
				<div class="card mt-12 overflow-hidden p-0">
					<!-- Header row -->
					<div class="grid grid-cols-[1fr_auto_auto] items-stretch border-b border-line bg-surface">
						<div class="px-5 py-4 sm:px-6">
							<span class="font-mono text-xs uppercase tracking-wide text-muted">Capability</span>
						</div>
						<div class="flex w-24 items-center justify-center border-l border-line bg-blush px-3 py-4 sm:w-36">
							<span class="font-mono text-xs font-semibold uppercase tracking-wide text-accent-ink">Dia</span>
						</div>
						<div class="flex w-24 items-center justify-center border-l border-line px-3 py-4 text-center sm:w-44">
							<span class="font-mono text-[11px] uppercase leading-tight tracking-wide text-faint">
								A stack of<br />single-purpose bots
							</span>
						</div>
					</div>

					<!-- Body rows -->
					<div class="divide-y divide-line">
						{#each rows as r (r.label)}
							<div class="grid grid-cols-[1fr_auto_auto] items-stretch">
								<div class="px-5 py-4 sm:px-6">
									<div class="text-[15px] font-medium text-ink">{r.label}</div>
									<div class="mt-0.5 text-[13px] leading-snug text-muted">{r.note}</div>
								</div>
								<div class="flex w-24 items-center justify-center border-l border-line bg-blush/60 sm:w-36">
									{#if r.dia === true}
										<span class="grid h-7 w-7 place-items-center rounded-full bg-accent text-white">
											<Check size={16} />
										</span>
									{:else if r.dia === 'partial'}
										<span class="font-mono text-xs text-muted">partial</span>
									{:else}
										<span class="grid h-7 w-7 place-items-center rounded-full border border-line-strong text-faint">
											<X size={16} />
										</span>
									{/if}
								</div>
								<div class="flex w-24 items-center justify-center border-l border-line sm:w-44">
									{#if r.usual === true}
										<span class="grid h-7 w-7 place-items-center rounded-full bg-accent text-white">
											<Check size={16} />
										</span>
									{:else if r.usual === 'partial'}
										<span class="font-mono text-xs text-muted">varies</span>
									{:else}
										<span class="grid h-7 w-7 place-items-center rounded-full border border-line-strong text-faint">
											<X size={16} />
										</span>
									{/if}
								</div>
							</div>
						{/each}
					</div>
				</div>
				<p class="mt-4 font-mono text-xs text-faint">
					&ldquo;Varies&rdquo; / &ldquo;partial&rdquo; — some bots in such a stack offer this, but rarely
					all of them, and rarely without a paid tier.
				</p>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── The cost of bot sprawl ───────────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">02</span>
					<span class="eyebrow">The cost of sprawl</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						Every extra bot is a cost you pay later.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-6">
						A multi-bot setup looks free at first. The real price shows up in permissions you have to
						trust, dashboards you have to remember, subscriptions that creep in, and settings that
						quietly fall out of sync.
					</p>
				</div>
			</Reveal>

			<div class="mt-12 grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-2">
				{#each costs as c, i (c.n)}
					<Reveal delay={i * 80} class="bg-surface">
						<div class="h-full p-7">
							<div class="flex items-center gap-3">
								<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent">
									<c.icon size={19} />
								</span>
								<span class="font-mono text-xs text-faint">{c.n}</span>
							</div>
							<h3 class="mt-4 text-lg font-semibold">{c.title}</h3>
							<p class="mt-1.5 text-sm leading-relaxed text-muted">{c.body}</p>
						</div>
					</Reveal>
				{/each}
			</div>
		</div>
	</section>

	<!-- ───────────────────────── How consolidation helps ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">03</span>
					<span class="eyebrow">Consolidate</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
			</Reveal>

			<div class="grid items-stretch gap-6 lg:grid-cols-2">
				<Reveal>
					<div class="relative h-full overflow-hidden rounded-2xl border border-line bg-[#111113] p-8 sm:p-10">
						<div class="grid-lines pointer-events-none absolute inset-0 opacity-[0.07]" aria-hidden="true"></div>
						<div class="relative">
							<span class="font-mono text-xs uppercase tracking-wider text-accent-ink">One bot</span>
							<h3 class="mt-3 text-2xl font-extrabold leading-tight tracking-tight text-white sm:text-3xl">
								Collapse the stack into a single source of truth.
							</h3>
							<p class="mt-4 text-base leading-relaxed text-white/65">
								When welcome, leveling, moderation, roles and commands all live in the same bot, they
								share the same realtime state and the same dashboard. There is one thing to grant
								access to, one place to change, and nothing left to drift apart.
							</p>
						</div>
					</div>
				</Reveal>

				<Reveal delay={120}>
					<div class="card h-full p-7 sm:p-8">
						<div class="mb-4 font-mono text-xs uppercase tracking-wide text-muted">What you get back</div>
						<ul class="grid gap-3">
							{#each consolidation as item (item)}
								<li class="flex items-start gap-2.5 text-[15px] text-ink/85">
									<Check size={18} class="mt-0.5 shrink-0 text-accent" />{item}
								</li>
							{/each}
						</ul>
						<div class="mt-7 border-t border-line pt-6">
							<a href="/#features" class="inline-flex items-center gap-1.5 font-mono text-sm text-accent-ink hover:text-accent">
								See every feature <ArrowRight size={14} />
							</a>
						</div>
					</div>
				</Reveal>
			</div>

			<Reveal delay={80}>
				<p class="mx-auto mt-12 max-w-2xl text-center text-base leading-relaxed text-muted">
					Dia is free and open source under the MIT license. Run it yourself from the
					<a href="https://github.com/dia-bot/dia" class="text-accent-ink underline-offset-2 hover:underline">
						source on GitHub</a
					>, or let us host it. Either way, it is one bot doing the work of five.
				</p>
				<div class="mt-6 flex flex-wrap justify-center gap-3">
					<a href={cta} class="btn btn-accent h-11 px-5">Get started <ArrowRight size={16} /></a>
					<a href="https://github.com/dia-bot/dia" class="btn btn-ghost h-11 px-5">
						<Github size={16} /> View the source
					</a>
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── Final CTA ───────────────────────── -->
	<CtaSection href={cta} heading="Replace the bot sprawl." />

	<SiteFooter />
</div>
