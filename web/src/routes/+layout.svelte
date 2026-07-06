<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { onNavigate } from '$app/navigation';
	import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
	import { setCsrf, loginURL } from '$lib/api';
	import { loginWithPopup } from '$lib/auth';
	import type { Snippet } from 'svelte';

	let { data, children }: { data: { csrf?: string | null }; children: Snippet } = $props();

	// One TanStack Query client for the app. Dashboard reads (server list, etc.)
	// are cached and shared across components, with a short stale window.
	const queryClient = new QueryClient({
		defaultOptions: {
			queries: { staleTime: 30_000, retry: 1, refetchOnWindowFocus: false }
		}
	});

	let loggingIn = $state(false);

	// Make the CSRF token available to the API client. Runs before any
	// user-initiated mutation (reads don't send the token).
	$effect(() => {
		setCsrf(data.csrf ?? '');
	});

	// Intercept clicks on any "log in" link and run the popup flow instead of a
	// full-page redirect, so the page underneath stays mounted. If JS is off the
	// link still works as a normal redirect login (progressive enhancement).
	onMount(() => {
		function onClick(e: MouseEvent) {
			if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey)
				return;
			const anchor = (e.target as HTMLElement | null)?.closest('a');
			if (!anchor || anchor.getAttribute('href') !== loginURL) return;
			e.preventDefault();
			if (loggingIn) return;
			loggingIn = true;
			loginWithPopup().finally(() => {
				loggingIn = false;
			});
		}
		document.addEventListener('click', onClick);
		return () => document.removeEventListener('click', onClick);
	});

	// Cross-page transitions via the View Transitions API (a no-op where the API
	// is unavailable). Dashboard page switches are SKIPPED here on purpose: they
	// animate in the live DOM (vertical content slide + sliding sidebar highlight
	// in servers/[id]/+layout.svelte), and wrapping them in a view transition
	// would snapshot and freeze the sidebar, hiding that highlight. Every other
	// navigation keeps the default crossfade.
	onNavigate((navigation) => {
		const doc = document as Document & {
			startViewTransition?: (cb: () => Promise<void>) => void;
		};
		if (!doc.startViewTransition) return;
		if (/^\/servers\/[^/]+(\/|$)/.test(navigation.to?.url.pathname ?? '')) return;
		return new Promise<void>((resolve) => {
			doc.startViewTransition!(async () => {
				resolve();
				await navigation.complete;
			});
		});
	});
</script>

<QueryClientProvider client={queryClient}>
	{@render children()}
</QueryClientProvider>

{#if loggingIn}
	<div class="login-overlay" role="status" aria-live="polite">
		<div class="login-card">
			<span class="login-spinner" aria-hidden="true"></span>
			<p class="login-eyebrow">Discord</p>
			<p class="login-text">Finish signing in the popup window…</p>
		</div>
	</div>
{/if}

<style>
	.login-overlay {
		position: fixed;
		inset: 0;
		z-index: 100;
		display: grid;
		place-items: center;
		background: color-mix(in srgb, var(--color-ink, #111) 55%, transparent);
		backdrop-filter: blur(3px);
		animation: login-fade 160ms ease-out;
	}
	.login-card {
		display: grid;
		justify-items: center;
		gap: 0.6rem;
		padding: 1.75rem 2rem;
		border-radius: 16px;
		background: var(--color-surface, #fff);
		border: 1px solid var(--color-line, #e5e5e5);
		box-shadow: 0 24px 60px -24px rgba(0, 0, 0, 0.5);
	}
	.login-spinner {
		width: 26px;
		height: 26px;
		border-radius: 50%;
		border: 2.5px solid var(--color-line, #e5e5e5);
		border-top-color: var(--color-accent, #ff6363);
		animation: login-spin 0.7s linear infinite;
	}
	.login-eyebrow {
		font-family: var(--font-mono, monospace);
		font-size: 0.7rem;
		letter-spacing: 0.09em;
		text-transform: uppercase;
		color: var(--color-accent-ink, #c2354a);
	}
	.login-text {
		font-family: var(--font-sans, system-ui);
		font-size: 0.92rem;
		color: var(--color-ink, #111);
	}
	@keyframes login-spin {
		to {
			transform: rotate(360deg);
		}
	}
	@keyframes login-fade {
		from {
			opacity: 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.login-overlay {
			animation: none;
		}
		.login-spinner {
			animation-duration: 1.4s;
		}
	}
</style>
