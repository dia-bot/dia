<script lang="ts">
	import { onMount } from 'svelte';

	// Animated number that counts up from 0 → `to` the first time it scrolls
	// into view. Eased, comma-formatted, reduced-motion aware.
	let {
		to,
		duration = 1700,
		prefix = '',
		suffix = '',
		decimals = 0,
		class: klass = ''
	}: {
		to: number;
		duration?: number;
		prefix?: string;
		suffix?: string;
		decimals?: number;
		class?: string;
	} = $props();

	let el: HTMLElement;
	let current = $state(0);

	const fmt = (n: number) =>
		n.toLocaleString('en-US', { minimumFractionDigits: decimals, maximumFractionDigits: decimals });

	onMount(() => {
		const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
		if (reduce || typeof IntersectionObserver === 'undefined') {
			current = to;
			return;
		}
		let raf = 0;
		const io = new IntersectionObserver(
			(entries) => {
				if (!entries[0]?.isIntersecting) return;
				io.disconnect();
				const start = performance.now();
				const step = (now: number) => {
					const t = Math.min(1, (now - start) / duration);
					current = to * (1 - Math.pow(1 - t, 3));
					if (t < 1) raf = requestAnimationFrame(step);
					else current = to;
				};
				raf = requestAnimationFrame(step);
			},
			{ threshold: 0.5 }
		);
		io.observe(el);
		return () => {
			io.disconnect();
			cancelAnimationFrame(raf);
		};
	});
</script>

<span bind:this={el} class={klass}>{prefix}{fmt(current)}{suffix}</span>
