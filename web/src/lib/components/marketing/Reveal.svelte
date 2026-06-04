<script lang="ts">
	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';

	// Scroll-reveal wrapper. Fades + lifts its content into view once it enters
	// the viewport. Honours prefers-reduced-motion (shows immediately, no motion).
	let {
		children,
		delay = 0,
		y = 18,
		once = true,
		class: klass = ''
	}: {
		children: Snippet;
		delay?: number;
		y?: number;
		once?: boolean;
		class?: string;
	} = $props();

	let el: HTMLElement;
	let shown = $state(false);

	onMount(() => {
		const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
		if (reduce || typeof IntersectionObserver === 'undefined') {
			shown = true;
			return;
		}
		const io = new IntersectionObserver(
			(entries) => {
				for (const e of entries) {
					if (e.isIntersecting) {
						shown = true;
						if (once) io.disconnect();
					} else if (!once) {
						shown = false;
					}
				}
			},
			{ threshold: 0.12, rootMargin: '0px 0px -7% 0px' }
		);
		io.observe(el);
		return () => io.disconnect();
	});
</script>

<div
	bind:this={el}
	class={klass}
	style="
		opacity: {shown ? 1 : 0};
		transform: translateY({shown ? 0 : y}px);
		transition: opacity 0.72s cubic-bezier(0.16, 1, 0.3, 1) {delay}ms,
			transform 0.72s cubic-bezier(0.16, 1, 0.3, 1) {delay}ms;
		will-change: opacity, transform;
	"
>
	{@render children()}
</div>
