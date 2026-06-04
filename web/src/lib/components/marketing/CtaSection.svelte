<script lang="ts">
	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Github from 'lucide-svelte/icons/github';

	// Shared closing call-to-action — an elevated dark surface panel with a soft
	// accent wash. Clean and premium, in the site's own language.
	let {
		heading = 'Bring Dia to your community.',
		sub = 'Authorise it in one click, switch on the features you want, and tune them from the dashboard with live previews.',
		href = '/',
		label = 'Get started'
	}: { heading?: string; sub?: string; href?: string; label?: string } = $props();
</script>

<section class="px-6 py-20 sm:py-24">
	<div class="mx-auto max-w-page">
		<div
			class="relative overflow-hidden rounded-[28px] border border-line bg-surface px-8 py-16 text-center shadow-[inset_0_1px_0_rgba(255,255,255,0.04),0_1px_2px_rgba(0,0,0,0.5),0_30px_70px_-32px_rgba(0,0,0,0.7)] sm:px-12 sm:py-20"
		>
			<div>
				<span class="eyebrow">[ get started ]</span>
				<h2 class="mx-auto mt-3 max-w-2xl text-3xl font-bold leading-[1.05] tracking-[-0.02em] sm:text-5xl">
					{heading}
				</h2>
				<p class="mx-auto mt-4 max-w-xl text-lg leading-relaxed text-muted">{sub}</p>
				<div class="mt-9 flex flex-wrap items-center justify-center gap-3">
					<a {href} class="btn btn-accent h-12 px-6 text-base">{label} <ArrowRight size={18} /></a>
					<a href="https://github.com/dia-bot/dia" class="btn btn-ghost h-12 px-5 text-base">
						<Github size={18} /> Star on GitHub
					</a>
				</div>
			</div>
		</div>
	</div>
</section>
