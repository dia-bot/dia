<script lang="ts">
	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Github from 'lucide-svelte/icons/github';
	import Check from 'lucide-svelte/icons/check';
	import Copy from 'lucide-svelte/icons/copy';

	// Shared closing call-to-action — the "monolith finale". A full-width near-black
	// band that closes every page like a colophon: an oversized, left-set grotesk
	// heading + a quiet mono identity ledger on the left; the actions docked right
	// against a vertical hairline. The ONLY colour is a single thin brand-gradient
	// rule welded to the top edge (one deliberate lit seam) and the one gradient
	// CTA button. No centered card, no ambient wash, no glow. The secondary action
	// is the real, copyable self-host command — Dia's identity made literal.
	// Props API preserved byte-for-byte: heading, sub, href, label (defaults are
	// load-bearing — home/compare rely on the default sub/label; no site passes label).
	let {
		heading = 'Bring Dia to your community.',
		sub = 'Authorise it in one click, switch on the features you want, and tune them from the dashboard with live previews.',
		href = '/',
		label = 'Get started'
	}: { heading?: string; sub?: string; href?: string; label?: string } = $props();

	// The product's own truths, stated plainly — not filler metrics.
	const ledger = ['Open-source', 'Self-hostable', 'One dashboard'];

	const cloneCmd = 'git clone https://github.com/dia-bot/dia && make up';

	let copied = $state(false);
	let copyTimer: ReturnType<typeof setTimeout> | undefined;

	function copyClone() {
		const clip = navigator.clipboard;
		if (!clip) return;
		clip
			.writeText(cloneCmd)
			.then(() => {
				copied = true;
				clearTimeout(copyTimer);
				copyTimer = setTimeout(() => (copied = false), 1600);
			})
			.catch(() => {});
	}
</script>

<section class="relative isolate overflow-hidden border-t border-line bg-ink-2">
	<!-- The single lit edge: one thin brand-gradient hairline across the very top.
	     The only colour in the band apart from the CTA button. -->
	<div aria-hidden="true" class="brand-gradient absolute inset-x-0 top-0 h-px opacity-70"></div>

	<div class="relative mx-auto max-w-page px-6 py-20 sm:py-28">
		<!-- tiny mono marker, top-left — the technical sign-off label -->
		<div class="flex items-center gap-3 font-mono text-[11px] uppercase tracking-[0.08em] text-muted">
			<span class="h-1.5 w-1.5 rounded-full bg-accent"></span>
			get started
			<span class="h-px w-10 bg-line-strong"></span>
		</div>

		<div class="mt-8 grid items-end gap-x-12 gap-y-12 lg:grid-cols-12">
			<!-- left: monumental heading + the quiet mono identity ledger (the one decorative move) -->
			<div class="lg:col-span-7">
				<h2
					class="max-w-[16ch] font-sans text-[clamp(2.5rem,6vw,4.75rem)] font-black leading-[0.95] tracking-[-0.035em] text-ink"
				>
					{heading}
				</h2>
				<p class="mt-6 max-w-[46ch] text-lg leading-relaxed text-muted">{sub}</p>

				<ul class="mt-7 flex flex-wrap items-center gap-x-3 gap-y-1.5">
					{#each ledger as item, i (item)}
						{#if i > 0}
							<li aria-hidden="true" class="h-1 w-1 rounded-full bg-faint/60"></li>
						{/if}
						<li class="font-mono text-[11px] font-medium uppercase tracking-[0.08em] text-faint">
							{item}
						</li>
					{/each}
				</ul>
			</div>

			<!-- right: actions, docked against a vertical hairline on large screens -->
			<div class="lg:col-span-5 lg:border-l lg:border-line lg:pl-12">
				<a
					{href}
					class="brand-gradient inline-flex h-12 items-center gap-2 rounded-xl px-6 text-[0.95rem] font-semibold text-white shadow-[0_8px_30px_-8px_rgba(178,68,252,0.5)] transition-[filter,transform] duration-150 hover:brightness-110"
				>
					{label} <ArrowRight size={18} />
				</a>

				<!-- the self-host path: a real, copyable command — terminal-credible -->
				<div class="mt-8">
					<div class="flex items-center justify-between">
						<span class="font-mono text-[11px] uppercase tracking-[0.08em] text-faint">
							or self-host
						</span>
						<button
							type="button"
							onclick={copyClone}
							class="inline-flex items-center gap-1.5 rounded-md px-1.5 py-1 font-mono text-[11px] text-muted transition-colors hover:text-ink focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-accent"
							aria-label="Copy install command"
						>
							{#if copied}
								<Check size={13} class="text-success" /> copied
							{:else}
								<Copy size={13} /> copy
							{/if}
						</button>
					</div>
					<div
						class="mt-2 flex items-start gap-2.5 overflow-hidden rounded-xl border border-line-strong bg-bg px-3.5 py-3 font-mono text-[12.5px] leading-relaxed"
					>
						<span class="select-none text-accent-ink">$</span>
						<code class="min-w-0 break-all text-ink">
							git clone <span class="text-muted">https://github.com/dia-bot/dia</span> &amp;&amp; make up
						</code>
					</div>
				</div>

				<a
					href="https://github.com/dia-bot/dia"
					class="group mt-5 inline-flex items-center gap-2.5 font-mono text-[11px] uppercase tracking-[0.08em] text-muted transition-colors hover:text-ink"
				>
					<Github size={14} class="text-faint transition-colors group-hover:text-ink" />
					<span class="h-px w-5 bg-line-strong transition-colors group-hover:bg-ink"></span>
					star on github · MIT · self-host free
				</a>
			</div>
		</div>
	</div>
</section>
