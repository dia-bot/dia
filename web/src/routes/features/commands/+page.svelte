<script lang="ts">
	import SiteNav from '$lib/components/marketing/SiteNav.svelte';
	import SiteFooter from '$lib/components/marketing/SiteFooter.svelte';
	import PageHero from '$lib/components/marketing/PageHero.svelte';
	import CtaSection from '$lib/components/marketing/CtaSection.svelte';
	import Reveal from '$lib/components/marketing/Reveal.svelte';
	import CommandBuilderDemo from '$lib/components/marketing/CommandBuilderDemo.svelte';
	import { loginURL } from '$lib/api';

	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import MessageSquare from 'lucide-svelte/icons/message-square';
	import LayoutPanelTop from 'lucide-svelte/icons/layout-panel-top';
	import Eye from 'lucide-svelte/icons/eye';
	import EyeOff from 'lucide-svelte/icons/eye-off';
	import Braces from 'lucide-svelte/icons/braces';
	import Type from 'lucide-svelte/icons/type';
	import Zap from 'lucide-svelte/icons/zap';
	import Check from 'lucide-svelte/icons/check';

	let { data }: { data: { user?: { username: string } | null } } = $props();
	const cta = $derived(data.user ? '/servers' : loginURL);

	// Placeholders supported in command responses. Literal braces are rendered
	// with {'{…}'} so svelte-check treats them as text, not interpolation.
	const placeholders = [
		{ token: '{user}', desc: "The member's display name — e.g. maya." },
		{ token: '{user.mention}', desc: 'A clickable @mention that pings the member.' },
		{ token: '{server}', desc: 'The name of the current server.' }
	];

	// The two response shapes a command can take.
	const responseKinds = [
		{
			icon: MessageSquare,
			title: 'Text response',
			body: 'A plain message reply. Quick to write, supports Discord markdown, and reads like any other chat message.'
		},
		{
			icon: LayoutPanelTop,
			title: 'Rich embed',
			body: 'A bordered card with a coloured accent bar, title, description and image — for rules, info panels and announcements.'
		}
	];

	// Public vs. ephemeral visibility.
	const visibility = [
		{
			icon: Eye,
			title: 'Public',
			body: 'The reply posts to the channel for everyone to see — the default for most commands.'
		},
		{
			icon: EyeOff,
			title: 'Ephemeral',
			body: 'Only the member who ran the command sees the reply. It vanishes on dismiss and never clutters the channel.'
		}
	];

	// Example commands shown as small tiles.
	const examples = [
		{ name: '/rules', desc: 'Post the server rules in a tidy embed.' },
		{ name: '/poll', desc: 'Drop a quick yes/no prompt for the channel.' },
		{ name: '/ping', desc: 'A friendly health check that replies to the caller.' }
	];
</script>

