<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api, previewImage, layoutPreview } from '$lib/api';
	import type { Layout } from '$lib/layout/schema';
	import { rankStarterLayout } from '$lib/layout/templates';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import Select from '$lib/components/Select.svelte';
	import MultiSelect from '$lib/components/MultiSelect.svelte';
	import ColorField from '$lib/components/ColorField.svelte';
	import SaveBar from '$lib/components/SaveBar.svelte';
	import CardStudioModal from '$lib/components/editor/CardStudioModal.svelte';
	import { Trash2, Frame } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'leveling';

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
	type Cfg = {
		xp_min: number;
		xp_max: number;
		cooldown_seconds: number;
		multiplier: number;
		announce_level_up: boolean;
		announce_channel: string;
		level_up_message: string;
		no_xp_channels: string[];
		no_xp_roles: string[];
		stack_rewards: boolean;
		rank_card: RankCard;
	};

	function defaults(): Cfg {
		return {
			xp_min: 15,
			xp_max: 25,
			cooldown_seconds: 60,
			multiplier: 1.0,
			announce_level_up: true,
			announce_channel: '',
			level_up_message: 'GG {user.mention}, you reached **level {level}**!',
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
	let loaded = $state(false);
	let saving = $state(false);
	let baseline = $state('');
	let bgType = $state<'gradient' | 'solid'>('gradient');
	let previewUrl = $state('');
	let studioOpen = $state(false);
	let studioLayout = $state<Layout>(); // local seed for the modal; only committed on Apply

	// Sample rank values for the live preview + the Card Studio canvas/server render.
	const rankSampleVars: Record<string, string> = {
		'{level}': '12',
		'{rank}': '1',
		'{xp}': '53,200',
		'{level.xp}': '450',
		'{level.needed}': '1,000',
		'{progress}': '45%'
	};
	// Token hint chips — fetched from the server's single source of truth
	// (leveling.RankVariables), with an inline fallback if the request fails.
	let rankVarTokens = $state<string[]>([
		'{user}',
		'{user.mention}',
		'{user.avatar}',
		'{level}',
		'{rank}',
		'{xp}',
		'{level.xp}',
		'{level.needed}',
		'{progress}'
	]);

	// Rewards
	let rewards = $state<any[]>([]);
	let newLevel = $state<number>(5);
	let newRole = $state('');
	let newRemovePrevious = $state(false);
	let rewardBusy = $state(false);

	// Leaderboard
	let board = $state<any[]>([]);
	let boardLoaded = $state(false);
	let boardLoading = $state(false);

	const channelOpts = $derived(store.textChannelOptions());
	const roleOpts = $derived(store.roleOptions());
	const announceOpts = $derived([
		{ value: '', label: 'Same channel' },
		{ value: 'dm', label: 'Direct message' },
		...channelOpts
	]);
	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

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
		cfg = {
			...d,
			...c,
			rank_card: {
				...d.rank_card,
				...(c.rank_card ?? {}),
				background: { ...d.rank_card.background, ...((c.rank_card?.background) ?? {}) }
			}
		};
		enabled = f.enabled;
		bgType = cfg.rank_card.background.from && cfg.rank_card.background.to ? 'gradient' : 'solid';
		loaded = true;
		baseline = JSON.stringify({ enabled, cfg });
		// Variable hint chips from the server's single source of truth.
		api
			.levelingVariables(store.id)
			.then((v) => {
				if (v.variables?.length) rankVarTokens = v.variables.map((x) => x.token);
			})
			.catch(() => {});
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
		bgType = cfg.rank_card.background.from && cfg.rank_card.background.to ? 'gradient' : 'solid';
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
</script>

<svelte:head><title>Leveling · {store.name} · Dia</title></svelte:head>

<header class="mb-6 flex items-start justify-between gap-4">
	<div>
		<h1 class="text-2xl font-bold tracking-tight">Leveling</h1>
		<p class="mt-1 text-muted">Reward activity with XP, level-up announcements and role rewards.</p>
	</div>
	<Toggle bind:checked={enabled} />
</header>

{#if !loaded}
	<div class="text-muted">Loading…</div>
{:else}
	<div class="space-y-5">
		<!-- XP settings -->
		<section class="card p-6">
			<h2 class="mb-4 text-base font-semibold">XP settings</h2>
			<div class="grid gap-4 sm:grid-cols-2">
				<Field label="Min XP per message">
					<input class="input" type="number" min="0" bind:value={cfg.xp_min} />
				</Field>
				<Field label="Max XP per message">
					<input class="input" type="number" min="0" bind:value={cfg.xp_max} />
				</Field>
				<Field label="Cooldown (seconds)" hint="How long before a message earns XP again.">
					<input class="input" type="number" min="0" bind:value={cfg.cooldown_seconds} />
				</Field>
				<Field label="XP multiplier">
					<input class="input" type="number" min="0" step="0.1" bind:value={cfg.multiplier} />
				</Field>
			</div>

			<label class="mb-5 flex items-center gap-3">
				<Toggle bind:checked={cfg.announce_level_up} />
				<span class="text-sm">Announce level-ups</span>
			</label>

			{#if cfg.announce_level_up}
				<Field label="Announce in" hint="Where level-up messages are sent.">
					<Select bind:value={cfg.announce_channel} options={announceOpts} placeholder="Same channel" />
				</Field>
				<Field
					label="Level-up message"
					hint="Placeholders: {'{user}'} {'{user.mention}'} {'{username}'} {'{level}'}"
				>
					<textarea class="input" rows="2" bind:value={cfg.level_up_message}></textarea>
				</Field>
			{/if}

			<Field label="No-XP channels" hint="Messages in these channels earn no XP.">
				<MultiSelect bind:value={cfg.no_xp_channels} options={channelOpts} placeholder="Add a channel…" />
			</Field>
			<Field label="No-XP roles" hint="Members with these roles earn no XP.">
				<MultiSelect bind:value={cfg.no_xp_roles} options={roleOpts} placeholder="Add a role…" />
			</Field>
		</section>

		<!-- Rank card -->
		<section class="card p-6">
			<div class="mb-4 flex items-center justify-between gap-3">
				<h2 class="text-base font-semibold">Rank card</h2>
				<button
					type="button"
					class="flex items-center gap-1.5 rounded-md border border-line-strong px-2.5 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-ink-2"
					onclick={() => {
						// Seed the modal from a local copy; commit only on Apply so Cancel reverts.
						studioLayout = cfg.rank_card.layout ?? rankStarterLayout();
						studioOpen = true;
					}}
				>
					<Frame size={13} />
					{cfg.rank_card.layout ? 'Edit in Card Studio' : 'Design in Card Studio'}
				</button>
			</div>

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
							onclick={() => setBgType(t as any)}
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
		</section>

		<!-- Level rewards -->
		<section class="card p-6">
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-base font-semibold">Level rewards</h2>
				<label class="flex items-center gap-2 text-sm text-muted">
					Stack rewards <Toggle bind:checked={cfg.stack_rewards} />
				</label>
			</div>

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
						<input class="input" type="number" min="1" bind:value={newLevel} />
					</div>
					<div>
						<span class="label">Role</span>
						<Select bind:value={newRole} options={roleOpts} placeholder="Select a role…" />
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
		</section>

		<!-- Leaderboard -->
		<section class="card p-6">
			<div class="mb-4 flex items-center justify-between">
				<h2 class="text-base font-semibold">Leaderboard</h2>
				<button type="button" class="btn btn-ghost" disabled={boardLoading} onclick={loadBoard}>
					{boardLoading ? 'Loading…' : boardLoaded ? 'Refresh' : 'Load leaderboard'}
				</button>
			</div>

			{#if boardLoaded}
				{#if board.length}
					<div class="divide-y divide-line rounded-xl border border-line">
						{#each board as e, i (e.user_id ?? i)}
							<div class="flex items-center justify-between gap-3 px-4 py-3 text-sm">
								<div class="flex items-center gap-3">
									<span class="w-6 text-right font-mono text-muted">#{e.rank ?? i + 1}</span>
									<span class="font-medium">&lt;@{e.user_id}&gt;</span>
								</div>
								<div class="flex items-center gap-4 text-muted">
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
		</section>
	</div>

	<SaveBar {dirty} {saving} onsave={save} onreset={reset} />

	{#if studioOpen && studioLayout}
		<CardStudioModal
			layout={studioLayout}
			guildId={store.id}
			extraVars={rankSampleVars}
			onApply={(l) => (cfg.rank_card.layout = l)}
			onClose={() => (studioOpen = false)}
		/>
	{/if}
{/if}
