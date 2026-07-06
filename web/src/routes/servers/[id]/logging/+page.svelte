<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import { defaultLogging, LOG_CATEGORIES, type LoggingConfig } from '$lib/logging/types';
	import Field from '$lib/components/Field.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import ModerationShell, { type ModTab } from '$lib/components/moderation/ModerationShell.svelte';
	import ModSection from '$lib/components/moderation/ModSection.svelte';
	import ModToggleRow from '$lib/components/moderation/ModToggleRow.svelte';
	import TabSwipe from '$lib/components/page/TabSwipe.svelte';
	import { ScrollText, SlidersHorizontal } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'logging';

	let enabled = $state(false);
	let cfg = $state<LoggingConfig>(defaultLogging());
	let loaded = $state(false);
	let loadError = $state('');
	let saving = $state(false);
	let baseline = $state('');
	let tab = $state('events');

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

	// Load is retryable: the feature call surfaces real failures (the shell shows a
	// retry panel) instead of leaving the page on a blank skeleton forever.
	async function load() {
		loadError = '';
		loaded = false;
		try {
			const f = await api.feature(store.id, FEATURE);
			const d = defaultLogging();
			const c = (f.config ?? {}) as Partial<LoggingConfig>;
			cfg = { ...d, ...c, ignored_channels: c.ignored_channels ?? d.ignored_channels };
			enabled = f.enabled;
			baseline = JSON.stringify({ enabled, cfg });
			loaded = true;
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load server logs.';
		}
	}
	onMount(load);

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

	// Subtabs are this page's own sections (not the sidebar's modules).
	const tabs = $derived<ModTab[]>([
		{ key: 'events', label: 'Events', icon: ScrollText, badge: `${enabledCount}/${LOG_CATEGORIES.length}` },
		{ key: 'routing', label: 'Routing', icon: SlidersHorizontal }
	]);
</script>

<svelte:head><title>Server Logs · {store.name} · Dia</title></svelte:head>

<ModerationShell
	icon={ScrollText}
	title="Server Logs"
	blurb="An audit trail of messages, members and cases."
	bind:enabled
	ready={loaded}
	error={loadError}
	onretry={load}
	toggleLabel="Server logging"
	{tabs}
	bind:active={tab}
	{dirty}
	{saving}
	onsave={save}
	onreset={reset}
>
	<TabSwipe key={tab} index={tabs.findIndex((t) => t.key === tab)}>
		{#if tab === 'events'}
			<!-- ── Events catalogue ── -->
			<ModSection label="Events" count={`${enabledCount} / ${LOG_CATEGORIES.length} on`}>
				<div class="grid gap-x-8 sm:grid-cols-2">
					{#each LOG_CATEGORIES as cat (cat.key)}
						<ModToggleRow
							title={cat.label}
							desc={cat.hint}
							bind:checked={cfg[boolKey(cat.key)]}
							label={cat.label}
							divided
						/>
					{/each}
				</div>
			</ModSection>
		{:else if tab === 'routing'}
			<!-- ── Destination ── -->
			<ModSection label="Destination">
				<div class="max-w-xl">
					<Field
						label="Default log channel"
						hint="Every enabled event lands here unless you route it elsewhere under Advanced."
					>
						<ChannelSelect bind:value={cfg.channel} />
					</Field>
				</div>
			</ModSection>

			<!-- ── Advanced routing: per-category overrides + ignored channels ── -->
			<ModSection
				label="Advanced routing"
				desc="Split logs across channels and silence noisy ones."
			>
				<div class="max-w-xl">
					<Field
						label="Message log channel (override)"
						hint="Routes deleted and edited message logs here instead of the default."
					>
						<ChannelSelect bind:value={cfg.message_channel} placeholder="Use default channel…" />
					</Field>
					<Field
						label="Member log channel (override)"
						hint="Routes joins, leaves, bans and role changes here instead of the default."
					>
						<ChannelSelect bind:value={cfg.member_channel} placeholder="Use default channel…" />
					</Field>
					<Field
						label="Ignored channels"
						hint="Activity in these channels is never logged (e.g. a busy spam or bot-command channel)."
					>
						<ChannelPicker
							multiple
							value={cfg.ignored_channels ?? []}
							onChange={(v) => (cfg.ignored_channels = v as string[])}
							placeholder="Add a channel…"
						/>
					</Field>
				</div>
			</ModSection>
		{/if}
	</TabSwipe>
</ModerationShell>
