<script lang="ts">
	import SiteNav from '$lib/components/marketing/SiteNav.svelte';
	import SiteFooter from '$lib/components/marketing/SiteFooter.svelte';
	import PageHero from '$lib/components/marketing/PageHero.svelte';
	import CtaSection from '$lib/components/marketing/CtaSection.svelte';
	import Reveal from '$lib/components/marketing/Reveal.svelte';
	import DiscordWindow from '$lib/components/marketing/DiscordWindow.svelte';
	import DiscordMessage from '$lib/components/marketing/DiscordMessage.svelte';
	import WelcomeCard from '$lib/components/marketing/WelcomeCard.svelte';
	import WelcomeEditor from '$lib/components/marketing/welcome/WelcomeEditor.svelte';
	import { loginURL } from '$lib/api';

	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Layers from 'lucide-svelte/icons/layers';
	import Move from 'lucide-svelte/icons/move';
	import Palette from 'lucide-svelte/icons/palette';
	import Image from 'lucide-svelte/icons/image';
	import Type from 'lucide-svelte/icons/type';
	import Eye from 'lucide-svelte/icons/eye';
	import LogOut from 'lucide-svelte/icons/log-out';

	let { data }: { data: { user?: { username: string } | null } } = $props();
	const cta = $derived(data.user ? '/servers' : loginURL);

	const mention = 'rounded bg-[#b244fc]/25 px-1 font-medium text-[#d9b8ff]';

	// The six layout templates exposed by the element-based editor.
	const layouts = [
		{ name: 'Centered', body: 'Avatar, title and subtitle stacked dead-center — the default greeting.' },
		{ name: 'Banner', body: 'A wide title band with the member name leading and a trailing accent.' },
		{ name: 'Split', body: 'Avatar to one side, copy to the other — room for a longer welcome line.' },
		{ name: 'Spotlight', body: 'A large avatar anchors the card with the name spotlit beneath it.' },
		{ name: 'Minimal', body: 'Type only. No avatar, no chrome — just a clean, confident line.' },
		{ name: 'Stacked', body: 'Title, subtitle and a small footer stacked tightly with breathing room.' }
	];

	// Theme presets shipped with the feature (internal/features/welcome/presets.go).
	const themes = [
		{ name: 'Aurora', desc: 'Pink → purple gradient, the Dia signature look.', dot: 'background: linear-gradient(45deg,#FF6363,#B244FC);' },
		{ name: 'Midnight', desc: 'Deep indigo gradient with a purple accent bar.', dot: 'background: linear-gradient(30deg,#1F1B2E,#3A2E5C);' },
		{ name: 'Blush', desc: 'Soft solid surface with near-black ink type.', dot: 'background: #F1DFDF;' },
		{ name: 'Sunset', desc: 'Warm coral-to-amber gradient for a bright entrance.', dot: 'background: linear-gradient(60deg,#FF6363,#FFB347);' }
	];

	// Placeholder tokens resolved per member at post time.
	const tokens = [
		{ token: '{user}', body: 'The new member, mentioned or named.' },
		{ token: '{count}', body: 'Your live member count after they join.' },
		{ token: '{server}', body: "The server's name." }
	];
</script>

<svelte:head>
	<title>Welcome cards · Dia</title>
	<meta
		name="description"
		content="Dia's welcome feature: an element-based card editor with six layouts and theme presets, gradient, solid or image backgrounds, and member, count and server placeholders. It posts the moment someone joins."
	/>
</svelte:head>

