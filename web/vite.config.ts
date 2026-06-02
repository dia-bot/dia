import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [tailwindcss(), sveltekit()],
	server: {
		// Allow reaching the dev server over Tailscale MagicDNS
		// (e.g. host.tailnet.ts.net) when running `make web PUBLIC_HOST=...`.
		// Raw IPs (incl. Tailscale 100.x addresses) are always allowed by Vite.
		allowedHosts: ['.ts.net']
	}
});
