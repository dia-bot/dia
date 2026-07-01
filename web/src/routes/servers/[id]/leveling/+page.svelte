<script lang="ts">
	import { onMount, onDestroy, getContext, setContext } from 'svelte';
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api, previewImage, layoutPreview } from '$lib/api';
	import type { Layout } from '$lib/layout/schema';
	import { rankStarterLayout } from '$lib/layout/templates';
	import type { Step } from '$lib/commands/types';
	import { AUTOMATION_CTX, EXPR_SCOPE_CTX } from '$lib/commands/expr-meta';
	import type { ExprScope } from '$lib/commands/expr-meta';
	import Field from '$lib/components/Field.svelte';
	import NumberField from '$lib/components/ui/NumberField.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ColorField from '$lib/components/ColorField.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import ReleaseDock from '$lib/components/page/ReleaseDock.svelte';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import CardStudioModal from '$lib/components/editor/CardStudioModal.svelte';
	import { TrendingUp, Trash2, Frame, Hash, Mail } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'leveling';

	// The level-up composer reads the same two contexts the welcome composer does:
	// AUTOMATION_CTX (false = the plain, non-canvas form) and EXPR_SCOPE_CTX (a
	// hint catalogue for the variable picker, never a runtime contract). Leveling
	// has no component handler, so we render buttons off in the editor below.
	setContext(AUTOMATION_CTX, false);
	const exprScope: ExprScope = {
		options: [],
		variables: [],
		steps: [],
		extraVars: [
			{ path: '.User.Username', label: 'User.Username', type: 'string', short: "Member's username" },
			{ path: '.User.GlobalName', label: 'User.GlobalName', type: 'string', short: "Member's display name" },
			{ path: '.Guild.Name', label: 'Guild.Name', type: 'string', short: 'Server name' },
			{ path: '.Guild.MemberCount', label: 'Guild.MemberCount', type: 'int', short: 'Live member count' }
		]
	};
	setContext(EXPR_SCOPE_CTX, exprScope);

	type Bg = { from?: string; to?: string; angle?: number; color?: string; image_url?: string };
	type RankCard = {
		layout?: Layout; // Card Studio design; when set it overrides the colors below
		background: Bg;
		accent_color: string;
		text_color: string;
		sub_text_color: string;
		bar_color: string;
		bar_bg_color: string;
	};
	// The rich level-up message (content + opaque embed specs), mirroring Go's
	// LevelUpMsg. The embeds pass through the composer untouched.
	type LevelUpMsg = { content: string; embeds: Record<string, unknown>[] };
	type Cfg = {
		xp_min: number;
		xp_max: number;
		cooldown_seconds: number;
		multiplier: number;
		announce_level_up: boolean;
		announce_channel: string;
		level_up_msg: LevelUpMsg;
		level_up_message: string; // legacy single-line message, kept for read migration
		no_xp_channels: string[];
		no_xp_roles: string[];
		stack_rewards: boolean;
		rank_card: RankCard;
	};

	const DEFAULT_LEVEL_UP = 'GG {user.mention}, you reached **level {level}**!';

	function defaults(): Cfg {
		return {
			xp_min: 15,
			xp_max: 25,
			cooldown_seconds: 60,
			multiplier: 1.0,
			announce_level_up: true,
			announce_channel: '',
			level_up_msg: { content: DEFAULT_LEVEL_UP, embeds: [] },
			level_up_message: '',
			no_xp_channels: [],
			no_xp_roles: [],
			stack_rewards: true,
			rank_card: {
				background: { from: '#1F1B2E', to: '#3A2E5C', angle: 30, color: '', image_url: '' },
				accent_color: '#B244FC',
				text_color: '#FFFFFF',
				sub_text_color: '#C9C3DA',
				bar_color: '#B244FC',
				bar_bg_color: ''
			}
		};
	}

	let enabled = $state(false);
	let cfg = $state<Cfg>(defaults());
	// The level-up message is edited as a live send_message Step, mirroring how
	// welcome edits cfg[tab].step. MessageEditor reassigns step.spec in place, so
	// this must be $state (proxied) for those edits to be tracked. A $effect below
	// syncs the edited content + opaque embeds back into cfg for dirty/save.
	let levelUpStep = $state<Step>({ id: 'levelup-msg', kind: 'send_message', spec: { content: DEFAULT_LEVEL_UP } });
	let tab = $state<'xp' | 'message'>('xp');
	let loaded = $state(false);
	let baseline = $state('');
	let bgType = $state<'gradient' | 'solid'>('gradient');
	let previewUrl = $state('');
	let studioOpen = $state(false);
	let studioLayout = $state<Layout>(); // local seed for the modal; only committed on Apply

	let savePhase = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
	let saveErr = $state('');

	// Sample rank values for the live preview + the Card Studio canvas/server render.
	const rankSampleVars: Record<string, string> = {
		'{level}': '12',
		'{rank}': '1',
		'{xp}': '53,200',
		'{level.xp}': '450',
		'{level.needed}': '1,000',
		'{progress}': '45%'
	};

	// Rewards (a separate, immediate-save resource — not part of the feature blob)
	let rewards = $state<any[]>([]);
	let newLevel = $state<number>(5);
	let newRole = $state('');
	let newRemovePrevious = $state(false);
	let rewardBusy = $state(false);

	// Leaderboard (lazy-loaded)
	let board = $state<any[]>([]);
	let boardLoaded = $state(false);
	let boardLoading = $state(false);

	const channelOpts = $derived(store.textChannelOptions());

	// The level-up message is edited as a live send_message Step (mirroring how
	// welcome edits cfg[tab].step). The step spec is the source of truth while
	// editing; msgFromStep folds it back into cfg.level_up_msg on save, and it is
	// part of serialize() so message edits count toward the dirty state. Embeds are
	// opaque EmbedSpec objects, passed through untouched.
	function msgFromStep(): LevelUpMsg {
		const spec = (levelUpStep.spec ?? {}) as Record<string, unknown>;
		return {
			content: (spec.content as string) ?? '',
			embeds: (spec.embeds as Record<string, unknown>[]) ?? []
		};
	}
	function stepFromMsg(m: LevelUpMsg): Step {
		const spec: Record<string, unknown> = { content: m.content };
		if (m.embeds?.length) spec.embeds = m.embeds;
		return { id: 'levelup-msg', kind: 'send_message', spec };
	}
	function serialize() {
		return JSON.stringify({ enabled, cfg, msg: msgFromStep() });
	}
	const dirty = $derived(loaded && serialize() !== baseline);

	function roleName(id: string) {
		return store.roles.find((r) => r.id === id)?.name ?? id;
	}

	async function loadRewards() {
		try {
			const r = await api.rewards(store.id);
			rewards = (r.rewards ?? []).sort((a: any, b: any) => a.level - b.level);
		} catch {
			rewards = [];
		}
	}

	onMount(async () => {
		const f = await api.feature(store.id, FEATURE);
		const d = defaults();
		const c = (f.config ?? {}) as Partial<Cfg>;
		// Seed the composer so existing guilds keep their message: prefer the new
		// rich content, then the legacy single-line message, then the default.
		const savedMsg = c.level_up_msg;
		const content = savedMsg?.content ?? c.level_up_message ?? DEFAULT_LEVEL_UP;
		const embeds = Array.isArray(savedMsg?.embeds) ? savedMsg!.embeds : [];
		cfg = {
			...d,
			...c,
			level_up_msg: { content, embeds },
			level_up_message: c.level_up_message ?? '',
			rank_card: {
				...d.rank_card,
				...(c.rank_card ?? {}),
				background: { ...d.rank_card.background, ...(c.rank_card?.background ?? {}) }
			}
		};
		// Seed the live composer step from the resolved message (new rich content,
		// legacy single-line, or the default) so an existing guild's message shows.
		levelUpStep = stepFromMsg({ content, embeds });
		enabled = f.enabled;
		bgType = cfg.rank_card.background.from && cfg.rank_card.background.to ? 'gradient' : 'solid';
		loaded = true;
		baseline = serialize();
		await loadRewards();
	});

	onDestroy(() => {
		clearTimeout(timer);
		if (previewUrl) URL.revokeObjectURL(previewUrl);
	});

	function setBgType(t: 'gradient' | 'solid') {
		bgType = t;
		if (t === 'solid') {
			cfg.rank_card.background.from = '';
			cfg.rank_card.background.to = '';
			if (!cfg.rank_card.background.color) cfg.rank_card.background.color = '#1F1B2E';
		} else {
			cfg.rank_card.background.color = '';
			if (!cfg.rank_card.background.from) cfg.rank_card.background.from = '#1F1B2E';
			if (!cfg.rank_card.background.to) cfg.rank_card.background.to = '#3A2E5C';
		}
	}

	// Live rank-card preview (debounced) — re-renders whenever theme fields change.
	let timer: ReturnType<typeof setTimeout>;
	$effect(() => {
		if (!loaded) return;
		// A Card Studio design takes precedence; otherwise the classic preset.
		const layout = cfg.rank_card.layout;
		const payload = layout
			? { mode: 'layout', layout }
			: {
					mode: 'rank',
					background: cfg.rank_card.background,
					accent_color: cfg.rank_card.accent_color,
					text_color: cfg.rank_card.text_color,
					sub_text_color: cfg.rank_card.sub_text_color,
					bar_color: cfg.rank_card.bar_color,
					bar_bg_color: cfg.rank_card.bar_bg_color,
					username: 'Member',
					rank: 1,
					level: 12,
					level_xp: 450,
					needed_xp: 1000,
					total_xp: 53200
				};
		const json = JSON.stringify(payload); // track deps
		clearTimeout(timer);
		timer = setTimeout(async () => {
			try {
				const p = JSON.parse(json);
				const url =
					p.mode === 'layout'
						? await layoutPreview(store.id, p.layout, rankSampleVars)
						: await previewImage(store.id, 'rank', p);
				if (previewUrl) URL.revokeObjectURL(previewUrl);
				previewUrl = url;
			} catch {
				/* preview is best-effort */
			}
		}, 400);
	});

	async function save() {
		if (savePhase === 'saving' || !dirty) return;
		savePhase = 'saving';
		saveErr = '';
		try {
			cfg.level_up_msg = msgFromStep();
			await api.saveFeature(store.id, FEATURE, enabled, cfg);
			if (store.detail)
				store.detail.features[FEATURE] = { enabled, config: cfg as unknown as Record<string, unknown> };
			baseline = serialize();
			savePhase = 'saved';
			setTimeout(() => {
				if (savePhase === 'saved') savePhase = 'idle';
			}, 1800);
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Something went wrong';
			savePhase = 'error';
		}
	}

	function reset() {
		const b = JSON.parse(baseline);
		enabled = b.enabled;
		cfg = b.cfg;
		levelUpStep = stepFromMsg(b.msg);
		bgType = cfg.rank_card.background.from && cfg.rank_card.background.to ? 'gradient' : 'solid';
		savePhase = 'idle';
		saveErr = '';
	}

	function onKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
			e.preventDefault();
			if (dirty) save();
		}
	}

	async function addReward() {
		if (!newRole || !newLevel) return;
		rewardBusy = true;
		try {
			await api.setReward(store.id, Number(newLevel), newRole, newRemovePrevious);
			newRole = '';
			newRemovePrevious = false;
			await loadRewards();
		} finally {
			rewardBusy = false;
		}
	}

	async function removeReward(level: number) {
		rewardBusy = true;
		try {
			await api.deleteReward(store.id, level);
			await loadRewards();
		} finally {
			rewardBusy = false;
		}
	}

	async function loadBoard() {
		boardLoading = true;
		try {
			const r = await api.leaderboard(store.id);
			board = r.entries ?? [];
			boardLoaded = true;
		} finally {
			boardLoading = false;
		}
	}

	const tabs = [
		{ k: 'xp', label: 'XP & rewards' },
		{ k: 'message', label: 'Level-up message' }
	] as const;
