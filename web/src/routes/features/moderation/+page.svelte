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
	import Flame from 'lucide-svelte/icons/flame';

	let { data }: { data: { user?: { username: string } | null } } = $props();
	const cta = $derived(data.user ? '/servers' : loginURL);

	const mention = 'rounded bg-[#b244fc]/25 px-1 font-medium text-[#d9b8ff]';

	// Automod detection triggers. Each rule pairs ONE trigger with its own stack of
	// actions, so the catalogue below is the menu you build rules from.
	const triggers = [
		{ title: 'Blocked words', body: 'A word and phrase list, matched as a substring, whole word or wildcard, with per-rule exceptions.' },
		{ title: 'Regex filters', body: 'Power-user RE2 patterns for structured spam, leetspeak and formats a word list cannot catch.' },
		{ title: 'Discord invites', body: 'Catch invite links to other servers, including obfuscated discord.gg variants. Allow specific codes.' },
		{ title: 'Links', body: 'Block every URL, allow only listed domains, or block only listed domains.' },
		{ title: 'Spam and flood', body: 'Trip when a member sends too many messages within a short window, across any channel.' },
		{ title: 'Duplicates', body: 'Catch the same message repeated over and over inside a window.' },
		{ title: 'Mention spam', body: 'Cap how many distinct users a single message may ping.' },
		{ title: 'Mass mentions', body: 'Stop @everyone, @here and role pings before a ping raid spreads.' },
		{ title: 'Excessive caps', body: 'Flag messages that are mostly uppercase, ignoring short ones to avoid false positives.' },
		{ title: 'Excessive emoji', body: 'Catch messages packed with too many unicode or custom emoji.' },
		{ title: 'Walls of text', body: 'Trip on messages with too many line breaks.' },
		{ title: 'Disruptive text', body: 'Detect zalgo and glitch text used to disrupt a channel.' },
		{ title: 'Spoiler spam', body: 'Limit how many spoiler spans one message may carry.' },
		{ title: 'Attachment flood', body: 'Cap how many attachments arrive in a single message.' },
		{ title: 'New account gate', body: 'Flag members whose account is younger than a threshold on join. Pair with a quarantine role.' },
		{ title: 'Username filter', body: 'Block bad usernames and nicknames on join and on change, by word or pattern.' }
	];

	// A layered action stack, run in order when a rule fires.
	const actionStack = [
		{ icon: Trash2, label: 'Delete', body: 'Remove the offending message.' },
		{ icon: TriangleAlert, label: 'Warn', body: 'Record a warn case and DM the member.' },
		{ icon: Clock, label: 'Timeout', body: 'Mute the member for a set duration.' },
		{ icon: Boot, label: 'Kick or ban', body: 'Remove the member, with optional message purge.' },
		{ icon: Shield, label: 'Role', body: 'Add a quarantine role or revoke an existing one.' },
		{ icon: MessageSquare, label: 'Notify', body: 'Post a notice or DM, then add infraction points.' }
	];

	// The escalation ladder: infraction points accumulate per member (with decay)
	// and crossing a threshold auto-applies a heavier action.
	const ladder = [
		{ points: '3 pts', label: 'Timeout', body: 'A short mute once a member starts collecting points.' },
		{ points: '5 pts', label: 'Longer timeout', body: 'The mute lengthens as the heat keeps climbing.' },
		{ points: '8 pts', label: 'Kick', body: 'Persistent offenders are removed from the server.' },
		{ points: '12 pts', label: 'Ban', body: 'The worst repeat offenders are blocked for good.' }
	];

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

</script>

