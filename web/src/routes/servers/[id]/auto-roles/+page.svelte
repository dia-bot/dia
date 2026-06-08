<script lang="ts">
	import { onMount } from 'svelte';
	import { getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import MultiSelect from '$lib/components/MultiSelect.svelte';
	import SaveBar from '$lib/components/SaveBar.svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'autorole';

	type Cfg = {
		roles: string[];
		include_bots: boolean;
		wait_for_screening: boolean;
	};

	function defaults(): Cfg {
		return {
			roles: [],
			include_bots: false,
			wait_for_screening: false
		};
	}

	let enabled = $state(false);
	let cfg = $state<Cfg>(defaults());
	let loaded = $state(false);
	let saving = $state(false);
	let baseline = $state('');

	const roleOpts = $derived(store.roleOptions());
	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	onMount(async () => {
		const f = await api.feature(store.id, FEATURE);
		const c = (f.config ?? {}) as Partial<Cfg>;
		cfg = { ...defaults(), ...c };
		enabled = f.enabled;
		loaded = true;
		baseline = JSON.stringify({ enabled, cfg });
	});

	async function save() {
		saving = true;
		try {
			await api.saveFeature(store.id, FEATURE, enabled, cfg);
			if (store.detail) store.detail.features[FEATURE] = { enabled, config: cfg };
			baseline = JSON.stringify({ enabled, cfg });
		} finally {
			saving = false;
		}
	}

	function reset() {
		const b = JSON.parse(baseline);
		enabled = b.enabled;
		cfg = b.cfg;
	}
</script>

<svelte:head><title>Auto Roles · {store.name} · Dia</title></svelte:head>

<header class="mb-6 flex items-start justify-between gap-4">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Auto Roles</h1>
		<p class="mt-1 text-muted">Automatically assign roles to members when they join the server.</p>
	</div>
	<Toggle bind:checked={enabled} />
</header>

{#if !loaded}
	<div class="text-muted">Loading…</div>
{:else}
	<div class="space-y-5">
		<section class="card p-4 sm:p-6">
			<h2 class="mb-4 text-base font-semibold">Roles</h2>
			<Field label="Roles to assign on join" hint="New members receive these roles automatically.">
				<MultiSelect bind:value={cfg.roles} options={roleOpts} placeholder="Add a role…" />
			</Field>
		</section>

		<section class="card p-4 sm:p-6">
			<h2 class="mb-4 text-base font-semibold">Options</h2>
			<label class="mb-4 flex items-center gap-3">
				<Toggle bind:checked={cfg.include_bots} />
				<span class="text-sm">Also give roles to bots</span>
			</label>
			<label class="flex items-center gap-3">
				<Toggle bind:checked={cfg.wait_for_screening} />
				<span class="text-sm">Wait until members pass membership screening</span>
			</label>
		</section>
	</div>

	<SaveBar {dirty} {saving} onsave={save} onreset={reset} />
{/if}
