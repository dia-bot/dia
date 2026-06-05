<script lang="ts">
	import SiteNav from '$lib/components/marketing/SiteNav.svelte';
	import SiteFooter from '$lib/components/marketing/SiteFooter.svelte';
	import PageHero from '$lib/components/marketing/PageHero.svelte';
	import CtaSection from '$lib/components/marketing/CtaSection.svelte';
	import Reveal from '$lib/components/marketing/Reveal.svelte';
	import WelcomeEditor from '$lib/components/marketing/welcome/WelcomeEditor.svelte';
	import LeaderboardDemo from '$lib/components/marketing/LeaderboardDemo.svelte';
	import DiscordWindow from '$lib/components/marketing/DiscordWindow.svelte';
	import DiscordMessage from '$lib/components/marketing/DiscordMessage.svelte';
	import { loginURL } from '$lib/api';

	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Check from 'lucide-svelte/icons/check';
	import Layers from 'lucide-svelte/icons/layers';
	import Repeat from 'lucide-svelte/icons/repeat';

	let { data }: { data: { user?: { username: string } | null } } = $props();
	const cta = $derived(data.user ? '/servers' : loginURL);

	const mention = 'rounded bg-[#b244fc]/25 px-1 font-medium text-[#d9b8ff]';

	// The real leveling settings, surfaced from the dashboard config (defaults shown).
	const settings = [
		{
			key: 'xp_min / xp_max',
			value: '15 – 25',
			desc: 'XP awarded per qualifying message, picked at random in this range.'
		},
		{
			key: 'cooldown_seconds',
			value: '60',
			desc: 'How long after a message before another can earn XP — stops spam farming.'
		},
		{
			key: 'multiplier',
			value: '1.0',
			desc: 'Global scale applied to every XP grant. Bump it for fast-moving servers.'
		},
		{
			key: 'no_xp_channels',
			value: '[ … ]',
			desc: 'Channels where messages earn nothing — bots, spam, off-topic.'
		},
		{
			key: 'no_xp_roles',
			value: '[ … ]',
			desc: 'Roles that never earn XP, e.g. muted members or other bots.'
		},
		{
			key: 'announce_channel',
			value: 'same · dm · #channel',
			desc: 'Where level-up announcements land — in place, by DM, or a dedicated channel.'
		}
	];

	// Reward modes — stack or replace, matching the stack_rewards / remove_previous config.
	const rewardModes = [
		{
			icon: Layers,
			title: 'Stack rewards',
			body: 'Members keep every reward role they earn. Level 5, 10 and 25 roles all pile up as they climb.'
		},
		{
			icon: Repeat,
			title: 'Replace previous',
			body: 'Each new tier swaps out the last, so members wear a single, current rank role at a time.'
		}
	];

	const rewardPoints = [
		'Map any level to any role',
		'Roles assigned the moment a member levels up',
		'Toggle stacking per reward',
		'Earned roles re-applied if a member rejoins'
	];
</script>

