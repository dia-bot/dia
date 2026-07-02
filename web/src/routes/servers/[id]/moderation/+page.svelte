<script lang="ts">
	import { onMount } from 'svelte';
	import { getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import Field from '$lib/components/Field.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import ChipInput from '$lib/components/automod/ChipInput.svelte';
	import ModerationShell, { type ModTab } from '$lib/components/moderation/ModerationShell.svelte';
	import ModSection from '$lib/components/moderation/ModSection.svelte';
	import ModToggleRow from '$lib/components/moderation/ModToggleRow.svelte';
	import ModLinkRow from '$lib/components/moderation/ModLinkRow.svelte';
	import TabSwipe from '$lib/components/page/TabSwipe.svelte';
	import { TRIGGERS_BY_KEY, type TriggerKey } from '$lib/moderation/automod';

	import Folder from 'lucide-svelte/icons/folder';
	import Activity from 'lucide-svelte/icons/activity';
	import ShieldCheck from 'lucide-svelte/icons/shield-check';
	import ShieldAlert from 'lucide-svelte/icons/shield-alert';
	import Flame from 'lucide-svelte/icons/flame';
	import Search from 'lucide-svelte/icons/search';
	import SlidersHorizontal from 'lucide-svelte/icons/sliders-horizontal';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'moderation';

	type Cfg = {
		log_channel: string;
		dm_on_action: boolean;
		reason_templates: string[];
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
			dm_on_action: false,
			reason_templates: []
		};
	}

	let enabled = $state(false);
	let cfg = $state<Cfg>(defaults());
	let loaded = $state(false);
	let loadError = $state('');
	let saving = $state(false);
	let baseline = $state('');

	let cases = $state<Case[]>([]);
	let stats = $state<Stats>({ hits_24h: 0, hits_7d: 0, rules: 0, offenders: [] });
	let infractions = $state<Infraction[]>([]);

	let tab = $state<'cases' | 'heat' | 'settings'>('cases');
	let userFilter = $state('');

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	const totalCases = $derived(cases.length);
	const activeCases = $derived(cases.filter((c) => c.active).length);

	// Client-side filter on the recent cases by user id (substring match so a
	// partial paste still narrows the list).
	const filteredCases = $derived(
		userFilter.trim() ? cases.filter((c) => c.user_id.includes(userFilter.trim())) : cases
	);

	// Load is retryable: the feature call surfaces real failures (the shell shows a
	// retry panel), while the dashboard extras degrade to empty rather than block.
	async function load() {
		loadError = '';
		loaded = false;
		try {
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
			baseline = JSON.stringify({ enabled, cfg });
			loaded = true;
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load moderation data.';
		}
	}
	onMount(load);

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

	// Action chip styling. Near-monochrome: only destructive actions (ban / unban)
	// carry the rose; everything else is a neutral ink chip. Shares the two tones
	// the automod rule chips use (see automod/tone.ts) so chips read the same
	// everywhere.
	function chipClass(action: string): string {
		switch ((action ?? '').toLowerCase()) {
			case 'ban':
			case 'unban':
				return 'border-accent/30 bg-blush text-accent-ink';
			default:
				return 'border-line bg-ink-2 text-muted';
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

	const stat = $derived([
		{
			icon: Folder,
			label: 'Total cases',
			value: numFmt.format(totalCases),
			sub: `${numFmt.format(activeCases)} still active`
		},
		{
			icon: Activity,
			label: 'Automod · 24h',
			value: numFmt.format(stats.hits_24h),
			sub: `${numFmt.format(stats.hits_7d)} in the last 7 days`
		},
		{
			icon: ShieldAlert,
			label: 'Active rules',
			value: numFmt.format(stats.rules),
			sub: 'filtering content'
		},
		{
			icon: Flame,
			label: 'Offenders',
			value: numFmt.format(stats.offenders.length),
			sub: 'carrying active heat'
		}
	]);

	// Subtabs are this page's own sections (not the sidebar's modules). Counts ride
	// along so the strip doubles as an at-a-glance summary.
	const tabs = $derived<ModTab[]>([
		{ key: 'cases', label: 'Cases', icon: Folder, badge: totalCases || '' },
		{ key: 'heat', label: 'Automod heat', icon: Flame, badge: stats.offenders.length || '' },
		{ key: 'settings', label: 'Settings', icon: SlidersHorizontal }
	]);
</script>

<svelte:head><title>Moderation · {store.name} · Dia</title></svelte:head>

<ModerationShell
	icon={ShieldCheck}
	title="Moderation"
	blurb="Cases, mod-action logging and live automod heat."
	bind:enabled
	ready={loaded}
	error={loadError}
	onretry={load}
	toggleLabel="Moderation logging"
	{tabs}
	bind:active={tab}
	{dirty}
	{saving}
	onsave={save}
	onreset={reset}
>
	{#snippet skeleton()}
		<div class="grid grid-cols-2 gap-px border-b border-line bg-line lg:grid-cols-4">
			{#each Array(4) as _, i (i)}
				<div class="bg-bg px-4 py-4 sm:px-5">
					<div class="skeleton h-3 w-20 rounded"></div>
					<div class="skeleton mt-3 h-7 w-16 rounded"></div>
					<div class="skeleton mt-2.5 h-3 w-24 rounded"></div>
				</div>
			{/each}
		</div>
		<div class="px-4 py-5 sm:px-5">
			<div class="skeleton h-9 w-52 rounded-lg"></div>
			<div class="skeleton mt-4 h-64 w-full rounded-lg"></div>
		</div>
	{/snippet}

	<!-- ── Persistent stat strip: a summary that rides above every subtab ── -->
	<section class="grid grid-cols-2 gap-px border-b border-line bg-line lg:grid-cols-4">
		{#each stat as s (s.label)}
			{@const Icon = s.icon}
			<div class="bg-bg px-4 py-4 sm:px-5">
				<div
					class="flex items-center gap-1.5 font-mono text-[10px] font-medium uppercase tracking-[0.12em] text-faint"
				>
					<Icon size={12} class="text-faint" />
					{s.label}
				</div>
				<div class="mt-2 text-[26px] font-semibold leading-none tabular-nums text-ink">
					{s.value}
				</div>
				<div class="mt-1.5 text-[11.5px] text-muted">{s.sub}</div>
			</div>
		{/each}
	</section>

	<TabSwipe key={tab} index={tabs.findIndex((t) => t.key === tab)}>
		{#if tab === 'cases'}
			<!-- ── Cases ── -->
			<section class="border-b border-line">
				<div class="flex flex-wrap items-center gap-x-3 gap-y-2 px-4 pt-3.5 sm:px-5">
					<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
						Recent cases
					</span>
					<label class="relative ml-auto inline-flex items-center">
						<Search size={14} class="pointer-events-none absolute left-2.5 text-faint" />
						<input
							type="text"
							inputmode="numeric"
							bind:value={userFilter}
							placeholder="Filter by user ID"
							class="w-44 rounded-lg border border-line bg-ink-2 py-1.5 pl-8 pr-3 text-[13px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none sm:w-52"
						/>
					</label>
				</div>

				<div class="pt-3">
					{#if cases.length === 0}
						<div class="px-4 py-12 text-center text-sm text-faint sm:px-5">
							No cases yet. Moderation actions taken in your server will appear here.
						</div>
					{:else if filteredCases.length === 0}
						<div class="px-4 py-12 text-center text-sm text-faint sm:px-5">
							No cases for user <code class="text-accent-ink">{userFilter}</code>.
						</div>
					{:else}
						<div class="overflow-x-auto">
							<table class="w-full text-[13px]">
								<thead>
									<tr
										class="border-y border-line bg-ink-2 text-left font-mono text-[10px] uppercase tracking-wide text-faint"
									>
										<th class="px-4 py-2 font-medium sm:px-5">Case</th>
										<th class="px-3 py-2 font-medium">Action</th>
										<th class="px-3 py-2 font-medium">User</th>
										<th class="hidden px-3 py-2 font-medium md:table-cell">Moderator</th>
										<th class="px-3 py-2 font-medium">Reason</th>
										<th class="px-4 py-2 font-medium sm:px-5">When</th>
									</tr>
								</thead>
								<tbody>
									{#each filteredCases as c (c.case)}
										<tr class="border-b border-line align-top last:border-b-0 hover:bg-ink-2/40">
											<td class="px-4 py-2.5 font-medium tabular-nums text-muted sm:px-5">#{c.case}</td>
											<td class="px-3 py-2.5">
												<span
													class="inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-[11px] font-medium capitalize {chipClass(
														c.action
													)}"
												>
													{c.action}
													{#if c.active}
														<span
															class="inline-block size-1.5 rounded-full bg-current opacity-70"
															title="Active"
														></span>
													{/if}
												</span>
											</td>
											<td class="px-3 py-2.5"
												><code class="text-accent-ink">&lt;@{c.user_id}&gt;</code></td
											>
											<td class="hidden px-3 py-2.5 md:table-cell"
												><code class="text-muted">&lt;@{c.moderator_id}&gt;</code></td
											>
											<td class="max-w-[22rem] px-3 py-2.5">
												<span class="text-ink">{c.reason || '—'}</span>
											</td>
											<td class="whitespace-nowrap px-4 py-2.5 text-faint sm:px-5">
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
			</section>
		{:else if tab === 'heat'}
			<!-- ── Automod heat ── -->
			<section class="border-b border-line">
				<div class="grid lg:grid-cols-2 lg:divide-x lg:divide-line">
					<!-- Leaderboard -->
					<div class="px-4 py-4 sm:px-5">
						<div class="eyebrow mb-3">Top offenders</div>
						{#if stats.offenders.length === 0}
							<p class="py-6 text-sm text-faint">
								No active automod heat. Repeat offenders climb the escalation ladder here.
							</p>
						{:else}
							<ol>
								{#each stats.offenders as o, i (o.user_id)}
									<li class="flex items-center gap-3 border-b border-line py-2.5 last:border-b-0">
										<span
											class="grid size-6 shrink-0 place-items-center rounded-full bg-ink-2 font-mono text-xs font-semibold text-muted"
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
											class="shrink-0 rounded-md border border-line-strong bg-ink-2 px-2 py-0.5 font-mono text-xs font-semibold tabular-nums text-muted"
										>
											{numFmt.format(o.total_points)} pts
										</span>
									</li>
								{/each}
							</ol>
						{/if}
					</div>

					<!-- Recent automod actions -->
					<div class="border-t border-line px-4 py-4 sm:px-5 lg:border-t-0">
						<div class="eyebrow mb-3">Recent automod actions</div>
						{#if infractions.length === 0}
							<p class="py-6 text-sm text-faint">No automod actions recorded yet.</p>
						{:else}
							<ul>
								{#each infractions as inf, i (inf.created_at + i)}
									<li class="border-b border-line py-2.5 last:border-b-0">
										<div class="flex items-center justify-between gap-2">
											<span
												class="inline-flex items-center gap-1.5 rounded-md border border-line-strong bg-ink-2 px-2 py-0.5 text-[11px] font-medium text-ink"
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
			</section>
		{:else}
			<!-- ── Settings ── -->
			<ModSection label="Settings" desc="Logging and reason autocomplete for /ban, /kick, /timeout, /warn.">
				<div class="max-w-xl">
					<Field
						label="Moderation log channel"
						hint="Use /ban /kick /timeout /warn in your server — actions are logged here."
					>
						<ChannelSelect bind:value={cfg.log_channel} />
					</Field>
					<ModToggleRow
						title="DM users when they're actioned"
						desc="Send the member a copy of the reason when a case is opened against them."
						bind:checked={cfg.dm_on_action}
						label="DM on action"
					/>
					<div class="mt-4 border-t border-line pt-5">
						<Field
							label="Reason templates"
							hint="Power the reason autocomplete on /ban, /kick, /timeout and /warn. Type a reason and press Enter to add it."
						>
							<ChipInput bind:value={cfg.reason_templates} placeholder="e.g. Spamming in chat" />
						</Field>
					</div>
				</div>
			</ModSection>

			<section class="border-b border-line">
				<ModLinkRow
					href={`/servers/${store.id}/automod`}
					icon={ShieldAlert}
					title="Tune automod rules"
					desc="Filters, the escalation ladder and anti-raid."
				/>
			</section>
		{/if}
	</TabSwipe>
</ModerationShell>
