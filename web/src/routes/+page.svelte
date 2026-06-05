<script lang="ts">
	import { loginURL } from '$lib/api';

	import SiteNav from '$lib/components/marketing/SiteNav.svelte';
	import SiteFooter from '$lib/components/marketing/SiteFooter.svelte';
	import CtaSection from '$lib/components/marketing/CtaSection.svelte';
	import Reveal from '$lib/components/marketing/Reveal.svelte';
	import WelcomeCard from '$lib/components/marketing/WelcomeCard.svelte';
	import DiscordWindow from '$lib/components/marketing/DiscordWindow.svelte';
	import DiscordMessage from '$lib/components/marketing/DiscordMessage.svelte';
	import RankCard from '$lib/components/marketing/RankCard.svelte';
	import WelcomeEditor from '$lib/components/marketing/welcome/WelcomeEditor.svelte';
	import LeaderboardDemo from '$lib/components/marketing/LeaderboardDemo.svelte';
	import AutomodDemo from '$lib/components/marketing/AutomodDemo.svelte';
	import CaseLogDemo from '$lib/components/marketing/CaseLogDemo.svelte';
	import RoleMenuDemo from '$lib/components/marketing/RoleMenuDemo.svelte';
	import CommandBuilderDemo from '$lib/components/marketing/CommandBuilderDemo.svelte';
	import RealtimeSyncDemo from '$lib/components/marketing/RealtimeSyncDemo.svelte';
	import Terminal from '$lib/components/marketing/Terminal.svelte';

	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Check from 'lucide-svelte/icons/check';

	let { data }: { data: { user?: { username: string } | null } } = $props();
	const cta = $derived(data.user ? '/servers' : loginURL);

	// Discord @mention pill — tinted with the violet accent on the dark theme.
	const mention = 'rounded bg-[#a472ff]/25 px-1 font-medium text-[#c4a6ff]';

	// Bento tile chrome — elevated dark surface, hairline, dark-correct depth, gentle lift.
	const tile =
		'group relative flex flex-col overflow-hidden rounded-[20px] border border-line bg-surface transition-[transform,box-shadow] duration-300 hover:-translate-y-1';
	const tileShadow =
		'box-shadow: inset 0 1px 0 rgba(255,255,255,0.04), 0 1px 2px rgba(0,0,0,0.5), 0 20px 48px -24px rgba(0,0,0,0.7);';

	const rolePills = [
		{ e: '🎮', n: 'Gamer', on: true },
		{ e: '🎨', n: 'Artist', on: false },
		{ e: '🎵', n: 'Music', on: true },
		{ e: '🔔', n: 'News', on: false },
		{ e: '📣', n: 'Events', on: false }
	];

	const stack = ['Elixir', 'Go', 'SvelteKit', 'PostgreSQL', 'Redis', 'NATS', 'Docker'];

	const faqs = [
		{
			q: "What's free, and what's the $3.99?",
			a: 'The core — welcome cards, leveling, moderation, self-serve roles and custom commands — is free on every server. The per-server Pro plan ($3.99/mo) covers the resource-heavy extras as we finalise exactly which features that includes.'
		},
		{
			q: 'Can I self-host instead of paying?',
			a: "Yes. Dia is MIT-licensed and fully self-hostable — the Elixir gateway, the Go worker and API, Postgres, Redis and NATS. One command brings the whole stack up on your own machine. Self-hosting is free, forever."
		},
		{
			q: 'Who owns my server data?',
			a: 'You do. Self-host and it never leaves your infrastructure. On the hosted plan we store only what a feature needs to work, and you can export or delete it from the dashboard at any time.'
		},
		{
			q: 'Will it replace the bots I run today?',
			a: 'That’s the point. Dia folds welcome, leveling, moderation, roles and custom commands into one bot and one realtime dashboard — so you configure once instead of juggling a stack of single-purpose bots.'
		},
		{
			q: 'Does setup need config files or tokens?',
			a: 'No. Authorise Dia for your server in one click, then flip features on and tune them from the dashboard with live previews. Changes apply instantly.'
		}
	];
</script>

