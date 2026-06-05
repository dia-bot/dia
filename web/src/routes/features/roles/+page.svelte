<script lang="ts">
	import SiteNav from '$lib/components/marketing/SiteNav.svelte';
	import SiteFooter from '$lib/components/marketing/SiteFooter.svelte';
	import PageHero from '$lib/components/marketing/PageHero.svelte';
	import CtaSection from '$lib/components/marketing/CtaSection.svelte';
	import Reveal from '$lib/components/marketing/Reveal.svelte';
	import RoleMenuDemo from '$lib/components/marketing/RoleMenuDemo.svelte';
	import DiscordWindow from '$lib/components/marketing/DiscordWindow.svelte';
	import DiscordMessage from '$lib/components/marketing/DiscordMessage.svelte';
	import { loginURL } from '$lib/api';

	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Check from 'lucide-svelte/icons/check';
	import ToggleRight from 'lucide-svelte/icons/toggle-right';
	import Palette from 'lucide-svelte/icons/palette';
	import ShieldCheck from 'lucide-svelte/icons/shield-check';
	import UserPlus from 'lucide-svelte/icons/user-plus';
	import Bot from 'lucide-svelte/icons/bot';
	import ClipboardCheck from 'lucide-svelte/icons/clipboard-check';
	import MousePointerClick from 'lucide-svelte/icons/mouse-pointer-click';

	let { data }: { data: { user?: { username: string } | null } } = $props();
	const cta = $derived(data.user ? '/servers' : loginURL);

	const mention = 'rounded bg-[#b244fc]/25 px-1 font-medium text-[#d9b8ff]';

	const modes = [
		{
			id: 'toggle',
			icon: ToggleRight,
			name: 'Toggle',
			tag: 'Add & remove freely',
			body: 'The default. Members switch any number of roles on and off — one click adds, another removes. Ideal for opt-in interests, ping groups and notification roles.'
		},
		{
			id: 'unique',
			icon: Palette,
			name: 'Unique',
			tag: 'One at a time',
			body: 'Only one role from the menu sticks at a time — picking another swaps it. Perfect for colour roles or pronoun sets where exactly one choice should win.'
		},
		{
			id: 'verify',
			icon: ShieldCheck,
			name: 'Verify',
			tag: 'Single-use opt-in',
			body: 'A one-click gate. The button grants a single role once — to confirm the rules and unlock the rest of the server. After that, there is nothing left to toggle.'
		}
	];

	const autoBullets = [
		{
			icon: UserPlus,
			title: 'Assign on join',
			body: 'Pick any number of roles and every new member receives them automatically the moment they arrive — no menu, no manual hand-out.'
		},
		{
			icon: Bot,
			title: 'Include bots, or not',
			body: 'Bots that join are skipped by default. Flip one switch to hand them the same roles when you want them grouped or coloured too.'
		},
		{
			icon: ClipboardCheck,
			title: 'Wait for screening',
			body: 'If your server uses membership screening, hold the roles until a member accepts the rules — so unverified accounts never get access.'
		}
	];
</script>

