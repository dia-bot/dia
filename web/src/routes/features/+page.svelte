<script lang="ts">
	import SiteNav from '$lib/components/marketing/SiteNav.svelte';
	import SiteFooter from '$lib/components/marketing/SiteFooter.svelte';
	import PageHero from '$lib/components/marketing/PageHero.svelte';
	import CtaSection from '$lib/components/marketing/CtaSection.svelte';
	import Reveal from '$lib/components/marketing/Reveal.svelte';
	import { loginURL } from '$lib/api';

	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import ImageIcon from 'lucide-svelte/icons/image';
	import TrendingUp from 'lucide-svelte/icons/trending-up';
	import Users from 'lucide-svelte/icons/users';
	import ShieldCheck from 'lucide-svelte/icons/shield-check';
	import TerminalIcon from 'lucide-svelte/icons/terminal';
	import LayoutDashboard from 'lucide-svelte/icons/layout-dashboard';

	let { data }: { data: { user?: { username: string } | null } } = $props();
	const cta = $derived(data.user ? '/servers' : loginURL);

	const features = [
		{ id: 'welcome', n: '01', tag: 'Onboard', title: 'Welcome', icon: ImageIcon, desc: 'Greet new members with a welcome card you compose yourself, posted the moment they join.' },
		{ id: 'leveling', n: '02', tag: 'Engage', title: 'Leveling & rank cards', icon: TrendingUp, desc: 'XP as members chat, level-up announcements, role rewards, leaderboards and themeable rank cards.' },
		{ id: 'roles', n: '03', tag: 'Self-serve', title: 'Reaction & auto roles', icon: Users, desc: 'Button and select role menus with toggle, unique and verify modes — plus roles on join.' },
		{ id: 'moderation', n: '04', tag: 'Protect', title: 'Moderation & AutoMod', icon: ShieldCheck, desc: 'Ban, kick, timeout and warn with a numbered case log, and rule-based content filtering.' },
		{ id: 'commands', n: '05', tag: 'Extend', title: 'Custom commands', icon: TerminalIcon, desc: 'Design your own slash commands from the dashboard — plain replies or rich embeds, no code.' },
		{ id: 'dashboard', n: '06', tag: 'Control', title: 'Realtime dashboard', icon: LayoutDashboard, desc: 'Discord-secured login, per-server permissions and live guild state synced over websockets.' }
	];
</script>

<svelte:head>
	<title>Features · Dia</title>
	<meta
		name="description"
		content="Every Dia feature in one place — welcome cards, leveling and rank cards, self-serve roles, moderation and AutoMod, custom slash commands, and a realtime dashboard."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} />

	<PageHero
		eyebrow="[ features ]"
		title="One bot. Six essentials."
		lede="The features communities install five separate bots for — unified into one open, self-hostable stack, configured from a single realtime dashboard."
	>
		<div class="flex flex-wrap items-center gap-3">
			<a href={cta} class="btn btn-accent h-12 px-6 text-base">Get started <ArrowRight size={18} /></a>
			<a href="/pricing" class="btn btn-ghost h-12 px-5 text-base">See pricing</a>
		</div>
	</PageHero>

	<section class="py-16 sm:py-20">
		<div class="mx-auto max-w-page px-6">
			<div class="grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-2 lg:grid-cols-3">
				{#each features as f, i (f.id)}
					<Reveal delay={(i % 3) * 80} class="bg-surface">
						<a href="/features/{f.id}" class="group flex h-full flex-col p-7 transition-colors hover:bg-bg">
							<div class="flex items-center justify-between">
								<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent">
									<f.icon size={20} />
								</span>
								<span class="font-mono text-xs text-faint">{f.n}</span>
							</div>
							<div class="mt-4 font-mono text-[11px] uppercase tracking-wide text-accent-ink">{f.tag}</div>
							<h2 class="mt-1 text-lg font-semibold group-hover:text-accent-ink">{f.title}</h2>
							<p class="mt-2 flex-1 text-sm leading-relaxed text-muted">{f.desc}</p>
							<span class="mt-5 inline-flex items-center gap-1 font-mono text-xs text-muted group-hover:text-accent-ink">
								Explore <ArrowRight size={13} />
							</span>
						</a>
					</Reveal>
				{/each}
			</div>
		</div>
	</section>

	<CtaSection href={cta} heading="One bot, every essential." sub="Add Dia and turn on what you need — free to start, $3.99/mo per server for the heavy lifting." />
	<SiteFooter />
</div>
