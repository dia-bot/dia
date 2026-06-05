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
	import Server from 'lucide-svelte/icons/server';
	import Sparkles from 'lucide-svelte/icons/sparkles';
	import Zap from 'lucide-svelte/icons/zap';

	let { data }: { data: { user?: { username: string } | null } } = $props();
	const cta = $derived(data.user ? '/servers' : loginURL);

	const freePoints = [
		'All six features, on every server',
		'Preset welcome & rank cards',
		'Realtime dashboard with Discord login',
		'Standard XP, AutoMod & command limits',
		'Automatic updates'
	];
	const proPoints = [
		'Everything in Free',
		'Full element-based card editor — welcome & rank',
		'Higher limits: more commands, menus & history',
		'Priority image rendering',
		'Support that keeps the lights on'
	];
	const selfPoints = [
		'The complete stack, one command up',
		'Every feature unlocked, no limits',
		'MIT licensed — fork and modify freely',
		'Your data stays on your machines',
		'No license keys, no phone-home'
	];

	const faqs = [
		{
			q: 'Is the core really free?',
			a: 'Yes. The six features and the realtime dashboard are free for every server, with no time limit and no credit card. Pro is optional — you only reach for it if you want the resource-heavy extras.'
		},
		{
			q: 'What actually costs money?',
			a: 'Only the features that cost us real compute — chiefly the advanced, element-based card editor and higher usage limits. We are still finalising the exact split, and will keep the everyday essentials in the free tier. The price stays $3.99 per server, per month.'
		},
		{
			q: 'Can I self-host everything for free?',
			a: 'Always. The whole stack — Elixir gateway, Go API and worker, Postgres, Redis and NATS — is MIT-licensed and in the repository. Self-hosting unlocks every feature with no limits; you just run it on your own infrastructure.'
		},
		{
			q: 'Do you store my data?',
			a: 'The hosted service stores only what it needs to run the features you turn on — your server configuration and the member data required for things like XP and the moderation case log. We don’t sell data or use it for advertising.'
		}
	];
</script>