<svelte:head>
	<title>Custom commands · Dia</title>
	<meta
		name="description"
		content="Build your own Discord slash commands from Dia's dashboard — plain text replies or rich embeds, public or ephemeral, with member and server placeholders. No code, registered to your server instantly."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} />

	<PageHero
		eyebrow="[ feature · commands ]"
		title="Build slash commands without writing code."
		lede="Design your own /commands from the dashboard — plain replies or rich embeds, public or ephemeral, with placeholders that fill in the member and server. They register to your server the moment you save."
	>
		<div class="flex flex-wrap items-center gap-3">
			<a href={cta} class="btn btn-accent h-12 px-6 text-base">
				Get started <ArrowRight size={18} />
			</a>
			<a href="/#features" class="btn btn-ghost h-12 px-5 text-base">All features</a>
		</div>
	</PageHero>

	<!-- ───────────────────────── 01 · Builder ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">01</span>
					<span class="eyebrow">Builder</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2
						class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6"
					>
						Compose it on the left, watch Discord update on the right.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							Give the command a name and description, type the reply, and attach an embed if you want
							one. The preview shows exactly how the command appears in Discord's picker and how Dia
							answers when a member runs it — no save-and-check loop.
						</p>
						<ul class="mt-5 grid gap-2.5 sm:grid-cols-2">
							{#each ['Live Discord preview as you type', 'Text reply, rich embed, or both', 'Public or ephemeral visibility', 'Enable or disable without deleting'] as p (p)}
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
					<CommandBuilderDemo />
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────────── 02 · Anatomy ───────────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">02</span>
					<span class="eyebrow">Anatomy</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2
						class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6"
					>
						Four choices shape every command.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							A command is a name, a description, and a response. The response decides how it reads,
							who sees it, and what it knows about the member who ran it.
						</p>
					</div>
				</div>
			</Reveal>

			<!-- response shape -->
			<Reveal delay={60}>
				<div class="mt-12">
					<div class="mb-3 font-mono text-xs uppercase tracking-wide text-muted">
						Response — text or embed
					</div>
					<div class="grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-2">
						{#each responseKinds as r (r.title)}
							<div class="bg-surface p-6">
								<div class="flex items-center gap-3">
									<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent">
										<r.icon size={19} />
									</span>
									<h3 class="text-lg font-semibold">{r.title}</h3>
								</div>
								<p class="mt-3 text-sm leading-relaxed text-muted">{r.body}</p>
							</div>
						{/each}
					</div>
				</div>
			</Reveal>

			<!-- visibility -->
			<Reveal delay={120}>
				<div class="mt-6">
					<div class="mb-3 font-mono text-xs uppercase tracking-wide text-muted">
						Visibility — public or ephemeral
					</div>
					<div class="grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-2">
						{#each visibility as v (v.title)}
							<div class="bg-surface p-6">
								<div class="flex items-center gap-3">
									<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent">
										<v.icon size={19} />
									</span>
									<h3 class="text-lg font-semibold">{v.title}</h3>
								</div>
								<p class="mt-3 text-sm leading-relaxed text-muted">{v.body}</p>
							</div>
						{/each}
					</div>
				</div>
			</Reveal>

			<!-- placeholders + name rules -->
			<div class="mt-6 grid items-start gap-6 lg:grid-cols-2">
				<Reveal delay={160}>
					<div class="card h-full p-6">
						<div class="flex items-center gap-3">
							<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent">
								<Braces size={19} />
							</span>
							<h3 class="text-lg font-semibold">Placeholders</h3>
						</div>
						<p class="mt-3 text-sm leading-relaxed text-muted">
							Drop a token into any response and Dia fills it in for the member who runs the command.
						</p>
						<ul class="mt-4 divide-y divide-line">
							{#each placeholders as ph (ph.token)}
								<li class="flex items-start gap-3 py-3 first:pt-0 last:pb-0">
									<code class="shrink-0 rounded-md bg-blush px-2 py-0.5 font-mono text-xs font-semibold text-accent-ink">
										{ph.token}
									</code>
									<span class="text-sm text-muted">{ph.desc}</span>
								</li>
							{/each}
						</ul>
					</div>
				</Reveal>

				<Reveal delay={200}>
					<div class="card h-full p-6">
						<div class="flex items-center gap-3">
							<span class="grid h-10 w-10 place-items-center rounded-lg bg-blush text-accent">
								<Type size={19} />
							</span>
							<h3 class="text-lg font-semibold">Name rules</h3>
						</div>
						<p class="mt-3 text-sm leading-relaxed text-muted">
							Discord is strict about command names. Dia keeps yours valid as you type.
						</p>
						<ul class="mt-4 space-y-2.5">
							{#each ['Lowercase letters a–z and digits 0–9', 'Hyphen ( - ) and underscore ( _ ) allowed', 'No spaces, uppercase, or symbols', 'Between 1 and 32 characters long'] as rule (rule)}
								<li class="flex items-start gap-2 text-sm text-ink/80">
									<Check size={16} class="mt-0.5 shrink-0 text-accent" />{rule}
								</li>
							{/each}
						</ul>
						<div class="mt-4 flex flex-wrap items-center gap-2 border-t border-line pt-4">
							<span class="font-mono text-xs uppercase tracking-wide text-muted">e.g.</span>
							{#each ['/rules', '/server-info', '/role_menu'] as ok (ok)}
								<code class="rounded-md border border-line-strong bg-bg px-2 py-1 font-mono text-xs text-muted">
									{ok}
								</code>
							{/each}
						</div>
					</div>
				</Reveal>
			</div>
		</div>
	</section>

	<!-- ───────────────────────── 03 · Examples ───────────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">03</span>
					<span class="eyebrow">Examples</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2
						class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6"
					>
						A few to start with.
					</h2>
					<div class="lg:col-span-6">
						<p class="text-lg leading-relaxed text-muted">
							These are just a starting point — every command is yours to write. Save one and it
							registers to your server instantly, ready to run.
						</p>
					</div>
				</div>
			</Reveal>

			<div class="mt-12 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
				{#each examples as ex, i (ex.name)}
					<Reveal delay={i * 80}>
						<div class="card h-full p-5">
							<code class="font-mono text-sm font-semibold text-accent-ink">{ex.name}</code>
							<p class="mt-2 text-sm leading-relaxed text-muted">{ex.desc}</p>
						</div>
					</Reveal>
				{/each}
			</div>

			<Reveal delay={120}>
				<div
					class="mt-12 flex flex-col items-start gap-4 rounded-xl border border-line bg-surface p-6 sm:flex-row sm:items-center sm:justify-between"
				>
					<div class="flex items-start gap-3">
						<span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-blush text-accent">
							<Zap size={19} />
						</span>
						<div>
							<h3 class="text-base font-semibold">Registered instantly</h3>
							<p class="mt-0.5 text-sm leading-relaxed text-muted">
								Save a command and Dia registers it to your server right away — no redeploy, no
								waiting. Disable or edit it any time from the dashboard.
							</p>
						</div>
					</div>
					<a href={cta} class="btn btn-accent h-11 shrink-0 px-5">
						Build a command <ArrowRight size={16} />
					</a>
				</div>
			</Reveal>
		</div>
	</section>

	<CtaSection href={cta} heading="Ship your own slash commands." sub="Build them in the dashboard — no code, registered in seconds." />
	<SiteFooter />
</div>
