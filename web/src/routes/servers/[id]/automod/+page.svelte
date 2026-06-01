<script lang="ts">
	import { onMount } from 'svelte';
	import { getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import Select from '$lib/components/Select.svelte';
	import MultiSelect from '$lib/components/MultiSelect.svelte';
	import SaveBar from '$lib/components/SaveBar.svelte';
	import { X } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'automod';

	type Action = 'delete' | 'warn' | 'timeout';
	type Cfg = {
		banned_words: string[];
		block_invites: boolean;
		block_links: boolean;
		max_mentions: number;
		action: Action;
		timeout_seconds: number;
		ignored_channels: string[];
		ignored_roles: string[];
	};

	function defaults(): Cfg {
		return {
			banned_words: [],
			block_invites: false,
			block_links: false,
			max_mentions: 5,
			action: 'delete',
			timeout_seconds: 600,
			ignored_channels: [],
			ignored_roles: []
		};
	}

	const actionOpts = [
		{ value: 'delete', label: 'Delete only' },
		{ value: 'warn', label: 'Warn' },
		{ value: 'timeout', label: 'Timeout' }
	];

	let enabled = $state(false);
	let cfg = $state<Cfg>(defaults());
	let loaded = $state(false);
	let saving = $state(false);
	let baseline = $state('');
	let wordInput = $state('');

	const channelOpts = $derived(store.textChannelOptions());
	const roleOpts = $derived(store.roleOptions());
	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	onMount(async () => {
		const f = await api.feature(store.id, FEATURE);
		const d = defaults();
		const c = (f.config ?? {}) as Partial<Cfg>;
		cfg = { ...d, ...c };
		enabled = f.enabled;
		loaded = true;
		baseline = JSON.stringify({ enabled, cfg });
	});

	function addWord() {
		const w = wordInput.trim().toLowerCase();
		wordInput = '';
		if (!w || cfg.banned_words.includes(w)) return;
		cfg.banned_words = [...cfg.banned_words, w];
	}

	function wordKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ',') {
			e.preventDefault();
			addWord();
		}
	}

	function removeWord(w: string) {
		cfg.banned_words = cfg.banned_words.filter((x) => x !== w);
	}

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
		wordInput = '';
	}
</script>

<svelte:head><title>AutoMod · {store.name} · Dia</title></svelte:head>

<header class="mb-6 flex items-start justify-between gap-4">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">AutoMod</h1>
		<p class="mt-1 text-muted">Automatically filter spam, invites and banned words.</p>
	</div>
	<Toggle bind:checked={enabled} />
</header>

{#if !loaded}
	<div class="text-muted">Loading…</div>
{:else}
	<div class="space-y-5">
		<!-- Rules -->
		<section class="card p-6">
			<h2 class="mb-4 text-base font-semibold">Rules</h2>

			<label class="mb-4 flex items-center justify-between gap-4">
				<span class="text-sm">Block Discord invites</span>
				<Toggle bind:checked={cfg.block_invites} />
			</label>
			<label class="mb-5 flex items-center justify-between gap-4">
				<span class="text-sm">Block links</span>
				<Toggle bind:checked={cfg.block_links} />
			</label>

			<Field label="Max mentions" hint="Messages with more mentions than this trigger the action. Set 0 to disable.">
				<input class="input" type="number" min="0" bind:value={cfg.max_mentions} />
			</Field>

			<Field label="Banned words" hint="Type a word and press Enter to add it.">
				{#if cfg.banned_words.length}
					<div class="mb-2 flex flex-wrap gap-1.5">
						{#each cfg.banned_words as w (w)}
							<span
								class="inline-flex items-center gap-1 rounded-full bg-blush px-2.5 py-1 text-xs font-medium text-accent-ink"
							>
								{w}
								<button type="button" onclick={() => removeWord(w)} aria-label="Remove">
									<X size={12} />
								</button>
							</span>
						{/each}
					</div>
				{/if}
				<input
					class="input"
					placeholder="Add a word…"
					bind:value={wordInput}
					onkeydown={wordKeydown}
					onblur={addWord}
				/>
			</Field>
		</section>

		<!-- Action -->
		<section class="card p-6">
			<h2 class="mb-4 text-base font-semibold">Action</h2>
			<Field label="When a rule is broken">
				<Select bind:value={cfg.action} options={actionOpts} />
			</Field>
			{#if cfg.action === 'timeout'}
				<Field label="Timeout duration" hint="How long the member is timed out, in seconds.">
					<input class="input" type="number" min="1" bind:value={cfg.timeout_seconds} />
				</Field>
			{/if}
		</section>

		<!-- Exemptions -->
		<section class="card p-6">
			<h2 class="mb-4 text-base font-semibold">Exemptions</h2>
			<Field label="Ignored channels" hint="AutoMod will not act in these channels.">
				<MultiSelect bind:value={cfg.ignored_channels} options={channelOpts} placeholder="Add a channel…" />
			</Field>
			<Field label="Ignored roles" hint="Members with these roles are never moderated.">
				<MultiSelect bind:value={cfg.ignored_roles} options={roleOpts} placeholder="Add a role…" />
			</Field>
		</section>
	</div>

	<SaveBar {dirty} {saving} onsave={save} onreset={reset} />
{/if}
