<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import {
		FEATURE,
		defaultConfig,
		GIVEAWAY_VARS,
		type GiveawayConfig,
		type GiveawaySummary
	} from '$lib/giveaway';
	import ModerationShell, { type ModTab } from '$lib/components/moderation/ModerationShell.svelte';
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
		Gift,
		Settings,
		Play,
		CheckCircle2,
		Plus,
		Trash2,
		Dices,
		Ban,
		ExternalLink
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);

	let enabled = $state(false);
	let cfg = $state<GiveawayConfig>(defaultConfig());
	let loaded = $state(false);
	let loadError = $state('');
	let saving = $state(false);
	let baseline = $state('');
	let tab = $state('settings');

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	// ── Giveaway list (Active / Ended tabs) ──────────────────────────────────
	let list = $state<GiveawaySummary[]>([]);
	let busyId = $state('');
	const activeGiveaways = $derived(
		list.filter((g) => g.status === 'running' || g.status === 'scheduled')
	);
	const endedGiveaways = $derived(list.filter((g) => g.status === 'ended'));

	const tabs = $derived<ModTab[]>([
		{ key: 'settings', label: 'Settings', icon: Settings },
		{ key: 'active', label: 'Active', icon: Play, badge: activeGiveaways.length || '' },
		{ key: 'ended', label: 'Ended', icon: CheckCircle2, badge: endedGiveaways.length || '' }
	]);

	function mergeConfig(d: GiveawayConfig, c: Partial<GiveawayConfig>): GiveawayConfig {
		return {
			...d,
			...c,
			embed: { ...d.embed, ...(c.embed ?? {}) },
			button: { ...d.button, ...(c.button ?? {}) },
			announce: { ...d.announce, ...(c.announce ?? {}) },
			requirements: { ...d.requirements, ...(c.requirements ?? {}) }
		};
	}

	async function load() {
		loadError = '';
		loaded = false;
		try {
			const f = await api.feature(store.id, FEATURE);
			cfg = mergeConfig(defaultConfig(), (f.config ?? {}) as Partial<GiveawayConfig>);
			enabled = f.enabled;
			baseline = JSON.stringify({ enabled, cfg });
			loaded = true;
			loadList();
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load giveaways.';
		}
	}
	onMount(load);

	async function loadList() {
		try {
			const r = await api.giveaways(store.id);
			list = (r.giveaways ?? []) as GiveawaySummary[];
		} catch {
			/* the list is best-effort; the settings tab still works */
		}
	}

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

	async function act(g: GiveawaySummary, fn: () => Promise<unknown>) {
		if (busyId) return;
		busyId = g.id;
		try {
			await fn();
			await loadList();
		} finally {
			busyId = '';
		}
	}

	// ── Bonus-entry list editor ──────────────────────────────────────────────
	function addBonus() {
		cfg.requirements.bonus_entries = [
			...(cfg.requirements.bonus_entries ?? []),
			{ role_id: '', entries: 2 }
		];
	}
	function removeBonus(i: number) {
		cfg.requirements.bonus_entries = (cfg.requirements.bonus_entries ?? []).filter(
			(_, j) => j !== i
		);
	}

	// ── Live embed preview (naive token substitution, for shape only) ─────────
	const sample: Record<string, string> = {
		Prize: 'Discord Nitro (1 month)',
		Description: 'Sample description',
		WinnerCount: '2',
		EntryCount: '48',
		Host: '@you',
		Winners: '@alice, @bob',
		WinnerList: '@alice\n@bob',
		Ends: 'in 2 hours',
		EndsAt: 'Today at 8:00 PM',
		Server: store.name || 'the server',
		MemberCount: '1,240',
		Channel: '#giveaways'
	};
	function preview(src: string): string {
		return (src || '').replace(/\{\{\s*\.(\w+)\s*\}\}/g, (_, k) => sample[k] ?? '');
	}

	const styleOptions = [
		{ value: 'primary', label: 'Blurple' },
		{ value: 'success', label: 'Green' },
		{ value: 'danger', label: 'Red' },
		{ value: 'secondary', label: 'Grey' }
	];
	const btnClass: Record<string, string> = {
		primary: 'bg-[#5865F2] text-white',
		success: 'bg-[#248046] text-white',
		danger: 'bg-[#da373c] text-white',
		secondary: 'bg-[#4e5058] text-white'
	};

	function relTime(iso?: string | null): string {
		if (!iso) return '';
		const diff = (new Date(iso).getTime() - Date.now()) / 1000;
		const abs = Math.abs(diff);
		const suffix = diff >= 0 ? 'from now' : 'ago';
		if (abs < 60) return `${Math.round(abs)}s ${suffix}`;
		if (abs < 3600) return `${Math.round(abs / 60)}m ${suffix}`;
		if (abs < 86400) return `${Math.round(abs / 3600)}h ${suffix}`;
		return `${Math.round(abs / 86400)}d ${suffix}`;
	}
	function jumpLink(g: GiveawaySummary): string {
		return `https://discord.com/channels/${store.id}/${g.channel_id}/${g.message_id}`;
	}

	const inputCls =
		'h-8 w-full rounded-md border border-line bg-bg px-2.5 text-[13px] text-ink placeholder:text-faint focus-visible:border-line-strong focus-visible:outline-none';
