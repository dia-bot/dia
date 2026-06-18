<script lang="ts">
	import { onMount, getContext } from 'svelte';
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
	import SaveBar from '$lib/components/SaveBar.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import RuleCard from '$lib/components/automod/RuleCard.svelte';
	import RuleEditor from '$lib/components/automod/RuleEditor.svelte';
	import TriggerPicker from '$lib/components/automod/TriggerPicker.svelte';
	import { Plus, ShieldAlert, Zap, ArrowRight, Trash2 } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'automod';

	let enabled = $state(false);
	let cfg = $state<AutomodConfig>(defaultConfig());
	let loaded = $state(false);
	let saving = $state(false);
	let baseline = $state('');

	let editingId = $state<string | null>(null);
	let pickerOpen = $state(false);
	let pendingDelete = $state<string | null>(null);

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	onMount(async () => {
		const f = await api.feature(store.id, FEATURE);
		const d = defaultConfig();
		const c = (f.config ?? {}) as Partial<AutomodConfig>;
		cfg = {
			...d,
			...c,
			rules: c.rules ?? d.rules,
			escalation: { ...d.escalation, ...(c.escalation ?? {}) },
			exempt_roles: c.exempt_roles ?? d.exempt_roles,
			exempt_channels: c.exempt_channels ?? d.exempt_channels
		};
		enabled = f.enabled;
		loaded = true;
		baseline = JSON.stringify({ enabled, cfg });
	});

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
		{ value: 'ban', label: 'Ban' }
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
</script>

<svelte:head><title>Automod · {store.name} · Dia</title></svelte:head>