</script>

<svelte:head><title>Leveling · {store.name} · Dia</title></svelte:head>
<svelte:window onkeydown={onKeydown} />

<div class="flex h-full flex-col bg-bg text-ink">
	<!-- ── Slab topbar ──────────────────────────────────────────────────── -->
	<PageTopbar
		eyebrow="Leveling"
		subtitle="Reward activity with XP, ranks, role rewards, and a level-up announcement."
	>
		{#snippet leading()}
			<div class="grid size-6 place-items-center rounded border border-line bg-surface text-accent-ink">
				<TrendingUp size={13} />
			</div>
		{/snippet}
		{#snippet actions()}
			<label class="ml-1 flex items-center gap-2 text-[12px]">
				<span class="hidden text-muted sm:inline">{enabled ? 'On' : 'Off'}</span>
				<Toggle bind:checked={enabled} label="Leveling system" />
			</label>
		{/snippet}
	</PageTopbar>

	<!-- ── Tab band ─────────────────────────────────────────────────────── -->
	<div class="flex min-h-10 shrink-0 flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-line/60 bg-bg px-5 py-1.5 md:flex-nowrap">
		<span class="hidden font-mono text-[10px] uppercase tracking-[0.14em] text-faint sm:inline">Editing</span>
		<div class="flex items-center gap-1 rounded-lg border border-line bg-ink-2 p-0.5">
			{#each tabs as t (t.k)}
				<button
					type="button"
					onclick={() => (tab = t.k)}
					class="flex items-center gap-1.5 rounded-md px-2.5 py-1 text-[12.5px] font-medium transition-colors {tab ===
					t.k
						? 'bg-surface text-ink shadow-[inset_0_1px_0_rgba(255,255,255,0.05)]'
						: 'text-muted hover:text-ink'}"
				>
					<span>{t.label}</span>
				</button>
			{/each}
		</div>
	</div>

	<!-- ── Body ─────────────────────────────────────────────────────────── -->
	<div class="relative min-h-0 flex-1 overflow-y-auto bg-bg">
		{#if !loaded}
			<div class="mx-auto w-full max-w-2xl p-6">
				<div class="skeleton mb-3 h-6 w-40 rounded"></div>
				<div class="skeleton h-72 w-full rounded-xl"></div>
			</div>
		{:else}
			{#key tab}
				<div in:fly={{ y: 8, duration: 160, easing: cubicOut }}>
					{#if tab === 'xp'}
						<!-- ── XP earning ─────────────────────────────────────── -->
						<SectionBar label="XP earning" />
						<div class="mx-auto w-full max-w-3xl px-5 py-6">
							<div class="grid gap-4 sm:grid-cols-2">
								<Field label="Min XP per message">
									<NumberField min={0} bind:value={cfg.xp_min} />
								</Field>
								<Field label="Max XP per message">
									<NumberField min={0} bind:value={cfg.xp_max} />
								</Field>
								<Field label="Cooldown (seconds)" hint="How long before a message earns XP again.">
									<NumberField min={0} bind:value={cfg.cooldown_seconds} />
								</Field>
								<Field label="XP multiplier">
									<NumberField min={0} step={0.1} bind:value={cfg.multiplier} />
								</Field>
							</div>
						</div>

						<!-- ── No-XP exclusions ───────────────────────────────── -->
						<SectionBar label="No-XP exclusions" />
						<div class="mx-auto w-full max-w-3xl px-5 py-6">
							<Field label="No-XP channels" hint="Messages in these channels earn no XP.">
								<ChannelPicker
									multiple
									value={cfg.no_xp_channels}
									onChange={(v) => (cfg.no_xp_channels = v as string[])}
									placeholder="Add a channel…"
								/>
							</Field>
							<Field label="No-XP roles" hint="Members with these roles earn no XP.">
								<RolePicker
									multiple
									value={cfg.no_xp_roles}
									onChange={(v) => (cfg.no_xp_roles = v as string[])}
									placeholder="Add a role…"
								/>
							</Field>
						</div>

						<!-- ── Rank card ──────────────────────────────────────── -->
						<SectionBar label="Rank card">
							<button
								type="button"
								class="inline-flex h-7 items-center gap-1.5 rounded-md border border-line px-2.5 text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink"
								onclick={() => {
									// Seed the modal from a local copy; commit only on Apply so Cancel reverts.
									studioLayout = cfg.rank_card.layout ?? rankStarterLayout();
									studioOpen = true;
								}}
							>
								<Frame size={13} />
								{cfg.rank_card.layout ? 'Edit in Card Studio' : 'Design in Card Studio'}
							</button>
						</SectionBar>
						<div class="mx-auto w-full max-w-3xl px-5 py-6">
							<!-- Live preview -->
							<div class="mb-5 overflow-hidden rounded-xl border border-line bg-ink-2">
								{#if previewUrl}
									<img src={previewUrl} alt="Rank card preview" class="w-full" />
								{:else}
									<div class="flex aspect-[934/282] items-center justify-center text-sm text-faint">
										Rendering preview…
									</div>
								{/if}
							</div>

							{#if cfg.rank_card.layout}
								<div class="flex flex-wrap items-center gap-2 rounded-xl border border-line bg-ink-2 p-3 text-sm">
									<span class="text-muted">Using a Card Studio design.</span>
									<button
										type="button"
										class="ml-auto text-xs font-medium text-muted underline-offset-2 hover:text-accent-ink hover:underline"
										onclick={() => (cfg.rank_card.layout = undefined)}
									>
										Revert to classic colours
									</button>
								</div>
								<p class="mt-2 text-[11px] text-faint">
									Card variables: <span class="font-mono">{'{{.User.Username}}'} {'{{.User.Avatar}}'} {'{{.Level}}'} {'{{.Rank}}'} {'{{.XP}}'} {'{{.Progress}}'} {'{{.Guild.Name}}'} {'{{.Guild.Icon}}'}</span>
								</p>
							{:else}
								<Field label="Background">
									<div class="mb-3 inline-flex rounded-lg border border-line-strong p-0.5">
										{#each ['gradient', 'solid'] as t (t)}
											<button
												type="button"
												onclick={() => setBgType(t as 'gradient' | 'solid')}
												class="rounded-md px-3 py-1 text-sm capitalize {bgType === t
													? 'bg-ink text-white'
													: 'text-muted'}"
											>
												{t}
											</button>
										{/each}
									</div>
									{#if bgType === 'gradient'}
										<div class="grid gap-3 sm:grid-cols-2">
											<ColorField label="From" bind:value={cfg.rank_card.background.from} />
											<ColorField label="To" bind:value={cfg.rank_card.background.to} />
										</div>
									{:else}
										<ColorField label="Color" bind:value={cfg.rank_card.background.color} />
									{/if}
								</Field>

								<div class="grid gap-4 sm:grid-cols-3">
									<ColorField label="Accent" bind:value={cfg.rank_card.accent_color} />
									<ColorField label="Text" bind:value={cfg.rank_card.text_color} />
									<ColorField label="Subtext" bind:value={cfg.rank_card.sub_text_color} />
								</div>
								<div class="grid gap-4 sm:grid-cols-2">
									<ColorField label="Progress bar" bind:value={cfg.rank_card.bar_color} />
									<ColorField label="Progress bar track" bind:value={cfg.rank_card.bar_bg_color} />
								</div>
							{/if}
						</div>

						<!-- ── Level rewards ──────────────────────────────────── -->
						<SectionBar label="Level rewards" count={rewards.length}>
							<label class="flex items-center gap-2 text-[12px] text-muted">
								Stack rewards <Toggle bind:checked={cfg.stack_rewards} />
							</label>
						</SectionBar>
						<div class="mx-auto w-full max-w-3xl px-5 py-6">
							{#if rewards.length}
								<div class="mb-4 divide-y divide-line rounded-xl border border-line">
									{#each rewards as r (r.level)}
										<div class="flex items-center justify-between gap-3 px-4 py-3">
											<div class="flex items-center gap-3">
												<span class="inline-flex h-7 min-w-7 items-center justify-center rounded-full bg-blush px-2 text-xs font-semibold text-accent-ink">
													Lvl {r.level}
												</span>
												<span class="text-sm font-medium">{roleName(r.role_id)}</span>
												{#if r.remove_previous}
													<span class="text-xs text-faint">replaces previous</span>
												{/if}
											</div>
											<button
												type="button"
												class="text-muted hover:text-accent disabled:opacity-50"
												disabled={rewardBusy}
												onclick={() => removeReward(r.level)}
												aria-label="Remove reward"
											>
												<Trash2 size={16} />
											</button>
										</div>
									{/each}
								</div>
							{:else}
								<p class="mb-4 text-sm text-muted">No level rewards yet.</p>
							{/if}

							<div class="rounded-xl border border-line bg-ink-2 p-4">
								<div class="grid items-end gap-3 sm:grid-cols-[7rem_1fr_auto]">
									<div>
										<span class="label">Level</span>
										<NumberField min={1} bind:value={newLevel} />
									</div>
									<div>
										<span class="label">Role</span>
										<RolePicker value={newRole} onChange={(v) => (newRole = v as string)} placeholder="Select a role…" />
									</div>
									<button
										type="button"
										class="btn btn-accent"
										disabled={rewardBusy || !newRole || !newLevel}
										onclick={addReward}
									>
										Add reward
									</button>
								</div>
								<label class="mt-3 flex items-center gap-3">
									<Toggle bind:checked={newRemovePrevious} />
									<span class="text-sm">Remove previously earned reward roles</span>
								</label>
							</div>
						</div>

						<!-- ── Leaderboard ────────────────────────────────────── -->
						<SectionBar label="Leaderboard">
							<button
								type="button"
								class="inline-flex h-7 items-center rounded-md border border-line px-2.5 text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink disabled:opacity-50"
								disabled={boardLoading}
								onclick={loadBoard}
							>
								{boardLoading ? 'Loading…' : boardLoaded ? 'Refresh' : 'Load leaderboard'}
							</button>
						</SectionBar>
						<div class="mx-auto w-full max-w-3xl px-5 py-6">
							{#if boardLoaded}
								{#if board.length}
									<div class="divide-y divide-line rounded-xl border border-line">
										{#each board as e, i (e.user_id ?? i)}
											<div class="flex items-center justify-between gap-3 px-4 py-3 text-sm">
												<div class="flex min-w-0 items-center gap-3">
													<span class="w-6 shrink-0 text-right font-mono text-muted">#{e.rank ?? i + 1}</span>
													<span class="truncate font-medium">&lt;@{e.user_id}&gt;</span>
												</div>
												<div class="flex shrink-0 items-center gap-4 text-muted">
													<span>Level {e.level}</span>
													<span class="font-mono text-xs">{e.xp} XP</span>
												</div>
											</div>
										{/each}
									</div>
								{:else}
									<p class="text-sm text-muted">No leaderboard entries yet.</p>
								{/if}
							{:else}
								<p class="text-sm text-muted">Load the leaderboard to see the top members by XP.</p>
							{/if}
						</div>
					{:else}
						<!-- ── Level-up message ───────────────────────────────── -->
						<SectionBar label="Announce" />
						<div class="mx-auto w-full max-w-2xl px-5 py-6 space-y-4">
							<div class="flex items-center justify-between gap-3 rounded-xl border border-accent/25 bg-accent/[0.06] px-3.5 py-2.5">
								<div class="min-w-0">
									<div class="truncate text-[12.5px] font-medium text-ink">Announce level-ups</div>
									<div class="truncate text-[11.5px] text-muted">Post a message when a member reaches a new level.</div>
								</div>
								<label class="flex shrink-0 items-center gap-2 text-[12px]">
									<span class="hidden text-muted sm:inline">{cfg.announce_level_up ? 'On' : 'Off'}</span>
									<Toggle bind:checked={cfg.announce_level_up} label="Announce level-ups" />
								</label>
							</div>

							{#if cfg.announce_level_up}
								<div class="flex flex-wrap items-center gap-2 text-[12.5px] text-muted">
									<Hash size={14} class="text-faint" />
									<span>Announce in</span>
									<div class="flex items-center gap-1 rounded-lg border border-line bg-ink-2 p-0.5">
										<button
											type="button"
											onclick={() => (cfg.announce_channel = '')}
											class="rounded-md px-2.5 py-1 text-[12px] font-medium transition-colors {cfg.announce_channel ===
											''
												? 'bg-surface text-ink'
												: 'text-muted hover:text-ink'}"
										>
											Same channel
										</button>
										<button
											type="button"
											onclick={() => (cfg.announce_channel = 'dm')}
											class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1 text-[12px] font-medium transition-colors {cfg.announce_channel ===
											'dm'
												? 'bg-surface text-ink'
												: 'text-muted hover:text-ink'}"
										>
											<Mail size={13} /> Direct message
										</button>
										<button
											type="button"
											onclick={() => {
												if (cfg.announce_channel === '' || cfg.announce_channel === 'dm') {
													cfg.announce_channel = channelOpts[0]?.value ?? '';
												}
											}}
											class="rounded-md px-2.5 py-1 text-[12px] font-medium transition-colors {cfg.announce_channel !==
												'' && cfg.announce_channel !== 'dm'
												? 'bg-surface text-ink'
												: 'text-muted hover:text-ink'}"
										>
											A channel
										</button>
									</div>
									{#if cfg.announce_channel !== '' && cfg.announce_channel !== 'dm'}
										<div class="min-w-[200px] max-w-xs flex-1">
											<ChannelSelect bind:value={cfg.announce_channel} placeholder="Channel to announce in" />
										</div>
									{/if}
								</div>
								<p class="text-[11.5px] text-faint">
									{#if cfg.announce_channel === ''}
										The message posts in the channel they leveled up in.
									{:else if cfg.announce_channel === 'dm'}
										The message is sent to the member as a direct message.
									{:else}
										The message posts in the chosen channel.
									{/if}
								</p>
							{/if}
						</div>

						<SectionBar label="Message" />
						<div class="mx-auto w-full max-w-2xl px-5 py-6">
							<div class="transition-opacity {cfg.announce_level_up ? '' : 'opacity-60'}">
								<MessageEditor step={levelUpStep} embeds components={false} clickPaths={false} />
							</div>
						</div>
					{/if}
				</div>
			{/key}
		{/if}

		<!-- Release dock — the saving experience -->
		{#if loaded}
			<ReleaseDock {dirty} phase={savePhase} error={saveErr} onsave={save} onreset={reset} />
		{/if}
	</div>

	{#if studioOpen && studioLayout}
		<CardStudioModal
			layout={studioLayout}
			guildId={store.id}
			extraVars={rankSampleVars}
			onApply={(l) => (cfg.rank_card.layout = l)}
			onClose={() => (studioOpen = false)}
		/>
	{/if}
</div>
