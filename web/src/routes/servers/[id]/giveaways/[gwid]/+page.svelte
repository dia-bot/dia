<script lang="ts">
	import { onMount, getContext, setContext } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import {
		FEATURE,
		defaultConfig,
		defaultSpec,
		newPresetID,
		parseDurationSeconds,
		GIVEAWAY_VARS,
		GIVEAWAY_SAMPLE,
		GIVEAWAY_SCOPE_VARS,
		type GiveawayConfig,
		type GiveawaySpec,
		type ComponentRow,
		type Component,
		type Preset,
		type RequirementConfig
	} from '$lib/giveaway';
	import type { Step } from '$lib/commands/types';
	import { AUTOMATION_CTX, EXPR_SCOPE_CTX, type ExprScope } from '$lib/commands/expr-meta';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import ModSection from '$lib/components/moderation/ModSection.svelte';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import Select from '$lib/components/Select.svelte';
	import NumberField from '$lib/components/ui/NumberField.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import ColorField from '$lib/components/ColorField.svelte';
	import TemplateField from '$lib/components/TemplateField.svelte';
	import ReleaseDock from '$lib/components/page/ReleaseDock.svelte';
	import {
		ChevronLeft,
		Gift,
		Save,
		Rocket,
		Trash2,
		Ban,
		CheckCircle2,
		Dices,
		Plus,
		Bookmark,
		Eye
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);

	// The MessageEditor variable picker reads these two contexts. Provide the
	// giveaway scope (so {{ .Prize }}, {{ .Ends }}, … are offered) and render in
	// its non-automation form.
	setContext(AUTOMATION_CTX, false);
	const exprScope: ExprScope = {
		options: [],
		variables: [],
		steps: [],
		extraVars: GIVEAWAY_SCOPE_VARS
	};
	setContext(EXPR_SCOPE_CTX, exprScope);

	const gwid = $derived($page.params.gwid ?? '');
	const presetParam = $derived($page.url.searchParams.get('preset') ?? '');
	// A `preset-<id>` (or `preset-new`) route edits a saved preset in its own
	// clearly-framed mode; anything else edits/creates a real giveaway.
	const isPreset = $derived(gwid.startsWith('preset-'));
	const presetId = $derived(isPreset ? gwid.slice('preset-'.length) : '');
	const isNewPreset = $derived(isPreset && presetId === 'new');
	const isNew = $derived(gwid === 'new');

	let loaded = $state(false);
	let loadError = $state('');
	let busy = $state('');
	let cfg = $state<GiveawayConfig>(defaultConfig());
	let featureEnabled = $state(false);
	let status = $state<'new' | 'draft' | 'scheduled' | 'running' | 'ended' | 'cancelled'>('new');
	let messageId = $state('');

	// ── Composition state ─────────────────────────────────────────────────────
	let name = $state('');
	let prize = $state('');
	let description = $state('');
	let channelId = $state('');
	let winnerCount = $state(1);
	let color = $state('');
	let imageUrl = $state('');
	let durationStr = $state('24h');
	let startInStr = $state('');
	let pingRoleId = $state('');
	let showRequirements = $state(true);
	let excludeHost = $state(false);
	let allowBotsToWin = $state(false);
	let btnLabel = $state('Enter Giveaway');
	let btnEmoji = $state('🎉');
	let btnStyle = $state('primary');
	// custom_id_suffix of the composed button that enters the giveaway ('' = use
	// the auto-added system Enter button styled by the fields above).
	let enterButtonSuffix = $state('');
	// suffix → saved automation id, for action buttons that fire an automation.
	let buttonActions = $state<Record<string, string>>({});
	// The guild's saved automations, for the action-button picker.
	let automationList = $state<{ id: string; name: string }[]>([]);
	let announce = $state(defaultSpec().announce);
	let req = $state<RequirementConfig>({});
	let sourcePresetId = $state('');
	// Bound to the preset picker; an effect applies the chosen preset.
	let presetPick = $state('');
	$effect(() => {
		const pid = presetPick;
		if (pid && pid !== sourcePresetId) {
			const p = cfg.presets.find((x) => x.id === pid);
			if (p) applyPreset(p);
		}
	});

	// The message (content + embeds) edited via the shared WYSIWYG MessageEditor.
	let msgStep = $state<Step>({
		id: 'gw-msg',
		kind: 'send_message',
		spec: { content: '', embeds: [], components: [] }
	});

	const readOnly = $derived(status === 'ended' || status === 'cancelled');
	const channelLocked = $derived(readOnly || status === 'running' || status === 'scheduled');

	function clone<T>(v: T): T {
		return JSON.parse(JSON.stringify(v)) as T;
	}

	function applySpec(spec: GiveawaySpec) {
		msgStep = {
			id: 'gw-msg',
			kind: 'send_message',
			spec: {
				content: spec.content ?? '',
				embeds: clone(spec.embeds ?? []),
				components: clone(spec.components ?? [])
			}
		};
		enterButtonSuffix = spec.enter_button_suffix ?? '';
		buttonActions = clone(spec.button_actions ?? {});
		btnLabel = spec.button?.label ?? 'Enter Giveaway';
		btnEmoji = spec.button?.emoji ?? '';
		btnStyle = spec.button?.style ?? 'primary';
		announce = clone(spec.announce ?? defaultSpec().announce);
		pingRoleId = spec.ping_role_id ?? '';
		showRequirements = spec.show_requirements ?? true;
		excludeHost = spec.exclude_host ?? false;
		allowBotsToWin = spec.allow_bots_to_win ?? false;
	}

	function applyPreset(p: Preset) {
		applySpec(p.spec);
		req = clone(p.requirements ?? {});
		if (p.default_channel_id) channelId = p.default_channel_id;
		if (p.default_duration) durationStr = p.default_duration;
		if (p.default_winner_count) winnerCount = p.default_winner_count;
		sourcePresetId = p.id;
		presetPick = p.id;
	}

	async function load() {
		loadError = '';
		loaded = false;
		try {
			const f = await api.feature(store.id, FEATURE);
			featureEnabled = !!f.enabled;
			cfg = { ...defaultConfig(), ...(f.config ?? {}) } as GiveawayConfig;
			if (!cfg.presets?.length) cfg.presets = defaultConfig().presets;

			if (isPreset) {
				// Editing a saved preset: hydrate from the config, not the giveaway API.
				status = 'new';
				const p = isNewPreset
					? (cfg.presets.find((x) => x.id === (cfg.default_preset_id || cfg.presets[0]?.id)) ??
						cfg.presets[0])
					: cfg.presets.find((x) => x.id === presetId);
				if (p) {
					applySpec(p.spec);
					req = clone(p.requirements ?? {});
					channelId = p.default_channel_id ?? '';
					durationStr = p.default_duration ?? '24h';
					winnerCount = p.default_winner_count ?? 1;
					name = isNewPreset ? '' : p.name;
				}
				baseline = snapshot();
				loaded = true;
				return;
			}
			if (isNew) {
				status = 'new';
				const pid = presetParam || cfg.default_preset_id || cfg.presets[0]?.id || '';
				const p = cfg.presets.find((x) => x.id === pid) ?? cfg.presets[0];
				if (p) applyPreset(p);
			} else {
				const g = await api.giveaway(store.id, gwid);
				status = g.status;
				messageId = g.message_id ?? '';
				name = g.name ?? '';
				prize = g.prize ?? '';
				description = g.description ?? '';
				channelId = g.channel_id ?? '';
				winnerCount = g.winner_count ?? 1;
				color = g.color ?? '';
				imageUrl = g.image_url ?? '';
				req = clone((g.requirements ?? {}) as RequirementConfig);
				applySpec((g.spec ?? defaultSpec()) as GiveawaySpec);
			}
			baseline = snapshot();
			loaded = true;
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load the giveaway.';
		}
	}
	onMount(load);
	// The saved automations an action button can point at (best-effort; the picker
	// just shows "No action" if this fails or there are none yet).
	onMount(async () => {
		try {
			const r = await api.automations(store.id);
			automationList = ((r.automations ?? []) as { id: string; name: string }[]).map((a) => ({
				id: a.id,
				name: a.name
			}));
		} catch {
			/* ignore */
		}
	});

	// Assemble the composed spec + create/update body from the current state.
	function buildSpec(): GiveawaySpec {
		const s = (msgStep.spec ?? {}) as Record<string, unknown>;
		return {
			content: (s.content as string) ?? '',
			embeds: ((s.embeds as GiveawaySpec['embeds']) ?? []) as GiveawaySpec['embeds'],
			components: ((s.components as GiveawaySpec['components']) ?? []) as GiveawaySpec['components'],
			enter_button_suffix: enterButtonSuffix,
			button_actions: buttonActions,
			button: { label: btnLabel, emoji: btnEmoji, style: btnStyle },
			announce,
			ping_role_id: pingRoleId,
			show_requirements: showRequirements,
			exclude_host: excludeHost,
			allow_bots_to_win: allowBotsToWin
		};
	}

	function meta() {
		return {
			name,
			prize,
			description,
			channel_id: channelId,
			winner_count: winnerCount,
			color,
			image_url: imageUrl,
			spec: buildSpec(),
			requirements: req
		};
	}

	function backToList() {
		goto(`/servers/${store.id}/giveaways`);
	}

	async function run(key: string, fn: () => Promise<unknown>, back = true) {
		if (busy) return;
		busy = key;
		try {
			await fn();
			if (back) backToList();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Action failed.';
		} finally {
			busy = '';
		}
	}

	async function saveDraft() {
		if (isNew) {
			await run('draft', () => api.createGiveaway(store.id, { ...meta(), status: 'draft' }));
		} else {
			await run('save', () => api.updateGiveaway(store.id, gwid, meta()), false);
		}
	}

	async function publish() {
		const durSecs = parseDurationSeconds(durationStr) || 86400;
		const startSecs = parseDurationSeconds(startInStr);
		if (isNew) {
			const now = Date.now();
			const startsAt = new Date(now + startSecs * 1000).toISOString();
			const endsAt = new Date(now + (startSecs + durSecs) * 1000).toISOString();
			await run('publish', () =>
				api.createGiveaway(store.id, {
					...meta(),
					status: startSecs > 0 ? 'scheduled' : 'running',
					starts_at: startsAt,
					ends_at: endsAt
				})
			);
		} else {
			// An existing draft: persist any edits, then start it.
			await run('publish', async () => {
				await api.updateGiveaway(store.id, gwid, meta());
				await api.startGiveaway(store.id, gwid, durSecs, startSecs);
			});
		}
	}

	async function saveAsPreset() {
		const overwrite = sourcePresetId && cfg.presets.some((p) => p.id === sourcePresetId);
		const preset: Preset = {
			id: overwrite ? sourcePresetId : newPresetID(),
			name: name.trim() || prize.trim() || 'New preset',
			default_channel_id: channelId,
			default_duration: durationStr,
			default_winner_count: winnerCount,
			spec: buildSpec(),
			requirements: req
		};
		const presets = overwrite
			? cfg.presets.map((p) => (p.id === preset.id ? preset : p))
			: [...cfg.presets, preset];
		const next = { ...cfg, presets };
		await run(
			'preset',
			async () => {
				await api.saveFeature(store.id, FEATURE, featureEnabled, next);
				cfg = next;
				sourcePresetId = preset.id;
			},
			false
		);
	}

	// ── Preset-editor actions (isPreset mode) ──────────────────────────────────
	function buildPreset(id: string): Preset {
		return {
			id,
			name: name.trim() || 'Preset',
			default_channel_id: channelId,
			default_duration: durationStr,
			default_winner_count: winnerCount,
			spec: buildSpec(),
			requirements: req
		};
	}
	async function savePreset() {
		const id = isNewPreset ? newPresetID() : presetId;
		const preset = buildPreset(id);
		const exists = cfg.presets.some((p) => p.id === id);
		const presets = exists
			? cfg.presets.map((p) => (p.id === id ? preset : p))
			: [...cfg.presets, preset];
		const next = { ...cfg, presets, default_preset_id: cfg.default_preset_id || id };
		await run('preset', () => api.saveFeature(store.id, FEATURE, featureEnabled, next));
	}
	async function deletePresetHere() {
		const presets = cfg.presets.filter((p) => p.id !== presetId);
		const next = {
			...cfg,
			presets,
			default_preset_id:
				cfg.default_preset_id === presetId ? (presets[0]?.id ?? '') : cfg.default_preset_id
		};
		await run('deletePreset', () => api.saveFeature(store.id, FEATURE, featureEnabled, next));
	}
	async function setDefaultHere() {
		const next = { ...cfg, default_preset_id: presetId };
		await run(
			'default',
			async () => {
				await api.saveFeature(store.id, FEATURE, featureEnabled, next);
				cfg = next;
			},
			false
		);
	}

	// ── Bonus-entry list editor ────────────────────────────────────────────────
	function addBonus() {
		req.bonus_entries = [...(req.bonus_entries ?? []), { role_id: '', entries: 2 }];
	}
	function removeBonus(i: number) {
		req.bonus_entries = (req.bonus_entries ?? []).filter((_, j) => j !== i);
	}

	const presetOptions = $derived(cfg.presets.map((p) => ({ value: p.id, label: p.name })));
	const isPresetUpdate = $derived(!!sourcePresetId && cfg.presets.some((p) => p.id === sourcePresetId));

	// The composed non-link buttons (each editable in the message preview). A
	// button's action (Enter / Run automation / Nothing) is set inline there.
	const messageButtons = $derived(
		(((msgStep.spec as Record<string, unknown>)?.components as ComponentRow[]) ?? [])
			.flatMap((r) => r.components ?? [])
			.filter((c) => (c.type ?? 'button') === 'button' && c.style !== 'link' && !!c.custom_id_suffix)
	);
	// Whether some visible button is wired to enter — else members can't enter.
	const hasEnterButton = $derived(messageButtons.some((c) => c.custom_id_suffix === enterButtonSuffix));
	const automationOptions = $derived([
		{ value: '', label: 'Choose an automation…' },
		...automationList.map((a) => ({ value: a.id, label: a.name }))
	]);
	// A button's current action mode, and setters, driven by the inline picker.
	function buttonMode(suffix: string): 'enter' | 'auto' | 'none' {
		if (enterButtonSuffix === suffix) return 'enter';
		if (suffix in buttonActions) return 'auto';
		return 'none';
	}
	function setButtonMode(suffix: string, mode: 'enter' | 'auto' | 'none') {
		if (mode === 'enter') {
			enterButtonSuffix = suffix;
			delete buttonActions[suffix];
			buttonActions = { ...buttonActions };
		} else {
			if (enterButtonSuffix === suffix) enterButtonSuffix = '';
			if (mode === 'auto') {
				if (!(suffix in buttonActions)) buttonActions[suffix] = '';
			} else {
				delete buttonActions[suffix];
				buttonActions = { ...buttonActions };
			}
		}
	}
	const canPublish = $derived(!!prize.trim() && !!channelId);
	const showTiming = $derived(isNew || status === 'draft');

	// ── Winner-announcement live preview ────────────────────────────────────────
	// The announcement/ended-embed fields are just template inputs, so a compact
	// Discord-styled preview shows how the drawn message actually looks. Values are
	// filled from the giveaway sample scope (with the live prize folded in); logic
	// isn't executed (the per-field "Test render" runs the real engine).
	const annSample = $derived({
		...GIVEAWAY_SAMPLE,
		Prize: prize.trim() || GIVEAWAY_SAMPLE.Prize,
		WinnerCount: winnerCount || 1,
		Server: store.name
	});
	function fillAnn(s: string | undefined): string {
		if (!s) return '';
		return s
			.replace(/\{\{\s*\.(\w+)\s*\}\}/g, (_m, k: string) => {
				const v = (annSample as Record<string, unknown>)[k];
				return v === undefined ? '' : String(v);
			})
			.replace(/\{\{[^}]*\}\}/g, '')
			.trim();
	}
	function hexColor(v: string): string {
		const h = (v || '').trim();
		return /^#?[0-9a-fA-F]{6}$/.test(h) ? (h.startsWith('#') ? h : `#${h}`) : '';
	}
	const annAccent = $derived(
		hexColor(color) ||
			hexColor(
				(((msgStep.spec as Record<string, unknown>)?.embeds as { color?: string }[]) ?? [])[0]?.color ?? ''
			) ||
			'#FF6363'
	);

	const inputCls =
		'h-8 w-full rounded-md border border-line bg-bg px-2.5 text-[13px] text-ink placeholder:text-faint focus-visible:border-line-strong focus-visible:outline-none disabled:opacity-60';
	const btnGhost =
		'inline-flex h-8 items-center gap-1.5 rounded-md border border-line px-3 text-[12px] font-medium text-ink hover:border-line-strong disabled:opacity-50';
	const btnStrong =
		'inline-flex h-8 items-center gap-1.5 rounded-md border border-line-strong px-3 text-[12px] font-medium text-ink hover:bg-ink-2 disabled:opacity-50';

	const statusChip: Record<string, string> = {
		draft: 'text-muted',
		scheduled: 'text-accent-ink',
		running: 'text-ink',
		ended: 'text-muted',
		cancelled: 'text-faint'
	};

	const title = $derived(
		isPreset
			? isNewPreset
				? 'New preset'
				: name || 'Edit preset'
			: isNew
				? 'New giveaway'
				: name || prize || 'Giveaway'
	);
	const isDefaultPreset = $derived(isPreset && !isNewPreset && cfg.default_preset_id === presetId);
	// Editing a preset writes the feature config, which is admin-only. Managers can
	// still create/run giveaways (giveaway CRUD), just not manage the preset library.
	const admin = $derived(store.admin);
	const presetBlocked = $derived(isPreset && store.detail !== null && !admin);

	// ── Unsaved-changes dock ────────────────────────────────────────────────────
	// The floating "Unsaved changes" pill (shared with every other tab) appears the
	// moment the composition differs from what's stored. It's for editing an
	// EXISTING thing (a live/scheduled/draft giveaway or a saved preset); creating a
	// brand-new one still goes through the header's Publish / Save draft / Save
	// preset actions, and a read-only ended giveaway has nothing to save.
	let baseline = $state('');
	let savePhase = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
	function snapshot(): string {
		return JSON.stringify({
			name, prize, description, channelId, winnerCount, color, imageUrl,
			durationStr, startInStr, pingRoleId, showRequirements, excludeHost,
			allowBotsToWin, btnLabel, btnEmoji, btnStyle, enterButtonSuffix, buttonActions, announce, req,
			spec: msgStep.spec
		});
	}
	const dirty = $derived(
		loaded && !isNew && !isNewPreset && !readOnly && !presetBlocked && snapshot() !== baseline
	);
	async function saveChanges() {
		if (savePhase === 'saving' || !dirty) return;
		savePhase = 'saving';
		loadError = '';
		try {
			if (isPreset) {
				const preset = buildPreset(presetId);
				const exists = cfg.presets.some((p) => p.id === presetId);
				const presets = exists
					? cfg.presets.map((p) => (p.id === presetId ? preset : p))
					: [...cfg.presets, preset];
				const next = { ...cfg, presets, default_preset_id: cfg.default_preset_id || presetId };
				await api.saveFeature(store.id, FEATURE, featureEnabled, next);
				cfg = next;
			} else {
				await api.updateGiveaway(store.id, gwid, meta());
			}
			baseline = snapshot();
			savePhase = 'saved';
			setTimeout(() => (savePhase = 'idle'), 1400);
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not save.';
			savePhase = 'error';
		}
	}
	function discardChanges() {
		savePhase = 'idle';
		load();
	}
	function onKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's' && dirty) {
			e.preventDefault();
			saveChanges();
		}
	}
