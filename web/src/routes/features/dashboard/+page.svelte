<script lang="ts">
	import SiteNav from '$lib/components/marketing/SiteNav.svelte';
	import SiteFooter from '$lib/components/marketing/SiteFooter.svelte';
	import PageHero from '$lib/components/marketing/PageHero.svelte';
	import CtaSection from '$lib/components/marketing/CtaSection.svelte';
	import Reveal from '$lib/components/marketing/Reveal.svelte';
	import RealtimeSyncDemo from '$lib/components/marketing/RealtimeSyncDemo.svelte';
	import WelcomeCard from '$lib/components/marketing/WelcomeCard.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { loginURL } from '$lib/api';

	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Check from 'lucide-svelte/icons/check';
	import Zap from 'lucide-svelte/icons/zap';
	import Lock from 'lucide-svelte/icons/lock';
	import Shield from 'lucide-svelte/icons/shield';
	import KeyRound from 'lucide-svelte/icons/key-round';
	import Fingerprint from 'lucide-svelte/icons/fingerprint';
	import RefreshCw from 'lucide-svelte/icons/refresh-cw';
	import Smartphone from 'lucide-svelte/icons/smartphone';
	import Eye from 'lucide-svelte/icons/eye';

	let { data }: { data: { user?: { username: string } | null } } = $props();
	const cta = $derived(data.user ? '/servers' : loginURL);

	// What the realtime socket actually carries — kept honest to internal/api/realtime.go.
	const events = [
		{ type: 'channel.upsert', note: 'channel created or renamed' },
		{ type: 'channel.delete', note: 'channel removed' },
		{ type: 'role.upsert', note: 'role created or edited' },
		{ type: 'role.delete', note: 'role removed' },
		{ type: 'guild.update', note: 'name, icon or member count' },
		{ type: 'member.count', note: 'someone joined or left' }
	];

	const security = [
		{
			icon: Fingerprint,
			title: 'Sign in with Discord',
			body: 'No new password to forget. You authorise once through Discord OAuth and land straight on your servers.'
		},
		{
			icon: Shield,
			title: 'Manage Server, or nothing',
			body: 'The dashboard only ever shows servers where you hold Administrator or Manage Server. Everyone else gets a 403.'
		},
		{
			icon: KeyRound,
			title: 'Server-side sessions',
			body: 'Logins are tracked in an http-only, same-site session cookie — never in client storage a script can read.'
		},
		{
			icon: Lock,
			title: 'CSRF-protected writes',
			body: 'Every change carries a double-submit token, so a setting can only be flipped from your own authenticated tab.'
		}
	];

	const work = [
		{
			icon: Eye,
			title: 'Instant previews',
			body: 'Card edits, embed colours and rank themes render as you type — what you see is exactly what posts to the channel.'
		},
		{
			icon: Zap,
			title: 'Per-feature toggles',
			body: 'Each feature is a switch. Turn welcome cards, leveling, AutoMod or custom commands on and off without touching the others.'
		},
		{
			icon: Smartphone,
			title: 'Every device',
			body: 'One responsive surface — tune your server from a laptop at your desk or your phone on the bus, same dashboard either way.'
		}
	];

	// Small live demo for the "built for the work" section.
	let toggles = $state([
		{ name: 'Welcome', desc: 'Greet new members', on: true },
		{ name: 'Leveling', desc: 'XP, ranks & rewards', on: true },
		{ name: 'Reaction roles', desc: 'Self-serve role menus', on: true },
		{ name: 'AutoMod', desc: 'Filter spam & links', on: false },
		{ name: 'Custom commands', desc: 'Your own /commands', on: false }
	]);
</script>