<svelte:head>
	<title>Moderation &amp; AutoMod · Dia</title>
	<meta
		name="description"
		content="Dia's moderation: /ban /kick /timeout /warn with reasons, optional DMs and a numbered, searchable case log, plus a rule-based AutoMod with many detection triggers (words, regex, invites, links, spam, mentions, caps, new-account gate and more), layered actions and an escalation ladder that climbs from warn to timeout to kick to ban as infraction points add up."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} />

	<PageHero
		eyebrow="[ feature · moderation ]"
		title="Keep the peace, on the record."
		lede="Ban, kick, timeout and warn with slash commands that write every action to a numbered, searchable case log. Then let a rule-based AutoMod catch words, regex, invites, links, spam, mention floods and risky new accounts, and climb an escalation ladder for repeat offenders."
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
						Build the rules. Stack the response.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							AutoMod is a list of rules, each one pairing a single detection trigger with its own
							stack of actions. Pick from many triggers (words, regex, invites, links, spam, mention
							floods, formatting and member checks), then decide exactly what happens when each fires:
							delete, warn, timeout, role, kick or ban, plus infraction points. Rules run in order and
							the first match wins, so behaviour stays predictable.
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
							<Shield size={16} class="text-accent" /> Detection triggers
						</h3>
						<p class="mt-1.5 text-sm leading-relaxed text-muted">
							Sixteen triggers, each the start of a rule you tune to your server.
						</p>
						<div class="mt-4 grid gap-2 sm:grid-cols-2">
							{#each triggers as t (t.title)}
								<div class="rounded-lg border border-line bg-surface p-3">
									<div class="text-[13px] font-semibold text-ink">{t.title}</div>
									<p class="mt-0.5 text-xs leading-relaxed text-muted">{t.body}</p>
								</div>
							{/each}
						</div>
					</div>
				</Reveal>
			</div>

			<!-- Layered actions per rule -->
			<Reveal delay={80}>
				<div class="mt-12">
					<div class="rounded-2xl bg-[#111113] p-8 sm:p-10">
						<span class="font-mono text-xs uppercase tracking-wider text-accent-ink">Layered actions</span>
						<h3 class="mt-3 max-w-2xl text-2xl font-extrabold tracking-tight text-white sm:text-3xl">
							Each rule decides exactly what happens.
						</h3>
						<p class="mt-4 max-w-2xl text-[15px] leading-relaxed text-white/65">
							A rule can run several actions in order: delete the message, warn the member, time them
							out, add a quarantine role or revoke one, post a notice or DM, and award infraction
							points. Mix them per rule so a stray link is handled differently from a ping raid.
						</p>
						<div class="mt-7 grid gap-px overflow-hidden rounded-xl bg-white/10 sm:grid-cols-2 lg:grid-cols-3">
							{#each actionStack as a (a.label)}
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
								Exempt the spaces that don't need policing: skip any channels and roles globally or
								per rule, and let staff and bots pass automatically.
							</span>
						</div>
					</div>
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── 04 · Escalation ladder ───────────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">04</span>
					<span class="eyebrow">Escalation ladder</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						Repeat offenders climb the ladder on their own.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							Actions can award infraction points that accumulate per member and decay over time.
							Cross a threshold and AutoMod applies a heavier action automatically, walking from a
							warn to a timeout to a kick to a ban as the heat builds. First-timers get a nudge,
							persistent abusers get removed, and you never have to track it by hand.
						</p>
					</div>
				</div>
			</Reveal>

			<Reveal delay={80}>
				<div class="mt-12 grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-2 lg:grid-cols-4">
					{#each ladder as rung (rung.label)}
						<div class="bg-surface p-6">
							<span class="font-mono text-xs font-semibold text-accent-ink">{rung.points}</span>
							<div class="mt-2 text-lg font-semibold">{rung.label}</div>
							<p class="mt-1 text-sm leading-relaxed text-muted">{rung.body}</p>
						</div>
					{/each}
				</div>
			</Reveal>

			<Reveal delay={140}>
				<div class="mt-6 flex items-start gap-2 font-mono text-xs leading-relaxed text-muted">
					<Flame size={14} class="mt-0.5 shrink-0 text-accent-ink" />
					<span>
						Every AutoMod hit also fires an event, so you can route it into Automations, post a
						staff alert, open a ticket or run any custom flow you build.
					</span>
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
						<p class="mt-1.5 text-sm leading-relaxed text-muted">Build rules, stack actions, set the escalation ladder and exemptions from a dashboard secured by Discord login.</p>
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