<svelte:head>
	<title>Dia — one Discord bot for your whole community</title>
	<meta
		name="description"
		content="Dia replaces a stack of single-purpose Discord bots with one open-source bot and a realtime dashboard: welcome cards, leveling, moderation, self-serve roles and custom commands. Free to start, $3.99/mo per server for the extras, or self-host it."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} overlay />

	<!-- ───────────────────────── Hero — MONOLITH ─────────────────────────
	     Type-first, near-black, Cursor-grade. One enormous near-white grotesk
	     headline, one gradient CTA, and a single large product surface. Colour is
	     reserved for deliberate objects (the CTA, the welcome card, the logo) —
	     no ambient purple wash. No badges, no chips, no grid stickers, no grain. -->
	<section class="relative isolate overflow-hidden border-b border-line bg-bg">
		<!-- backdrop: a faint near-white vignette for depth — no colored glow -->
		<div aria-hidden="true" class="pointer-events-none absolute inset-0">
			<div
				class="absolute inset-x-0 top-0 h-[45%]"
				style="background: radial-gradient(50% 100% at 50% 0%, color-mix(in srgb, var(--color-ink) 4%, transparent), transparent 70%);"
			></div>
		</div>

		<div class="relative mx-auto max-w-page px-6 pt-36 sm:pt-44">
			<!-- ── monumental headline ── -->
			<Reveal y={20}>
				<h1
					class="max-w-[15ch] font-sans font-black leading-[0.95] tracking-[-0.035em] text-ink text-[clamp(3rem,9vw,6.5rem)]"
				>
					Five bots in one.<br /><span class="text-muted">None of the sprawl.</span>
				</h1>
			</Reveal>

			<Reveal delay={90} y={18}>
				<p class="mt-8 max-w-[46ch] text-lg leading-relaxed text-muted sm:text-xl">
					Welcome cards, leveling, moderation, self-serve roles and custom commands — one open bot,
					run from a single realtime dashboard or self-hosted on your own machine.
				</p>
			</Reveal>

			<!-- ONE gradient CTA + a single quiet self-host link -->
			<Reveal delay={170} y={16}>
				<div class="mt-10 flex flex-wrap items-center gap-x-6 gap-y-3">
					<a
						href={cta}
						class="brand-gradient inline-flex h-12 items-center gap-2 rounded-xl px-6 text-[0.95rem] font-semibold text-white shadow-[0_8px_30px_-8px_rgba(178,68,252,0.5)] transition-[filter,transform] duration-150 hover:brightness-110"
					>
						Get started <ArrowRight size={18} />
					</a>
					<a
						href="https://github.com/dia-bot/dia"
						class="group inline-flex items-center gap-2 font-mono text-[11px] uppercase tracking-[0.08em] text-muted transition-colors hover:text-ink"
					>
						<span class="h-px w-6 bg-line-strong transition-colors group-hover:bg-ink"></span>
						or self-host · MIT
					</a>
				</div>
			</Reveal>

			<!-- ── HERO PRODUCT SURFACE: a choreographed join, staged in one real #welcome ──
				     A hairline "timeline spine" runs down a real gutter column inside the chat
				     (never a breakout that fights the window's overflow-hidden), pinning three
				     numbered beats: maya joins → Dia welcomes (the climax) → she grabs roles.
				     The gradient WelcomeCard is the dominant signature object — lifted on a
				     dark-correct lit-edge + black ambient + a single 1px brand ring. Staggered
				     Reveal delays pace the sequence (Reveal self-guards reduced-motion). On
				     mobile it collapses to a clean single-column transcript so the proof that it
				     really posted is never hidden. -->
				<div class="relative mt-20 sm:mt-28">
					<Reveal delay={260} y={28}>
						<DiscordWindow
							channel="welcome"
							title="Aurora"
							topic="Say hi to new members"
							members="1,284 online"
							channels={['welcome', 'general', 'introductions', 'level-ups', 'roles']}
						>
							<!-- 2-track grid: a narrow spine gutter + the message track. The spine
							     and its numbered nodes live entirely INSIDE this grid (no negative
							     margins escape the frame), so the window's overflow-hidden never
							     clips them. Gutter collapses on mobile to a single column. -->
							<div class="relative grid grid-cols-1 gap-x-3.5 sm:grid-cols-[34px_minmax(0,1fr)]">
								<!-- the spine: one continuous faint near-white hairline (a dark line
								     would vanish on dark), with a single brand-lit segment over the
								     climax beat — the one deliberate colour event on the rail. -->
								<div aria-hidden="true" class="relative hidden sm:block">
									<span
										class="absolute left-1/2 top-4 bottom-4 w-px -translate-x-1/2"
										style="background: linear-gradient(to bottom, transparent, rgba(255,255,255,0.10) 14%, rgba(255,255,255,0.10) 86%, transparent);"
									></span>
									<span
										class="absolute left-1/2 top-[30%] h-[40%] w-px -translate-x-1/2"
										style="background: linear-gradient(to bottom, transparent, rgba(178,68,252,0.5), transparent);"
									></span>
								</div>

								<!-- message track -->
								<div class="min-w-0 space-y-3.5">
									<!-- BEAT 01 — the member arrives (a real Discord join line) -->
									<Reveal delay={360} y={14}>
										<div class="relative flex items-center gap-2.5 text-[13px] text-[#949ba4]">
											<span class="beat-node beat-node-01">01</span>
											<svg viewBox="0 0 24 24" class="h-4 w-4 shrink-0 text-[#23a55a]" fill="none" aria-hidden="true">
												<path
													d="M5 12h11M11 7l5 5-5 5"
													stroke="currentColor"
													stroke-width="2"
													stroke-linecap="round"
													stroke-linejoin="round"
												/>
											</svg>
											<span><span class="font-medium text-[#dbdee1]">maya</span> joined the server.</span>
											<span class="ml-auto hidden font-mono text-[11px] text-[#6d7178] sm:inline">4:19 PM</span>
										</div>
									</Reveal>

									<Reveal delay={460} y={14}>
										<DiscordMessage author="maya" color="#1aa179" time="Today at 4:19 PM">
											just joined, hi everyone 👋
										</DiscordMessage>
									</Reveal>

									<!-- BEAT 02 — Dia welcomes her. THE CLIMAX. The bot line reads as a real
									     grouped message; the WelcomeCard is the dominant object below it —
									     oversized (max-w-[560px]), nudged toward the left edge of the chat
									     column for an off-center, placed composition (a small positive
									     offset only — it never exits the window frame). -->
									<Reveal delay={580} y={18}>
										<div class="relative">
											<span class="beat-node beat-node-lit">02</span>
											<DiscordMessage brand author="Dia" time="Today at 4:19 PM">
												Welcome to <strong class="font-semibold text-[#f2f3f5]">Aurora</strong>,
												<span class={mention}>@maya</span> — you're our
												<strong class="font-semibold text-[#f2f3f5]">1,024th</strong> member.
											</DiscordMessage>

											<!-- the signature artifact: the gradient WelcomeCard, dominant -->
											<div class="mt-2.5 pl-0 sm:pl-[52px]">
												<div class="relative w-full max-w-[560px] sm:-ml-2 lg:-ml-3">
													<div class="wc-frame overflow-hidden rounded-2xl">
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
													</div>
													<!-- provenance caption — mono, quiet, one tiny solid-accent dot -->
													<div class="mt-2 flex items-center gap-2 font-mono text-[10.5px] uppercase tracking-[0.08em] text-faint">
														<span class="h-1 w-1 rounded-full bg-accent"></span>
														posted automatically
														<span class="text-line-strong">·</span>
														<span class="text-muted">~40ms</span>
													</div>
												</div>
											</div>
										</div>
									</Reveal>

									<!-- BEAT 03 — she's in, and grabs her roles -->
									<Reveal delay={760} y={14}>
										<div class="relative">
											<span class="beat-node beat-node-03">03</span>
											<DiscordMessage author="kai" color="#c79bff" time="Today at 4:20 PM">
												welcome maya! 🙌 grab your roles in <span class={mention}>#roles</span>
											</DiscordMessage>
										</div>
									</Reveal>
								</div>
							</div>
						</DiscordWindow>
					</Reveal>
				</div>
		</div>
	</section>

	<!-- ───────────────────────── Value strip ───────────────────────── -->
	<section class="border-y border-line bg-surface">
		<Reveal>
			<div class="mx-auto grid max-w-page grid-cols-1 divide-y divide-line px-6 sm:grid-cols-3 sm:divide-x sm:divide-y-0">
				<div class="py-10 sm:pr-10">
					<div class="font-mono text-[11px] uppercase tracking-[0.08em] text-accent-ink">all-in-one</div>
					<h3 class="mt-2.5 text-lg font-semibold">One bot, not five</h3>
					<p class="mt-1.5 text-sm leading-relaxed text-muted">
						Welcome, leveling, moderation, self-serve roles and custom commands — one install, one
						dashboard.
					</p>
				</div>
				<div class="py-10 sm:px-10">
					<div class="font-mono text-[11px] uppercase tracking-[0.08em] text-accent-ink">open source</div>
					<h3 class="mt-2.5 text-lg font-semibold">MIT licensed</h3>
					<p class="mt-1.5 text-sm leading-relaxed text-muted">
						Read every line, own your data, and self-host the whole stack on your own machine — free,
						forever.
					</p>
				</div>
				<div class="py-10 sm:pl-10">
					<div class="font-mono text-[11px] uppercase tracking-[0.08em] text-accent-ink">pricing</div>
					<h3 class="mt-2.5 text-lg font-semibold">Free, then $3.99<span class="font-normal text-muted">/mo</span></h3>
					<p class="mt-1.5 text-sm leading-relaxed text-muted">
						The core is free on every server. Pro adds the resource-heavy extras, billed per server.
					</p>
				</div>
			</div>
		</Reveal>
	</section>

	<!-- ───────────────────────── Bento overview ───────────────────────── -->
	<section class="py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<span class="eyebrow">[ features ]</span>
				<div class="mt-3 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
					<h2 class="max-w-2xl text-3xl font-bold tracking-[-0.02em] sm:text-[2.7rem] sm:leading-[1.05]">
						Five features. One bot.
					</h2>
					<a href="/features" class="inline-flex items-center gap-1.5 text-sm font-medium text-accent-ink hover:text-accent">
						See all features <ArrowRight size={14} />
					</a>
				</div>
			</Reveal>

			<Reveal delay={80}>
				<div class="mt-10 grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-12 lg:auto-rows-[218px]">
					<!-- Welcome (big, 2 rows) -->
					<a href="/features/welcome" class="{tile} sm:col-span-2 lg:col-span-5 lg:row-span-2" style={tileShadow}>
						<div class="flex flex-1 items-center p-6">
							<div class="w-full overflow-hidden rounded-xl ring-1 ring-white/10 shadow-[0_24px_60px_-30px_rgba(0,0,0,0.8)]">
								<WelcomeCard from="#FF6363" to="#B244FC" angle={45} title="Welcome, {'{user}'}!" subtitle="You're member #{'{count}'} of {'{server}'}" username="maya" count={1024} server="Aurora" />
							</div>
						</div>
						<div class="border-t border-line p-6">
							<div class="font-mono text-[11px] uppercase tracking-wide text-accent-ink">01 · onboard</div>
							<div class="mt-1.5 flex items-center justify-between gap-2">
								<h3 class="text-lg font-semibold">Welcome cards</h3>
								<ArrowRight size={16} class="shrink-0 text-faint transition-transform group-hover:translate-x-0.5 group-hover:text-accent-ink" />
							</div>
							<p class="mt-1 max-w-md text-sm leading-relaxed text-muted">Compose every layer — drag, restyle and theme the card, then it posts the moment a member joins.</p>
						</div>
					</a>

					<!-- Leveling (wide, real rank card) -->
					<a href="/features/leveling" class="{tile} lg:col-span-7" style={tileShadow}>
						<div class="flex flex-1 flex-col gap-4 p-6 lg:flex-row lg:items-center lg:gap-5">
							<div class="min-w-0 flex-1">
								<div class="font-mono text-[11px] uppercase tracking-wide text-accent-ink">02 · engage</div>
								<h3 class="mt-1.5 text-lg font-semibold">Leveling &amp; rank cards</h3>
								<p class="mt-1 text-sm leading-relaxed text-muted">XP for taking part, generated rank cards, and a live leaderboard.</p>
							</div>
							<div class="w-full lg:w-[300px] lg:shrink-0">
								<div class="overflow-hidden rounded-xl ring-1 ring-white/10">
									<RankCard from="#1F1B2E" to="#3A2E5C" angle={30} accent="#a472ff" barColor="#a472ff" username="Luna" rank={1} level={42} levelXp={820} neededXp={1000} totalXp={184200} />
								</div>
								<div class="mt-2 text-center font-mono text-[11px] text-faint">rank #1 · 184,200 XP total</div>
							</div>
						</div>
					</a>

					<!-- Moderation -->
					<a href="/features/moderation" class="{tile} lg:col-span-4" style={tileShadow}>
						<div class="flex-1 space-y-2 p-6">
							<div class="flex items-center gap-2 text-[13px]">
								<span class="rounded-full border border-[color-mix(in_srgb,var(--color-danger)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_18%,transparent)] px-2 py-0.5 text-xs font-medium text-[var(--color-danger)]">ban</span>
								<code class="text-accent-ink">@scammer</code>
								<span class="ml-auto font-mono text-[11px] text-faint">#143</span>
							</div>
							<div class="flex items-center gap-2 text-[13px]">
								<span class="rounded-full bg-blush px-2 py-0.5 text-xs font-medium text-accent-ink">timeout</span>
								<code class="text-muted">@raid_alt</code>
								<span class="ml-auto font-mono text-[11px] text-faint">#145</span>
							</div>
							<div class="flex items-center gap-1.5 pt-1 font-mono text-[11px] text-muted"><span class="h-1.5 w-1.5 rounded-full bg-success"></span> AutoMod active</div>
						</div>
						<div class="border-t border-line p-6 pt-4">
							<div class="font-mono text-[11px] uppercase tracking-wide text-accent-ink">03 · protect</div>
							<h3 class="mt-1 text-base font-semibold">Moderation &amp; AutoMod</h3>
						</div>
					</a>

					<!-- Commands -->
					<a href="/features/commands" class="{tile} lg:col-span-3" style={tileShadow}>
						<div class="flex-1 p-6">
							<div class="rounded-xl bg-ink-2 p-3.5 font-mono text-[12.5px] leading-relaxed">
								<div><span class="text-[#f0a9b1]">/</span><span class="text-[#e6e8ec]">rules</span> <span class="text-[#80858f]"># show the rules</span></div>
								<div class="mt-2 text-[#9aa0aa]">↳ <span class="text-[#e6e8ec]">embed</span> · <span class="text-[#e6e8ec]">ephemeral</span></div>
								<div class="mt-1 text-[#9aa0aa]">↳ posts <span class="text-[#e6e8ec]">“Server rules”</span></div>
							</div>
						</div>
						<div class="border-t border-line p-6 pt-4">
							<div class="font-mono text-[11px] uppercase tracking-wide text-accent-ink">04 · extend</div>
							<h3 class="mt-1 text-base font-semibold">Custom commands</h3>
						</div>
					</a>

					<!-- Roles -->
					<a href="/features/roles" class="{tile} lg:col-span-5" style={tileShadow}>
						<div class="flex flex-1 flex-wrap content-start gap-2 p-6">
							{#each rolePills as r (r.n)}
								<span class="inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[13px] {r.on ? 'border-accent/40 bg-blush text-accent-ink' : 'border-line-strong text-muted'}">
									<span>{r.e}</span> {r.n}{#if r.on}<Check size={12} />{/if}
								</span>
							{/each}
						</div>
						<div class="border-t border-line p-6 pt-4">
							<div class="font-mono text-[11px] uppercase tracking-wide text-accent-ink">05 · self-serve</div>
							<h3 class="mt-1 text-base font-semibold">Reaction &amp; auto roles</h3>
						</div>
					</a>

					<!-- Dashboard (full width climax) -->
					<a href="/features/dashboard" class="{tile} sm:col-span-2 lg:col-span-7" style={tileShadow}>
						<div class="grid flex-1 items-center gap-6 p-6 sm:grid-cols-[1.2fr_1fr] sm:p-7">
							<div>
								<div class="font-mono text-[11px] uppercase tracking-wide text-accent-ink">control surface</div>
								<div class="mt-1.5 flex items-center gap-2">
									<h3 class="text-lg font-semibold">One realtime dashboard</h3>
									<ArrowRight size={16} class="shrink-0 text-faint transition-transform group-hover:translate-x-0.5 group-hover:text-accent-ink" />
								</div>
								<p class="mt-1 max-w-md text-sm leading-relaxed text-muted">Flip features on, tune them with live previews, and watch changes apply to your server the instant you save.</p>
							</div>
							<div class="space-y-2">
								{#each ['Welcome', 'Leveling', 'AutoMod'] as f, i (f)}
									<div class="flex items-center justify-between rounded-lg border border-line bg-bg px-3.5 py-2">
										<span class="text-[13px] font-medium">{f}</span>
										<span class="relative h-4 w-7 rounded-full {i === 2 ? 'bg-line-strong' : 'bg-accent'}">
											<span class="absolute top-0.5 h-3 w-3 rounded-full bg-white {i === 2 ? 'left-0.5' : 'right-0.5'}"></span>
										</span>
									</div>
								{/each}
							</div>
						</div>
					</a>
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── Deep: Welcome editor — STUDIO BENCH ─────────────────────────
	 Near-black bench (bg-ink-2) breaks the run of bg / bg-surface deep sections and lets the
	 editor's own .card lift forward. Deliberately drops the page's repeated
	 [eyebrow]+7/5 split-headline+paragraph rhythm: a ruled index line, an oversized offset
	 headline, and a 3-column mono SPEC LEDGER (drag / layout / theme) that reads as a real
	 app status bar — not marketing prose. The only colour event is a single 1px lit top edge
	 on the bench frame; everything else is monochrome hairlines + crop-mark corner ticks. -->
<section class="relative border-y border-line bg-ink-2 py-20 sm:py-28">
	<div class="mx-auto max-w-page px-6">
		<!-- ── Masthead: ruled index + oversized offset headline (no eyebrow/split/paragraph) ── -->
		<Reveal y={20}>
			<div>
				<!-- ruled index line replaces the bracket eyebrow -->
				<div class="flex items-baseline gap-3">
					<span class="font-mono text-[12px] font-medium tracking-[0.08em] text-faint">[ welcome ]</span>
					<span class="h-px flex-1 translate-y-[-1px] bg-line"></span>
					<span class="font-mono text-[11px] uppercase tracking-[0.14em] text-faint">studio</span>
				</div>

				<!-- editorial headline: small lead line, then an oversized underscored offset line -->
				<h2 class="mt-6 font-sans font-black leading-[0.95] tracking-[-0.035em] text-ink">
					<span class="block text-[clamp(1.85rem,4.4vw,3rem)]">Design welcome cards,</span>
					<span
						class="mt-1 block w-fit pb-1 text-[clamp(2.3rem,7vw,5rem)] leading-[0.9] tracking-[-0.04em]"
						style="border-bottom: 2px solid var(--color-line-strong);"
						>element <span class="text-muted">by</span> element.</span
					>
				</h2>
			</div>
		</Reveal>

		<!-- ── Spec ledger: replaces the marketing paragraph. Reads like an app status bar. ── -->
		<Reveal delay={70} y={16}>
			<div
				class="mt-9 flex flex-col divide-y divide-line border-y border-line text-sm sm:flex-row sm:divide-x sm:divide-y-0 sm:border-y-0"
			>
				<div class="flex items-baseline gap-3 py-4 pr-8 sm:py-0">
					<span class="font-mono text-[11px] uppercase tracking-[0.08em] text-faint">drag</span>
					<span class="text-muted">Move any layer on the canvas, or nudge with arrow keys.</span>
				</div>
				<div class="flex items-baseline gap-3 py-4 sm:px-8 sm:py-0">
					<span class="font-mono text-[11px] uppercase tracking-[0.08em] text-faint">layout</span>
					<span class="text-muted">Six composable presets — recompose, don't restart.</span>
				</div>
				<div class="flex items-baseline gap-3 py-4 sm:py-0 sm:pl-8">
					<span class="font-mono text-[11px] uppercase tracking-[0.08em] text-faint">theme</span>
					<span class="text-muted">Swatch the whole card, then tune per element. It's live.</span>
				</div>
			</div>
		</Reveal>

		<!-- ── The bench: editor framed as a file on a drafting table (FILE strip + crop ticks) ── -->
		<Reveal delay={140} y={26}>
			<div class="mt-12 sm:mt-14">
				<!-- spec strip: treats the editor like a file on the bench -->
				<div class="mb-3 flex items-center gap-3">
					<span class="font-mono text-[11px] tracking-[0.06em] text-faint">FILE</span>
					<span class="font-mono text-[12px] text-muted">welcome.card</span>
					<span class="h-px flex-1 bg-line"></span>
					<span class="font-mono text-[11px] tracking-[0.06em] text-faint">1024 × 450 · EDITABLE</span>
				</div>

				<div class="bench-frame relative">
					<!-- four crop-mark corner ticks (decorative drafting motif, behind interaction) -->
					<span class="tick tick-tl" aria-hidden="true"></span>
					<span class="tick tick-tr" aria-hidden="true"></span>
					<span class="tick tick-bl" aria-hidden="true"></span>
					<span class="tick tick-br" aria-hidden="true"></span>

					<div class="bench-inset">
						<WelcomeEditor />
					</div>
				</div>
			</div>
		</Reveal>
	</div>
</section>

	<!-- ───────────────────────── Deep: Leveling ───────────────────────── -->
	<section class="py-20 sm:py-28">
		<div class="mx-auto grid max-w-page items-center gap-12 px-6 lg:grid-cols-2">
			<Reveal>
				<div>
					<span class="eyebrow">[ leveling ]</span>
					<h2 class="mt-3 text-3xl font-bold tracking-[-0.02em] sm:text-4xl">Reward the people who show up.</h2>
					<p class="mt-5 text-lg leading-relaxed text-muted">
						Members earn XP for taking part. Dia renders a personal rank card on request and keeps a
						live leaderboard — so your most active people get the recognition that keeps them around.
					</p>
					<div class="mt-7 overflow-hidden rounded-2xl ring-1 ring-white/10">
						<RankCard from="#1F1B2E" to="#3A2E5C" angle={30} accent="#a472ff" barColor="#a472ff" username="Luna" rank={1} level={42} levelXp={820} neededXp={1000} totalXp={184200} />
					</div>
					<a href="/features/leveling" class="mt-6 inline-flex items-center gap-1.5 text-sm font-medium text-accent-ink hover:text-accent">
						How leveling works <ArrowRight size={14} />
					</a>
				</div>
			</Reveal>
			<Reveal delay={120}><LeaderboardDemo /></Reveal>
		</div>
	</section>

	<!-- ───────────────────────── Deep: Moderation ───────────────────────── -->
	<section class="border-t border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<span class="eyebrow">[ moderation ]</span>
				<div class="mt-3 grid items-end gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-bold tracking-[-0.02em] sm:text-[2.6rem] sm:leading-[1.05] lg:col-span-7">
						A durable record of every action.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-5">
						AutoMod watches for invites, links, spam and banned words, then deletes and applies your
						chosen action. Every removal — automated or manual — lands in an immutable case log.
					</p>
				</div>
			</Reveal>
			<div class="mt-12 grid items-start gap-6 lg:grid-cols-2">
				<Reveal><AutomodDemo /></Reveal>
				<Reveal delay={120}><CaseLogDemo /></Reveal>
			</div>
		</div>
	</section>

	<!-- ───────────────────────── Deep: Roles & Commands ───────────────────────── -->
	<section class="py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<span class="eyebrow">[ self-serve &amp; extend ]</span>
				<h2 class="mt-3 max-w-2xl text-3xl font-bold tracking-[-0.02em] sm:text-[2.6rem] sm:leading-[1.05]">
					Let members serve themselves. Add your own commands.
				</h2>
			</Reveal>
			<div class="mt-12 space-y-16">
				<Reveal>
					<div>
						<h3 class="text-lg font-semibold">Self-serve roles</h3>
						<p class="mt-1.5 max-w-2xl text-sm leading-relaxed text-muted">Reaction and button menus members assign themselves — toggle, unique-choice or verify modes. Try it:</p>
						<div class="mt-6"><RoleMenuDemo /></div>
					</div>
				</Reveal>
				<Reveal delay={120}>
					<div>
						<h3 class="text-lg font-semibold">Custom commands</h3>
						<p class="mt-1.5 max-w-2xl text-sm leading-relaxed text-muted">Author slash commands in the dashboard and watch the Discord reply update live. No code:</p>
						<div class="mt-6"><CommandBuilderDemo /></div>
					</div>
				</Reveal>
			</div>
		</div>
	</section>

	<!-- ───────────────────────── Deep: Realtime dashboard ───────────────────────── -->
	<section class="border-t border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<span class="eyebrow">[ control ]</span>
				<div class="mt-3 grid items-end gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-bold tracking-[-0.02em] sm:text-[2.6rem] sm:leading-[1.05] lg:col-span-7">
						One state, two windows.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-5">
						The dashboard and your server are the same state, synced both ways. Create a channel in
						Discord and it appears in Dia with no refresh — change a setting in Dia and it’s live at once.
					</p>
				</div>
			</Reveal>
			<Reveal delay={100}>
				<div class="mt-12"><RealtimeSyncDemo /></div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── Self-host ───────────────────────── -->
	<section class="border-y border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto grid max-w-page items-center gap-12 px-6 lg:grid-cols-2">
			<Reveal>
				<div>
					<span class="eyebrow">[ open source ]</span>
					<h2 class="mt-3 text-3xl font-bold tracking-[-0.02em] sm:text-4xl">Yours to run, end to end.</h2>
					<p class="mt-5 text-lg leading-relaxed text-muted">
						Dia is MIT-licensed and self-hostable: an Elixir gateway, a Go API and worker, Postgres,
						Redis and NATS. One command brings the whole stack up.
					</p>
					<div class="mt-6 flex flex-wrap gap-2">
						{#each stack as t (t)}
							<span class="rounded-md border border-line-strong bg-bg px-2.5 py-1 font-mono text-xs text-muted">{t}</span>
						{/each}
					</div>
					<div class="mt-7 flex flex-wrap gap-3">
						<a href="https://github.com/dia-bot/dia" class="btn btn-primary h-11 px-5">View the source</a>
						<a href="https://github.com/dia-bot/dia/tree/main/deploy" class="btn btn-ghost h-11 px-5">Read the docs <ArrowRight size={16} /></a>
					</div>
				</div>
			</Reveal>
			<Reveal delay={120}><Terminal /></Reveal>
		</div>
	</section>

	<!-- ───────────────────────── FAQ ───────────────────────── -->
	<section class="py-20 sm:py-28">
		<div class="mx-auto grid max-w-page gap-12 px-6 lg:grid-cols-[0.8fr_1.2fr]">
			<Reveal>
				<div class="lg:sticky lg:top-24 self-start">
					<span class="eyebrow">[ questions ]</span>
					<h2 class="mt-3 text-3xl font-bold tracking-[-0.02em] sm:text-4xl">Before you add it.</h2>
					<p class="mt-4 max-w-sm text-base leading-relaxed text-muted">
						The things serious admins ask first. Still unsure? <a href="/contact" class="font-medium text-accent-ink hover:text-accent">Get in touch</a>.
					</p>
				</div>
			</Reveal>
			<Reveal delay={80}>
				<div class="divide-y divide-line border-y border-line">
					{#each faqs as f, i (f.q)}
						<details class="group/faq">
							<summary class="flex cursor-pointer list-none items-center gap-4 rounded-lg py-5 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent">
								<span class="font-mono text-xs text-faint">{String(i + 1).padStart(2, '0')}</span>
								<span class="flex-1 text-base font-semibold">{f.q}</span>
								<span class="shrink-0 text-faint transition-transform duration-200 group-open/faq:rotate-45">
									<svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true"><path d="M8 1v14M1 8h14" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" /></svg>
								</span>
							</summary>
							<p class="max-w-2xl pb-5 pl-9 text-[15px] leading-relaxed text-muted">{f.a}</p>
						</details>
					{/each}
				</div>
			</Reveal>
		</div>
	</section>

	<CtaSection heading="Add Dia to your community." href={cta} />
	<SiteFooter />
</div>

<style>
	/* ── Hero welcome moment: climax frame + timeline nodes ── */
	/* Climax frame: the WelcomeCard is lifted above the transcript with
	   dark-correct elevation only — a 1px lit top edge, a single 1px
	   brand-tinted ring (the one allowed brand edge), and a black ambient
	   shadow. No light drop shadow, no gradient wash. */
	.wc-frame {
		box-shadow:
			inset 0 1px 0 rgba(255, 255, 255, 0.06),
			0 0 0 1px rgba(178, 68, 252, 0.18),
			0 24px 56px -20px rgba(0, 0, 0, 0.78);
	}

	/* Timeline node: a small numbered mono tick pinned into the spine gutter,
	   to the LEFT of the message track but still INSIDE the window frame (the
	   34px gutter column lives in DiscordWindow's padded message area). */
	.beat-node {
		position: absolute;
		left: -26px;
		top: 0.375rem;
		display: none;
		place-items: center;
		width: 22px;
		height: 22px;
		border-radius: 999px;
		background: var(--color-bg);
		border: 1px solid var(--color-line-strong);
		font-family: var(--font-mono);
		font-size: 9.5px;
		font-weight: 600;
		letter-spacing: 0.04em;
		color: var(--color-faint);
		line-height: 1;
	}
	/* beat 01 sits centered on its single-line row */
	.beat-node-01 {
		position: static;
		display: none;
	}
	@media (min-width: 640px) {
		.beat-node,
		.beat-node-01 {
			display: grid;
		}
		.beat-node-01 {
			position: absolute;
			left: -26px;
			top: 50%;
			transform: translateY(-50%);
		}
	}
	/* the climax node lights with the solid accent — one tiny deliberate colour event */
	.beat-node-lit {
		border-color: color-mix(in srgb, var(--color-accent) 60%, transparent);
		color: var(--color-accent-ink);
		box-shadow: 0 0 0 4px color-mix(in srgb, var(--color-accent) 12%, transparent);
	}
	/* Entrance polish for the lit node ONLY — a gentle ring settle. Reveal
	   handles the lift; this never transforms the Reveal-lifted element, so
	   no double-bounce. Fully gated behind reduced-motion. */
	@media (prefers-reduced-motion: no-preference) {
		.beat-node-lit {
			animation: beat-light 0.7s ease-out both;
			animation-delay: 0.62s;
		}
		@keyframes beat-light {
			from {
				box-shadow: 0 0 0 0 color-mix(in srgb, var(--color-accent) 0%, transparent);
				border-color: var(--color-line-strong);
				color: var(--color-faint);
			}
			to {
				box-shadow: 0 0 0 4px color-mix(in srgb, var(--color-accent) 12%, transparent);
				border-color: color-mix(in srgb, var(--color-accent) 60%, transparent);
				color: var(--color-accent-ink);
			}
		}
	}

	/* ── Welcome studio bench: drafting frame + crop ticks ── */
	/* The bench frame: a hairline gutter around the editor with a single deliberate lit
	   top-edge — the only place a gradient touches a surface here, and only as a 1px edge
	   (border-box), never a wash. Depth comes from the .card recipe + this lit edge. */
	.bench-frame {
		padding: 14px;
		border: 1px solid var(--color-line);
		border-radius: 20px;
		background:
			linear-gradient(var(--color-ink-2), var(--color-ink-2)) padding-box,
			/* 1px lit edge: rose→violet only along the very top, fading to the hairline */
				linear-gradient(180deg, rgba(255, 99, 99, 0.35), var(--color-line) 2px, var(--color-line))
				border-box;
		box-shadow:
			inset 0 1px 0 rgba(255, 255, 255, 0.04),
			0 28px 60px -30px rgba(0, 0, 0, 0.8);
	}
	@media (min-width: 640px) {
		.bench-frame {
			padding: 22px;
		}
	}
	.bench-inset {
		border-radius: 12px;
		overflow: hidden;
	}

	/* Crop-mark corner ticks — drafting motif. Two 1px hairlines per corner. Static. */
	.tick {
		position: absolute;
		width: 14px;
		height: 14px;
		pointer-events: none;
		opacity: 0.7;
	}
	.tick::before,
	.tick::after {
		content: '';
		position: absolute;
		background: var(--color-line-strong);
	}
	.tick::before {
		width: 14px;
		height: 1px;
	}
	.tick::after {
		width: 1px;
		height: 14px;
	}
	.tick-tl {
		top: -1px;
		left: -1px;
	}
	.tick-tl::before {
		top: 0;
		left: 0;
	}
	.tick-tl::after {
		top: 0;
		left: 0;
	}
	.tick-tr {
		top: -1px;
		right: -1px;
	}
	.tick-tr::before {
		top: 0;
		right: 0;
	}
	.tick-tr::after {
		top: 0;
		right: 0;
	}
	.tick-bl {
		bottom: -1px;
		left: -1px;
	}
	.tick-bl::before {
		bottom: 0;
		left: 0;
	}
	.tick-bl::after {
		bottom: 0;
		left: 0;
	}
	.tick-br {
		bottom: -1px;
		right: -1px;
	}
	.tick-br::before {
		bottom: 0;
		right: 0;
	}
	.tick-br::after {
		bottom: 0;
		right: 0;
	}
</style>
