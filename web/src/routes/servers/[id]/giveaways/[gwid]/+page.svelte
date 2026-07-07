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
		Bookmark
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
	let msgStep = $state<Step>({ id: 'gw-msg', kind: 'send_message', spec: { content: '', embeds: [] } });

	const readOnly = $derived(status === 'ended' || status === 'cancelled');
	const channelLocked = $derived(readOnly || status === 'running' || status === 'scheduled');

	function clone<T>(v: T): T {
		return JSON.parse(JSON.stringify(v)) as T;
	}

	function applySpec(spec: GiveawaySpec) {
		msgStep = {
			id: 'gw-msg',
			kind: 'send_message',
			spec: { content: spec.content ?? '', embeds: clone(spec.embeds ?? []) }
		};
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
			loaded = true;
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load the giveaway.';
		}
	}
	onMount(load);

	// Assemble the composed spec + create/update body from the current state.
	function buildSpec(): GiveawaySpec {
		const s = (msgStep.spec ?? {}) as Record<string, unknown>;
		return {
			content: (s.content as string) ?? '',
			embeds: ((s.embeds as GiveawaySpec['embeds']) ?? []) as GiveawaySpec['embeds'],
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

	const styleOptions = [
		{ value: 'primary', label: 'Blurple' },
		{ value: 'success', label: 'Green' },
		{ value: 'danger', label: 'Red' },
		{ value: 'secondary', label: 'Grey' }
	];
	const presetOptions = $derived(cfg.presets.map((p) => ({ value: p.id, label: p.name })));
	const isPresetUpdate = $derived(!!sourcePresetId && cfg.presets.some((p) => p.id === sourcePresetId));
	const canPublish = $derived(!!prize.trim() && !!channelId);
	const showTiming = $derived(isNew || status === 'draft');

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
</script>

<svelte:head>
	<title>{title} · Giveaways · Dia</title>
</svelte:head>

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
					<button type="button" disabled={!!busy || !name.trim()} onclick={savePreset} class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90 disabled:opacity-40">
						<Bookmark size={13} /> Save preset
					</button>
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
					<button type="button" disabled={!!busy} onclick={() => run('save', () => api.updateGiveaway(store.id, gwid, meta()), false)} class={btnStrong}>
						<Save size={13} /> Save changes
					</button>
					{#if status === 'running'}
						<button type="button" disabled={!!busy} onclick={() => run('end', () => api.endGiveaway(store.id, gwid))} class={btnGhost}>
							<CheckCircle2 size={13} /> End now
						</button>
					{/if}
					<button type="button" disabled={!!busy} onclick={() => run('cancel', () => api.cancelGiveaway(store.id, gwid))} class="{btnGhost} hover:text-danger">
						<Ban size={13} /> Cancel
					</button>
				{:else}
					<button type="button" disabled={!!busy} onclick={saveDraft} class={btnStrong}>
						<Save size={13} /> Save draft
					</button>
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
				<ModSection label="Message" desc="Edited like a message in any other tab. The Enter button is added automatically.">
					<div class="max-w-2xl">
						{#if !readOnly}
							<p class="mb-2 text-[12px] text-muted">
								Use variables like
								<code class="rounded bg-surface px-1 font-mono text-[11px]">{'{{ .Ends }}'}</code>,
								<code class="rounded bg-surface px-1 font-mono text-[11px]">{'{{ .EntryCount }}'}</code>,
								<code class="rounded bg-surface px-1 font-mono text-[11px]">{'{{ .Winners }}'}</code>.
							</p>
						{/if}
						<div class={readOnly ? 'pointer-events-none opacity-70' : ''}>
							<MessageEditor step={msgStep} embeds clickPaths={false} />
						</div>
					</div>
				</ModSection>

				<ModSection label="Enter button" desc="The button members click to enter.">
					<div class="grid max-w-2xl gap-3 sm:grid-cols-3">
						<Field label="Label"><input class={inputCls} bind:value={btnLabel} placeholder="Enter Giveaway" disabled={readOnly} /></Field>
						<Field label="Emoji" hint="A glyph, or name:id."><input class={inputCls} bind:value={btnEmoji} placeholder="🎉" disabled={readOnly} /></Field>
						<Field label="Colour"><Select bind:value={btnStyle} options={styleOptions} /></Field>
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
					<div class="flex max-w-2xl flex-col gap-3">
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