<div class="min-h-screen">
	<SiteNav user={data.user ?? null} />

	<PageHero
		eyebrow="[ feature · welcome ]"
		title="First impressions, art-directed down to the pixel."
		lede="Compose a welcome card yourself — layers, layouts, typography and colour, with a live preview. When someone joins, Dia renders it for that member and posts it the moment they arrive."
	>
		<div class="flex flex-wrap items-center gap-3">
			<a href={cta} class="btn btn-accent h-12 px-6 text-base">Get started <ArrowRight size={18} /></a>
			<a href="/#features" class="btn btn-ghost h-12 px-5 text-base">All features</a>
		</div>
		<div class="mt-6 flex flex-wrap items-center gap-x-5 gap-y-2 font-mono text-xs text-muted">
			<span class="inline-flex items-center gap-1.5"><Layers size={14} class="text-accent" /> Element-based editor</span>
			<span class="inline-flex items-center gap-1.5"><Palette size={14} class="text-accent" /> Six layouts + presets</span>
			<span class="inline-flex items-center gap-1.5"><Eye size={14} class="text-accent" /> Live preview</span>
		</div>
	</PageHero>

	<!-- ───────────────────── Centerpiece: the editor ───────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">01</span>
					<span class="eyebrow">The editor</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						A real composer, not a fixed form.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-6">
						Pick a layout, drop in text, an avatar, a badge or a divider, then move and style any
						layer until it's exactly right. The canvas renders the same card your members will
						see — no guesswork, no save-and-refresh.
					</p>
				</div>
			</Reveal>

			<Reveal delay={80}>
				<div class="mt-12">
					<WelcomeEditor />
				</div>
			</Reveal>

			<Reveal delay={120}>
				<div class="mt-8 grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-3">
					{#each [{ icon: Move, t: 'Move & arrange', b: 'Drag layers on the canvas or reorder them in the layer list.' }, { icon: Type, t: 'Style every layer', b: 'Font, weight, colour, opacity and rotation, per element.' }, { icon: Eye, t: 'See it live', b: 'The preview is the renderer — what you build is what posts.' }] as c (c.t)}
						<div class="bg-surface p-6">
							<span class="grid h-9 w-9 place-items-center rounded-lg bg-blush text-accent"><c.icon size={17} /></span>
							<h3 class="mt-3.5 text-[15px] font-semibold">{c.t}</h3>
							<p class="mt-1 text-sm leading-relaxed text-muted">{c.b}</p>
						</div>
					{/each}
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────── Posts the moment they join ───────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">02</span>
					<span class="eyebrow">On join</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						Posts the moment they join.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-6">
						Choose the channel and write the message. When a member arrives, Dia fills in the
						placeholders, renders their card, and posts both — as a plain message or a rich embed.
						Want a quieter touch? Send the same greeting as a DM instead.
					</p>
				</div>
			</Reveal>

			<Reveal delay={80}>
				<div class="mx-auto mt-12 max-w-2xl">
					<DiscordWindow channel="welcome" title="Aurora" topic="Say hi to new members" members="1,284 online">
						<DiscordMessage author="maya" color="#1aa179" time="Today at 4:19 PM">
							just joined, hi everyone 👋
						</DiscordMessage>
						<DiscordMessage brand author="Dia" time="Today at 4:19 PM">
							Hey <span class={mention}>@maya</span>, welcome to
							<strong class="font-semibold text-[#f2f3f5]">Aurora</strong>! 🎉
							<div class="mt-2 max-w-[460px]">
								<WelcomeCard
									from="#FF6363"
									to="#B244FC"
									angle={45}
									title="Welcome, {'{user}'}!"
									subtitle="You're member #{'{count}'} of {'{server}'}"
									accent="#FFFFFF"
									text="#FFFFFF"
									subtext="#F7E9F2"
									username="maya"
									count={1284}
									server="Aurora"
								/>
							</div>
						</DiscordMessage>
					</DiscordWindow>
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────── Layouts & themes ───────────────────── -->
	<section class="border-b border-line py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">03</span>
					<span class="eyebrow">Layouts &amp; themes</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						Six layouts, themes to match.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-6">
						Start from a template, then recolour it with a preset or your own palette. Backgrounds
						can be a gradient, a solid fill, or an image — and a one-click preset sets the
						background, accent and text colours together.
					</p>
				</div>
			</Reveal>

			<!-- layout templates -->
			<Reveal delay={80}>
				<div class="mt-12 grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-2 lg:grid-cols-3">
					{#each layouts as l, i (l.name)}
						<div class="bg-surface p-6">
							<div class="flex items-baseline gap-3">
								<span class="font-mono text-xs text-faint">{String(i + 1).padStart(2, '0')}</span>
								<h3 class="text-[15px] font-semibold">{l.name}</h3>
							</div>
							<p class="mt-1.5 text-sm leading-relaxed text-muted">{l.body}</p>
						</div>
					{/each}
				</div>
			</Reveal>

			<!-- theme presets list -->
			<Reveal delay={120}>
				<div class="mt-10">
					<div class="mb-4 font-mono text-xs uppercase tracking-wide text-muted">Theme presets</div>
					<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
						{#each themes as t (t.name)}
							<div class="card flex items-center gap-3 p-4">
								<span class="h-9 w-9 shrink-0 rounded-lg ring-1 ring-line-strong" style={t.dot}></span>
								<div class="min-w-0">
									<div class="text-sm font-semibold">{t.name}</div>
									<div class="mt-0.5 text-xs leading-snug text-muted">{t.desc}</div>
								</div>
							</div>
						{/each}
					</div>
				</div>
			</Reveal>

			<!-- static card examples across themes -->
			<Reveal delay={160}>
				<div class="mt-10 grid gap-5 lg:grid-cols-3">
					<figure>
						<div class="rounded-2xl bg-ink-2 p-2.5 ring-1 ring-line">
							<WelcomeCard
								from="#FF6363"
								to="#B244FC"
								angle={45}
								accent="#FFFFFF"
								text="#FFFFFF"
								subtext="#F7E9F2"
								username="maya"
								count={1284}
								server="Aurora"
							/>
						</div>
						<figcaption class="mt-2.5 flex items-center gap-2 font-mono text-xs text-muted">
							<span class="text-accent-ink">aurora</span> · gradient #FF6363 → #B244FC
						</figcaption>
					</figure>
					<figure>
						<div class="rounded-2xl bg-ink-2 p-2.5 ring-1 ring-line">
							<WelcomeCard
								from="#1F1B2E"
								to="#3A2E5C"
								angle={30}
								accent="#B244FC"
								text="#FFFFFF"
								subtext="#C9C3DA"
								title="Welcome aboard, {'{user}'}"
								subtitle="Member #{'{count}'} · {'{server}'}"
								username="kai"
								count={1285}
								server="Aurora"
							/>
						</div>
						<figcaption class="mt-2.5 flex items-center gap-2 font-mono text-xs text-muted">
							<span class="text-accent-ink">midnight</span> · gradient #1F1B2E → #3A2E5C
						</figcaption>
					</figure>
					<figure>
						<div class="rounded-2xl bg-ink-2 p-2.5 ring-1 ring-line">
							<WelcomeCard
								from=""
								to=""
								color="#F1DFDF"
								accent="#FF6363"
								text="#2B2233"
								subtext="#7A6B73"
								title="Hi {'{user}'} 👋"
								subtitle="You're #{'{count}'} in {'{server}'}"
								username="rio"
								count={1286}
								server="Aurora"
							/>
						</div>
						<figcaption class="mt-2.5 flex items-center gap-2 font-mono text-xs text-muted">
							<span class="text-accent-ink">blush</span> · solid #F1DFDF · ink #2B2233
						</figcaption>
					</figure>
				</div>
			</Reveal>
		</div>
	</section>

	<!-- ───────────────────── Make it yours ───────────────────── -->
	<section class="border-b border-line bg-surface py-20 sm:py-28">
		<div class="mx-auto max-w-page px-6">
			<Reveal>
				<div class="mb-3 flex items-baseline gap-4">
					<span class="font-mono text-xs font-medium text-accent-ink">04</span>
					<span class="eyebrow">Make it yours</span>
					<span class="h-px flex-1 bg-line"></span>
				</div>
				<div class="grid items-start gap-6 lg:grid-cols-12">
					<h2 class="text-3xl font-extrabold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.08] lg:col-span-6">
						Placeholders that fill themselves in.
					</h2>
					<p class="text-lg leading-relaxed text-muted lg:col-span-6">
						Write your message and card copy once with placeholder tokens. Dia swaps them for real
						values per member when the card is rendered — so every greeting is personal without any
						extra work.
					</p>
				</div>
			</Reveal>

			<Reveal delay={80}>
				<div class="mt-12 grid gap-px overflow-hidden rounded-xl border border-line bg-line sm:grid-cols-3">
					{#each tokens as t (t.token)}
						<div class="bg-surface p-6">
							<code class="inline-block rounded-md border border-line-strong bg-bg px-2.5 py-1 font-mono text-sm text-accent-ink">{t.token}</code>
							<p class="mt-3 text-sm leading-relaxed text-muted">{t.body}</p>
						</div>
					{/each}
				</div>
			</Reveal>

			<Reveal delay={120}>
				<div class="mt-6 grid items-start gap-6 lg:grid-cols-[1.4fr_1fr]">
					<div class="card overflow-hidden">
						<div class="flex items-center gap-2 border-b border-line bg-bg px-4 py-2.5 font-mono text-xs text-muted">
							<Type size={13} class="text-accent-ink" /> message template
						</div>
						<pre class="overflow-x-auto px-4 py-4 font-mono text-[13px] leading-relaxed text-ink"><code>Hey {'{user.mention}'}, welcome to **{'{server}'}**! 🎉
You're our {'{count}'}th member.</code></pre>
					</div>
					<div class="card flex flex-col gap-3 p-5">
						<span class="grid h-9 w-9 place-items-center rounded-lg bg-blush text-accent"><LogOut size={17} /></span>
						<h3 class="text-[15px] font-semibold">Optional leave messages</h3>
						<p class="text-sm leading-relaxed text-muted">
							Turn on leave notices to post a quiet line when someone departs — to its own channel
							if you like — using the same <code class="rounded bg-bg px-1 font-mono text-[12px] text-accent-ink">{'{username}'}</code>
							and <code class="rounded bg-bg px-1 font-mono text-[12px] text-accent-ink">{'{count}'}</code> tokens.
						</p>
					</div>
				</div>
			</Reveal>

			<Reveal delay={160}>
				<div class="mt-8 flex flex-wrap items-center gap-3 rounded-xl border border-line bg-bg px-5 py-4">
					<span class="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-blush text-accent"><Image size={17} /></span>
					<p class="text-sm text-muted">
						Pair welcome cards with <strong class="font-semibold text-ink">Leveling</strong> for
						themeable rank cards that reward the regulars.
					</p>
					<a href="/features/leveling" class="ml-auto inline-flex items-center gap-1.5 font-mono text-sm text-accent-ink hover:text-accent">
						Explore leveling <ArrowRight size={14} />
					</a>
				</div>
			</Reveal>
		</div>
	</section>

	<CtaSection href={cta} heading="Greet every new member." sub="Design the card once and let Dia post it the moment someone joins." />
	<SiteFooter />
</div>