<svelte:head>
	<title>Realtime dashboard · Dia</title>
	<meta
		name="description"
		content="Dia's dashboard is a clean, realtime control surface for your Discord server — secured by Discord OAuth, gated to Manage Server, and synced live over websockets so new channels and roles appear with no refresh."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} />

	<PageHero
		eyebrow="[ feature · dashboard ]"
		title="Configure once. Watch it sync live."
		lede="Every setting lives in one clean dashboard secured by your Discord login. Create a channel or a role in Discord and it shows up here instantly — no save button, no refresh, no waiting."
	>
		<div class="flex flex-wrap items-center gap-3">
			<a href={cta} class="btn btn-accent h-12 px-6 text-base">Get started <ArrowRight size={18} /></a>
			<a href="/#features" class="btn btn-ghost h-12 px-5 text-base">All features</a>
		</div>
		<div class="mt-6 flex flex-wrap items-center gap-x-5 gap-y-2 font-mono text-xs text-muted">
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> Discord OAuth</span>
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> Live over websockets</span>
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> Works on every device</span>
		</div>
	</PageHero>

	<!-- ───────────────────────── 01 · Realtime sync ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">01</span>
					<span class="eyebrow">Realtime sync</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						Your server's state, mirrored the moment it changes.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							The dashboard holds an open websocket to Dia. When a channel or role is created,
							renamed or deleted in Discord — by you or anyone else — the change lands here in
							a heartbeat. No polling, no stale dropdowns, no reloading the tab to pick the channel
							you just made.
						</p>
						<ul class="mt-5 grid gap-2.5 sm:grid-cols-2">
							{#each ['Live guild state over websockets', 'Channels & roles, always current', 'Member count updates as people join', 'Reconnects on its own if you drop'] as p (p)}
								<li class="flex items-start gap-2 text-sm text-ink/80">
									<Check size={16} class="mt-0.5 shrink-0 text-accent" />{p}
								</li>
							{/each}
						</ul>
					</div>
				</div>
			</Reveal>

			<Reveal delay={80}>
				<div class="mt-12">
					<div class="mb-2 font-mono text-xs uppercase tracking-wide text-muted">Discord → dashboard · live</div>
					<RealtimeSyncDemo />
				</div>
			</Reveal>

			<Reveal delay={120}>
				<div class="mt-10 rounded-2xl bg-[#111113] p-7 sm:p-9">
					<div class="grid gap-8 lg:grid-cols-[1fr_1.15fr] lg:items-center">
						<div>
							<span class="font-mono text-xs uppercase tracking-wider text-accent-ink">On the wire</span>
							<h3 class="mt-3 text-xl font-semibold text-white">A small, typed stream — nothing more.</h3>
							<p class="mt-3 text-[15px] leading-relaxed text-white/65">
								Dia streams a compact message for each guild-state change, scoped to the one
								server you're viewing. The socket only opens after the same permission check the
								rest of the dashboard uses — so it carries your server, and only your server.
							</p>
						</div>
						<div class="space-y-1.5">
							{#each events as e (e.type)}
								<div class="flex items-center justify-between gap-3 rounded-lg bg-white/[0.04] px-3.5 py-2.5 ring-1 ring-white/5">
									<span class="font-mono text-[13px] font-medium text-[#d9b8ff]">{e.type}</span>
									<span class="text-right text-xs text-white/55">{e.note}</span>
								</div>
							{/each}
						</div>
					</div>
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── 02 · Secured by Discord ───────────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">02</span>
					<span class="eyebrow">Secured by Discord</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						Access follows your Discord permissions — exactly.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							There's no separate account system to manage and no extra credentials to leak. You
							log in with Discord, and Dia trusts what Discord already knows about you: it lists the
							servers you can manage, and it gates every page and every websocket behind that same
							check.
						</p>
					</div>
				</div>
			</Reveal>

			<div class="mt-12 grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-2">
				{#each security as s, i (s.title)}
					<Reveal delay={i * 80} class="bg-surface">
						<div class="h-full p-7">
							<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent">
								<s.icon size={19} />
							</span>
							<h3 class="mt-4 text-lg font-semibold">{s.title}</h3>
							<p class="mt-1.5 text-sm leading-relaxed text-muted">{s.body}</p>
						</div>
					</Reveal>
				{/each}
			</div>

			<Reveal delay={120}>
				<div class="mt-10 flex flex-col gap-4 rounded-2xl border border-line bg-bg p-6 sm:flex-row sm:items-center sm:gap-6 sm:p-7">
					<span class="grid h-11 w-11 shrink-0 place-items-center rounded-xl bg-blush text-accent-ink">
						<Shield size={20} />
					</span>
					<p class="text-[15px] leading-relaxed text-muted">
						Concretely: you need <strong class="font-semibold text-ink">Administrator</strong> or
						<strong class="font-semibold text-ink">Manage&nbsp;Server</strong> on a guild to open
						its dashboard. If you lose that role, you lose access the next time your permissions are
						read — the same rule the bot enforces in Discord.
					</p>
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── 03 · Built for the work ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">03</span>
					<span class="eyebrow">Built for the work</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						A control surface, not a settings dump.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							Configuration is grouped by feature, every change previews before it ships, and the
							whole thing is fast on a phone. You spend your time deciding what your server should
							do — not hunting through nested menus to do it.
						</p>
					</div>
				</div>
			</Reveal>

			<div class="mt-12 grid gap-6 lg:grid-cols-3">
				{#each work as w, i (w.title)}
					<Reveal delay={i * 80}>
						<div class="card h-full p-6">
							<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent">
								<w.icon size={19} />
							</span>
							<h3 class="mt-4 text-lg font-semibold">{w.title}</h3>
							<p class="mt-1.5 text-sm leading-relaxed text-muted">{w.body}</p>
						</div>
					</Reveal>
				{/each}
			</div>

			<Reveal delay={100}>
				<div class="mt-12 grid items-start gap-6 lg:grid-cols-2">
					<!-- per-feature toggles -->
					<div class="card p-5">
						<div class="mb-4 flex items-center justify-between">
							<h3 class="text-[15px] font-semibold">Features</h3>
							<span class="rounded-full bg-blush px-2 py-0.5 text-[11px] font-semibold text-accent-ink">Aurora</span>
						</div>
						<div class="divide-y divide-line">
							{#each toggles as t (t.name)}
								<label class="flex cursor-pointer items-center justify-between gap-4 py-3">
									<span class="min-w-0">
										<span class="block text-sm font-medium">{t.name}</span>
										<span class="block text-xs text-muted">{t.desc}</span>
									</span>
									<Toggle bind:checked={t.on} label={t.name} />
								</label>
							{/each}
						</div>
					</div>

					<!-- instant preview -->
					<div class="card p-5">
						<div class="mb-4 flex items-center justify-between">
							<h3 class="text-[15px] font-semibold">Welcome card</h3>
							<span class="inline-flex items-center gap-1.5 rounded-full bg-blush px-2 py-0.5 text-[11px] font-semibold text-accent-ink">
								<RefreshCw size={11} /> live preview
							</span>
						</div>
						<WelcomeCard
							from="#FF6363"
							to="#B244FC"
							angle={45}
							title="Welcome, {'{user}'}!"
							subtitle="You're member #{'{count}'} of {'{server}'}"
							username="maya"
							count={1024}
							server="Aurora"
						/>
						<p class="mt-4 text-xs leading-relaxed text-muted">
							Placeholders like <code class="rounded bg-bg px-1 py-0.5 font-mono text-[11px] text-accent-ink">{'{user}'}</code>
							and <code class="rounded bg-bg px-1 py-0.5 font-mono text-[11px] text-accent-ink">{'{count}'}</code>
							fill in per member. The preview is the real renderer — no surprises when it posts.
						</p>
					</div>
				</div>
			</Reveal>
		</div>
	</section>

	<CtaSection href={cta} heading="Run your server from one tab." sub="Sign in with Discord and configure everything in one realtime dashboard." />

	<SiteFooter />
</div>