<svelte:head>
	<title>Roles · Dia</title>
	<meta
		name="description"
		content="Self-serve roles for Discord, done right: post a button menu and let members assign their own roles — toggle freely, swap a single colour role, or gate access behind a one-click verify. Plus auto roles that assign on join."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} />

	<PageHero
		eyebrow="[ feature · roles ]"
		title="Self-serve roles, on your terms."
		lede="Post a menu and let members assign their own roles with buttons — no reactions to babysit. Choose how each menu behaves, then let new arrivals get the right roles the moment they join."
	>
		<div class="flex flex-wrap items-center gap-3">
			<a href={cta} class="btn btn-accent h-12 px-6 text-base">Get started <ArrowRight size={18} /></a>
			<a href="/#features" class="btn btn-ghost h-12 px-5 text-base">All features</a>
		</div>
	</PageHero>

	<!-- ───────────────────────── Self-serve menus ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">01</span>
					<span class="eyebrow">Self-serve menus</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						Buttons, not reactions to babysit.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							Build a menu in the dashboard — a title and a list of role options, each with an
							optional label, emoji and description. Members click to assign and click again to
							remove. No stray reactions, no orphaned messages.
						</p>
						<ul class="mt-5 grid gap-2.5 sm:grid-cols-2">
							{#each ['Button-based, instant feedback', 'Label, emoji & description per option', 'As many menus as you need', 'Posts to any channel you choose'] as p (p)}
								<li class="flex items-start gap-2 text-sm text-ink/80">
									<Check size={16} class="mt-0.5 shrink-0 text-accent" />{p}
								</li>
							{/each}
						</ul>
					</div>
				</div>
			</Reveal>

			<Reveal delay={80}>
				<div class="mx-auto mt-12 max-w-2xl"><RoleMenuDemo /></div>
			</Reveal>

			<Reveal delay={120}>
				<div class="mx-auto mt-6 flex max-w-2xl items-center gap-2.5 rounded-xl border border-line bg-blush/40 p-4 text-sm text-muted">
					<MousePointerClick size={16} class="mt-0.5 shrink-0 text-accent-ink" />
					<span>
						Save a menu, then run
						<code class="rounded bg-surface px-1.5 py-0.5 font-mono text-[13px] text-accent-ink"
							>/reactionroles post</code
						>
						to drop it into a channel — ready for members to click.
					</span>
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── Three modes ───────────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">02</span>
					<span class="eyebrow">Three modes</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						One control that decides how a menu behaves.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							Every menu runs in one of three modes. The mode is the whole behaviour — pick it when you
							build the menu and members get exactly the right experience.
						</p>
					</div>
				</div>
			</Reveal>

			<div class="mt-12 grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-3">
				{#each modes as m, i (m.id)}
					<Reveal delay={i * 90} class="bg-surface">
						<div class="flex h-full flex-col p-7">
							<div class="flex items-center gap-3">
								<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent">
									<m.icon size={19} />
								</span>
								<span class="font-mono text-xs text-faint">MODE {String(i + 1).padStart(2, '0')}</span>
							</div>
							<h3 class="mt-4 text-lg font-semibold">{m.name}</h3>
							<div class="mt-1 font-mono text-xs uppercase tracking-wide text-accent-ink">{m.tag}</div>
							<p class="mt-2.5 text-sm leading-relaxed text-muted">{m.body}</p>
						</div>
					</Reveal>
				{/each}
			</div>

			<Reveal delay={120}>
				<p class="mt-6 font-mono text-xs uppercase tracking-wide text-muted">
					Switch modes on the demo above to see each one in action.
				</p>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── Auto roles on join ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">03</span>
					<span class="eyebrow">Auto roles</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						The right roles, the moment they arrive.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							Not everything should wait for a click. Choose the roles every newcomer should have and
							Dia assigns them on join — so members land in the right place from their very first
							second in the server.
						</p>
					</div>
				</div>
			</Reveal>

			<div class="mt-12 grid items-start gap-6 lg:grid-cols-2">
				<Reveal>
					<div class="grid gap-px overflow-hidden rounded-xl border border-line bg-line">
						{#each autoBullets as b (b.title)}
							<div class="bg-surface p-6">
								<div class="flex items-start gap-4">
									<span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-blush text-accent">
										<b.icon size={19} />
									</span>
									<div>
										<h3 class="text-base font-semibold">{b.title}</h3>
										<p class="mt-1 text-sm leading-relaxed text-muted">{b.body}</p>
									</div>
								</div>
							</div>
						{/each}
					</div>
				</Reveal>

				<Reveal delay={120}>
					<div>
						<div class="mb-2 font-mono text-xs uppercase tracking-wide text-muted">On join · live</div>
						<DiscordWindow channel="general" title="Aurora" topic="A member just joined" members="1,284 online">
								<DiscordMessage author="rafa" color="#3ba55c" time="Today at 9:41 AM">
									<span class="text-[#949ba4]">joined the server.</span>
								</DiscordMessage>
							</DiscordWindow>
							<div class="mt-3 flex flex-wrap items-center gap-2 rounded-xl border border-line bg-surface p-3 text-sm">
								<span class="text-muted">rafa's roles, the moment they join:</span>
								<span class="rounded-full bg-blush px-2 py-0.5 text-xs font-medium text-accent-ink">@Member</span>
								<span class="rounded-full bg-blush px-2 py-0.5 text-xs font-medium text-accent-ink">@Unverified</span>
							</div>
							<p class="mt-2 text-xs text-muted">Assigned silently — auto-roles adds the roles without posting a message.</p>
					</div>
				</Reveal>
			</div>
		</div>
	</section>

	<CtaSection href={cta} heading="Let members pick their roles." sub="Post a menu, set the mode, and hand role control to your community." />
	<SiteFooter />
</div>