<header class="mb-6 flex items-start justify-between gap-4">
	<div class="flex items-start gap-3">
		<span class="mt-0.5 grid size-9 shrink-0 place-items-center rounded-xl border border-line bg-ink-2 text-accent-ink">
			<ShieldAlert size={18} />
		</span>
		<div>
			<h1 class="text-xl font-semibold tracking-tight text-ink">Automod</h1>
			<p class="mt-1 max-w-xl text-sm text-muted">
				Build rule-based filters for content, spam, mentions and members. Each rule pairs a trigger
				with the actions it should take, and feeds an escalation ladder.
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
		<div class="skeleton h-40 w-full rounded-card"></div>
		<div class="skeleton h-64 w-full rounded-card"></div>
	</div>
{:else}
	<div class="space-y-5">
		<!-- Global settings -->
		<section class="card p-5">
			<div class="eyebrow mb-4">Global settings</div>
			<div class="grid gap-5 md:grid-cols-2">
				<div class="flex items-center justify-between gap-4">
					<div>
						<div class="text-sm font-medium text-ink">Ignore bots</div>
						<p class="text-xs text-muted">Never moderate messages from bots.</p>
					</div>
					<Toggle bind:checked={cfg.ignore_bots} label="Ignore bots" />
				</div>
				<div class="flex items-center justify-between gap-4">
					<div>
						<div class="text-sm font-medium text-ink">Ignore moderators</div>
						<p class="text-xs text-muted">Skip members who can manage messages.</p>
					</div>
					<Toggle bind:checked={cfg.ignore_mods} label="Ignore moderators" />
				</div>
				<div>
					<span class="label">Exempt roles</span>
					<RolePicker
						multiple
						value={cfg.exempt_roles}
						onChange={(v) => (cfg.exempt_roles = v as string[])}
						placeholder="Add a role…"
					/>
					<p class="hint">Members with any of these roles are never moderated.</p>
				</div>
				<div>
					<span class="label">Exempt channels</span>
					<ChannelPicker
						multiple
						value={cfg.exempt_channels}
						onChange={(v) => (cfg.exempt_channels = v as string[])}
						placeholder="Add a channel…"
					/>
					<p class="hint">Automod stays out of these channels entirely.</p>
				</div>
				<div class="md:col-span-2 md:max-w-sm">
					<span class="label">Alert channel</span>
					<ChannelSelect bind:value={cfg.alert_channel} />
					<p class="hint">Optional. A log of every automod hit is posted here.</p>
				</div>
			</div>
		</section>

		<!-- Rules -->
		<section class="card p-5">
			<div class="mb-4 flex items-center justify-between gap-4">
				<div>
					<div class="eyebrow">Rules</div>
					<p class="mt-0.5 text-xs text-muted">
						{cfg.rules.length === 0
							? 'No rules yet.'
							: `${cfg.rules.length} rule${cfg.rules.length === 1 ? '' : 's'}, checked top to bottom.`}
					</p>
				</div>
				<button
					type="button"
					onclick={() => (pickerOpen = true)}
					class="btn btn-accent h-9 px-3 text-sm"
				>
					<Plus size={15} /> Add rule
				</button>
			</div>

			{#if cfg.rules.length === 0}
				<div class="rounded-2xl border border-dashed border-line px-6 py-12 text-center">
					<span class="mx-auto mb-3 grid size-11 place-items-center rounded-xl border border-line bg-ink-2 text-accent-ink">
						<ShieldAlert size={20} />
					</span>
					<h3 class="text-sm font-semibold text-ink">No rules yet</h3>
					<p class="mx-auto mt-1 max-w-sm text-xs text-muted">
						Add your first rule to start filtering. Pick a trigger (blocked words, spam, invites,
						mention floods and more), then choose what happens when it trips.
					</p>
					<button
						type="button"
						onclick={() => (pickerOpen = true)}
						class="btn btn-accent mx-auto mt-4 h-9 px-3 text-sm"
					>
						<Plus size={15} /> Add a rule
					</button>
				</div>
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
		</section>

		<!-- Escalation -->
		<section class="card p-5">
			<div class="mb-4 flex items-center justify-between gap-4">
				<div>
					<div class="eyebrow">Escalation ladder</div>
					<p class="mt-0.5 max-w-lg text-xs text-muted">
						Rules that add infraction points feed this ladder. As a member's active points cross a
						tier, the matching punishment fires automatically.
					</p>
				</div>
				<Toggle bind:checked={cfg.escalation.enabled} label="Escalation enabled" />
			</div>

			{#if cfg.escalation.enabled}
				<div class="mb-5 max-w-xs">
					<span class="label">Points decay</span>
					<div class="flex items-center gap-2">
						<div class="w-32">
							<NumberField bind:value={cfg.escalation.decay_hours} min={1} max={8760} />
						</div>
						<span class="text-xs text-muted">hours until a point is forgiven</span>
					</div>
				</div>

				<div class="space-y-2">
					{#each cfg.escalation.tiers as tier, i (i)}
						<div class="flex flex-wrap items-end gap-3 rounded-xl border border-line bg-ink-2/30 p-3">
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
							{/if}
							<button
								type="button"
								onclick={() => removeTier(i)}
								class="ml-auto grid size-9 place-items-center rounded-lg text-faint transition-colors hover:bg-blush hover:text-accent-ink"
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
					class="mt-3 inline-flex items-center gap-1.5 rounded-lg border border-line-strong px-2.5 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-ink-2"
				>
					<Plus size={13} /> Add tier
				</button>
			{:else}
				<p class="text-xs text-faint">
					Escalation is off. Rules still run their own actions, but repeat offenders are not punished
					on a points ladder.
				</p>
			{/if}
		</section>

		<!-- Automations integration callout -->
		<section class="rounded-card border border-line bg-ink-2 p-5">
			<div class="flex items-start gap-3.5">
				<span class="grid size-9 shrink-0 place-items-center rounded-xl border border-line-strong bg-surface text-accent-ink">
					<Zap size={18} />
				</span>
				<div class="min-w-0 flex-1">
					<div class="eyebrow">Automations</div>
					<h3 class="mt-1 text-sm font-semibold text-ink">Every automod hit emits an event</h3>
					<p class="mt-1 max-w-2xl text-xs leading-relaxed text-muted">
						Each time a rule trips, automod publishes an event your Automations can react to. Use it
						to post to a mod channel, notify a role, open a ticket, or anything else, all without
						touching code. These fields are in scope:
					</p>
					<div class="mt-3 flex flex-wrap gap-1.5">
						{#each eventFields as f (f.token)}
							<span
								title={f.desc}
								class="rounded-md border border-line bg-surface px-1.5 py-1 font-mono text-[11px] text-muted"
							>
								{f.token}
							</span>
						{/each}
					</div>
					<a
						href="/servers/{store.id}/automations"
						class="mt-4 inline-flex items-center gap-1.5 rounded-lg border border-line-strong bg-surface px-3 py-2 text-xs font-medium text-ink transition-colors hover:border-faint"
					>
						Build an automation <ArrowRight size={14} class="text-accent-ink" />
					</a>
				</div>
			</div>
		</section>
	</div>

	<SaveBar {dirty} {saving} onsave={save} onreset={reset} />

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
{/if}
