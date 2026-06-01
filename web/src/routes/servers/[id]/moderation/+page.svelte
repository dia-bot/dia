<script lang="ts">
	import { onMount } from 'svelte';
	import { getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import Select from '$lib/components/Select.svelte';
	import SaveBar from '$lib/components/SaveBar.svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'moderation';

	type Cfg = {
		log_channel: string;
		dm_on_action: boolean;
	};

	type Case = {
		case: number;
		action: string;
		user_id: string;
		moderator_id: string;
		reason: string;
		created_at: string;
		active: boolean;
	};

	function defaults(): Cfg {
		return {
			log_channel: '',
			dm_on_action: false
		};
	}

	let enabled = $state(false);
	let cfg = $state<Cfg>(defaults());
	let loaded = $state(false);
	let saving = $state(false);
	let baseline = $state('');
	let cases = $state<Case[]>([]);
	let casesLoading = $state(true);

	const channelOpts = $derived(store.textChannelOptions());
	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	onMount(async () => {
		const [f, c] = await Promise.all([
			api.feature(store.id, FEATURE),
			api.cases(store.id).catch(() => ({ cases: [] }))
		]);
		cfg = { ...defaults(), ...((f.config ?? {}) as Partial<Cfg>) };
		enabled = f.enabled;
		cases = (c.cases ?? []) as Case[];
		casesLoading = false;
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

	// Small colored chip styling per action type — single purple accent palette,
	// danger reserved for destructive actions.
	function chipClass(action: string): string {
		switch ((action ?? '').toLowerCase()) {
			case 'ban':
			case 'unban':
				return 'border-[color-mix(in_srgb,var(--color-danger)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_10%,white)] text-[var(--color-danger)]';
			case 'kick':
				return 'border-line-strong bg-[#faf3f5] text-pink';
			case 'timeout':
			case 'mute':
				return 'border-line-strong bg-blush text-accent-ink';
			case 'warn':
				return 'border-line-strong bg-[#fbf3e6] text-[#b07a16]';
			default:
				return 'border-line-strong bg-[#faf5f8] text-muted';
		}
	}

	function fmtDate(s: string): string {
		if (!s) return '';
		const d = new Date(s);
		if (isNaN(d.getTime())) return s;
		return d.toLocaleString(undefined, {
			month: 'short',
			day: 'numeric',
			hour: '2-digit',
			minute: '2-digit'
		});
	}
</script>

<svelte:head><title>Moderation · {store.name} · Dia</title></svelte:head>

<header class="mb-6 flex items-start justify-between gap-4">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Moderation</h1>
		<p class="mt-1 text-muted">Log moderation actions and review your server's case history.</p>
	</div>
	<Toggle bind:checked={enabled} />
</header>

{#if !loaded}
	<div class="text-muted">Loading…</div>
{:else}
	<div class="space-y-5">
		<!-- Settings -->
		<section class="card p-6">
			<h2 class="mb-4 text-base font-semibold">Settings</h2>
			<Field
				label="Moderation log channel"
				hint="Use /ban /kick /timeout /warn in your server — actions are logged here."
			>
				<Select bind:value={cfg.log_channel} options={channelOpts} placeholder="Select a channel…" />
			</Field>
			<label class="flex items-center gap-3">
				<Toggle bind:checked={cfg.dm_on_action} />
				<span class="text-sm">DM users when they're actioned</span>
			</label>
		</section>

		<!-- Recent cases -->
		<section class="card p-6">
			<h2 class="mb-4 text-base font-semibold">Recent cases</h2>
			{#if casesLoading}
				<div class="text-sm text-muted">Loading cases…</div>
			{:else if cases.length === 0}
				<div class="rounded-xl border border-dashed border-line-strong px-4 py-10 text-center text-sm text-faint">
					No cases yet. Moderation actions taken in your server will appear here.
				</div>
			{:else}
				<div class="overflow-hidden rounded-xl border border-line">
					<table class="w-full text-sm">
						<thead>
							<tr class="border-b border-line bg-[#faf5f8] text-left text-xs uppercase tracking-wide text-muted">
								<th class="px-3 py-2 font-medium">Case</th>
								<th class="px-3 py-2 font-medium">Action</th>
								<th class="px-3 py-2 font-medium">User</th>
								<th class="px-3 py-2 font-medium">Moderator</th>
								<th class="px-3 py-2 font-medium">Reason</th>
								<th class="px-3 py-2 font-medium">When</th>
							</tr>
						</thead>
						<tbody>
							{#each cases as c (c.case)}
								<tr class="border-b border-line last:border-b-0 align-top">
									<td class="px-3 py-2.5 font-medium tabular-nums text-muted">#{c.case}</td>
									<td class="px-3 py-2.5">
										<span
											class="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium capitalize {chipClass(
												c.action
											)}"
										>
											{c.action}
											{#if c.active}
												<span class="inline-block h-1.5 w-1.5 rounded-full bg-current opacity-70"></span>
											{/if}
										</span>
									</td>
									<td class="px-3 py-2.5"><code class="text-accent-ink">&lt;@{c.user_id}&gt;</code></td>
									<td class="px-3 py-2.5"><code class="text-muted">&lt;@{c.moderator_id}&gt;</code></td>
									<td class="px-3 py-2.5 max-w-[18rem]">
										<span class="text-ink">{c.reason || '—'}</span>
									</td>
									<td class="px-3 py-2.5 whitespace-nowrap text-faint">{fmtDate(c.created_at)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</section>
	</div>

	<SaveBar {dirty} {saving} onsave={save} onreset={reset} />
{/if}
