<script lang="ts">
	// Picks one of the guild's automations by id, for the run_automation step.
	// Self-contained: it reads the guild from context and fetches the list, so it
	// works in both the automations and custom-command editors.
	import { getContext, onMount } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';

	let { value = '', onChange }: { value?: string; onChange: (id: string) => void } = $props();

	const store = getContext<GuildStore>(GUILD_CTX);
	let list = $state<{ id: string; name: string }[]>([]);
	let loaded = $state(false);

	onMount(async () => {
		try {
			const r = await api.automations(store.id);
			list = (r.automations ?? []).map((a: { id: string; name?: string }) => ({
				id: a.id,
				name: a.name || 'Untitled automation'
			}));
		} catch {
			/* best-effort: leave the list empty */
		}
		loaded = true;
	});

	// The referenced automation may have been deleted since it was wired.
	const missing = $derived(!!value && loaded && !list.some((a) => a.id === value));
</script>

<select class="input" {value} onchange={(e) => onChange((e.currentTarget as HTMLSelectElement).value)}>
	<option value="">{loaded ? 'Select an automation…' : 'Loading…'}</option>
	{#each list as a (a.id)}
		<option value={a.id}>{a.name}</option>
	{/each}
	{#if missing}
		<option value={value}>(removed automation)</option>
	{/if}
</select>
