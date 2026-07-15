<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { slide } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import type { SocialCapability, SocialSubscription } from '$lib/social';
	import Toggle from '$lib/components/Toggle.svelte';
	import Field from '$lib/components/Field.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import EmptyBlock from '$lib/components/page/EmptyBlock.svelte';
	import {
		Megaphone,
		Twitch,
		Youtube,
		Radio,
		Cloud,
		Rss,
		Twitter,
		Instagram,
		Music2,
		Lock,
		Plus,
		Pencil,
		Trash2,
		Send,
		ExternalLink,
		TriangleAlert,
		Loader2
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'social';

	// Provider presentation: lucide icon + brand accent. The catalogue itself
	// (which providers exist, which are unlocked) comes from the API — it is
	// detected from the server's environment credentials.
	const PROVIDER_ICONS: Record<string, typeof Megaphone> = {
		twitch: Twitch,
		youtube: Youtube,
		kick: Radio,
		bluesky: Cloud,
		rss: Rss,
		x: Twitter,
		instagram: Instagram,
		tiktok: Music2
	};
	const PROVIDER_COLORS: Record<string, string> = {
		twitch: '#9146FF',
		youtube: '#FF0000',
		kick: '#53FC18',
		bluesky: '#0085FF',
		rss: '#ff6363'
	};

	let caps = $state<SocialCapability[]>([]);
	let subs = $state<SocialSubscription[]>([]);
	let limit = $state(3);
	let enabled = $state(false);
	let loaded = $state(false);
	let loadErr = $state('');

	const capByProvider = $derived(new Map(caps.map((c) => [c.provider, c])));
	const atLimit = $derived(subs.length >= limit);

	onMount(async () => {
		try {
			const [f, list] = await Promise.all([api.feature(store.id, FEATURE), api.social(store.id)]);
			enabled = f.enabled;
			caps = list.capabilities;
			subs = list.subscriptions;
			limit = list.limit;
			loaded = true;
		} catch (e) {
			loadErr = e instanceof Error ? e.message : 'Failed to load social alerts';
		}
	});

	async function toggleFeature(v: boolean) {
		try {
			await api.saveFeature(store.id, FEATURE, v, {});
			if (store.detail) store.detail.features[FEATURE] = { enabled: v, config: {} };
		} catch {
			enabled = !v; // revert on failure
		}
	}

	// ── Add / edit editor ──────────────────────────────────────────────────────
	let editorOpen = $state(false);
	let editing = $state<SocialSubscription | null>(null); // null = creating
	let fProvider = $state('');
	let fAccount = $state('');
	let fChannel = $state('');
	let fPingRole = $state('');
	let fTemplate = $state('');
	let fEmbed = $state(true);
	let saving = $state(false);
	let saveErr = $state('');

	function openAdd(provider: string) {
		editing = null;
		fProvider = provider;
		fAccount = '';
		fChannel = '';
		fPingRole = '';
		fTemplate = '';
		fEmbed = true;
		saveErr = '';
		editorOpen = true;
	}

	function openEdit(sub: SocialSubscription) {
		editing = sub;
		fProvider = sub.provider;
		fAccount = sub.account_name;
		fChannel = sub.channel_id;
		fPingRole = sub.ping_role_id;
		fTemplate = sub.template;
		fEmbed = sub.embed;
		saveErr = '';
		editorOpen = true;
	}

	async function save() {
		if (saving) return;
		saveErr = '';
		if (!editing && !fAccount.trim()) {
			saveErr = 'Enter the account to follow.';
			return;
		}
		if (!fChannel) {
			saveErr = 'Pick a channel to announce in.';
			return;
		}
		saving = true;
		try {
			if (editing) {
				const res = await api.updateSocial(store.id, editing.id, {
					channel_id: fChannel,
					ping_role_id: fPingRole,
					template: fTemplate,
					embed: fEmbed,
					enabled: editing.enabled
				});
				subs = subs.map((s) => (s.id === res.subscription.id ? res.subscription : s));
			} else {
				const res = await api.createSocial(store.id, {
					provider: fProvider,
					account: fAccount,
					channel_id: fChannel,
					ping_role_id: fPingRole,
					template: fTemplate,
					embed: fEmbed
				});
				subs = [...subs, res.subscription];
				// The first subscription auto-enables the feature server-side.
				if (!enabled && subs.length === 1) enabled = true;
			}
			editorOpen = false;
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Something went wrong';
		} finally {
			saving = false;
		}
	}

	async function toggleSub(sub: SocialSubscription, v: boolean) {
		const prev = sub.enabled;
		sub.enabled = v;
		try {
			const res = await api.updateSocial(store.id, sub.id, {
				channel_id: sub.channel_id,
				ping_role_id: sub.ping_role_id,
				template: sub.template,
				embed: sub.embed,
				enabled: v
			});
			subs = subs.map((s) => (s.id === res.subscription.id ? res.subscription : s));
		} catch {
			sub.enabled = prev;
		}
	}

	// ── Test & delete ──────────────────────────────────────────────────────────
	let testingId = $state('');
	let testedId = $state('');
	async function test(sub: SocialSubscription) {
		if (testingId) return;
		testingId = sub.id;
		testedId = '';
		try {
			await api.testSocial(store.id, sub.id);
			testedId = sub.id;
			setTimeout(() => (testedId = ''), 2000);
		} catch {
			/* row keeps its state; the channel simply didn't get a message */
		} finally {
			testingId = '';
		}
	}

	let confirmDelete = $state<SocialSubscription | null>(null);
	async function doDelete() {
		const sub = confirmDelete;
		if (!sub) return;
		try {
			await api.deleteSocial(store.id, sub.id);
			subs = subs.filter((s) => s.id !== sub.id);
		} catch {
			/* keep the row on failure */
		}
	}

	function channelName(id: string): string {
		return store.channels.find((c) => c.id === id)?.name ?? id;
	}
	function providerName(key: string): string {
		return capByProvider.get(key)?.name ?? key;
	}
	const available = $derived(caps.filter((c) => c.status === 'available'));
	const comingSoon = $derived(caps.filter((c) => c.status === 'coming_soon'));
</script>

<svelte:head><title>Social Alerts · {store.name} · Dia</title></svelte:head>

<div class="relative flex h-full flex-col bg-bg text-ink">
	<PageTopbar
		eyebrow="Social Alerts"
		subtitle="Announce streams, uploads and posts from followed accounts."
	>
		{#snippet leading()}
			<div class="grid size-6 place-items-center rounded border border-line bg-surface text-muted">
				<Megaphone size={13} />
			</div>
		{/snippet}
		{#snippet actions()}
			<label class="ml-1 flex items-center gap-2 text-[12px]">
				<span class="hidden text-muted sm:inline">{enabled ? 'On' : 'Off'}</span>
				<Toggle bind:checked={enabled} label="Social alerts" onchange={toggleFeature} />
			</label>
		{/snippet}
	</PageTopbar>

	<div class="relative min-h-0 flex-1 overflow-y-auto bg-bg pb-16">
		{#if loadErr}
			<div class="px-5 py-8 text-[12.5px] text-danger">{loadErr}</div>
		{:else if !loaded}
			<div class="p-6">
				<div class="skeleton mb-3 h-6 w-40 rounded"></div>
				<div class="skeleton h-40 w-full rounded"></div>
			</div>
		{:else}
			<!-- ── Providers ─────────────────────────────────────────────────── -->
			<SectionBar label="Platforms" />
			<div class="grid grid-cols-2 gap-3 px-5 py-5 sm:grid-cols-3 lg:grid-cols-4">
				{#each available as cap (cap.provider)}
					{@const Icon = PROVIDER_ICONS[cap.provider] ?? Megaphone}
					<button
						type="button"
						onclick={() => openAdd(cap.provider)}
						disabled={atLimit}
						class="group flex items-center gap-3 rounded-lg border border-line bg-surface px-3.5 py-3 text-left transition-colors hover:border-line-strong disabled:cursor-not-allowed disabled:opacity-50"
						title={atLimit ? 'Subscription limit reached' : `Follow a ${cap.name} account`}
					>
						<span
							class="grid size-8 shrink-0 place-items-center rounded-md border border-line bg-bg"
							style="color: {PROVIDER_COLORS[cap.provider] ?? 'var(--color-muted)'}"
						>
							<Icon size={16} />
						</span>
						<span class="min-w-0">
							<span class="block truncate text-[12.5px] font-medium text-ink">{cap.name}</span>
							<span class="block truncate text-[11px] text-muted">{cap.input}</span>
						</span>
						<Plus size={14} class="ml-auto shrink-0 text-faint group-hover:text-muted" />
					</button>
				{/each}
				{#each comingSoon as cap (cap.provider)}
					{@const Icon = PROVIDER_ICONS[cap.provider] ?? Megaphone}
					<div
						class="flex items-center gap-3 rounded-lg border border-dashed border-line bg-bg px-3.5 py-3 opacity-70"
						title="Not available on this deployment yet"
					>
						<span class="grid size-8 shrink-0 place-items-center rounded-md border border-line bg-surface text-faint">
							<Icon size={16} />
						</span>
						<span class="min-w-0">
							<span class="block truncate text-[12.5px] font-medium text-muted">{cap.name}</span>
							<span
								class="block font-mono text-[9.5px] font-medium uppercase tracking-[0.12em] text-faint"
							>
								Coming soon
							</span>
						</span>
						<Lock size={13} class="ml-auto shrink-0 text-faint" />
					</div>
				{/each}
			</div>

			<!-- ── Editor ────────────────────────────────────────────────────── -->
			{#if editorOpen}
				{@const cap = capByProvider.get(fProvider)}
				<div transition:slide={{ duration: dur(220), easing: cubicOut }}>
					<SectionBar
						label={editing
							? `Edit · ${providerName(fProvider)} · ${editing.account_name}`
							: `Follow on ${providerName(fProvider)}`}
					/>
					<div class="border-b border-line/60 px-5 py-5">
						<div class="grid gap-x-8 lg:grid-cols-2">
							<div class="min-w-0">
								{#if !editing}
									<Field label={cap?.input ?? 'Account'} hint="Resolved and validated when you save.">
										<input
											type="text"
											bind:value={fAccount}
											placeholder={cap?.input ?? ''}
											class="input w-full"
										/>
									</Field>
								{/if}
								<Field label="Announce in" hint="New activity posts to this channel.">
									<ChannelPicker value={fChannel} onChange={(v) => (fChannel = v as string)} />
								</Field>
								<Field label="Ping role" hint="Optional role mentioned with each announcement.">
									<RolePicker
										includeManaged
										value={fPingRole}
										onChange={(v) => (fPingRole = v as string)}
										placeholder="No ping"
									/>
								</Field>
							</div>
							<div class="min-w-0">
								<Field
									label="Message"
									hint="Go template; leave empty for the default. Values: {'{{ .Account }}'}, {'{{ .Title }}'}, {'{{ .URL }}'}, {'{{ .Game }}'}, {'{{ .Platform }}'}."
								>
									<textarea
										bind:value={fTemplate}
										rows={3}
										placeholder={'🔴 **{{ .Account }}** is now live: {{ .Title }}'}
										class="input w-full resize-y font-mono text-[12px]"
									></textarea>
								</Field>
								<div class="flex items-center justify-between gap-4 border-b border-line/60 pb-4">
									<div class="min-w-0">
										<div class="text-[12.5px] font-medium text-ink">Rich embed</div>
										<div class="mt-0.5 text-[12px] text-muted">
											Attach a card with the title, link and preview. Off posts the bare link instead.
										</div>
									</div>
									<Toggle bind:checked={fEmbed} label="Rich embed" />
								</div>
							</div>
						</div>
						{#if saveErr}
							<p class="mt-3 flex items-center gap-1.5 text-[12px] text-danger">
								<TriangleAlert size={13} />
								{saveErr}
							</p>
						{/if}
						<div class="mt-4 flex items-center gap-2">
							<button
								type="button"
								onclick={save}
								disabled={saving}
								class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90 disabled:opacity-60"
							>
								{#if saving}<Loader2 size={13} class="animate-spin" />{/if}
								{editing ? 'Save changes' : 'Follow account'}
							</button>
							<button
								type="button"
								onclick={() => (editorOpen = false)}
								class="h-8 rounded-md border border-line bg-bg px-3 text-[12px] font-medium text-ink hover:border-line-strong"
							>
								Cancel
							</button>
						</div>
					</div>
				</div>
			{/if}

			<!-- ── Followed accounts ─────────────────────────────────────────── -->
			<SectionBar label="Followed accounts" count={`${subs.length} / ${limit}`} />
			{#if !enabled && subs.length}
				<div class="flex items-center gap-2 border-b border-line/60 px-5 py-3 text-[12px] text-muted">
					<span class="size-1.5 shrink-0 rounded-full bg-faint/40"></span>
					Social alerts is off. Turn it on, top-right, to start announcing.
				</div>
			{/if}
			{#if subs.length === 0}
				<EmptyBlock
					title="No accounts followed yet"
					body="Pick a platform above to follow a creator. Dia announces when they go live, upload or post."
				/>
			{:else}
				{#each subs as sub (sub.id)}
					{@const Icon = PROVIDER_ICONS[sub.provider] ?? Megaphone}
					<div class="flex items-center gap-3 border-b border-line/60 px-5 py-3.5 {sub.enabled ? '' : 'opacity-55'}">
						<span
							class="grid size-8 shrink-0 place-items-center rounded-md border border-line bg-surface"
							style="color: {PROVIDER_COLORS[sub.provider] ?? 'var(--color-muted)'}"
						>
							<Icon size={15} />
						</span>
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<a
									href={sub.account_url}
									target="_blank"
									rel="noreferrer"
									class="inline-flex min-w-0 items-center gap-1 text-[12.5px] font-medium text-ink hover:underline"
								>
									<span class="truncate">{sub.account_name}</span>
									<ExternalLink size={11} class="shrink-0 text-faint" />
								</a>
								{#if sub.live}
									<span
										class="rounded-sm bg-danger/15 px-1.5 py-px font-mono text-[9.5px] font-semibold uppercase tracking-wider text-danger"
									>
										Live
									</span>
								{/if}
								{#if sub.hook_status === 'error'}
									<span
										class="inline-flex items-center gap-1 font-mono text-[10px] text-danger"
										title={sub.last_error}
									>
										<TriangleAlert size={11} /> webhook error
									</span>
								{/if}
							</div>
							<div class="mt-0.5 truncate text-[11.5px] text-muted">
								{providerName(sub.provider)} → #{channelName(sub.channel_id)}
								{#if sub.ping_role_id}
									· pings a role
								{/if}
							</div>
						</div>
						<button
							type="button"
							onclick={() => test(sub)}
							disabled={!!testingId}
							class="hidden h-7 items-center gap-1.5 rounded-md border border-line bg-bg px-2 text-[11.5px] font-medium text-muted hover:border-line-strong hover:text-ink disabled:opacity-50 sm:inline-flex"
							title="Send a sample announcement to the channel"
						>
							{#if testingId === sub.id}
								<Loader2 size={12} class="animate-spin" />
							{:else}
								<Send size={12} />
							{/if}
							{testedId === sub.id ? 'Sent' : 'Test'}
						</button>
						<button
							type="button"
							onclick={() => openEdit(sub)}
							class="grid h-7 w-7 place-items-center rounded-md border border-line bg-bg text-muted hover:border-line-strong hover:text-ink"
							aria-label="Edit subscription"
						>
							<Pencil size={12} />
						</button>
						<button
							type="button"
							onclick={() => (confirmDelete = sub)}
							class="grid h-7 w-7 place-items-center rounded-md border border-line bg-bg text-muted hover:border-line-strong hover:text-danger"
							aria-label="Delete subscription"
						>
							<Trash2 size={12} />
						</button>
						<Toggle
							checked={sub.enabled}
							label="Subscription enabled"
							onchange={(v) => toggleSub(sub, v)}
						/>
					</div>
				{/each}
			{/if}
		{/if}
	</div>
</div>

<ConfirmDialog
	open={!!confirmDelete}
	title="Unfollow {confirmDelete?.account_name ?? 'this account'}?"
	description="Announcements for this account stop immediately. This can't be undone."
	confirmLabel="Unfollow"
	onconfirm={doDelete}
	oncancel={() => (confirmDelete = null)}
/>
