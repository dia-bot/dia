<script lang="ts">
	import SiteNav from '$lib/components/marketing/SiteNav.svelte';
	import SiteFooter from '$lib/components/marketing/SiteFooter.svelte';
	import PageHero from '$lib/components/marketing/PageHero.svelte';
	import Reveal from '$lib/components/marketing/Reveal.svelte';

	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Mail from 'lucide-svelte/icons/mail';
	import Github from 'lucide-svelte/icons/github';
	import MessageCircle from 'lucide-svelte/icons/message-circle';
	import Shield from 'lucide-svelte/icons/shield';
	import MapPin from 'lucide-svelte/icons/map-pin';
	import FileText from 'lucide-svelte/icons/file-text';
	import Lock from 'lucide-svelte/icons/lock';

	let { data }: { data: { user?: { username: string } | null } } = $props();

	const channels = [
		{
			id: 'email',
			icon: Mail,
			label: 'Email',
			title: 'Talk to a human',
			body: 'General questions, partnerships, press, or anything about the hosted service. We read everything that lands here.',
			href: 'mailto:hello@dia.xyz',
			action: 'hello@dia.xyz'
		},
		{
			id: 'issues',
			icon: Github,
			label: 'GitHub Issues',
			title: 'Bugs & feature requests',
			body: 'Found something broken, or have an idea? Open an issue on the repo so it is tracked in the open alongside everything else.',
			href: 'https://github.com/dia-bot/dia/issues',
			action: 'Open an issue',
			external: true
		},
		{
			id: 'discussions',
			icon: MessageCircle,
			label: 'GitHub Discussions',
			title: 'Questions & help',
			body: 'Setup help, self-hosting questions, or just want to compare notes? Start a thread and the community can chime in.',
			href: 'https://github.com/dia-bot/dia/discussions',
			action: 'Start a discussion',
			external: true
		},
		{
			id: 'security',
			icon: Shield,
			label: 'Security',
			title: 'Responsible disclosure',
			body: 'Think you have found a vulnerability? Email us privately so we can investigate and fix it before any public report.',
			href: 'mailto:security@dia.xyz',
			action: 'security@dia.xyz'
		}
	];
</script>

<svelte:head>
	<title>Contact · Dia</title>
	<meta
		name="description"
		content="Get in touch with Dia — email us, open a GitHub issue or discussion, or report a security issue. Dia is an open-source Discord bot operated by Mindroot Ltd."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} />

	<PageHero
		eyebrow="[ contact ]"
		title="Get in touch."
		lede="Dia is built in the open. Whether you have a question, a bug, an idea, or a security report, here is the fastest way to reach us — pick the channel that fits."
	>
		<div class="flex flex-wrap items-center gap-3">
			<a href="mailto:hello@dia.xyz" class="btn btn-accent h-12 px-6 text-base">
				Email us <ArrowRight size={18} />
			</a>
			<a href="https://github.com/dia-bot/dia" class="btn btn-ghost h-12 px-5 text-base">
				<Github size={18} /> View the source
			</a>
		</div>
	</PageHero>

	<!-- ───────────────────────── Contact channels ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-10 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">01</span>
					<span class="eyebrow">Channels</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
			</Reveal>

			<div class="grid gap-5 sm:grid-cols-2">
				{#each channels as c, i (c.id)}
					<Reveal delay={i * 80}>
						<a
							href={c.href}
							target={c.external ? '_blank' : undefined}
							rel={c.external ? 'noreferrer' : undefined}
							class="card group flex h-full flex-col p-7 transition-colors hover:border-line-strong"
						>
							<div class="flex items-center gap-3">
								<span class="grid h-11 w-11 place-items-center rounded-xl bg-blush text-accent">
									<c.icon size={20} />
								</span>
								<span class="font-mono text-xs uppercase tracking-wide text-muted">{c.label}</span>
							</div>
							<h3 class="mt-5 text-lg font-semibold tracking-[-0.01em]">{c.title}</h3>
							<p class="mt-2 text-sm leading-relaxed text-muted">{c.body}</p>
							<span
								class="mt-5 inline-flex items-center gap-1.5 font-mono text-sm text-accent-ink transition-colors group-hover:text-accent"
							>
								{c.action} <ArrowRight size={14} />
							</span>
						</a>
					</Reveal>
				{/each}
			</div>
		</div>
	</section>

	<!-- ───────────────────────── Company ───────────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-10 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">02</span>
					<span class="eyebrow">Company</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
			</Reveal>

			<div class="grid items-start gap-10 lg:grid-cols-12">
				<Reveal class="lg:col-span-7">
					<div class="card p-8">
						<div class="flex items-start gap-4">
							<span class="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-blush text-accent">
								<MapPin size={20} />
							</span>
							<div>
								<h3 class="text-xl font-extrabold tracking-[-0.02em]">Mindroot Ltd</h3>
								<p class="mt-1 font-mono text-xs uppercase tracking-wide text-muted">
									Company no. 16543299
								</p>
								<address class="mt-4 text-sm not-italic leading-relaxed text-ink/80">
									71–75 Shelton Street<br />
									London, WC2H&nbsp;9JQ<br />
									United Kingdom
								</address>
							</div>
						</div>
						<div class="mt-7 border-t border-line pt-6 text-sm leading-relaxed text-muted">
							The hosted Dia service is operated by Mindroot Ltd, registered in England and Wales. Dia
							itself is free and open source under the MIT licence, and the full stack is yours to
							self-host from
							<a
								href="https://github.com/dia-bot/dia"
								class="text-accent-ink underline-offset-2 hover:underline">github.com/dia-bot/dia</a
							>.
						</div>
					</div>
				</Reveal>

				<Reveal delay={120} class="lg:col-span-5">
					<div class="space-y-3">
						<a
							href="/terms"
							class="card group flex items-center gap-4 px-6 py-5 transition-colors hover:border-line-strong"
						>
							<span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-blush text-accent">
								<FileText size={18} />
							</span>
							<div class="min-w-0 flex-1">
								<div class="text-sm font-semibold">Terms of Service</div>
								<div class="text-xs text-muted">The rules for using the hosted service.</div>
							</div>
							<ArrowRight size={16} class="text-faint transition-colors group-hover:text-accent-ink" />
						</a>
						<a
							href="/privacy"
							class="card group flex items-center gap-4 px-6 py-5 transition-colors hover:border-line-strong"
						>
							<span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-blush text-accent">
								<Lock size={18} />
							</span>
							<div class="min-w-0 flex-1">
								<div class="text-sm font-semibold">Privacy Policy</div>
								<div class="text-xs text-muted">What we collect, store, and never sell.</div>
							</div>
							<ArrowRight size={16} class="text-faint transition-colors group-hover:text-accent-ink" />
						</a>
						<div
							class="flex items-center gap-2 px-6 pt-2 font-mono text-xs text-muted"
						>
							<span class="inline-flex items-center gap-1.5">
								<span class="h-1.5 w-1.5 rounded-full bg-success"></span> Open source · MIT
							</span>
						</div>
					</div>
				</Reveal>
			</div>
		</div>
	</section>

	<SiteFooter />
</div>
