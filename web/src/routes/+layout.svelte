<script lang="ts">
	import '../app.css';
	import { setCsrf } from '$lib/api';
	import type { Snippet } from 'svelte';

	let { data, children }: { data: { csrf?: string | null }; children: Snippet } = $props();

	// Make the CSRF token available to the API client. Runs before any
	// user-initiated mutation (reads don't send the token).
	$effect(() => {
		setCsrf(data.csrf ?? '');
	});
</script>

{@render children()}
