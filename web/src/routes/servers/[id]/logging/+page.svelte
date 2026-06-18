<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import { defaultLogging, LOG_CATEGORIES, type LoggingConfig } from '$lib/logging/types';
	import Toggle from '$lib/components/Toggle.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import SaveBar from '$lib/components/SaveBar.svelte';
	import { ScrollText, ChevronDown, SlidersHorizontal } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'logging';

	let enabled = $state(false);
	let cfg = $state<LoggingConfig>(defaultLogging());
	let loaded = $state(false);
	let saving = $state(false);
	let baseline = $state('');
	let advancedOpen = $state(false);

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	// Boolean category keys (everything in LOG_CATEGORIES) so we can bind toggles
	// without TS complaining about the union value type.
	const boolKey = (k: keyof LoggingConfig) => k as
		| 'message_delete'
		| 'message_edit'
		| 'member_join'
		| 'member_leave'
		| 'member_ban'
		| 'member_unban'
		| 'role_changes'
		| 'mod_actions';

	onMount(async () => {
		const f = await api.feature(store.id, FEATURE);
		const d = defaultLogging();
		const c = (f.config ?? {}) as Partial<LoggingConfig>;
		cfg = { ...d, ...c, ignored_channels: c.ignored_channels ?? d.ignored_channels };
		enabled = f.enabled;
		loaded = true;
		baseline = JSON.stringify({ enabled, cfg });
	});

	async function save() {
		saving = true;
		try {
			await api.saveFeature(store.id, FEATURE, enabled, cfg);
			if (store.detail)
				store.detail.features[FEATURE] = {
					enabled,
					config: cfg as unknown as Record<string, unknown>
				};
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

	const enabledCount = $derived(LOG_CATEGORIES.filter((c) => cfg[boolKey(c.key)]).length);
</script>

<svelte:head><title>Server Logs · {store.name} · Dia</title></svelte:head>

<header class="mb-6 flex items-start justify-between gap-4">
	<div class="flex items-start gap-3">
		<span class="mt-0.5 grid size-9 shrink-0 place-items-center rounded-xl border border-line bg-ink-2 text-accent-ink">
			<ScrollText size={18} />
		</span>
		<div>
			<h1 class="text-xl font-semibold tracking-tight text-ink">Server Logs</h1>
			<p class="mt-1 max-w-xl text-sm text-muted">
				Keep an audit trail of what happens in your server: message edits and deletes, member joins
				and leaves, bans, role changes and moderation cases.
			</p>
		</div>
	</div>
	<label class="flex shrink-0 items-center gap-2.5 text-sm">
		<span class="text-muted">{enabled ? 'Enabled' : 'Disabled'}</span>
		<Toggle bind:checked={enabled} />
	</label>
</header>

{#if !loaded}
	<div class="space-y-4">
		<div class="skeleton h-28 w-full rounded-card"></div>
		<div class="skeleton h-72 w-full rounded-card"></div>
	</div>
{:else}
	<div class="space-y-5">
		<!-- Default channel -->
		<section class="card p-5">
			<div class="eyebrow mb-4">Destination</div>
			<div class="max-w-sm">
				<span class="label">Default log channel</span>
				<ChannelSelect bind:value={cfg.channel} />
				<p class="hint">Every enabled event lands here unless you route it elsewhere under Advanced.</p>
			</div>
		</section>

		<!-- Categories -->
		<section class="card p-5">
			<div class="mb-4 flex items-center justify-between gap-4">
				<div>
					<div class="eyebrow">Events</div>
					<p class="mt-0.5 text-xs text-muted">{enabledCount} of {LOG_CATEGORIES.length} enabled.</p>
				</div>
			</div>
			<div class="grid gap-x-6 gap-y-1 sm:grid-cols-2">
				{#each LOG_CATEGORIES as cat (cat.key)}
					<label class="flex items-start justify-between gap-4 rounded-xl px-2 py-2.5 transition-colors hover:bg-ink-2/40">
						<div class="min-w-0">
							<div class="text-sm font-medium text-ink">{cat.label}</div>
							<p class="text-xs text-muted">{cat.hint}</p>
						</div>
						<Toggle bind:checked={cfg[boolKey(cat.key)]} label={cat.label} />
					</label>
				{/each}
			</div>
		</section>

		<!-- Advanced: per-category routing + ignored channels -->
		<section class="card overflow-hidden">
			<button
				type="button"
				onclick={() => (advancedOpen = !advancedOpen)}
				class="flex w-full items-center justify-between gap-3 p-5 text-left"
			>
				<span class="flex items-center gap-2.5">
					<SlidersHorizontal size={15} class="text-accent-ink" />
					<span>
						<span class="block text-sm font-semibold text-ink">Advanced routing</span>
						<span class="block text-xs text-muted">Split logs across channels and silence noisy ones.</span>
					</span>
				</span>
				<ChevronDown size={16} class="shrink-0 text-faint transition-transform duration-150 {advancedOpen ? 'rotate-180' : ''}" />
			</button>
			{#if advancedOpen}
				<div class="space-y-5 border-t border-line p-5">
					<div class="grid gap-5 md:grid-cols-2">
						<div>
							<span class="label">Message log channel <span class="text-faint">(override)</span></span>
							<ChannelSelect bind:value={cfg.message_channel} placeholder="Use default channel…" />
							<p class="hint">Routes deleted and edited message logs here instead of the default.</p>
						</div>
						<div>
							<span class="label">Member log channel <span class="text-faint">(override)</span></span>
							<ChannelSelect bind:value={cfg.member_channel} placeholder="Use default channel…" />
							<p class="hint">Routes joins, leaves, bans and role changes here instead of the default.</p>
						</div>
					</div>
					<div>
						<span class="label">Ignored channels</span>
						<ChannelPicker
							multiple
							value={cfg.ignored_channels ?? []}
							onChange={(v) => (cfg.ignored_channels = v as string[])}
							placeholder="Add a channel…"
						/>
						<p class="hint">Activity in these channels is never logged (e.g. a busy spam or bot-command channel).</p>
					</div>
				</div>
			{/if}
		</section>
	</div>

	<SaveBar {dirty} {saving} onsave={save} onreset={reset} />
{/if}
