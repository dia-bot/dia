<script lang="ts">
	import SiteNav from '$lib/components/marketing/SiteNav.svelte';
	import SiteFooter from '$lib/components/marketing/SiteFooter.svelte';
	import PageHero from '$lib/components/marketing/PageHero.svelte';
	import CtaSection from '$lib/components/marketing/CtaSection.svelte';
	import Reveal from '$lib/components/marketing/Reveal.svelte';
	import DiscordWindow from '$lib/components/marketing/DiscordWindow.svelte';
	import DiscordMessage from '$lib/components/marketing/DiscordMessage.svelte';
	import DiscordEmbed from '$lib/components/marketing/DiscordEmbed.svelte';
	import AutomodDemo from '$lib/components/marketing/AutomodDemo.svelte';
	import CaseLogDemo from '$lib/components/marketing/CaseLogDemo.svelte';

	import { loginURL } from '$lib/api';
	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Check from 'lucide-svelte/icons/check';
	import Gavel from 'lucide-svelte/icons/gavel';
	import Boot from 'lucide-svelte/icons/log-out';
	import Clock from 'lucide-svelte/icons/clock';
	import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
	import MessageSquare from 'lucide-svelte/icons/message-square';
	import ScrollText from 'lucide-svelte/icons/scroll-text';
	import Shield from 'lucide-svelte/icons/shield';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import EyeOff from 'lucide-svelte/icons/eye-off';

	let { data }: { data: { user?: { username: string } | null } } = $props();
	const cta = $derived(data.user ? '/servers' : loginURL);

	const mention = 'rounded bg-[#b244fc]/25 px-1 font-medium text-[#d9b8ff]';

	// The four manual moderation slash commands, every one with an optional reason
	// that is written verbatim into the case log.
	const commands = [
		{
			icon: Gavel,
			name: '/ban',
			usage: '/ban user reason',
			body: 'Remove a member and block their return. The reason is recorded on the case.'
		},
		{
			icon: Boot,
			name: '/kick',
			usage: '/kick user reason',
			body: 'Remove a member without a ban — they can rejoin with a fresh invite.'
		},
		{
			icon: Clock,
			name: '/timeout',
			usage: '/timeout user duration reason',
			body: 'Mute a member for a set duration so the conversation can cool off.'
		},
		{
			icon: TriangleAlert,
			name: '/warn',
			usage: '/warn user reason',
			body: 'Put a member on record with a formal warning — no removal, full history.'
		}
	];

	// AutoMod rule types. Filters are independent toggles; the response to any of
	// them is one shared, server-wide action (see the action note below).
	const rules = [
		{ title: 'Block invites', body: 'Catch Discord invite links before they spread, including obfuscated discord.gg variants.' },
		{ title: 'Block links', body: 'Strip URLs from chat to shut down phishing, promo spam and drive-by self-promotion.' },
		{ title: 'Max mentions', body: 'Cap how many mentions a single message may carry — set the threshold, or 0 to disable.' },
		{ title: 'Banned words', body: 'Maintain a word list; any message containing one trips the filter.' }
	];

	// AutoMod always deletes first, then optionally escalates — one global action,
	// not a per-rule one.
	const actions = [
		{ icon: Trash2, label: 'Delete only', body: 'The message is removed. Nothing else happens.' },
		{ icon: TriangleAlert, label: 'Delete + warn', body: 'The message is removed and the member is warned, with a case logged.' },
		{ icon: Clock, label: 'Delete + timeout', body: 'The message is removed and the member is timed out for the duration you set.' }
	];
</script>

