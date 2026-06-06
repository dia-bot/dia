<!--
  OAuth finisher. /auth/callback (server) sets the cookie and redirects here.
  By now the session cookie exists. If we're the login popup (window.name set by
  window.open survives the cross-origin Discord hop, unlike window.opener under
  COOP) we message the opener and close. Otherwise it was a full-page login, so
  we navigate into the app ourselves.
-->
<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';

	let error = $state<string | null>(null);

	onMount(() => {
		const err = new URLSearchParams(window.location.search).get('error');
		error = err;
		const msg = { type: 'dia-auth', ok: !err, error: err };

		// Always notify the opener — robust even if window.name/opener get severed
		// across the Discord hop, so the main tab never waits forever.
		try {
			const ch = new BroadcastChannel('dia-auth');
			ch.postMessage(msg);
			ch.close();
		} catch {
			/* no BroadcastChannel */
		}
		try {
			window.opener?.postMessage(msg, window.location.origin);
		} catch {
			/* opener gone */
		}

		// In the popup: close. Full-page login: the cookie is set, so navigate.
		if (window.name === 'dia-login' || window.opener != null) {
			const t = setTimeout(() => window.close(), 150);
			return () => clearTimeout(t);
		}
		void goto(err ? '/?login_error=1' : '/servers');
	});
</script>

<div class="wrap">
	<span class="spinner" class:err={error} aria-hidden="true"></span>
	<p class="eyebrow">{error ? 'Sign-in failed' : 'Discord'}</p>
	<p class="msg">{error ? 'You can close this window and try again.' : 'Signing you in…'}</p>
</div>

<style>
	.wrap {
		display: grid;
		place-content: center;
		justify-items: center;
		gap: 0.55rem;
		min-height: 100dvh;
		padding: 2rem;
		text-align: center;
		background: var(--color-bg, #fff);
		color: var(--color-ink, #111);
	}
	.spinner {
		width: 26px;
		height: 26px;
		margin-bottom: 0.4rem;
		border-radius: 50%;
		border: 2.5px solid var(--color-line, #e5e5e5);
		border-top-color: var(--color-accent, #ff6363);
		animation: spin 0.7s linear infinite;
	}
	.spinner.err {
		animation: none;
		border-top-color: var(--color-line, #e5e5e5);
	}
	.eyebrow {
		font-family: var(--font-mono, monospace);
		font-size: 0.72rem;
		letter-spacing: 0.09em;
		text-transform: uppercase;
		color: var(--color-accent-ink, #c2354a);
	}
	.msg {
		font-family: var(--font-sans, system-ui);
		color: var(--color-muted, #666);
		max-width: 28ch;
	}
	@keyframes spin {
		to {
			transform: rotate(360deg);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.spinner {
			animation-duration: 1.4s;
		}
	}
</style>
