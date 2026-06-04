<script lang="ts">
	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';
	import SiteNav from './SiteNav.svelte';
	import SiteFooter from './SiteFooter.svelte';

	// Shared layout for Terms / Privacy: clean Dia-branded hero, a sticky
	// scrollspy contents rail, and typographic prose. Content is passed as a
	// snippet of <section id> blocks matching `sections`.
	let {
		title,
		description = '',
		kind,
		effective = '2026-06-02',
		updated = '2026-06-02',
		sections,
		user = null,
		children
	}: {
		title: string;
		description?: string;
		kind: 'terms' | 'privacy';
		effective?: string;
		updated?: string;
		sections: { id: string; num: string; title: string }[];
		user?: { username: string } | null;
		children: Snippet;
	} = $props();

	const docs = [
		{ key: 'terms', label: 'Terms of Service', href: '/terms' },
		{ key: 'privacy', label: 'Privacy Policy', href: '/privacy' }
	];

	let activeId = $state('');

	onMount(() => {
		activeId = sections[0]?.id ?? '';
		const els = sections
			.map((s) => document.getElementById(s.id))
			.filter((e): e is HTMLElement => !!e);
		if (!els.length) return;
		const onScroll = () => {
			const probe = window.innerHeight * 0.28;
			let cur = els[0].id;
			for (const el of els) {
				if (el.getBoundingClientRect().top - probe <= 0) cur = el.id;
				else break;
			}
			activeId = cur;
		};
		onScroll();
		window.addEventListener('scroll', onScroll, { passive: true });
		return () => window.removeEventListener('scroll', onScroll);
	});

	const fmt = (d: string) =>
		new Date(d + 'T00:00:00').toLocaleDateString('en-GB', {
			day: 'numeric',
			month: 'long',
			year: 'numeric'
		});
</script>

<svelte:head>
	<title>{title} · Dia</title>
	<meta name="description" content={description || title} />
</svelte:head>

<div class="min-h-screen">
	<SiteNav {user} />

	<!-- hero -->
	<section class="border-b border-line bg-bg">
		<div class="mx-auto max-w-page px-6 py-14 sm:py-16">
			<span class="eyebrow">[ legal ]</span>
			<h1 class="mt-3 text-4xl font-extrabold tracking-[-0.03em] sm:text-5xl">{title}</h1>
			<div class="mt-4 flex flex-wrap items-center gap-x-4 gap-y-1 font-mono text-xs text-muted">
				<span>Effective {fmt(effective)}</span>
				<span class="h-1 w-1 rounded-full bg-line-strong"></span>
				<span>Last updated {fmt(updated)}</span>
			</div>
		</div>
	</section>

	<!-- body -->
	<div class="mx-auto grid max-w-page grid-cols-1 gap-10 px-6 py-12 lg:grid-cols-[240px_minmax(0,1fr)] lg:gap-16">
		<!-- contents rail -->
		<aside class="hidden self-start lg:sticky lg:top-24 lg:block">
			<div class="mb-4 text-[11px] font-semibold uppercase tracking-wider text-muted">
				On this page
			</div>
			<ol class="space-y-px text-[13px]">
				{#each sections as s (s.id)}
					<li>
						<a
							href={`#${s.id}`}
							class="flex items-baseline gap-3 border-l-2 py-1.5 pl-3 transition-colors {activeId ===
							s.id
								? 'border-accent font-semibold text-accent-ink'
								: 'border-transparent text-muted hover:text-ink'}"
						>
							<span class="w-5 shrink-0 font-mono text-[10.5px] text-faint">{s.num}</span>
							<span class="leading-snug">{s.title}</span>
						</a>
					</li>
				{/each}
			</ol>

			<div class="mt-8 border-t border-line pt-6">
				<div class="mb-3 text-[11px] font-semibold uppercase tracking-wider text-muted">
					Documents
				</div>
				<ul class="space-y-1">
					{#each docs as d (d.key)}
						<li>
							<a
								href={d.href}
								aria-current={kind === d.key ? 'page' : undefined}
								class="block text-[13px] {kind === d.key
									? 'font-semibold text-accent-ink'
									: 'text-muted hover:text-ink'}">{d.label}</a
							>
						</li>
					{/each}
				</ul>
			</div>
		</aside>

		<!-- prose -->
		<article
			class="max-w-3xl [&_a]:font-medium [&_a]:text-accent-ink [&_a]:underline [&_a]:decoration-line-strong [&_a]:underline-offset-2 hover:[&_a]:text-accent [&_code]:rounded [&_code]:bg-blush [&_code]:px-1.5 [&_code]:py-0.5 [&_code]:font-mono [&_code]:text-[0.9em] [&_code]:text-accent-ink [&_h2]:mb-3 [&_h2]:mt-12 [&_h2]:text-[22px] [&_h2]:font-bold [&_h2]:tracking-tight [&_h2]:text-ink md:[&_h2]:text-[26px] [&_h3]:mb-2 [&_h3]:mt-7 [&_h3]:text-base [&_h3]:font-semibold [&_h3]:text-ink [&_li]:marker:text-faint [&_ol]:list-decimal [&_ol]:space-y-1.5 [&_ol]:pl-5 [&_ol]:text-[14.5px] [&_ol]:text-ink/85 [&_p]:text-[15px] [&_p]:leading-relaxed [&_p]:text-ink/85 [&_section]:scroll-mt-24 [&_strong]:font-semibold [&_strong]:text-ink [&_ul]:list-disc [&_ul]:space-y-1.5 [&_ul]:pl-5 [&_ul]:text-[14.5px] [&_ul]:text-ink/85"
		>
			{@render children()}
			<div class="mt-16 border-t border-line pt-6 text-[12.5px] text-muted">
				Questions about this document? Email
				<a href="mailto:hello@dia.xyz">hello@dia.xyz</a>.
			</div>
		</article>
	</div>

	<SiteFooter />
</div>
