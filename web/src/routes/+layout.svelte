<script lang="ts">
	import '../app.css';
	import { setCsrf } from '$lib/api';
	import type { Snippet } from 'svelte';

	let { data, children }: { data: { csrf?: string | null }; children: Snippet } = $props();

	// Make the CSRF token available to the API client (initial + on updates).
	setCsrf(data.csrf ?? '');
	$effect(() => {
		setCsrf(data.csrf ?? '');
	});
</script>

{@render children()}