<svelte:head>
	<title>Leveling · Dia</title>
	<meta
		name="description"
		content="Dia's leveling system rewards active members with XP, level-up announcements, themeable rank cards, role rewards and a server leaderboard — all tuned from one dashboard, or self-hosted."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} />

	<PageHero
		eyebrow="[ feature · leveling ]"
		title="Reward the regulars, automatically."
		lede="Members earn XP as they chat, level up with announcements, and unlock role rewards — each backed by a generated rank card you can theme to match your server."
	>
		<div class="flex flex-wrap items-center gap-3">
			<a href={cta} class="btn btn-accent h-12 px-6 text-base">
				Get started <ArrowRight size={18} />
			</a>
			<a href="/#features" class="btn btn-ghost h-12 px-5 text-base">All features</a>
		</div>
	</PageHero>

	<!-- ───────────────────────── 01 · Rank cards ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">01</span>
					<span class="eyebrow">Rank cards</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2
						class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6"
					>
						A rank card you build, element by element.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-6">
						Every <span class="font-mono text-sm text-accent-ink">/rank</span> shows the member's
						progress on a card you style — gradient or solid background, accent, text and progress-bar
						colours. Drag any layer to reposition it, restyle every element, or start from a template.
					</p>
				</div>
			</Reveal>

			<Reveal delay={80}>
				<div class="mt-12">
					<WelcomeEditor mode="rank" />
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── 02 · Level-ups & rewards ───────────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">02</span>
					<span class="eyebrow">Level-ups & rewards</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2
						class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6"
					>
						Announce the climb. Unlock the roles.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-6">
						When a member levels up, Dia posts a message you write — in place, by DM, or in a
						dedicated channel — and grants any role rewards you've mapped to that level.
					</p>
				</div>
			</Reveal>

			<div class="mt-12 grid items-start gap-6 lg:grid-cols-2">
				<Reveal delay={80}>
					<DiscordWindow channel="level-ups" title="Aurora" topic="Celebrate the regulars">
						<DiscordMessage author="maya" color="#1aa179" time="Today at 8:02 PM">
							finally cracked the top 10 🔥
						</DiscordMessage>
						<DiscordMessage brand author="Dia" time="Today at 8:03 PM">
							GG <span class={mention}>@maya</span>, you reached
							<strong class="font-semibold text-[#f2f3f5]">level 12</strong>! 🎉
						</DiscordMessage>
					</DiscordWindow>
					<div class="mt-3 font-mono text-xs uppercase tracking-wide text-muted">
						level_up_message · GG {'{user.mention}'}, you reached **level {'{level}'}**!
					</div>
				</Reveal>

				<Reveal delay={160}>
					<div class="card p-6">
						<h3 class="text-base font-semibold">Role rewards</h3>
						<ul class="mt-4 grid gap-2.5">
							{#each rewardPoints as p (p)}
								<li class="flex items-start gap-2 text-sm text-ink/80">
									<Check size={16} class="mt-0.5 shrink-0 text-accent" />{p}
								</li>
							{/each}
						</ul>

						<div class="mt-6 grid gap-px overflow-hidden rounded-xl border border-line bg-line">
							{#each rewardModes as m (m.title)}
								<div class="bg-surface p-5">
									<div class="flex items-center gap-2.5">
										<span class="grid h-8 w-8 place-items-center rounded-lg bg-blush text-accent">
											<m.icon size={16} />
										</span>
										<h4 class="text-sm font-semibold">{m.title}</h4>
									</div>
									<p class="mt-2 text-sm leading-relaxed text-muted">{m.body}</p>
								</div>
							{/each}
						</div>
					</div>
				</Reveal>
			</div>
		</div>
	</section>

	<!-- ───────────────────────── 03 · Leaderboard ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">03</span>
					<span class="eyebrow">Leaderboard</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2
						class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6"
					>
						The standings, ranked by XP.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-6">
						<span class="font-mono text-sm text-accent-ink">/leaderboard</span> ranks your most active
						members by total XP. The same board lives in the dashboard, so you can see who's leading at
						a glance.
					</p>
				</div>
			</Reveal>

			<Reveal delay={80}>
				<div class="mx-auto mt-12 max-w-2xl">
					<LeaderboardDemo />
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── 04 · XP controls ───────────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">04</span>
					<span class="eyebrow">XP controls</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2
						class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6"
					>
						Tune exactly how XP is earned.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-6">
						Every knob lives in the dashboard. Set the rates, add a cooldown so chat can't be farmed,
						and exclude the channels and roles that shouldn't count.
					</p>
				</div>
			</Reveal>

			<Reveal delay={80}>
				<div class="mt-12 overflow-hidden rounded-2xl border border-line bg-bg">
					<div class="flex items-center gap-2 border-b border-line px-5 py-3">
						<span class="font-mono text-xs uppercase tracking-wide text-muted">
							guild_feature_configs · leveling
						</span>
					</div>
					<dl class="divide-y divide-line">
						{#each settings as s (s.key)}
							<div class="grid gap-1.5 px-5 py-4 sm:grid-cols-[20rem_1fr] sm:items-baseline sm:gap-6">
								<dt class="flex items-baseline gap-3">
									<span class="font-mono text-sm text-accent-ink">{s.key}</span>
									<span class="font-mono text-xs text-faint">{s.value}</span>
								</dt>
								<dd class="text-sm leading-relaxed text-muted">{s.desc}</dd>
							</div>
						{/each}
					</dl>
				</div>
			</Reveal>
		</div>
	</section>

	<CtaSection href={cta} heading="Reward the regulars." sub="Turn on leveling and let activity earn its own roles." />

	<SiteFooter />
</div>