<svelte:head>
	<title>Pricing · Dia</title>
	<meta
		name="description"
		content="Dia's core is free for every server. Pro is $3.99/mo per server for the resource-heavy extras like the advanced card editor and higher limits — or self-host the MIT-licensed stack for free."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} />

	<PageHero
		eyebrow="[ pricing ]"
		title="Start free. Pay only for the heavy lifting."
		lede="The core bot is free for every server. Pro unlocks the features that cost real compute — $3.99 a month, per server. Or self-host the whole stack for nothing."
	>
		<div class="flex flex-wrap items-center gap-3">
			<a href={cta} class="btn btn-accent h-12 px-6 text-base">Get started <ArrowRight size={18} /></a>
			<a href="/features" class="btn btn-ghost h-12 px-5 text-base">See features</a>
		</div>
		<div class="mt-6 flex flex-wrap items-center gap-x-5 gap-y-2 font-mono text-xs text-muted">
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> Free core</span>
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> $3.99 / server Pro</span>
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> Free to self-host</span>
		</div>
	</PageHero>

	<!-- ───────────────────────── Plans ───────────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">01</span>
					<span class="eyebrow">Plans</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<p class="max-w-2xl text-lg leading-relaxed text-muted">
					Three ways to run Dia. Start free, add Pro on the servers that want the extras, or host the
					whole thing yourself.
				</p>
			</Reveal>

			<div class="mt-10 grid gap-5 lg:grid-cols-3">
				<!-- Free -->
				<Reveal class="h-full">
					<div class="card flex h-full flex-col p-7">
						<div class="flex items-center gap-3">
							<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent"><Sparkles size={19} /></span>
							<h3 class="text-lg font-semibold">Free</h3>
						</div>
						<div class="mt-6 flex items-baseline gap-2">
							<span class="text-4xl font-extrabold tracking-tight sm:text-5xl">$0</span>
							<span class="font-mono text-xs text-muted">/ forever</span>
						</div>
						<p class="mt-3 text-sm leading-relaxed text-muted">
							The whole bot for everyday communities. Add it, configure it, done.
						</p>
						<ul class="mt-6 grid flex-1 gap-2.5">
							{#each freePoints as p (p)}
								<li class="flex items-start gap-2 text-sm text-ink/80"><Check size={16} class="mt-0.5 shrink-0 text-accent" />{p}</li>
							{/each}
						</ul>
						<div class="mt-7 pt-1"><a href={cta} class="btn btn-ghost h-11 w-full px-5">Add Dia</a></div>
					</div>
				</Reveal>

				<!-- Pro (highlighted) -->
				<Reveal delay={100} class="h-full">
					<div class="card relative flex h-full flex-col p-7 ring-1 ring-accent">
						<span class="absolute -top-3 left-7 rounded-full bg-accent px-2.5 py-0.5 font-mono text-[11px] font-semibold text-white">
							Most popular
						</span>
						<div class="flex items-center gap-3">
							<span class="grid h-10 w-10 place-items-center rounded-lg bg-accent text-white"><Zap size={19} /></span>
							<h3 class="text-lg font-semibold">Pro</h3>
						</div>
						<div class="mt-6 flex items-baseline gap-1.5">
							<span class="text-4xl font-extrabold tracking-tight sm:text-5xl">$3.99</span>
							<span class="font-mono text-xs text-muted">/ mo · per server</span>
						</div>
						<p class="mt-3 text-sm leading-relaxed text-muted">
							For servers that want the resource-heavy extras. Per server, cancel anytime.
						</p>
						<ul class="mt-6 grid flex-1 gap-2.5">
							{#each proPoints as p (p)}
								<li class="flex items-start gap-2 text-sm text-ink/80"><Check size={16} class="mt-0.5 shrink-0 text-accent" />{p}</li>
							{/each}
						</ul>
						<div class="mt-7 pt-1"><a href={cta} class="btn btn-accent h-11 w-full px-5">Go Pro <ArrowRight size={16} /></a></div>
					</div>
				</Reveal>

				<!-- Self-hosted -->
				<Reveal delay={200} class="h-full">
					<div class="card flex h-full flex-col p-7">
						<div class="flex items-center gap-3">
							<span class="grid h-10 w-10 place-items-center rounded-lg border border-line-strong bg-bg text-ink"><Server size={19} /></span>
							<h3 class="text-lg font-semibold">Self-hosted</h3>
							<span class="ml-auto rounded-full border border-line-strong bg-bg px-2.5 py-0.5 font-mono text-[11px] font-semibold text-muted">MIT</span>
						</div>
						<div class="mt-6 flex items-baseline gap-2">
							<span class="text-4xl font-extrabold tracking-tight sm:text-5xl">Free</span>
							<span class="font-mono text-xs text-muted">/ open source</span>
						</div>
						<p class="mt-3 text-sm leading-relaxed text-muted">
							Run the entire stack yourself. Every feature unlocked, your infrastructure.
						</p>
						<ul class="mt-6 grid flex-1 gap-2.5">
							{#each selfPoints as p (p)}
								<li class="flex items-start gap-2 text-sm text-ink/80"><Check size={16} class="mt-0.5 shrink-0 text-accent" />{p}</li>
							{/each}
						</ul>
						<div class="mt-7 pt-1">
							<a href="https://github.com/dia-bot/dia/tree/main/deploy" class="btn btn-primary h-11 w-full px-5"><Github size={16} /> Self-host it</a>
						</div>
					</div>
				</Reveal>
			</div>

			<!-- Provisional note -->
			<Reveal delay={120}>
				<div class="mt-5 flex items-start gap-3 rounded-xl border border-line bg-bg p-4 text-sm text-muted">
					<span class="mt-0.5 shrink-0 font-mono text-xs text-accent-ink">note</span>
					<p>
						Pro exists to cover the features that genuinely cost compute — we're still finalising
						exactly which ones land there, and the everyday essentials stay free. Self-hosting always
						unlocks everything.
					</p>
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── FAQ ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">02</span>
					<span class="eyebrow">FAQ</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<h2 class="text-3xl font-extrabold tracking-tight sm:text-4xl">Questions, answered.</h2>
			</Reveal>

			<div class="mt-10 grid gap-5 lg:grid-cols-2">
				{#each faqs as f, i (f.q)}
					<Reveal delay={(i % 2) * 90}>
						<div class="card h-full p-6">
							<h3 class="text-base font-semibold">{f.q}</h3>
							<p class="mt-2 text-sm leading-relaxed text-muted">
								{f.a}{#if f.q === 'Do you store my data?'}
									{' '}<a href="/privacy" class="font-medium text-accent-ink hover:text-accent">Read the privacy policy</a>.
								{/if}
							</p>
						</div>
					</Reveal>
				{/each}
			</div>
		</div>
	</section>

	<CtaSection href={cta} heading="Start free. Go Pro when you need it." sub="Every essential is free. Pro is $3.99 a month per server — or self-host it all for nothing." />

	<SiteFooter />
</div>
