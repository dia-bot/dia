<script lang="ts">
	import { onMount } from 'svelte';

	// A terminal that types out the self-host flow line-by-line when scrolled into
	// view. Reduced-motion shows the whole transcript at once.
	type Line = { kind: 'cmd' | 'ok' | 'info' | 'dim'; text: string };
	const lines: Line[] = [
		{ kind: 'cmd', text: 'git clone https://github.com/dia-bot/dia' },
		{ kind: 'cmd', text: 'make up' },
		{ kind: 'ok', text: 'infra up — postgres · redis · nats' },
		{ kind: 'ok', text: 'migrations applied (embedded)' },
		{ kind: 'ok', text: 'gateway connected — 1 shard' },
		{ kind: 'ok', text: 'worker online — 5 plugins loaded' },
		{ kind: 'info', text: 'dashboard ready → http://localhost:5173' }
	];

	let el: HTMLElement;
	let shown = $state(0);

	onMount(() => {
		const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
		if (reduce || typeof IntersectionObserver === 'undefined') {
			shown = lines.length;
			return;
		}
		let interval: ReturnType<typeof setInterval>;
		const io = new IntersectionObserver(
			(e) => {
				if (!e[0]?.isIntersecting) return;
				io.disconnect();
				interval = setInterval(() => {
					shown += 1;
					if (shown >= lines.length) clearInterval(interval);
				}, 430);
			},
			{ threshold: 0.5 }
		);
		io.observe(el);
		return () => {
			io.disconnect();
			clearInterval(interval);
		};
	});
</script>

<div
	bind:this={el}
	class="overflow-hidden rounded-2xl bg-[#0f1117] shadow-[0_30px_70px_-25px_rgba(0,0,0,0.6)] ring-1 ring-white/10"
>
	<div class="flex h-9 items-center gap-2 bg-[#1b1d23] px-4">
		<span class="h-3 w-3 rounded-full bg-[#ff5f57]/90"></span>
		<span class="h-3 w-3 rounded-full bg-[#febc2e]/90"></span>
		<span class="h-3 w-3 rounded-full bg-[#28c840]/90"></span>
		<span class="flex-1 text-center text-[12px] font-medium text-[#6b7280]">zsh — dia</span>
		<span class="w-[42px]"></span>
	</div>
	<div class="space-y-1.5 p-5 font-mono text-[13px] leading-relaxed">
		{#each lines as l, i (i)}
			{#if i < shown}
				<div class="line flex min-w-0 items-start gap-2">
					{#if l.kind === 'cmd'}
						<span class="select-none text-[#a472ff]">$</span>
						<span class="break-all text-[#e6e8ec]">{l.text}</span>
					{:else if l.kind === 'ok'}
						<span class="select-none text-[#3ba55c]">✓</span>
						<span class="text-[#9aa0aa]">{l.text}</span>
					{:else if l.kind === 'info'}
						<span class="select-none text-[#a472ff]">→</span>
						<span class="break-all font-semibold text-[#e6e8ec]">{l.text}</span>
					{:else}
						<span class="text-[#6b7280]">{l.text}</span>
					{/if}
				</div>
			{/if}
		{/each}
		<span class="inline-block h-4 w-2 translate-y-0.5 bg-[#e6e8ec] {shown >= lines.length ? 'blink' : ''}"></span>
	</div>
</div>

<style>
	.line {
		animation: in 0.28s ease-out both;
	}
	@keyframes in {
		from {
			opacity: 0;
			transform: translateY(3px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}
	.blink {
		animation: blink 1.1s steps(1) infinite;
	}
	@keyframes blink {
		50% {
			opacity: 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.line,
		.blink {
			animation: none;
		}
	}
</style>