</script>

<svelte:head>
	<title>Giveaways · {store.name} · Dia</title>
</svelte:head>

<ModerationShell
	icon={Gift}
	title="Giveaways"
	blurb="Host prize draws with an Enter button, entry requirements and automatic winner draws."
	toggleLabel="Giveaways"
	bind:enabled
	bind:active={tab}
	{tabs}
	ready={loaded}
	error={loadError}
	onretry={load}
	{dirty}
	{saving}
	onsave={save}
	onreset={reset}
>
	{#if tab === 'settings'}
		<!-- ── Defaults ───────────────────────────────────────────────────── -->
		<ModSection label="Defaults" desc="Pre-filled when someone runs /giveaway start without those options.">
			<div class="grid gap-4 sm:grid-cols-2">
				<Field label="Default channel" hint="Where new giveaways post if none is given.">
					<ChannelSelect bind:value={cfg.default_channel_id} />
				</Field>
				<Field label="Default duration" hint="e.g. 30m, 2h, 3d, 1w.">
					<input class={inputCls} bind:value={cfg.default_duration} placeholder="24h" />
				</Field>
				<Field label="Default winners">
					<NumberField bind:value={cfg.default_winner_count} min={1} max={20} />
				</Field>
				<Field label="Start ping role" hint="Pinged above the giveaway when it starts.">
					<RolePicker
						value={cfg.ping_role_id ?? ''}
						onChange={(v) => (cfg.ping_role_id = v as string)}
					/>
				</Field>
			</div>
			<Field label="Manager roles" hint="These roles can create and manage giveaways (admins always can).">
				<RolePicker
					value={cfg.manager_roles ?? []}
					multiple
					onChange={(v) => (cfg.manager_roles = v as string[])}
				/>
			</Field>
		</ModSection>

		<!-- ── Embed + live preview ──────────────────────────────────────────── -->
		<ModSection label="Giveaway embed" desc="Every text field is a template — use the variables the picker offers.">
			<div class="grid gap-6 lg:grid-cols-[1fr_minmax(280px,360px)]">
				<div class="flex flex-col gap-4">
					<div class="grid gap-4 sm:grid-cols-2">
						<Field label="Accent colour"><ColorField bind:value={cfg.embed.color} /></Field>
						<Field label="Thumbnail URL" hint="Optional small image, top-right.">
							<input class={inputCls} bind:value={cfg.embed.thumbnail} placeholder="https://…" />
						</Field>
					</div>
					<TemplateField
						label="Title"
						bind:value={cfg.embed.title}
						guildId={store.id}
						variables={GIVEAWAY_VARS}
						rows={1}
					/>
					<TemplateField
						label="Description"
						bind:value={cfg.embed.description}
						guildId={store.id}
						variables={GIVEAWAY_VARS}
						rows={3}
					/>
					<TemplateField
						label="Footer"
						bind:value={cfg.embed.footer_text}
						guildId={store.id}
						variables={GIVEAWAY_VARS}
						rows={1}
					/>
					<div class="grid gap-4 sm:grid-cols-2">
						<Field label="“Hosted by” label"><input class={inputCls} bind:value={cfg.embed.hosted_by_label} /></Field>
						<Field label="“Ends” label"><input class={inputCls} bind:value={cfg.embed.ends_label} /></Field>
						<Field label="“Winners” label"><input class={inputCls} bind:value={cfg.embed.winners_label} /></Field>
						<Field label="“Entries” label"><input class={inputCls} bind:value={cfg.embed.entries_label} /></Field>
					</div>
					<label class="flex items-center gap-2 text-[13px] text-ink">
						<Toggle bind:checked={cfg.embed.show_timestamp} label="Show end timestamp in the footer" />
						Show end timestamp in the footer
					</label>
				</div>

				<!-- Preview card -->
				<div class="lg:sticky lg:top-4 lg:self-start">
					<div class="mb-2 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
						Preview
					</div>
					<div class="rounded-lg bg-[#313338] p-3 text-[13px] shadow-sm">
						<div class="flex gap-3 rounded-md" style="border-left: 4px solid {cfg.embed.color || '#FF6363'}; background:#2b2d31; padding:10px 12px;">
							<div class="min-w-0 flex-1">
								<div class="font-semibold text-white">{preview(cfg.embed.title) || 'Giveaway'}</div>
								{#if cfg.embed.description}
									<div class="mt-1 whitespace-pre-wrap text-[#dbdee1]">{preview(cfg.embed.description)}</div>
								{/if}
								<div class="mt-2 grid grid-cols-2 gap-2 text-[12px]">
									<div>
										<div class="font-semibold text-white">{cfg.embed.hosted_by_label || 'Hosted by'}</div>
										<div class="text-[#dbdee1]">@you</div>
									</div>
									<div>
										<div class="font-semibold text-white">{cfg.embed.ends_label || 'Ends'}</div>
										<div class="text-[#dbdee1]">in 2 hours</div>
									</div>
									<div>
										<div class="font-semibold text-white">{cfg.embed.winners_label || 'Winners'}</div>
										<div class="text-[#dbdee1]">2</div>
									</div>
									{#if cfg.show_entry_count}
										<div>
											<div class="font-semibold text-white">{cfg.embed.entries_label || 'Entries'}</div>
											<div class="text-[#dbdee1]">48</div>
										</div>
									{/if}
								</div>
								{#if cfg.embed.footer_text || cfg.embed.show_timestamp}
									<div class="mt-2 text-[11px] text-[#949ba4]">
										{preview(cfg.embed.footer_text)}{#if cfg.embed.show_timestamp}{cfg.embed.footer_text ? ' • ' : ''}Today at 8:00 PM{/if}
									</div>
								{/if}
							</div>
						</div>
						<div class="mt-2">
							<span class="inline-flex h-8 items-center gap-1.5 rounded-[3px] px-3 text-[13px] font-medium {btnClass[cfg.button.style] ?? btnClass.primary}">
								{cfg.button.emoji}
								{cfg.button.label || 'Enter Giveaway'}
							</span>
						</div>
					</div>
				</div>
			</div>
		</ModSection>

		<!-- ── Entry button ──────────────────────────────────────────────────── -->
		<ModSection label="Entry button" desc="The button members click to enter.">
			<div class="grid gap-4 sm:grid-cols-3">
				<Field label="Label"><input class={inputCls} bind:value={cfg.button.label} placeholder="Enter Giveaway" /></Field>
				<Field label="Emoji" hint="A glyph, or name:id for a custom emoji.">
					<input class={inputCls} bind:value={cfg.button.emoji} placeholder="🎉" />
				</Field>
				<Field label="Colour"><Select bind:value={cfg.button.style} options={styleOptions} /></Field>
			</div>
		</ModSection>

		<!-- ── Winner announcement ───────────────────────────────────────────── -->
		<ModSection label="Winner announcement" desc="Posted in-channel when the giveaway is drawn.">
			<TemplateField
				label="Announcement message"
				bind:value={cfg.announce.message}
				guildId={store.id}
				variables={GIVEAWAY_VARS}
				rows={2}
			/>
			<div class="grid gap-4 sm:grid-cols-2">
				<TemplateField
					label="Ended embed title"
					bind:value={cfg.announce.ended_title}
					guildId={store.id}
					variables={GIVEAWAY_VARS}
					rows={1}
				/>
				<TemplateField
					label="Ended embed footer"
					bind:value={cfg.announce.ended_footer}
					guildId={store.id}
					variables={GIVEAWAY_VARS}
					rows={1}
				/>
			</div>
			<TemplateField
				label="No-winners message"
				bind:value={cfg.announce.no_winners_message}
				guildId={store.id}
				variables={GIVEAWAY_VARS}
				rows={1}
			/>
			<div class="mt-1 flex flex-col gap-2">
				<label class="flex items-center gap-2 text-[13px] text-ink">
					<Toggle bind:checked={cfg.announce.ping_winners} label="Ping winners" /> Ping the winners in the announcement
				</label>
				<label class="flex items-center gap-2 text-[13px] text-ink">
					<Toggle bind:checked={cfg.announce.jump_button} label="Jump button" /> Add a “Jump to giveaway” button
				</label>
				<label class="flex items-center gap-2 text-[13px] text-ink">
					<Toggle bind:checked={cfg.announce.dm_winners} label="DM winners" /> DM each winner when they win
				</label>
			</div>
			{#if cfg.announce.dm_winners}
				<TemplateField
					label="Winner DM"
					bind:value={cfg.announce.dm_message}
					guildId={store.id}
					variables={GIVEAWAY_VARS}
					rows={2}
				/>
			{/if}
		</ModSection>

		<!-- ── Requirements ──────────────────────────────────────────────────── -->
		<ModSection
			label="Default entry requirements"
			desc="Applied to every new giveaway (a giveaway keeps the rules it was created with)."
		>
			<div class="grid gap-4 sm:grid-cols-2">
				<Field label="Required roles" hint="Must hold at least one to enter.">
					<RolePicker
						value={cfg.requirements.required_roles ?? []}
						multiple
						onChange={(v) => (cfg.requirements.required_roles = v as string[])}
					/>
				</Field>
				<Field label="Blocked roles" hint="Holding any of these blocks entry.">
					<RolePicker
						value={cfg.requirements.blocked_roles ?? []}
						multiple
						onChange={(v) => (cfg.requirements.blocked_roles = v as string[])}
					/>
				</Field>
				<Field label="Bypass roles" hint="Skip all requirements (still earn bonus entries).">
					<RolePicker
						value={cfg.requirements.bypass_roles ?? []}
						multiple
						onChange={(v) => (cfg.requirements.bypass_roles = v as string[])}
					/>
				</Field>
			</div>
			<div class="grid gap-4 sm:grid-cols-3">
				<Field label="Min account age (days)" hint="0 = off. Filters new alt accounts.">
					<NumberField bind:value={cfg.requirements.min_account_age_days} min={0} />
				</Field>
				<Field label="Min time in server (days)" hint="0 = off.">
					<NumberField bind:value={cfg.requirements.min_member_age_days} min={0} />
				</Field>
				<Field label="Min level" hint="0 = off. Needs the Leveling feature.">
					<NumberField bind:value={cfg.requirements.min_level} min={0} />
				</Field>
			</div>

			<Field label="Bonus entries" hint="Give a role extra weighted tickets in the draw.">
				<div class="flex flex-col gap-2">
					{#each cfg.requirements.bonus_entries ?? [] as bonus, i (i)}
						<div class="flex items-center gap-2">
							<div class="min-w-0 flex-1">
								<RolePicker
									value={bonus.role_id}
									onChange={(v) => (bonus.role_id = v as string)}
								/>
							</div>
							<span class="font-mono text-[11px] text-faint">+</span>
							<div class="w-20">
								<NumberField bind:value={bonus.entries} min={1} max={100} />
							</div>
							<button
								type="button"
								onclick={() => removeBonus(i)}
								class="grid size-8 shrink-0 place-items-center rounded-md border border-line text-muted hover:border-line-strong hover:text-danger"
								aria-label="Remove bonus entry"
							>
								<Trash2 size={14} />
							</button>
						</div>
					{/each}
					<button
						type="button"
						onclick={addBonus}
						class="inline-flex h-8 w-fit items-center gap-1.5 rounded-md border border-line px-2.5 text-[12px] font-medium text-ink hover:border-line-strong"
					>
						<Plus size={13} /> Add bonus role
					</button>
				</div>
			</Field>
		</ModSection>

		<!-- ── Behaviour ─────────────────────────────────────────────────────── -->
		<ModSection label="Behaviour">
			<div class="flex flex-col gap-2">
				<label class="flex items-center gap-2 text-[13px] text-ink">
					<Toggle bind:checked={cfg.show_entry_count} label="Show entry count" /> Show the live entry count on the embed
				</label>
				<label class="flex items-center gap-2 text-[13px] text-ink">
					<Toggle bind:checked={cfg.show_requirements} label="Show requirements" /> List the requirements on the embed
				</label>
				<label class="flex items-center gap-2 text-[13px] text-ink">
					<Toggle bind:checked={cfg.exclude_host} label="Exclude host" /> The host can't win their own giveaway
				</label>
				<label class="flex items-center gap-2 text-[13px] text-ink">
					<Toggle bind:checked={cfg.allow_bots_to_win} label="Allow bots" /> Allow bot accounts to enter
				</label>
			</div>
		</ModSection>
	{:else if tab === 'active'}
		{#if activeGiveaways.length === 0}
			<div class="px-5 py-16 text-center">
				<p class="text-[13px] font-medium text-ink">No active giveaways</p>
				<p class="mt-1 text-[12px] text-muted">Start one in Discord with <code class="rounded bg-surface px-1 font-mono">/giveaway start</code>.</p>
			</div>
		{:else}
			<div class="divide-y divide-line">
				{#each activeGiveaways as g (g.id)}
					<div class="flex items-center gap-3 px-4 py-3 sm:px-5">
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<span class="truncate text-[13px] font-semibold text-ink">{g.prize}</span>
								<span class="shrink-0 rounded-full border border-line px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide text-muted">
									{g.status}
								</span>
							</div>
							<div class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] text-muted">
								<span>{g.status === 'scheduled' ? 'starts' : 'ends'} {relTime(g.status === 'scheduled' ? g.starts_at : g.ends_at)}</span>
								<span>{g.entry_count} entries</span>
								<span>{g.winner_count} winner(s)</span>
								<span>in <a class="text-accent-ink hover:underline" href={jumpLink(g)} target="_blank" rel="noreferrer">#channel</a></span>
							</div>
						</div>
						<div class="flex shrink-0 items-center gap-1.5">
							{#if g.status === 'running'}
								<button
									type="button"
									disabled={busyId === g.id}
									onclick={() => act(g, () => api.endGiveaway(store.id, g.id))}
									class="inline-flex h-7 items-center gap-1 rounded-md border border-line-strong px-2.5 text-[12px] font-medium text-ink hover:bg-ink-2 disabled:opacity-50"
								>
									<CheckCircle2 size={13} /> End now
								</button>
							{/if}
							<button
								type="button"
								disabled={busyId === g.id}
								onclick={() => act(g, () => api.cancelGiveaway(store.id, g.id))}
								class="inline-flex h-7 items-center gap-1 rounded-md border border-line px-2.5 text-[12px] font-medium text-muted hover:border-line-strong hover:text-danger disabled:opacity-50"
							>
								<Ban size={13} /> Cancel
							</button>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	{:else if tab === 'ended'}
		{#if endedGiveaways.length === 0}
			<div class="px-5 py-16 text-center">
				<p class="text-[13px] font-medium text-ink">No ended giveaways yet</p>
			</div>
		{:else}
			<div class="divide-y divide-line">
				{#each endedGiveaways as g (g.id)}
					<div class="flex items-center gap-3 px-4 py-3 sm:px-5">
						<div class="min-w-0 flex-1">
							<div class="truncate text-[13px] font-semibold text-ink">{g.prize}</div>
							<div class="mt-0.5 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[11px] text-muted">
								<span>ended {relTime(g.ended_at)}</span>
								<span>{g.entry_count} entries</span>
								<span>
									{#if g.winners.length}
										{g.winners.length} winner(s) drawn
									{:else}
										no winners
									{/if}
								</span>
								{#if g.message_id}
									<a class="inline-flex items-center gap-0.5 text-accent-ink hover:underline" href={jumpLink(g)} target="_blank" rel="noreferrer">jump <ExternalLink size={11} /></a>
								{/if}
							</div>
						</div>
						<button
							type="button"
							disabled={busyId === g.id}
							onclick={() => act(g, () => api.rerollGiveaway(store.id, g.id, 0))}
							class="inline-flex h-7 shrink-0 items-center gap-1 rounded-md border border-line-strong px-2.5 text-[12px] font-medium text-ink hover:bg-ink-2 disabled:opacity-50"
						>
							<Dices size={13} /> Reroll
						</button>
					</div>
				{/each}
			</div>
		{/if}
	{/if}
</ModerationShell>
