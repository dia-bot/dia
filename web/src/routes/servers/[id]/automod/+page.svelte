<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import {
		defaultConfig,
		newRule,
		type AutomodConfig,
		type AutomodRule,
		type EscalationTier,
		type TriggerKey
	} from '$lib/moderation/automod';
	import Toggle from '$lib/components/Toggle.svelte';
	import Select from '$lib/components/Select.svelte';
	import NumberField from '$lib/components/ui/NumberField.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import Field from '$lib/components/Field.svelte';
	import ModerationShell, { type ModTab } from '$lib/components/moderation/ModerationShell.svelte';
	import ModSection from '$lib/components/moderation/ModSection.svelte';
	import ModToggleRow from '$lib/components/moderation/ModToggleRow.svelte';
	import ModLinkRow from '$lib/components/moderation/ModLinkRow.svelte';
	import TabSwipe from '$lib/components/page/TabSwipe.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import RuleCard from '$lib/components/automod/RuleCard.svelte';
	import RuleEditor from '$lib/components/automod/RuleEditor.svelte';
	import TriggerPicker from '$lib/components/automod/TriggerPicker.svelte';
	import AutomationPicker from '$lib/components/commands/AutomationPicker.svelte';
	import { Plus, Zap, Trash2, Lock, ShieldAlert, Users, Server, SlidersHorizontal } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'automod';

	let enabled = $state(false);
	let cfg = $state<AutomodConfig>(defaultConfig());
	let loaded = $state(false);
	let loadError = $state('');
	let saving = $state(false);
	let baseline = $state('');

	let tab = $state('rules');
	let editingId = $state<string | null>(null);
	let pickerOpen = $state(false);
	let pendingDelete = $state<string | null>(null);

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	// Load is retryable: the feature call surfaces real failures (the shell shows a
	// retry panel). Native Discord AutoMod loads best-effort afterwards and owns its
	// own error state, so it never blocks the page.
	async function load() {
		loadError = '';
		loaded = false;
		try {
			const f = await api.feature(store.id, FEATURE);
			const d = defaultConfig();
			const c = (f.config ?? {}) as Partial<AutomodConfig>;
			cfg = {
				...d,
				...c,
				rules: c.rules ?? d.rules,
				escalation: { ...d.escalation, ...(c.escalation ?? {}) },
				raid: { ...d.raid, ...(c.raid ?? {}) },
				exempt_roles: c.exempt_roles ?? d.exempt_roles,
				exempt_channels: c.exempt_channels ?? d.exempt_channels
			};
			enabled = f.enabled;
			baseline = JSON.stringify({ enabled, cfg });
			loaded = true;
			loadNativeRules();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load automod.';
		}
	}
	onMount(load);

	// ── Anti-raid ───────────────────────────────────────────────────────────────
	const raidActionOpts = [
		{ value: 'kick', label: 'Kick joiners' },
		{ value: 'ban', label: 'Ban joiners' },
		{ value: 'timeout', label: 'Timeout joiners' }
	];
	function setRaidAction(v: string) {
		cfg.raid.action = v as 'kick' | 'ban' | 'timeout';
		if (cfg.raid.action === 'timeout' && !cfg.raid.timeout_seconds) cfg.raid.timeout_seconds = 600;
	}

	// ── Native Discord AutoMod ───────────────────────────────────────────────────
	// These live on Discord's side (not Dia's rule engine). We load them best-effort:
	// the bot may lack Manage Server, in which case we show a friendly notice.
	type NativeRule = {
		id?: string;
		name: string;
		enabled: boolean;
		trigger_type: number;
		event_type: number;
		trigger_metadata?: {
			mention_total_limit?: number;
			keyword_filter?: string[];
			presets?: number[];
		};
		actions?: { type: number; metadata?: Record<string, unknown> }[];
	};
	let nativeRules = $state<NativeRule[]>([]);
	let nativeLoaded = $state(false);
	let nativeError = $state('');
	let nativeBusy = $state(false);
	let mentionLimit = $state(5);

	const NATIVE_TRIGGER_LABELS: Record<number, string> = {
		1: 'Keyword',
		3: 'Spam',
		4: 'Keyword preset',
		5: 'Mention spam'
	};

	async function loadNativeRules() {
		try {
			const r = await api.automodRules(store.id);
			nativeRules = (r.rules ?? []) as NativeRule[];
			nativeError = '';
		} catch (e) {
			nativeError = e instanceof Error ? e.message : 'Could not load native rules.';
		} finally {
			nativeLoaded = true;
		}
	}

	function findNative(triggerType: number): NativeRule | undefined {
		return nativeRules.find((r) => r.trigger_type === triggerType);
	}

	async function saveNative(rule: NativeRule) {
		if (nativeBusy) return;
		nativeBusy = true;
		try {
			await api.saveAutomodRule(store.id, rule);
			await loadNativeRules();
		} catch (e) {
			nativeError = e instanceof Error ? e.message : 'Could not save the native rule.';
		} finally {
			nativeBusy = false;
		}
	}
	async function removeNative(rule: NativeRule) {
		if (nativeBusy || !rule.id) return;
		nativeBusy = true;
		try {
			await api.deleteAutomodRule(store.id, rule.id);
			await loadNativeRules();
		} catch (e) {
			nativeError = e instanceof Error ? e.message : 'Could not delete the native rule.';
		} finally {
			nativeBusy = false;
		}
	}

	function addMentionSpam() {
		saveNative({
			name: 'Block mention spam',
			enabled: true,
			trigger_type: 5,
			event_type: 1,
			trigger_metadata: { mention_total_limit: Math.max(1, mentionLimit) },
			actions: [{ type: 1 }]
		});
	}
	function addKeywordPreset() {
		// presets: 1 = profanity, 2 = sexual content, 3 = slurs.
		saveNative({
			name: 'Block profanity & slurs',
			enabled: true,
			trigger_type: 4,
			event_type: 1,
			trigger_metadata: { presets: [1, 2, 3] },
			actions: [{ type: 1 }]
		});
	}
	function toggleNative(rule: NativeRule) {
		saveNative({ ...rule, enabled: !rule.enabled });
	}

	// ── Rules ──────────────────────────────────────────────────────────────────
	function pickTrigger(key: TriggerKey) {
		const r = newRule(key);
		cfg.rules = [...cfg.rules, r];
		editingId = r.id;
	}
	function duplicateRule(rule: AutomodRule) {
		const copy: AutomodRule = JSON.parse(JSON.stringify(rule));
		copy.id = `${rule.id}_copy${Math.random().toString(36).slice(2, 6)}`;
		copy.name = `${rule.name} (copy)`;
		const i = cfg.rules.findIndex((r) => r.id === rule.id);
		cfg.rules = [...cfg.rules.slice(0, i + 1), copy, ...cfg.rules.slice(i + 1)];
		editingId = copy.id;
	}
	function confirmDelete() {
		const id = pendingDelete;
		pendingDelete = null;
		if (!id) return;
		cfg.rules = cfg.rules.filter((r) => r.id !== id);
		if (editingId === id) editingId = null;
	}

	// ── Escalation tiers ────────────────────────────────────────────────────────
	const tierActionOpts = [
		{ value: 'timeout', label: 'Timeout' },
		{ value: 'kick', label: 'Kick' },
		{ value: 'ban', label: 'Ban' },
		{ value: 'run_automation', label: 'Run automation' }
	];
	function addTier() {
		const last = cfg.escalation.tiers.at(-1);
		const points = last ? last.points + 3 : 3;
		cfg.escalation.tiers = [...cfg.escalation.tiers, { points, action: 'timeout', duration: 600 }];
	}
	function removeTier(i: number) {
		cfg.escalation.tiers = cfg.escalation.tiers.filter((_, idx) => idx !== i);
	}
	function setTierAction(t: EscalationTier, v: string) {
		t.action = v as EscalationTier['action'];
		if (t.action === 'timeout' && !t.duration) t.duration = 600;
		if (t.action === 'run_automation' && t.automation === undefined) t.automation = '';
	}

	// ── Save / reset ────────────────────────────────────────────────────────────
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
		editingId = null;
	}

	// Event fields the automod hit exposes to Automations. These mirror the
	// .Event.* keys the automations runtime builds (snake_case) plus the offending
	// member, which is the top-level .User / .Member scope (not under .Event).
	const eventFields = [
		{ token: '{{ .Event.rule_name }}', desc: 'Name of the rule that fired' },
		{ token: '{{ .Event.trigger_type }}', desc: 'Trigger key (e.g. words, spam)' },
		{ token: '{{ .Event.reason }}', desc: 'Human description of the hit' },
		{ token: '{{ .Event.actions }}', desc: 'Action types applied, in order' },
		{ token: '{{ .Event.points }}', desc: 'Points added by this hit' },
		{ token: '{{ .Event.total_points }}', desc: 'User infraction total after' },
		{ token: '{{ .Event.escalated }}', desc: 'Escalation action, if any' },
		{ token: '{{ .User.Mention }}', desc: 'The offending member' },
		{ token: '{{ .Event.channel_id }}', desc: 'Where it happened' },
		{ token: '{{ .Event.content }}', desc: 'Offending message (truncated)' }
	];

	// Subtabs are this page's own sections. Counts and status pips ride along so the
	// strip doubles as an at-a-glance summary.
	const tabs = $derived<ModTab[]>([
		{ key: 'rules', label: 'Rules', icon: ShieldAlert, badge: cfg.rules.length || '' },
		{ key: 'escalation', label: 'Escalation', icon: Zap, dot: cfg.escalation.enabled },
		{ key: 'raid', label: 'Anti-raid', icon: Users, dot: cfg.raid.enabled },
		{ key: 'native', label: 'Discord native', icon: Server, badge: nativeRules.length || '' },
		{ key: 'settings', label: 'Settings', icon: SlidersHorizontal }
	]);