<svelte:head>
	<title>Moderation &amp; AutoMod · Dia</title>
	<meta
		name="description"
		content="Dia's moderation: /ban /kick /timeout /warn with reasons, optional DMs and a numbered, searchable case log — plus AutoMod that blocks invites, links, mention spam and banned words with one server-wide action and per-channel and per-role exemptions."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} />

	<PageHero
		eyebrow="[ feature · moderation ]"
		title="Keep the peace, on the record."
		lede="Ban, kick, timeout and warn with slash commands that write every action to a numbered, searchable case log. Then let AutoMod catch invites, links, mention spam and banned words before anyone has to."
	>
		<div class="flex flex-wrap items-center gap-3">
			<a href={cta} class="btn btn-accent h-12 px-6 text-base">Get started <ArrowRight size={18} /></a>
			<a href="/#features" class="btn btn-ghost h-12 px-5 text-base">All features</a>
		</div>
		<div class="mt-6 flex flex-wrap items-center gap-x-5 gap-y-2 font-mono text-xs text-muted">
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> Slash-command native</span>
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> Numbered case log</span>
			<span class="inline-flex items-center gap-1.5"><Check size={14} class="text-accent" /> AutoMod built in</span>
		</div>
	</PageHero>

	<!-- ───────────────────────── 01 · Manual moderation ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">01</span>
					<span class="eyebrow">Manual moderation</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						Four commands. Every one leaves a trail.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							Moderation is slash-command native — no prefixes to remember, no message bots to babysit.
							Each command takes an optional reason that is stored verbatim on the case, members can be
							DM'd when they're actioned, and everything lands in the channel you choose.
						</p>
						<ul class="mt-5 grid gap-2.5 sm:grid-cols-2">
							<li class="flex items-start gap-2 text-sm text-ink/80"><Check size={16} class="mt-0.5 shrink-0 text-accent" />Reasons recorded on every action</li>
							<li class="flex items-start gap-2 text-sm text-ink/80"><Check size={16} class="mt-0.5 shrink-0 text-accent" />Optional DM to the member on action</li>
							<li class="flex items-start gap-2 text-sm text-ink/80"><Check size={16} class="mt-0.5 shrink-0 text-accent" />One configurable mod-log channel</li>
							<li class="flex items-start gap-2 text-sm text-ink/80"><Check size={16} class="mt-0.5 shrink-0 text-accent" />Permission-gated to your moderators</li>
						</ul>
					</div>
				</div>
			</Reveal>

			<div class="mt-12 grid items-start gap-6 lg:grid-cols-2">
				<Reveal delay={80}>
					<div class="card divide-y divide-line overflow-hidden">
						{#each commands as c (c.name)}
							<div class="flex items-start gap-4 p-5">
								<span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-blush text-accent">
									<c.icon size={18} />
								</span>
								<div class="min-w-0">
									<div class="flex flex-wrap items-baseline gap-x-3 gap-y-1">
										<span class="font-mono text-sm font-semibold text-accent-ink">{c.name}</span>
										<code class="font-mono text-xs text-faint">{c.usage}</code>
									</div>
									<p class="mt-1 text-sm leading-relaxed text-muted">{c.body}</p>
								</div>
							</div>
						{/each}
					</div>
				</Reveal>

				<Reveal delay={160}>
					<div>
						<div class="mb-2 font-mono text-xs uppercase tracking-wide text-muted">Mod log · live</div>
						<DiscordWindow channel="mod-log" title="Aurora" topic="Moderation actions" members="1,284 online" channels={['general', 'mod-log', 'reports']}>
							<DiscordMessage author="maya" color="#1aa179" time="Today at 2:41 PM">
								<code class="rounded bg-black/30 px-1 font-mono text-[13px] text-[#dbdee1]">/timeout</code>
								<span class={mention}>@troll22</span> duration: 1h reason: spamming after a warning
							</DiscordMessage>
							<DiscordMessage brand author="Dia" time="Today at 2:41 PM">
								<DiscordEmbed
									color="#B244FC"
									title="Case #142 · Timeout"
									fields={[
										{ name: 'Member', value: '@troll22' },
										{ name: 'Moderator', value: '@maya' },
										{ name: 'Duration', value: '1 hour' },
										{ name: 'Reason', value: 'Spamming after a warning' }
									]}
									footer="Member notified by DM · just now"
									footerIcon
								/>
							</DiscordMessage>
						</DiscordWindow>
					</div>
				</Reveal>
			</div>
		</div>
	</section>

	<!-- ───────────────────────── 02 · Case log ───────────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">02</span>
					<span class="eyebrow">Case log</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						A numbered record you can actually search.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							Every manual action and every AutoMod removal becomes a case with its own number —
							the member, the moderator, the reason and the time, all in one place. Cases are
							sequential, so nothing goes missing and a moderator can be brought up to speed in
							seconds.
						</p>
						<ul class="mt-5 grid gap-2.5 sm:grid-cols-2">
							<li class="flex items-start gap-2 text-sm text-ink/80"><Check size={16} class="mt-0.5 shrink-0 text-accent" />Sequential case numbers</li>
							<li class="flex items-start gap-2 text-sm text-ink/80"><Check size={16} class="mt-0.5 shrink-0 text-accent" />Member, moderator &amp; reason</li>
							<li class="flex items-start gap-2 text-sm text-ink/80"><Check size={16} class="mt-0.5 shrink-0 text-accent" />Manual and AutoMod actions together</li>
							<li class="flex items-start gap-2 text-sm text-ink/80"><Check size={16} class="mt-0.5 shrink-0 text-accent" />Reviewable from the dashboard</li>
						</ul>
					</div>
				</div>
			</Reveal>

			<Reveal delay={80}>
				<div class="mt-12">
					<CaseLogDemo />
				</div>
			</Reveal>

			<Reveal delay={140}>
				<div class="mt-6 flex items-start gap-2 font-mono text-xs leading-relaxed text-muted">
					<ScrollText size={14} class="mt-0.5 shrink-0 text-accent-ink" />
					<span>Cases are written the moment an action lands and stay on record — review your server's full history from the dashboard.</span>
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── 03 · AutoMod ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">03</span>
					<span class="eyebrow">AutoMod</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						Catch the obvious stuff before anyone sees it.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							Turn on the filters you want and AutoMod watches every message. The moment one trips,
							the message is removed and — if you've asked it to — the member is warned or timed out,
							with a case logged automatically.
						</p>
					</div>
				</div>
			</Reveal>

			<div class="mt-12 grid items-start gap-6 lg:grid-cols-2">
				<Reveal delay={80}>
					<div>
						<div class="mb-2 font-mono text-xs uppercase tracking-wide text-muted">AutoMod · live</div>
						<AutomodDemo />
					</div>
				</Reveal>

				<Reveal delay={160}>
					<div class="card p-6">
						<h3 class="flex items-center gap-2 text-[15px] font-semibold">
							<Shield size={16} class="text-accent" /> The rules
						</h3>
						<div class="mt-4 divide-y divide-line">
							{#each rules as r (r.title)}
								<div class="py-3.5 first:pt-0 last:pb-0">
									<div class="text-sm font-semibold text-ink">{r.title}</div>
									<p class="mt-1 text-sm leading-relaxed text-muted">{r.body}</p>
								</div>
							{/each}
						</div>
					</div>
				</Reveal>
			</div>

			<!-- One global action — accuracy: not per-rule -->
			<Reveal delay={80}>
				<div class="mt-12">
					<div class="rounded-2xl bg-[#111113] p-8 sm:p-10">
						<span class="font-mono text-xs uppercase tracking-wider text-accent-ink">One action, every violation</span>
						<h3 class="mt-3 max-w-2xl text-2xl font-extrabold tracking-tight text-white sm:text-3xl">
							AutoMod always deletes first — then escalates, if you ask.
						</h3>
						<p class="mt-4 max-w-2xl text-[15px] leading-relaxed text-white/65">
							There's one global action for every rule, not a different one per filter. AutoMod removes
							the offending message, then optionally warns or times the member out. Pick the response
							once and it applies to invites, links, mention spam and banned words alike.
						</p>
						<div class="mt-7 grid gap-px overflow-hidden rounded-xl bg-white/10 sm:grid-cols-3">
							{#each actions as a (a.label)}
								<div class="bg-[#111113] p-5">
									<span class="grid h-9 w-9 place-items-center rounded-lg bg-white/[0.06] text-[#c79bff]">
										<a.icon size={17} />
									</span>
									<div class="mt-3 text-sm font-semibold text-white">{a.label}</div>
									<p class="mt-1 text-[13px] leading-relaxed text-white/55">{a.body}</p>
								</div>
							{/each}
						</div>
						<div class="mt-6 flex items-start gap-2 text-[13px] leading-relaxed text-white/55">
							<EyeOff size={15} class="mt-0.5 shrink-0 text-[#c79bff]" />
							<span>
								Exempt the spaces that don't need policing: AutoMod skips any channels you ignore, and
								members with an exempt role are never moderated.
							</span>
						</div>
					</div>
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── How it fits together ───────────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-12">
					<span class="eyebrow">Why it holds up</span>
					<h2 class="mt-3 text-3xl font-extrabold tracking-tight sm:text-4xl">Manual and automatic, one trail.</h2>
				</div>
			</Reveal>
			<div class="grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-3">
				<Reveal class="bg-surface">
					<div class="h-full p-7">
						<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent"><MessageSquare size={19} /></span>
						<h3 class="mt-4 text-lg font-semibold">Slash-command native</h3>
						<p class="mt-1.5 text-sm leading-relaxed text-muted">No prefixes, no message scanning to run your tools. Commands are gated by Discord permissions.</p>
					</div>
				</Reveal>
				<Reveal delay={90} class="bg-surface">
					<div class="h-full p-7">
						<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent"><ScrollText size={19} /></span>
						<h3 class="mt-4 text-lg font-semibold">Shared case log</h3>
						<p class="mt-1.5 text-sm leading-relaxed text-muted">Moderator actions and AutoMod removals share one numbered record — so the history is always complete.</p>
					</div>
				</Reveal>
				<Reveal delay={180} class="bg-surface">
					<div class="h-full p-7">
						<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent"><Shield size={19} /></span>
						<h3 class="mt-4 text-lg font-semibold">Tunable, not rigid</h3>
						<p class="mt-1.5 text-sm leading-relaxed text-muted">Choose the filters, the action and the exemptions from a dashboard secured by Discord login.</p>
					</div>
				</Reveal>
			</div>

			<Reveal delay={80}>
				<p class="mt-10 max-w-2xl text-sm leading-relaxed text-faint">
					Dia is free and open source under the MIT licence, and self-hostable — the entire stack runs
					on your own infrastructure if you'd rather host it yourself. No premium tiers gate moderation.
				</p>
			</Reveal>
		</div>
	</section>

	<CtaSection href={cta} heading="Keep the peace, on the record." sub="Manual tools plus AutoMod, every action logged to a case." />
	<SiteFooter />
</div>
