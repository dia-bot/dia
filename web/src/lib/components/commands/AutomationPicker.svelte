<script lang="ts">
	// Picks one of the guild's automations by id, for the run_automation step.
	// Self-contained: it reads the guild from context and fetches the list, so it
	// works in both the automations and custom-command editors. Renders through the
	// shared custom <Select> (Bits UI, portalled + keyboard) — not a native dropdown.
	import { getContext, onMount } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import Select from '$lib/components/Select.svelte';

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

	// The referenced automation may have been deleted since it was wired; keep it
	// listed (rather than silently clearing) so the user sees something's off.
	const missing = $derived(!!value && loaded && !list.some((a) => a.id === value));

	const options = $derived([
		{ value: '', label: loaded ? 'No automation' : 'Loading…' },
		...list.map((a) => ({ value: a.id, label: a.name })),
		...(missing ? [{ value, label: '(removed automation)' }] : [])
	]);
</script>

<Select
	bind:value={() => value, (v) => onChange(v)}
	{options}
	placeholder="Select an automation…"
/>