</script>

<svelte:head><title>Automod · {store.name} · Dia</title></svelte:head>

<ModerationShell
	icon={ShieldAlert}
	title="Automod"
	blurb="Rule filters, an escalation ladder and anti-raid."
	bind:enabled
	ready={loaded}
	error={loadError}
	onretry={load}
	toggleLabel="Automod"
	{tabs}
	bind:active={tab}
	{dirty}
	{saving}
	onsave={save}
	onreset={reset}
>
	{#snippet actions()}
		<a
			href="/servers/{store.id}/automations"
			class="hidden items-center gap-1.5 rounded-lg border border-line-strong bg-surface px-2.5 py-1.5 text-[12px] font-medium text-muted transition-colors hover:text-ink sm:inline-flex"
		>
			<Zap size={13} class="text-faint" /> Build an automation
		</a>
	{/snippet}

	<TabSwipe key={tab} index={tabs.findIndex((t) => t.key === tab)}>
		{#if tab === 'rules'}
			<!-- Rules -->
			<ModSection
				label="Rules"
				desc={cfg.rules.length === 0 ? 'No rules yet.' : `Checked top to bottom.`}
				count={cfg.rules.length}
			>
				{#snippet actions()}
					<button
						type="button"
						onclick={() => (pickerOpen = true)}
						class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line-strong px-2.5 text-[12px] font-medium text-ink transition-colors hover:bg-ink-2"
					>
						<Plus size={13} /> Add rule
					</button>
				{/snippet}

				{#if cfg.rules.length === 0}
					<p class="py-10 text-center text-sm text-faint">
						No rules yet. Add your first rule to start filtering: pick a trigger (blocked words, spam,
						invites, mention floods and more), then choose what happens when it trips.
					</p>
				{:else}
					<div class="space-y-3">
						{#each cfg.rules as rule (rule.id)}
							<div>
								<RuleCard
									{rule}
									editing={editingId === rule.id}
									onedit={() => (editingId = editingId === rule.id ? null : rule.id)}
									onduplicate={() => duplicateRule(rule)}
									ondelete={() => (pendingDelete = rule.id)}
									onflow={() => goto('/servers/' + store.id + '/automations/automod.rule.' + rule.id)}
								/>
								{#if editingId === rule.id}
									<div class="mt-2">
										<RuleEditor {rule} onclose={() => (editingId = null)} />
									</div>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</ModSection>
		{:else if tab === 'escalation'}
			<!-- Escalation ladder -->
			<ModSection label="Escalation ladder" desc="Repeat offenders climb a points ladder.">
				{#snippet actions()}
					<Toggle bind:checked={cfg.escalation.enabled} label="Escalation enabled" />
				{/snippet}

				<p class="max-w-2xl text-xs text-muted">
					Rules that add infraction points feed this ladder. As a member's active points cross a
					tier, the matching action fires automatically: a timeout, kick, ban, or a full automation
					flow you build (DM, log, custom branching, and more).
				</p>

				{#if cfg.escalation.enabled}
					<div class="mt-4 max-w-xs">
						<span class="label">Points decay</span>
						<div class="flex items-center gap-2">
							<div class="w-32">
								<NumberField bind:value={cfg.escalation.decay_hours} min={1} max={8760} />
							</div>
							<span class="text-xs text-muted">hours until a point is forgiven</span>
						</div>
					</div>

					<div class="mt-4">
						{#each cfg.escalation.tiers as tier, i (i)}
							<div class="flex flex-wrap items-end gap-3 border-b border-line py-3 last:border-b-0">
								<div>
									<span class="label !mb-1 text-xs">At points</span>
									<div class="w-24">
										<NumberField bind:value={tier.points} min={1} max={100} />
									</div>
								</div>
								<div>
									<span class="label !mb-1 text-xs">Action</span>
									<div class="w-36">
										<Select
											bind:value={() => tier.action, (v) => setTierAction(tier, v)}
											options={tierActionOpts}
										/>
									</div>
								</div>
								{#if tier.action === 'timeout'}
									<div>
										<span class="label !mb-1 text-xs">For</span>
										<div class="flex items-center gap-2">
											<div class="w-28">
												<NumberField bind:value={tier.duration} min={1} />
											</div>
											<span class="text-xs text-muted">seconds</span>
										</div>
									</div>
								{:else if tier.action === 'run_automation'}
									<div class="min-w-[13rem] flex-1">
										<span class="label !mb-1 text-xs">Automation</span>
										<AutomationPicker
											value={tier.automation ?? ''}
											onChange={(id) => (tier.automation = id)}
										/>
										{#if !tier.automation}
											<p class="mt-1 text-[11px] text-accent-ink">
												Pick an automation to run at this threshold.
											</p>
										{/if}
									</div>
								{/if}
								<button
									type="button"
									onclick={() => removeTier(i)}
									class="ml-auto grid size-9 place-items-center rounded-lg text-faint transition-colors hover:text-danger"
									aria-label="Remove tier"
								>
									<Trash2 size={15} />
								</button>
							</div>
						{/each}
					</div>

					<button
						type="button"
						onclick={addTier}
						class="mt-3 inline-flex items-center gap-1.5 rounded-md border border-line-strong px-2.5 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-ink-2"
					>
						<Plus size={13} /> Add tier
					</button>
				{:else}
					<p class="mt-3 text-xs text-faint">
						Escalation is off. Rules still run their own actions, but repeat offenders are not
						punished on a points ladder.
					</p>
				{/if}
			</ModSection>
		{:else if tab === 'raid'}
			<!-- Anti-raid -->
			<ModSection label="Anti-raid" desc="Clamp down on join floods.">
				{#snippet actions()}
					<Toggle bind:checked={cfg.raid.enabled} label="Anti-raid enabled" />
				{/snippet}

				<p class="max-w-2xl text-xs text-muted">
					Join-velocity guard. When a burst of members arrives faster than your threshold, the server
					enters raid mode and new joiners are actioned until it calms.
				</p>

				{#if cfg.raid.enabled}
					<div class="mt-4 flex flex-wrap items-end gap-2 text-sm text-muted">
						<div>
							<span class="label !mb-1 text-xs">Trips at</span>
							<div class="w-24">
								<NumberField bind:value={cfg.raid.threshold} min={2} max={500} />
							</div>
						</div>
						<span class="pb-2.5">joins within</span>
						<div>
							<span class="label !mb-1 text-xs">Window</span>
							<div class="w-24">
								<NumberField bind:value={cfg.raid.window} min={1} max={600} />
							</div>
						</div>
						<span class="pb-2.5">seconds.</span>
					</div>

					<div class="mt-5 grid max-w-2xl gap-x-6 sm:grid-cols-2">
						<Field label="Action on joiners" hint="Applied to members who join while raid mode is active.">
							<Select
								bind:value={() => cfg.raid.action, setRaidAction}
								options={raidActionOpts}
							/>
						</Field>
						{#if cfg.raid.action === 'timeout'}
							<div class="mb-5">
								<span class="label">Timeout duration</span>
								<div class="flex items-center gap-2">
									<div class="w-28">
										<NumberField bind:value={cfg.raid.timeout_seconds} min={1} max={2419200} />
									</div>
									<span class="text-xs text-muted">seconds</span>
								</div>
							</div>
						{/if}
					</div>

					<div class="max-w-2xl border-t border-line">
						<ModToggleRow
							title="Only action new accounts"
							desc="During a raid, spare established accounts and only catch fresh ones."
							bind:checked={cfg.raid.only_new_accounts}
							label="Only new accounts"
						/>
					</div>
					{#if cfg.raid.only_new_accounts}
						<div class="max-w-xs">
							<span class="label">New account threshold</span>
							<div class="flex items-center gap-2">
								<div class="w-28">
									<NumberField bind:value={cfg.raid.new_account_hours} min={1} max={8760} />
								</div>
								<span class="text-xs text-muted">hours old or younger</span>
							</div>
						</div>
					{/if}

					<div class="mt-4 max-w-sm">
						<Field label="Alert channel" hint="Optional. A heads-up is posted here when raid mode trips and lifts.">
							<ChannelSelect bind:value={cfg.raid.alert_channel} />
						</Field>
					</div>
				{:else}
					<p class="mt-3 text-xs text-faint">
						Anti-raid is off. Turn it on to automatically clamp down when a flood of accounts joins at
						once.
					</p>
				{/if}
			</ModSection>
		{:else if tab === 'native'}
			<!-- Native Discord AutoMod -->
			<ModSection
				label="Discord AutoMod (native)"
				desc="Runs on Discord's servers, before the bot sees a message."
			>
				<p class="max-w-2xl text-xs text-muted">
					These rules run on Discord's own servers at zero latency, before a message even reaches the
					bot. They are separate from Dia's rules above. Use them for the heaviest, always-on blocks;
					use Dia's rules for everything richer.
				</p>

				{#if !nativeLoaded}
					<div class="skeleton mt-4 h-20 w-full rounded-lg"></div>
				{:else if nativeError}
					<div class="mt-4 flex items-start gap-2.5 text-xs text-muted">
						<Lock size={14} class="mt-0.5 shrink-0 text-faint" />
						<div>
							<span class="font-medium text-ink">Native AutoMod isn't available right now.</span>
							Make sure Dia has the <span class="font-mono">Manage Server</span> permission, then reload.
							<span class="block text-faint">({nativeError})</span>
						</div>
					</div>
				{:else}
					<!-- Existing native rules -->
					{#if nativeRules.length}
						<ul class="mt-4">
							{#each nativeRules as r (r.id ?? r.name)}
								<li class="flex items-center justify-between gap-3 border-b border-line py-2.5 last:border-b-0">
									<div class="min-w-0">
										<div class="flex items-center gap-2">
											<span class="truncate text-sm font-medium text-ink">{r.name}</span>
											<span class="shrink-0 rounded-full border border-line-strong bg-surface px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide text-muted">
												{NATIVE_TRIGGER_LABELS[r.trigger_type] ?? 'Rule'}
											</span>
										</div>
										{#if r.trigger_type === 5 && r.trigger_metadata?.mention_total_limit}
											<p class="text-[11px] text-faint">Blocks messages with more than {r.trigger_metadata.mention_total_limit} mentions.</p>
										{/if}
									</div>
									<div class="flex shrink-0 items-center gap-2">
										<Toggle checked={r.enabled} disabled={nativeBusy} onchange={() => toggleNative(r)} label="Rule enabled" />
										<button
											type="button"
											onclick={() => removeNative(r)}
											disabled={nativeBusy}
											class="grid size-8 place-items-center rounded-lg text-faint transition-colors hover:text-danger disabled:opacity-40"
											aria-label="Delete native rule"
										>
											<Trash2 size={14} />
										</button>
									</div>
								</li>
							{/each}
						</ul>
					{:else}
						<p class="mt-4 text-xs text-faint">No native rules yet. Add a preset below.</p>
					{/if}

					<!-- Quick-add presets -->
					<div class="mt-4 border-t border-line pt-4">
						<div class="eyebrow">Quick add</div>
						<div class="mt-3 flex flex-wrap items-end gap-3">
							<div>
								<span class="label !mb-1 text-xs">Max mentions per message</span>
								<div class="w-24">
									<NumberField bind:value={mentionLimit} min={1} max={50} />
								</div>
							</div>
							<button
								type="button"
								onclick={addMentionSpam}
								disabled={nativeBusy || !!findNative(5)}
								class="inline-flex items-center gap-1.5 rounded-md border border-line-strong px-2.5 py-2 text-xs font-medium text-ink transition-colors hover:bg-ink-2 disabled:opacity-40"
							>
								<Plus size={13} /> {findNative(5) ? 'Mention block added' : 'Block mention spam'}
							</button>
							<button
								type="button"
								onclick={addKeywordPreset}
								disabled={nativeBusy || !!findNative(4)}
								class="inline-flex items-center gap-1.5 rounded-md border border-line-strong px-2.5 py-2 text-xs font-medium text-ink transition-colors hover:bg-ink-2 disabled:opacity-40"
							>
								<Plus size={13} /> {findNative(4) ? 'Profanity preset added' : 'Block profanity & slurs (preset)'}
							</button>
						</div>
					</div>
				{/if}
			</ModSection>
		{:else if tab === 'settings'}
			<!-- Global settings -->
			<ModSection
				label="Global settings"
				desc="Who and where automod skips, plus where hits are logged."
			>
				<div class="max-w-2xl">
					<div class="mb-4">
						<ModToggleRow
							title="Ignore bots"
							desc="Never moderate messages from bots."
							bind:checked={cfg.ignore_bots}
							label="Ignore bots"
							divided
						/>
						<ModToggleRow
							title="Ignore moderators"
							desc="Skip members who can manage messages."
							bind:checked={cfg.ignore_mods}
							label="Ignore moderators"
							divided
						/>
					</div>
					<div class="grid gap-x-6 sm:grid-cols-2">
						<Field label="Exempt roles" hint="Members with any of these roles are never moderated.">
							<RolePicker
								multiple
								value={cfg.exempt_roles}
								onChange={(v) => (cfg.exempt_roles = v as string[])}
								placeholder="Add a role…"
							/>
						</Field>
						<Field label="Exempt channels" hint="Automod stays out of these channels entirely.">
							<ChannelPicker
								multiple
								value={cfg.exempt_channels}
								onChange={(v) => (cfg.exempt_channels = v as string[])}
								placeholder="Add a channel…"
							/>
						</Field>
					</div>
					<div class="max-w-sm">
						<Field label="Alert channel" hint="Optional. A log of every automod hit is posted here.">
							<ChannelSelect bind:value={cfg.alert_channel} />
						</Field>
					</div>
				</div>
			</ModSection>

			<!-- Automations integration -->
			<ModSection label="Automations" desc="Every automod hit emits an event you can react to.">
				<p class="max-w-2xl text-xs text-muted">
					Each time a rule trips, automod publishes an event your Automations can react to. Use it to
					post to a mod channel, notify a role, open a ticket, or anything else, all without touching
					code.
				</p>
				<div class="mt-4 flex flex-wrap gap-1.5">
					{#each eventFields as f (f.token)}
						<span
							title={f.desc}
							class="rounded-md border border-line bg-surface px-1.5 py-1 font-mono text-[11px] text-muted"
						>
							{f.token}
						</span>
					{/each}
				</div>
			</ModSection>

			<section class="border-b border-line">
				<ModLinkRow
					href={`/servers/${store.id}/automations`}
					icon={Zap}
					title="Build an automation"
					desc="React to every automod hit on the canvas."
				/>
			</section>
		{/if}
	</TabSwipe>

	<!-- Pickers and dialogs stay mounted across subtabs. -->
	<TriggerPicker bind:open={pickerOpen} onpick={pickTrigger} />

	<ConfirmDialog
		bind:open={() => pendingDelete !== null, (v) => { if (!v) pendingDelete = null; }}
		title="Delete this rule?"
		description="The rule and its actions will be removed. This cannot be undone once you save."
		confirmLabel="Delete rule"
		cancelLabel="Keep it"
		onconfirm={confirmDelete}
		oncancel={() => (pendingDelete = null)}
	/>
</ModerationShell>