</script>

<svelte:head>
	<title>{title} · Giveaways · Dia</title>
</svelte:head>

<!-- Per-button action picker, rendered inline inside each button in the message
     preview (see MessageEditor buttonExtras). Edit the button's LOOK in the
     preview; set what it DOES here. -->
{#snippet buttonAction({ component }: { component: Component; ri: number; ci: number })}
	{@const suffix = component.custom_id_suffix}
	{#if suffix}
		{@const mode = buttonMode(suffix)}
		<div class="mt-2 space-y-1.5">
			<span class="font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground">Action</span>
			<div class="flex rounded-md border border-input p-0.5" role="radiogroup" aria-label="Button action">
				<button
					type="button"
					role="radio"
					aria-checked={mode === 'enter'}
					class="flex-1 rounded px-2 py-0.5 text-[10px] font-medium transition-colors {mode === 'enter' ? 'bg-secondary text-foreground' : 'text-muted-foreground hover:text-foreground'}"
					onclick={() => setButtonMode(suffix, 'enter')}
				>
					Enter giveaway
				</button>
				<button
					type="button"
					role="radio"
					aria-checked={mode === 'auto'}
					class="flex-1 rounded px-2 py-0.5 text-[10px] font-medium transition-colors {mode === 'auto' ? 'bg-secondary text-foreground' : 'text-muted-foreground hover:text-foreground'}"
					onclick={() => setButtonMode(suffix, 'auto')}
				>
					Run automation
				</button>
				<button
					type="button"
					role="radio"
					aria-checked={mode === 'none'}
					class="flex-1 rounded px-2 py-0.5 text-[10px] font-medium transition-colors {mode === 'none' ? 'bg-secondary text-foreground' : 'text-muted-foreground hover:text-foreground'}"
					onclick={() => setButtonMode(suffix, 'none')}
				>
					Nothing
				</button>
			</div>
			{#if mode === 'auto'}
				{#if automationList.length === 0}
					<p class="text-[10px] leading-snug text-muted-foreground">
						No automations yet. Build one on the Automations tab, then pick it here.
					</p>
				{:else}
					<Select bind:value={buttonActions[suffix]} options={automationOptions} />
				{/if}
			{/if}
		</div>
	{/if}
{/snippet}

<div class="flex h-full flex-col bg-bg text-ink">
	<!-- ── Header (matches the shared page-header chrome) ─────────────────────── -->
	<header class="flex min-h-12 shrink-0 flex-wrap items-center gap-2.5 border-b border-line bg-bg px-4 py-2 sm:px-5">
		<button
			type="button"
			onclick={backToList}
			class="grid size-6 shrink-0 place-items-center rounded border border-line bg-surface text-muted hover:text-ink"
			aria-label="Back to giveaways"
		>
			<ChevronLeft size={14} />
		</button>
		<Gift size={14} class="shrink-0 text-accent-ink" />
		<span class="truncate text-[13px] font-semibold tracking-tight text-ink">{title}</span>
		{#if isPreset}
			<span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.14em] text-accent-ink">preset</span>
		{:else if !isNew}
			<span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.14em] {statusChip[status] ?? 'text-muted'}">
				{status}
			</span>
		{/if}

		<div class="ml-auto flex flex-wrap items-center justify-end gap-1.5">
			{#if isPreset}
				{#if admin}
					{#if !isNewPreset && !isDefaultPreset}
						<button type="button" disabled={!!busy} onclick={setDefaultHere} class={btnGhost}>Set as default</button>
					{/if}
					{#if !isNewPreset && cfg.presets.length > 1}
						<button type="button" disabled={!!busy} onclick={deletePresetHere} class="grid size-8 place-items-center rounded-md border border-line text-muted hover:border-line-strong hover:text-danger disabled:opacity-50" aria-label="Delete preset">
							<Trash2 size={14} />
						</button>
					{/if}
					{#if isNewPreset}
						<button type="button" disabled={!!busy || !name.trim()} onclick={savePreset} class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90 disabled:opacity-40">
							<Bookmark size={13} /> Save preset
						</button>
					{/if}
				{/if}
			{:else if readOnly}
				{#if status === 'ended'}
					<button type="button" disabled={!!busy} onclick={() => run('reroll', () => api.rerollGiveaway(store.id, gwid, 0), false)} class={btnStrong}>
						<Dices size={13} /> Reroll
					</button>
				{/if}
			{:else}
				{#if admin}
					<button type="button" disabled={!!busy} onclick={saveAsPreset} class={btnGhost}>
						<Bookmark size={13} /> {isPresetUpdate ? 'Update preset' : 'Save as preset'}
					</button>
				{/if}
				{#if status === 'running' || status === 'scheduled'}
					{#if status === 'running'}
						<button type="button" disabled={!!busy} onclick={() => run('end', () => api.endGiveaway(store.id, gwid))} class={btnGhost}>
							<CheckCircle2 size={13} /> End now
						</button>
					{/if}
					<button type="button" disabled={!!busy} onclick={() => run('cancel', () => api.cancelGiveaway(store.id, gwid))} class="{btnGhost} hover:text-danger">
						<Ban size={13} /> Cancel
					</button>
				{:else}
					{#if isNew}
						<button type="button" disabled={!!busy} onclick={saveDraft} class={btnStrong}>
							<Save size={13} /> Save draft
						</button>
					{/if}
					{#if status === 'draft'}
						<button type="button" disabled={!!busy} onclick={() => run('delete', () => api.deleteGiveaway(store.id, gwid))} class="grid size-8 place-items-center rounded-md border border-line text-muted hover:border-line-strong hover:text-danger disabled:opacity-50" aria-label="Delete draft">
							<Trash2 size={14} />
						</button>
					{/if}
					<button type="button" disabled={!!busy || !canPublish} onclick={publish} class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90 disabled:opacity-40">
						<Rocket size={13} /> {parseDurationSeconds(startInStr) > 0 ? 'Schedule' : 'Publish'}
					</button>
				{/if}
			{/if}
		</div>
	</header>

	<!-- ── Body ──────────────────────────────────────────────────────────────── -->
	<div class="min-h-0 flex-1 overflow-y-auto">
		{#if loadError}
			<div class="border-b border-danger/40 bg-danger/5 px-4 py-2 text-[12px] text-danger sm:px-5">{loadError}</div>
		{/if}

		{#if !loaded}
			<div class="grid place-items-center py-24 text-[13px] text-muted">Loading…</div>
		{:else if presetBlocked}
			<div class="grid place-items-center py-24 px-6 text-center">
				<div class="flex max-w-md flex-col items-center gap-3">
					<span class="grid size-11 place-items-center rounded-full border border-line bg-surface text-muted">
						<Bookmark size={18} />
					</span>
					<div>
						<p class="text-[14px] font-semibold text-ink">Presets are admin-only</p>
						<p class="mt-1 text-[12px] text-muted">
							You can still create and manage giveaways. Ask a server admin to change the preset library.
						</p>
					</div>
					<button
						type="button"
						onclick={backToList}
						class="mt-1 inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90"
					>
						Back to giveaways
					</button>
				</div>
			</div>
		{:else}
			<div class="pb-24">
				<ModSection label="Message" desc="Compose the message, embeds and buttons right here. Click a button to set what it does: enter the giveaway, run one of your automations, or open a link.">
					<div class="max-w-2xl">
						{#if !readOnly}
							<p class="mb-2 text-[12px] text-muted">
								Use variables like
								<code class="rounded bg-surface px-1 font-mono text-[11px]">{'{{ .Ends }}'}</code>,
								<code class="rounded bg-surface px-1 font-mono text-[11px]">{'{{ .EntryCount }}'}</code>,
								<code class="rounded bg-surface px-1 font-mono text-[11px]">{'{{ .Winners }}'}</code>.
							</p>
						{/if}
						{#if !readOnly && !hasEnterButton}
							<div class="mb-2 rounded-md border border-accent/40 bg-accent/5 px-2.5 py-1.5 text-[12px] text-accent-ink">
								No Enter button yet. Add a button and set its action to <span class="font-medium">Enter giveaway</span>, or members won't be able to enter.
							</div>
						{/if}
						<div class={readOnly ? 'pointer-events-none opacity-70' : ''}>
							<MessageEditor step={msgStep} embeds components clickPaths={false} buttonExtras={buttonAction} />
						</div>
					</div>
				</ModSection>

				<ModSection label={isPreset ? 'Preset defaults' : 'Setup'} desc={isPreset ? 'Pre-filled when a giveaway starts from this preset.' : ''}>
					<div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
						{#if cfg.presets.length > 1 && showTiming}
							<Field label="Start from preset" hint="Prefills the message and settings.">
								<Select bind:value={presetPick} options={presetOptions} />
							</Field>
						{/if}
						{#if !isPreset}
							<Field label="Prize" hint="What you're giving away."><input class={inputCls} bind:value={prize} placeholder="Discord Nitro (1 month)" disabled={readOnly} /></Field>
						{/if}
						<Field label={isPreset ? 'Preset name' : 'Internal name'} hint={isPreset ? '' : 'Optional label for the list.'}>
							<input class={inputCls} bind:value={name} placeholder={isPreset ? 'Preset name' : prize || 'Giveaway'} disabled={readOnly} />
						</Field>
						<Field label={isPreset ? 'Default channel' : 'Channel'} hint={channelLocked && !readOnly ? 'Locked once live.' : ''}>
							<div class={channelLocked ? 'pointer-events-none opacity-60' : ''}>
								<ChannelSelect bind:value={channelId} />
							</div>
						</Field>
						<Field label={isPreset ? 'Default winners' : 'Winners'}><NumberField bind:value={winnerCount} min={1} max={100} /></Field>
						{#if !isPreset}
							<Field label="Accent colour"><ColorField bind:value={color} /></Field>
						{/if}
						{#if showTiming || isPreset}
							<Field label={isPreset ? 'Default duration' : 'Duration'} hint="e.g. 30m, 2h, 3d, 1w."><input class={inputCls} bind:value={durationStr} placeholder="24h" /></Field>
						{/if}
						{#if showTiming && !isPreset}
							<Field label="Start in" hint="Blank = start now."><input class={inputCls} bind:value={startInStr} placeholder="now" /></Field>
						{/if}
						{#if !isPreset}
							<Field label="Image URL" hint="Optional large image."><input class={inputCls} bind:value={imageUrl} placeholder="https://…" disabled={readOnly} /></Field>
						{/if}
						<Field label="Start ping role" hint="Pinged above the giveaway on start.">
							<RolePicker value={pingRoleId} onChange={(v) => (pingRoleId = v as string)} />
						</Field>
					</div>
				</ModSection>

				<ModSection label="Entry requirements" desc="Who can enter, and who gets bonus tickets.">
					<div class="grid gap-4 lg:grid-cols-2">
						<Field label="Required roles" hint="Must hold at least one to enter.">
							<RolePicker value={req.required_roles ?? []} multiple onChange={(v) => (req.required_roles = v as string[])} />
						</Field>
						<Field label="Blocked roles" hint="Holding any blocks entry.">
							<RolePicker value={req.blocked_roles ?? []} multiple onChange={(v) => (req.blocked_roles = v as string[])} />
						</Field>
						<Field label="Bypass roles" hint="Skip all requirements.">
							<RolePicker value={req.bypass_roles ?? []} multiple onChange={(v) => (req.bypass_roles = v as string[])} />
						</Field>
						<div class="grid grid-cols-3 gap-3">
							<Field label="Min account age (d)"><NumberField bind:value={req.min_account_age_days} min={0} /></Field>
							<Field label="Min in server (d)"><NumberField bind:value={req.min_member_age_days} min={0} /></Field>
							<Field label="Min level"><NumberField bind:value={req.min_level} min={0} /></Field>
						</div>
					</div>
					<div class="mt-4">
						<Field label="Bonus entries" hint="Give a role extra weighted tickets in the draw.">
							<div class="flex max-w-xl flex-col gap-2">
								{#each req.bonus_entries ?? [] as bonus, i (i)}
									<div class="flex items-center gap-2">
										<div class="min-w-0 flex-1"><RolePicker value={bonus.role_id} onChange={(v) => (bonus.role_id = v as string)} /></div>
										<span class="font-mono text-[11px] text-faint">+</span>
										<div class="w-16"><NumberField bind:value={bonus.entries} min={1} max={100} /></div>
										<button type="button" onclick={() => removeBonus(i)} class="grid size-8 shrink-0 place-items-center rounded-md border border-line text-muted hover:border-line-strong hover:text-danger" aria-label="Remove bonus">
											<Trash2 size={14} />
										</button>
									</div>
								{/each}
								<button type="button" onclick={addBonus} class="inline-flex h-8 w-fit items-center gap-1.5 rounded-md border border-line px-2.5 text-[12px] font-medium text-ink hover:border-line-strong">
									<Plus size={13} /> Add bonus role
								</button>
							</div>
						</Field>
					</div>
				</ModSection>

				<ModSection label="Winner announcement" desc="Posted in-channel when the giveaway is drawn.">
					<div class="grid gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
						<div class="flex flex-col gap-3">
							<TemplateField label="Announcement message" bind:value={announce.message} guildId={store.id} variables={GIVEAWAY_VARS} sample={GIVEAWAY_SAMPLE} rows={2} />
							<div class="grid gap-3 sm:grid-cols-2">
								<TemplateField label="Ended embed title" bind:value={announce.ended_title} guildId={store.id} variables={GIVEAWAY_VARS} sample={GIVEAWAY_SAMPLE} rows={1} />
								<TemplateField label="Ended embed footer" bind:value={announce.ended_footer} guildId={store.id} variables={GIVEAWAY_VARS} sample={GIVEAWAY_SAMPLE} rows={1} />
							</div>
							<TemplateField label="No-winners message" bind:value={announce.no_winners_message} guildId={store.id} variables={GIVEAWAY_VARS} sample={GIVEAWAY_SAMPLE} rows={1} />
							<div class="flex flex-col gap-2">
								<label class="flex items-center gap-2 text-[13px] text-ink">
									<Toggle bind:checked={announce.ping_winners} label="Ping winners" /> Ping the winners in the announcement
								</label>
								<label class="flex items-center gap-2 text-[13px] text-ink">
									<Toggle bind:checked={announce.jump_button} label="Jump button" /> Add a “Jump to giveaway” button
								</label>
								<label class="flex items-center gap-2 text-[13px] text-ink">
									<Toggle bind:checked={announce.dm_winners} label="DM winners" /> DM each winner when they win
								</label>
							</div>
							{#if announce.dm_winners}
								<TemplateField label="Winner DM" bind:value={announce.dm_message} guildId={store.id} variables={GIVEAWAY_VARS} sample={GIVEAWAY_SAMPLE} rows={2} />
							{/if}
						</div>

						<!-- Live preview of the drawn announcement -->
						<div class="lg:border-l lg:border-line lg:pl-5">
							<div class="mb-2 flex items-center gap-1.5 font-mono text-[10px] uppercase tracking-[0.14em] text-faint">
								<Eye size={12} /> Preview
							</div>
							<div class="rounded-lg border border-line bg-[#313338] p-3 text-[#dbdee1]">
								<div class="overflow-hidden rounded-[4px] bg-[#2b2d31]" style="border-left: 4px solid {annAccent}">
									<div class="px-3 py-2.5">
										<div class="text-[14px] font-semibold text-white">
											{fillAnn(announce.ended_title) || prize.trim() || 'Giveaway'}
										</div>
										<div class="mt-2">
											<div class="text-[12px] font-semibold text-white">Winners</div>
											<div class="text-[13px] text-[#dbdee1]">@alex, @sam</div>
										</div>
										<div class="mt-2">
											<div class="text-[12px] font-semibold text-white">Hosted by</div>
											<div class="text-[13px] text-[#dbdee1]">@you</div>
										</div>
										{#if fillAnn(announce.ended_footer)}
											<div class="mt-2 text-[11px] text-[#949ba4]">{fillAnn(announce.ended_footer)} · just now</div>
										{/if}
									</div>
								</div>
								{#if fillAnn(announce.message)}
									<div class="mt-2 text-[13px] leading-relaxed whitespace-pre-wrap text-[#dbdee1]">{fillAnn(announce.message)}</div>
								{/if}
								{#if announce.jump_button}
									<div class="mt-2">
										<span class="inline-flex items-center rounded-[3px] border border-[#4e5058] px-3 py-1.5 text-[13px] font-medium text-white">Jump to giveaway</span>
									</div>
								{/if}
							</div>
							{#if announce.dm_winners && fillAnn(announce.dm_message)}
								<div class="mt-3">
									<div class="mb-1 font-mono text-[10px] uppercase tracking-[0.14em] text-faint">Winner DM</div>
									<div class="rounded-lg border border-line bg-[#313338] p-3 text-[13px] leading-relaxed whitespace-pre-wrap text-[#dbdee1]">{fillAnn(announce.dm_message)}</div>
								</div>
							{/if}
							<p class="mt-2 text-[11px] leading-relaxed text-muted">Sample values shown; the bot adds the timestamp automatically.</p>
						</div>
					</div>
				</ModSection>

				<ModSection label="Behaviour">
					<div class="flex flex-col gap-2">
						<label class="flex items-center gap-2 text-[13px] text-ink">
							<Toggle bind:checked={showRequirements} label="Show requirements" /> List the rules on the embed
						</label>
						<label class="flex items-center gap-2 text-[13px] text-ink">
							<Toggle bind:checked={excludeHost} label="Exclude host" /> The host can't win their own giveaway
						</label>
						<label class="flex items-center gap-2 text-[13px] text-ink">
							<Toggle bind:checked={allowBotsToWin} label="Allow bots" /> Allow bot accounts to enter
						</label>
					</div>
				</ModSection>
			</div>
		{/if}
	</div>
</div>

<svelte:window onkeydown={onKeydown} />
<ReleaseDock {dirty} phase={savePhase} error={loadError} onsave={saveChanges} onreset={discardChanges} />
