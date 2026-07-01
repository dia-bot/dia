<script lang="ts">
	import { onMount, getContext, setContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import { defaultVerification, type VerificationConfig, type VerifyMode } from '$lib/verification/types';
	import type { Step } from '$lib/commands/types';
	import { AUTOMATION_CTX, EXPR_SCOPE_CTX } from '$lib/commands/expr-meta';
	import type { ExprScope } from '$lib/commands/expr-meta';
	import Field from '$lib/components/Field.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import NumberField from '$lib/components/ui/NumberField.svelte';
	import ModerationShell, { type ModTab } from '$lib/components/moderation/ModerationShell.svelte';
	import ModSection from '$lib/components/moderation/ModSection.svelte';
	import ModToggleRow from '$lib/components/moderation/ModToggleRow.svelte';
	import ModLinkRow from '$lib/components/moderation/ModLinkRow.svelte';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import AutomationPicker from '$lib/components/commands/AutomationPicker.svelte';
	import {
		MousePointerClick,
		ShieldQuestion,
		Lock,
		Clock,
		ScrollText,
		UserCheck,
		BookOpen,
		Hash,
		ShieldCheck,
		Zap
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'verification';

	// MessageEditor reads two contexts. We provide both so it renders in its
	// non-automation form (raw custom-id editing hidden, no canvas) and its
	// variable picker offers the gate scope. The scope is a hint catalogue only.
	setContext(AUTOMATION_CTX, false);
	const exprScope: ExprScope = {
		options: [],
		variables: [],
		steps: [],
		extraVars: [
			{ path: '.User.Mention', label: 'User.Mention', type: 'string', short: 'Mentions the joining member' },
			{ path: '.User.GlobalName', label: 'User.GlobalName', type: 'string', short: "Member's display name" },
			{ path: '.Guild.Name', label: 'Guild.Name', type: 'string', short: 'Server name' },
			{ path: '.Guild.MemberCount', label: 'Guild.MemberCount', type: 'int', short: 'Live member count' }
		]
	};
	setContext(EXPR_SCOPE_CTX, exprScope);

	let enabled = $state(false);
	let cfg = $state<VerificationConfig>(defaultVerification());
	let loaded = $state(false);
	let loadError = $state('');
	let saving = $state(false);
	let baseline = $state('');

	let tab = $state('setup');

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	// Subtabs are this page's own sections (not the sidebar's modules).
	const tabs = $derived<ModTab[]>([
		{ key: 'setup', label: 'Setup', icon: ShieldQuestion },
		{ key: 'guide', label: 'How it works', icon: BookOpen }
	]);

	// ── The gate message as an editable `send_message` Step ───────────────────
	// MessageEditor edits a Step in place (step.spec.{content,embeds,components}).
	// We seed it from cfg on load and write the three fields back into cfg on
	// every change, so save() persists the exact JSON the Go side decodes 1:1.
	let msgStep = $state<Step>({ id: 'verify-msg', kind: 'send_message', spec: { content: '' } });

	function buildStep(c: VerificationConfig): Step {
		// content falls back to the legacy welcome_text only as a seed for the
		// editor; once edited it's saved under `content`.
		const spec: Record<string, unknown> = { content: c.content || c.welcome_text || '' };
		if (Array.isArray(c.embeds) && c.embeds.length) spec.embeds = c.embeds;
		if (Array.isArray(c.components) && c.components.length) spec.components = c.components;
		return { id: 'verify-msg', kind: 'send_message', spec };
	}

	// Pull the three editable fields off the step back onto cfg. Runs on every
	// edit (the editor mutates step.spec reactively), keeping cfg the source of
	// truth for dirty-tracking and save.
	function syncFromStep() {
		const spec = (msgStep.spec ?? {}) as Record<string, unknown>;
		cfg.content = (spec.content as string) ?? '';
		cfg.embeds = (spec.embeds as unknown[]) ?? [];
		cfg.components = (spec.components as unknown[]) ?? [];
		reconcileButtonActions();
	}
	// Mirror the step's spec into cfg whenever the editor touches it.
	$effect(() => {
		// Touch the reactive spec so this effect re-runs on any in-place edit.
		void JSON.stringify(msgStep.spec);
		if (loaded) syncFromStep();
	});

	// ── Custom buttons -> automation mapping ──────────────────────────────────
	// Each non-link button in components carries a custom_id_suffix; we map it to
	// an automation id in cfg.button_actions[suffix]. The "Verify" button is never
	// in components (the backend injects it), so nothing here can shadow it.
	type Btn = { suffix: string; label: string };
	const customButtons = $derived.by<Btn[]>(() => {
		const rows = (cfg.components ?? []) as { components?: Record<string, unknown>[] }[];
		const out: Btn[] = [];
		for (const row of rows) {
			for (const c of row?.components ?? []) {
				// Only actionable (non-link) buttons can run an automation; link
				// buttons just open a URL and Discord handles them.
				if (c?.type === 'button' && c?.style !== 'link' && c?.url == null) {
					const suffix = (c.custom_id_suffix as string) ?? '';
					if (suffix) out.push({ suffix, label: (c.label as string) || 'Button' });
				}
			}
		}
		return out;
	});

	function actionFor(suffix: string): string {
		return (cfg.button_actions ?? []).find((a) => a.suffix === suffix)?.automation_id ?? '';
	}
	function setAction(suffix: string, id: string) {
		const list = (cfg.button_actions ?? []).filter((a) => a.suffix !== suffix);
		if (id) list.push({ suffix, automation_id: id });
		cfg.button_actions = list;
	}
	// Drop mappings whose button no longer exists (renamed / removed), so saved
	// button_actions never accumulates orphans.
	function reconcileButtonActions() {
		const live = new Set(customButtons.map((b) => b.suffix));
		const next = (cfg.button_actions ?? []).filter((a) => live.has(a.suffix));
		if (next.length !== (cfg.button_actions ?? []).length) cfg.button_actions = next;
	}

	// Load is retryable: the feature call surfaces real failures (the shell shows a
	// retry panel) instead of hanging on a blank skeleton.
	async function load() {
		loadError = '';
		loaded = false;
		try {
			const f = await api.feature(store.id, FEATURE);
			const d = defaultVerification();
			const c = (f.config ?? {}) as Partial<VerificationConfig>;
			cfg = { ...d, ...c };
			enabled = f.enabled;
			msgStep = buildStep(cfg);
			baseline = JSON.stringify({ enabled, cfg });
			loaded = true;
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load this page.';
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
		msgStep = buildStep(cfg);
	}

	function setMode(v: string) {
		cfg.mode = v as VerifyMode;
	}
</script>

<svelte:head><title>Verification · {store.name} · Dia</title></svelte:head>

<ModerationShell
	icon={UserCheck}
	title="Verification"
	blurb="Gate new members behind a human check."
	bind:enabled
	ready={loaded}
	error={loadError}
	onretry={load}
	toggleLabel="Verification"
	{tabs}
	bind:active={tab}
	{dirty}
	{saving}
	onsave={save}
	onreset={reset}
>
	{#if tab === 'setup'}
		<!-- ── Two-column composer: message editor (left) + settings rail (right) ── -->
		<div class="flex flex-col gap-0 lg:flex-row lg:items-stretch">
			<!-- LEFT — the gate message composer -->
			<div class="min-w-0 flex-1 border-b border-line lg:border-b-0 lg:border-r">
				<div class="px-4 py-5 sm:px-5">
					<!-- Channel header: where the prompt is posted -->
					<div class="mb-2 flex flex-wrap items-center gap-2 text-[12.5px] text-muted">
						<Hash size={14} class="text-faint" />
						<span>Posts in</span>
						<div class="min-w-[200px] max-w-xs flex-1">
							<ChannelSelect bind:value={cfg.channel} placeholder="Channel to post the gate in" />
						</div>
					</div>
					<p class="mb-3 flex items-center gap-1.5 text-[11.5px] text-faint">
						<ShieldCheck size={12} class="text-faint" />
						A <span class="font-medium text-muted">Verify</span> button is added automatically as the
						first button — you don't add it here.
					</p>

					<!-- The rich gate message: content, embeds, custom buttons / selects -->
					<MessageEditor step={msgStep} embeds components clickPaths={false} />

					<!-- Button actions: map each custom button to an automation -->
					{#if customButtons.length > 0}
						<div class="mt-6 border-t border-line/60 pt-5">
							<div class="mb-1 flex items-center gap-2">
								<Zap size={13} class="text-accent-ink" />
								<span class="text-[12.5px] font-medium text-ink">Button actions</span>
							</div>
							<p class="mb-3 text-[11.5px] text-muted">
								Run an automation when one of your custom buttons is clicked. Link buttons just open
								their URL.
							</p>
							<div class="space-y-3">
								{#each customButtons as b (b.suffix)}
									<div class="rounded-lg border border-line bg-bg p-3">
										<div class="mb-1.5 flex items-center gap-2">
											<MousePointerClick size={12} class="text-faint" />
											<span class="truncate text-[12.5px] font-medium text-ink">{b.label}</span>
											<span class="font-mono text-[10px] text-faint">{b.suffix}</span>
										</div>
										<AutomationPicker
											value={actionFor(b.suffix)}
											onChange={(id) => setAction(b.suffix, id)}
										/>
										<p class="mt-1.5 text-[10.5px] text-muted">Run this automation when clicked.</p>
									</div>
								{/each}
							</div>
						</div>
					{/if}
				</div>
			</div>

			<!-- RIGHT — the settings rail -->
			<aside class="w-full shrink-0 lg:w-[22rem]">
				<!-- Challenge mode -->
				<ModSection label="Challenge" desc="How members prove they're human.">
					<div class="grid gap-2.5">
						<button
							type="button"
							onclick={() => setMode('button')}
							class="flex items-start gap-2.5 rounded-xl border px-3.5 py-3 text-left text-xs transition-colors {cfg.mode ===
							'button'
								? 'border-line-strong bg-ink-2'
								: 'border-line hover:border-line-strong'}"
						>
							<MousePointerClick size={16} class="mt-0.5 shrink-0 text-muted" />
							<div>
								<div class="text-sm font-medium text-ink">Button click</div>
								<p class="mt-0.5 text-muted">
									Lowest friction. A single button verifies instantly.
								</p>
							</div>
						</button>
						<button
							type="button"
							onclick={() => setMode('captcha')}
							class="flex items-start gap-2.5 rounded-xl border px-3.5 py-3 text-left text-xs transition-colors {cfg.mode ===
							'captcha'
								? 'border-line-strong bg-ink-2'
								: 'border-line hover:border-line-strong'}"
						>
							<ShieldQuestion size={16} class="mt-0.5 shrink-0 text-muted" />
							<div>
								<div class="text-sm font-medium text-ink">Captcha challenge</div>
								<p class="mt-0.5 text-muted">
									Adds a short human-only puzzle. Stronger against bots.
								</p>
							</div>
						</button>
					</div>
				</ModSection>

				<!-- Roles -->
				<ModSection label="Roles" desc="The lock-out role and the role granted on success.">
					<Field label="Unverified role">
						<RolePicker
							value={cfg.unverified_role}
							onChange={(v) => (cfg.unverified_role = v as string)}
							placeholder="Select a role…"
						/>
					</Field>
					<Field
						label="Verified role (optional)"
						hint="Granted on success. Handy if you open channels to a verified role rather than @everyone."
					>
						<RolePicker
							value={cfg.verified_role}
							onChange={(v) => (cfg.verified_role = v as string)}
							placeholder="Select a role…"
						/>
					</Field>
					<p class="-mt-2 text-[11px] text-muted">
						<Lock size={11} class="-mt-0.5 mr-0.5 inline text-faint" />
						The unverified role is assigned the moment a member joins. Deny it View Channel at the
						category level, otherwise the gate does nothing.
					</p>
				</ModSection>

				<!-- On verify -->
				<ModSection label="On verify" desc="Optionally hand off to an automation when a member passes.">
					<Field
						label="Run automation"
						hint='Launches the flow with the new member as .User. Any automation can also trigger on "Member verified" directly.'
					>
						<AutomationPicker
							value={cfg.run_automation ?? ''}
							onChange={(id) => (cfg.run_automation = id)}
						/>
					</Field>
				</ModSection>

				<!-- Targeting & cleanup -->
				<ModSection
					label="Targeting & cleanup"
					desc="Who gets gated, and what happens to members who never verify."
				>
					<ModToggleRow
						title="Only challenge suspicious accounts"
						desc="Skip the gate for established accounts and only verify brand-new ones."
						bind:checked={cfg.only_suspicious}
						label="Only suspicious"
					/>
					{#if cfg.only_suspicious}
						<div class="border-t border-line py-4">
							<span class="label">Minimum account age</span>
							<div class="flex items-center gap-2">
								<div class="w-24">
									<NumberField bind:value={cfg.min_account_age_hours} min={1} max={8760} />
								</div>
								<span class="text-xs text-muted">hours before an account is trusted</span>
							</div>
						</div>
						<ModToggleRow
							title="Require a profile picture"
							desc="Treat joiners with no avatar as suspicious, so they're gated too."
							bind:checked={cfg.require_avatar}
							label="Require avatar"
							divided
						/>
					{/if}
					<div class="border-t border-line pt-4">
						<span class="label">Kick if not verified within</span>
						<div class="flex items-center gap-2">
							<div class="w-24">
								<NumberField bind:value={cfg.kick_after_minutes} min={0} max={1440} />
							</div>
							<span class="text-xs text-muted">
								<Clock size={11} class="-mt-0.5 mr-0.5 inline text-faint" />
								minutes ({cfg.kick_after_minutes === 0
									? 'never kick'
									: `${cfg.kick_after_minutes}m`})
							</span>
						</div>
						<p class="hint">Set to 0 to leave unverified members in place indefinitely.</p>
					</div>
				</ModSection>
			</aside>
		</div>
	{:else if tab === 'guide'}
		<!-- ── How it works ── -->
		<ModSection label="How it works">
			<ol class="max-w-2xl space-y-4 text-xs text-muted">
				<li class="flex items-start gap-3">
					<span
						class="grid size-6 shrink-0 place-items-center rounded-full border border-line-strong bg-ink-2 font-mono text-[11px] font-semibold text-muted"
						>1</span
					>
					<span
						>A member joins and is given the <strong class="text-ink">unverified role</strong>,
						which locks them out of your channels.</span
					>
				</li>
				<li class="flex items-start gap-3">
					<span
						class="grid size-6 shrink-0 place-items-center rounded-full border border-line-strong bg-ink-2 font-mono text-[11px] font-semibold text-muted"
						>2</span
					>
					<span
						>They complete the check posted in the <strong class="text-ink">gate channel</strong>
						(a button tap or captcha). Any custom buttons can run an automation.</span
					>
				</li>
				<li class="flex items-start gap-3">
					<span
						class="grid size-6 shrink-0 place-items-center rounded-full border border-line-strong bg-ink-2 font-mono text-[11px] font-semibold text-muted"
						>3</span
					>
					<span
						>The unverified role is removed (and an optional
						<strong class="text-ink">verified role</strong> added), unlocking the server.</span
					>
				</li>
			</ol>
		</ModSection>

		<!-- ── Logging cross-link ── -->
		<section class="border-b border-line">
			<ModLinkRow
				href="/servers/{store.id}/logging"
				icon={ScrollText}
				title="Log joins and verifications"
				desc="Send every join and check to a channel."
			/>
		</section>
	{/if}
</ModerationShell>
