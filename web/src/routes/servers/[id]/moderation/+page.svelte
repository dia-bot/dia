<script lang="ts">
	import { onMount } from 'svelte';
	import { getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import SaveBar from '$lib/components/SaveBar.svelte';
	import { TRIGGERS_BY_KEY, type TriggerKey } from '$lib/moderation/automod';

	import Folder from 'lucide-svelte/icons/folder';
	import Activity from 'lucide-svelte/icons/activity';
	import ShieldAlert from 'lucide-svelte/icons/shield-alert';
	import Flame from 'lucide-svelte/icons/flame';
	import Search from 'lucide-svelte/icons/search';

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
		expires_at?: string | null;
	};

	type Offender = {
		user_id: string;
		total_points: number;
		hits: number;
		last_at: string;
	};

	type Infraction = {
		user_id: string;
		rule_id?: string;
		rule_name: string;
		trigger_type: string;
		points: number;
		reason: string;
		created_at: string;
		channel_id?: string | null;
	};

	type Stats = {
		hits_24h: number;
		hits_7d: number;
		rules: number;
		offenders: Offender[];
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
	let stats = $state<Stats>({ hits_24h: 0, hits_7d: 0, rules: 0, offenders: [] });
	let infractions = $state<Infraction[]>([]);

	let tab = $state<'cases' | 'heat'>('cases');
	let userFilter = $state('');

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	const totalCases = $derived(cases.length);
	const activeCases = $derived(cases.filter((c) => c.active).length);

	// Client-side filter on the recent cases by user id (substring match so a
	// partial paste still narrows the list).
	const filteredCases = $derived(
		userFilter.trim() ? cases.filter((c) => c.user_id.includes(userFilter.trim())) : cases
	);

	onMount(async () => {
		const [f, c, s, inf] = await Promise.all([
			api.feature(store.id, FEATURE),
			api.cases(store.id).catch(() => ({ cases: [] })),
			api
				.automodStats(store.id)
				.catch(() => ({ hits_24h: 0, hits_7d: 0, rules: 0, offenders: [] })),
			api.infractions(store.id).catch(() => ({ infractions: [] }))
		]);
		cfg = { ...defaults(), ...((f.config ?? {}) as Partial<Cfg>) };
		enabled = f.enabled;
		cases = (c.cases ?? []) as Case[];
		stats = s as Stats;
		infractions = (inf.infractions ?? []) as Infraction[];
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

	// Action chip styling per type. Single rose/charcoal palette; danger reserved
	// for destructive actions.
	function chipClass(action: string): string {
		switch ((action ?? '').toLowerCase()) {
			case 'ban':
			case 'unban':
				return 'border-[color-mix(in_srgb,var(--color-danger)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_18%,transparent)] text-[var(--color-danger)]';
			case 'kick':
				return 'border-[color-mix(in_srgb,var(--color-pink)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-pink)_15%,transparent)] text-[var(--color-pink)]';
			case 'timeout':
			case 'mute':
				return 'border-line-strong bg-blush text-accent-ink';
			case 'warn':
				return 'border-line-strong bg-ink-2 text-ink';
			default:
				return 'border-line-strong bg-surface text-muted';
		}
	}

	function triggerLabel(type: string): string {
		return TRIGGERS_BY_KEY[type as TriggerKey]?.label ?? type ?? 'Automod';
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

	// Compact relative time ("3m", "2h", "5d") for dense rows.
	function relTime(s: string): string {
		if (!s) return '';
		const d = new Date(s);
		if (isNaN(d.getTime())) return s;
		const secs = Math.max(0, (Date.now() - d.getTime()) / 1000);
		if (secs < 60) return 'just now';
		const mins = Math.floor(secs / 60);
		if (mins < 60) return `${mins}m ago`;
		const hrs = Math.floor(mins / 60);
		if (hrs < 24) return `${hrs}h ago`;
		const days = Math.floor(hrs / 24);
		if (days < 30) return `${days}d ago`;
		return fmtDate(s);
	}

	const numFmt = new Intl.NumberFormat();
</script>

<svelte:head><title>Moderation · {store.name} · Dia</title></svelte:head>

<header class="mb-6 flex items-start justify-between gap-4">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Moderation</h1>
		<p class="mt-1 text-muted">
			Log moderation actions, review your case history, and watch automod heat.
		</p>
	</div>
	<Toggle bind:checked={enabled} />
</header>

{#if !loaded}
	<div class="text-muted">Loading…</div>
{:else}
	<div class="space-y-5">
		<!-- Stats row -->
		<section class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
			<div class="card p-4">
				<div class="flex items-center gap-2 font-mono text-[11px] uppercase tracking-wide text-muted">
					<Folder size={13} class="text-accent-ink" /> Total cases
				</div>
				<div class="mt-2 text-2xl font-bold tabular-nums">{numFmt.format(totalCases)}</div>
				<div class="mt-0.5 text-xs text-faint">{numFmt.format(activeCases)} still active</div>
			</div>
			<div class="card p-4">
				<div class="flex items-center gap-2 font-mono text-[11px] uppercase tracking-wide text-muted">
					<Activity size={13} class="text-accent-ink" /> Automod · 24h
				</div>
				<div class="mt-2 text-2xl font-bold tabular-nums">{numFmt.format(stats.hits_24h)}</div>
				<div class="mt-0.5 text-xs text-faint">{numFmt.format(stats.hits_7d)} in the last 7 days</div>
			</div>
			<div class="card p-4">
				<div class="flex items-center gap-2 font-mono text-[11px] uppercase tracking-wide text-muted">
					<ShieldAlert size={13} class="text-accent-ink" /> Active rules
				</div>
				<div class="mt-2 text-2xl font-bold tabular-nums">{numFmt.format(stats.rules)}</div>
				<div class="mt-0.5 text-xs text-faint">
					<a class="text-accent-ink hover:underline" href={`/servers/${store.id}/automod`}>Edit automod</a>
				</div>
			</div>
			<div class="card p-4">
				<div class="flex items-center gap-2 font-mono text-[11px] uppercase tracking-wide text-muted">
					<Flame size={13} class="text-accent-ink" /> Offenders
				</div>
				<div class="mt-2 text-2xl font-bold tabular-nums">{numFmt.format(stats.offenders.length)}</div>
				<div class="mt-0.5 text-xs text-faint">carrying active heat</div>
			</div>
		</section>

		<!-- Settings -->
		<section class="card p-4 sm:p-6">
			<h2 class="mb-4 text-base font-semibold">Settings</h2>
			<Field
				label="Moderation log channel"
				hint="Use /ban /kick /timeout /warn in your server — actions are logged here."
			>
				<ChannelSelect bind:value={cfg.log_channel} />
			</Field>
			<label class="flex items-center gap-3">
				<Toggle bind:checked={cfg.dm_on_action} />
				<span class="text-sm">DM users when they're actioned</span>
			</label>
		</section>

		<!-- Tabs: cases / heat -->
		<section class="card overflow-hidden">
			<div class="flex items-center gap-1 border-b border-line px-4 pt-3">
				<button
					class="-mb-px border-b-2 px-3 py-2 text-sm font-medium transition {tab === 'cases'
						? 'border-accent text-ink'
						: 'border-transparent text-muted hover:text-ink'}"
					onclick={() => (tab = 'cases')}
				>
					Cases
				</button>
				<button
					class="-mb-px border-b-2 px-3 py-2 text-sm font-medium transition {tab === 'heat'
						? 'border-accent text-ink'
						: 'border-transparent text-muted hover:text-ink'}"
					onclick={() => (tab = 'heat')}
				>
					Automod heat
				</button>
			</div>

			{#if tab === 'cases'}
				<div class="p-4 sm:p-6">
					<div class="mb-4 flex flex-wrap items-center justify-between gap-3">
						<h3 class="text-sm font-semibold">Recent cases</h3>
						<label class="relative inline-flex items-center">
							<Search size={14} class="pointer-events-none absolute left-2.5 text-faint" />
							<input
								type="text"
								inputmode="numeric"
								bind:value={userFilter}
								placeholder="Filter by user ID"
								class="w-52 rounded-lg border border-line bg-surface py-1.5 pl-8 pr-3 text-sm text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
							/>
						</label>
					</div>

					{#if casesLoading}
						<div class="text-sm text-muted">Loading cases…</div>
					{:else if cases.length === 0}
						<div
							class="rounded-xl border border-dashed border-line-strong px-4 py-10 text-center text-sm text-faint"
						>
							No cases yet. Moderation actions taken in your server will appear here.
						</div>
					{:else if filteredCases.length === 0}
						<div
							class="rounded-xl border border-dashed border-line-strong px-4 py-10 text-center text-sm text-faint"
						>
							No cases for user <code class="text-accent-ink">{userFilter}</code>.
						</div>
					{:else}
						<div class="overflow-x-auto rounded-xl border border-line">
							<table class="w-full text-sm">
								<thead>
									<tr
										class="border-b border-line bg-ink-2 text-left text-xs uppercase tracking-wide text-muted"
									>
										<th class="px-3 py-2 font-medium">Case</th>
										<th class="px-3 py-2 font-medium">Action</th>
										<th class="px-3 py-2 font-medium">User</th>
										<th class="px-3 py-2 font-medium">Moderator</th>
										<th class="px-3 py-2 font-medium">Reason</th>
										<th class="px-3 py-2 font-medium">When</th>
									</tr>
								</thead>
								<tbody>
									{#each filteredCases as c (c.case)}
										<tr class="border-b border-line align-top last:border-b-0">
											<td class="px-3 py-2.5 font-medium tabular-nums text-muted">#{c.case}</td>
											<td class="px-3 py-2.5">
												<span
													class="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium capitalize {chipClass(
														c.action
													)}"
												>
													{c.action}
													{#if c.active}
														<span
															class="inline-block h-1.5 w-1.5 rounded-full bg-current opacity-70"
															title="Active"
														></span>
													{/if}
												</span>
											</td>
											<td class="px-3 py-2.5"><code class="text-accent-ink">&lt;@{c.user_id}&gt;</code></td>
											<td class="px-3 py-2.5"><code class="text-muted">&lt;@{c.moderator_id}&gt;</code></td>
											<td class="max-w-[18rem] px-3 py-2.5">
												<span class="text-ink">{c.reason || '—'}</span>
											</td>
											<td class="whitespace-nowrap px-3 py-2.5 text-faint">
												{relTime(c.created_at)}
												{#if c.active && c.expires_at}
													<div class="text-[11px] text-faint">expires {fmtDate(c.expires_at)}</div>
												{/if}
											</td>
										</tr>
									{/each}
								</tbody>
							</table>
						</div>
					{/if}
				</div>
			{:else}
				<div class="grid gap-6 p-4 sm:p-6 lg:grid-cols-2">
					<!-- Leaderboard -->
					<div>
						<h3 class="mb-3 text-sm font-semibold">Top offenders</h3>
						{#if stats.offenders.length === 0}
							<div
								class="rounded-xl border border-dashed border-line-strong px-4 py-10 text-center text-sm text-faint"
							>
								No active automod heat. Repeat offenders climb the escalation ladder here.
							</div>
						{:else}
							<ol class="overflow-hidden rounded-xl border border-line">
								{#each stats.offenders as o, i (o.user_id)}
									<li class="flex items-center gap-3 border-b border-line px-3 py-2.5 last:border-b-0">
										<span
											class="grid h-6 w-6 shrink-0 place-items-center rounded-full bg-ink-2 font-mono text-xs font-semibold text-muted"
										>
											{i + 1}
										</span>
										<div class="min-w-0 flex-1">
											<code class="text-accent-ink">&lt;@{o.user_id}&gt;</code>
											<div class="text-[11px] text-faint">
												{o.hits} hit{o.hits === 1 ? '' : 's'} · last {relTime(o.last_at)}
											</div>
										</div>
										<span
											class="shrink-0 rounded-full border border-line-strong bg-blush px-2 py-0.5 font-mono text-xs font-semibold text-accent-ink"
										>
											{numFmt.format(o.total_points)} pts
										</span>
									</li>
								{/each}
							</ol>
						{/if}
					</div>

					<!-- Recent automod actions -->
					<div>
						<h3 class="mb-3 text-sm font-semibold">Recent automod actions</h3>
						{#if infractions.length === 0}
							<div
								class="rounded-xl border border-dashed border-line-strong px-4 py-10 text-center text-sm text-faint"
							>
								No automod actions recorded yet.
							</div>
						{:else}
							<ul class="overflow-hidden rounded-xl border border-line">
								{#each infractions as inf, i (inf.created_at + i)}
									<li class="border-b border-line px-3 py-2.5 last:border-b-0">
										<div class="flex items-center justify-between gap-2">
											<span
												class="inline-flex items-center gap-1.5 rounded-full border border-line-strong bg-ink-2 px-2 py-0.5 text-xs font-medium text-ink"
											>
												{triggerLabel(inf.trigger_type)}
											</span>
											<span class="whitespace-nowrap font-mono text-[11px] text-faint">
												{relTime(inf.created_at)}
											</span>
										</div>
										<div class="mt-1.5 flex items-center justify-between gap-2 text-sm">
											<code class="text-accent-ink">&lt;@{inf.user_id}&gt;</code>
											{#if inf.points > 0}
												<span class="font-mono text-[11px] text-muted">+{inf.points} pts</span>
											{/if}
										</div>
										{#if inf.reason || inf.rule_name}
											<div class="mt-0.5 text-xs text-muted">
												{inf.reason || inf.rule_name}
											</div>
										{/if}
									</li>
								{/each}
							</ul>
						{/if}
					</div>
				</div>
			{/if}
		</section>
	</div>

	<SaveBar {dirty} {saving} onsave={save} onreset={reset} />
{/if}
