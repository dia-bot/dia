<script lang="ts">
	import { onMount, onDestroy, getContext, setContext } from 'svelte';
	import { beforeNavigate, goto } from '$app/navigation';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api, layoutPreview } from '$lib/api';
	import type { Layout } from '$lib/layout/schema';
	import { templateLayout } from '$lib/layout/templates';
	import type { Step } from '$lib/commands/types';
	import { AUTOMATION_CTX, EXPR_SCOPE_CTX } from '$lib/commands/expr-meta';
	import type { ExprScope } from '$lib/commands/expr-meta';
	import Toggle from '$lib/components/Toggle.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import ReleaseDock from '$lib/components/page/ReleaseDock.svelte';
	import TabSwipe from '$lib/components/page/TabSwipe.svelte';
	import SubTabs from '$lib/components/page/SubTabs.svelte';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import CardStudioModal from '$lib/components/editor/CardStudioModal.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import {
		Send,
		Image as ImageIcon,
		UserPlus,
		UserMinus,
		Zap,
		ExternalLink,
		Hash,
		Mail
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'welcome';
	const base = $derived(`/servers/${store.id}`);

	// MessageEditor (the slash-command composer) reads two contexts. We provide
	// both so its variable picker offers the welcome scope and it renders in its
	// non-automation form. The scope is a hint catalogue only — never a runtime
	// contract — so listing the tokens welcome injects is safe.
	setContext(AUTOMATION_CTX, false);
	const exprScope: ExprScope = {
		options: [],
		variables: [],
		steps: [],
		extraVars: [
			{ path: '.User.GlobalName', label: 'User.GlobalName', type: 'string', short: "Member's display name" },
			{ path: '.User.Avatar', label: 'User.Avatar', type: 'string', short: 'Avatar image URL' },
			{ path: '.Count', label: 'Count', type: 'int', short: 'Member count after they joined' },
			{ path: '.CountOrdinal', label: 'CountOrdinal', type: 'string', short: 'Member count, like 1,024th' },
			{ path: '.Guild.MemberCount', label: 'Guild.MemberCount', type: 'int', short: 'Live member count' }
		]
	};
	setContext(EXPR_SCOPE_CTX, exprScope);

	// ── Persisted (saved) shape — mirrors internal/features/welcome/config.go ──
	type Field = { name: string; value: string; inline: boolean };
	type Embed = {
		enabled: boolean;
		color: string;
		author_name: string;
		author_icon: string;
		title: string;
		url: string;
		description: string;
		fields: Field[];
		thumbnail: string;
		image_url: string;
		footer_text: string;
		footer_icon: string;
		timestamp: boolean;
	};
	type Card = { enabled: boolean; layout?: Layout };
	// One row of message components (buttons / selects), mirroring the Go
	// ComponentRow shape the MessageEditor produces in `spec.components`.
	type CompRow = { components: Record<string, unknown>[] };
	type SavedMsg = {
		enabled: boolean;
		channel_id: string;
		content: string;
		ping_user: boolean;
		embeds: Embed[];
		components: CompRow[];
		// Per-button click programs, authored on the Welcome automation flow.
		// The page never edits these; it round-trips them untouched so saving
		// the message here can't wipe actions wired on the canvas.
		actions: Record<string, unknown>[];
		card: Card;
		// The DM carries the same rich surface as the channel message (embeds,
		// buttons / selects and their click actions), mirroring Go's DMConfig.
		// attach_card also attaches the channel message's card image to the DM
		// (there is no separate DM card design).
		dm: {
			enabled: boolean;
			attach_card: boolean;
			content: string;
			embeds: Embed[];
			components: CompRow[];
			actions: Record<string, unknown>[];
		};
	};
	type SavedCfg = { welcome: SavedMsg; goodbye: SavedMsg };

	// ── In-memory editor shape — the message + dm are real `send_message` /
	// `send_dm` Steps so the slash-command MessageEditor can edit them in place.
	type MsgState = {
		enabled: boolean;
		channel_id: string;
		step: Step;
		// Carried through untouched (click actions are edited on the flow, not here).
		actions: Record<string, unknown>[];
		card: Card;
		// The DM is its own send_dm Step; its click actions ride along untouched too.
		dm: { enabled: boolean; attach_card: boolean; step: Step; actions: Record<string, unknown>[] };
	};
	type CfgState = { welcome: MsgState; goodbye: MsgState };

	function savedDefaults(): SavedCfg {
		return {
			welcome: {
				enabled: true,
				channel_id: '',
				content: 'Hey {user.mention}, welcome to **{server}**! 🎉',
				ping_user: true,
				embeds: [],
				components: [],
				actions: [],
				card: { enabled: true, layout: templateLayout('aurora') },
				dm: { enabled: false, attach_card: false, content: '', embeds: [], components: [], actions: [] }
			},
			goodbye: {
				enabled: false,
				channel_id: '',
				content: '**{user.name}** just left. We are now {count} members.',
				ping_user: false,
				embeds: [],
				components: [],
				actions: [],
				card: { enabled: false, layout: templateLayout('midnight') },
				dm: { enabled: false, attach_card: false, content: '', embeds: [], components: [], actions: [] }
			}
		};
	}

	// Strip welcome's per-embed `enabled` flag; MessageEditor's EmbedSpec has the
	// same field names otherwise, so the rest passes straight through.
	function toSpecEmbed(e: Embed): Record<string, unknown> {
		// Keep every field, `enabled` included: EmbedBuilder treats the embed as
		// opaque and spreads it on each edit, so the flag rides along untouched and
		// fromState can preserve a stored disabled embed instead of forcing it on.
		return { ...e };
	}
	function toSavedEmbed(e: Record<string, unknown>): Embed {
		const f = (e.fields as Field[]) ?? [];
		return {
			enabled: (e.enabled as boolean) ?? true,
			color: (e.color as string) ?? '',
			author_name: (e.author_name as string) ?? '',
			author_icon: (e.author_icon as string) ?? '',
			title: (e.title as string) ?? '',
			url: (e.url as string) ?? '',
			description: (e.description as string) ?? '',
			fields: Array.isArray(f)
				? f.map((x) => ({ name: x.name ?? '', value: x.value ?? '', inline: !!x.inline }))
				: [],
			thumbnail: (e.thumbnail as string) ?? '',
			image_url: (e.image_url as string) ?? '',
			footer_text: (e.footer_text as string) ?? '',
			footer_icon: (e.footer_icon as string) ?? '',
			timestamp: !!e.timestamp
		};
	}

	function toState(id: string, m: SavedMsg): MsgState {
		const spec: Record<string, unknown> = { content: m.content ?? '' };
		if (m.embeds?.length) spec.embeds = m.embeds.map(toSpecEmbed);
		if (m.components?.length) spec.components = m.components;
		// undefined allowed_mentions = members ping (the safe default). Only encode
		// the suppressed case, matching MessageEditor's convention.
		if (m.ping_user === false) spec.allowed_mentions = { users: false, roles: false, everyone: false };
		const dmSpec: Record<string, unknown> = { content: m.dm?.content ?? '' };
		if (m.dm?.embeds?.length) dmSpec.embeds = m.dm.embeds.map(toSpecEmbed);
		if (m.dm?.components?.length) dmSpec.components = m.dm.components;
		return {
			enabled: m.enabled,
			channel_id: m.channel_id ?? '',
			step: { id: `${id}-msg`, kind: 'send_message', spec },
			actions: m.actions ?? [],
			card: { enabled: m.card?.enabled ?? false, layout: m.card?.layout },
			dm: {
				enabled: m.dm?.enabled ?? false,
				attach_card: m.dm?.attach_card ?? false,
				step: { id: `${id}-dm`, kind: 'send_dm', spec: dmSpec },
				actions: m.dm?.actions ?? []
			}
		};
	}
	function fromState(st: MsgState): SavedMsg {
		const spec = (st.step.spec ?? {}) as Record<string, unknown>;
		const am = spec.allowed_mentions as { users?: boolean } | undefined;
		const dmSpec = (st.dm.step.spec ?? {}) as Record<string, unknown>;
		return {
			enabled: st.enabled,
			channel_id: st.channel_id,
			content: (spec.content as string) ?? '',
			ping_user: am ? am.users !== false : true,
			embeds: ((spec.embeds as Record<string, unknown>[]) ?? []).map(toSavedEmbed),
			components: (spec.components as CompRow[]) ?? [],
			actions: st.actions ?? [],
			card: { enabled: st.card.enabled, layout: st.card.layout },
			dm: {
				enabled: st.dm.enabled,
				attach_card: st.dm.attach_card,
				content: (dmSpec.content as string) ?? '',
				embeds: ((dmSpec.embeds as Record<string, unknown>[]) ?? []).map(toSavedEmbed),
				components: (dmSpec.components as CompRow[]) ?? [],
				actions: st.dm.actions ?? []
			}
		};
	}

	function mergeMsg(d: SavedMsg, c?: Partial<SavedMsg>): SavedMsg {
		if (!c) return d;
		return {
			...d,
			...c,
			embeds: c.embeds ?? d.embeds,
			components: c.components ?? d.components,
			actions: c.actions ?? d.actions,
			card: c.card ? { enabled: c.card.enabled ?? d.card.enabled, layout: c.card.layout } : d.card,
			dm: c.dm
				? {
						...d.dm,
						...c.dm,
						embeds: c.dm.embeds ?? d.dm.embeds,
						components: c.dm.components ?? d.dm.components,
						actions: c.dm.actions ?? d.dm.actions
					}
				: d.dm
		};
	}

	let enabled = $state(false);
	let cfg = $state<CfgState>({ welcome: toState('w', savedDefaults().welcome), goodbye: toState('g', savedDefaults().goodbye) });
	let tab = $state<'welcome' | 'goodbye'>('welcome');
	let loaded = $state(false);
	let testing = $state(false);
	let testMsg = $state('');
	let baseline = $state('');
	let previewUrl = $state('');
	let studioOpen = $state(false);
	let studioLayout = $state<Layout>(); // local seed for the modal; only committed on Apply

	let savePhase = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
	let saveErr = $state('');

	function serialize() {
		return JSON.stringify({ enabled, cfg });
	}
	const dirty = $derived(loaded && serialize() !== baseline);

	// The two triggers this built-in automation listens on (member_join / member_leave).
	const TRIGGERS = {
		welcome: { label: 'Member joins', key: 'member_join', verb: 'joins', icon: UserPlus, builtin: 'welcome.join' },
		goodbye: { label: 'Member leaves', key: 'member_leave', verb: 'leaves', icon: UserMinus, builtin: 'welcome.leave' }
	} as const;
	const trigger = $derived(TRIGGERS[tab]);

	// Aspect ratio of the active card, so the inline loading placeholder matches
	// the real canvas size (defaults handled by MessageEditor when undefined).
	const cardAspect = $derived.by(() => {
		const l = cfg[tab].card.layout;
		return l ? `${l.width}/${l.height}` : undefined;
	});

	onMount(async () => {
		const f = await api.feature(store.id, FEATURE);
		const d = savedDefaults();
		const c = (f.config ?? {}) as Partial<SavedCfg>;
		cfg = {
			welcome: toState('w', mergeMsg(d.welcome, c.welcome)),
			goodbye: toState('g', mergeMsg(d.goodbye, c.goodbye))
		};
		enabled = f.enabled;
		loaded = true;
		baseline = serialize();
	});

	function openStudio() {
		// Seed the modal from a local copy; commit to cfg only on Apply so opening
		// (then cancelling) the studio never dirties the page.
		studioLayout = cfg[tab].card.layout ?? templateLayout(tab === 'welcome' ? 'aurora' : 'midnight');
		studioOpen = true;
	}
	// Add / remove the card straight from the artifact. Turning it on just opens
	// the studio; card.enabled is committed on Apply (see onApply below) so
	// opening then cancelling never leaves an enabled card with no design.
	function toggleCard(on: boolean) {
		if (on) {
			openStudio();
		} else {
			cfg[tab].card.enabled = false;
		}
	}

	// Live card preview (debounced), rendered through the real layout engine.
	// previewUrl feeds the inline card slot in the message bubble. We mirror the
	// live blob in a non-reactive var (liveUrl) so revoking it never re-triggers
	// this effect, and a sequence token discards superseded / out-of-order renders
	// (fast edits, trigger switches, toggling the card off mid-request).
	let timer: ReturnType<typeof setTimeout>;
	let liveUrl = '';
	let previewSeq = 0;
	let previewTab: 'welcome' | 'goodbye' | '' = '';

	function setPreview(url: string) {
		if (liveUrl && liveUrl !== url) URL.revokeObjectURL(liveUrl);
		liveUrl = url;
		previewUrl = url;
	}

	$effect(() => {
		const t = tab;
		const card = cfg[t].card;
		const json = card.enabled && card.layout ? JSON.stringify(card.layout) : '';

		clearTimeout(timer);
		const seq = ++previewSeq;
		const tabChanged = t !== previewTab;
		previewTab = t;

		if (!loaded || !json) {
			setPreview('');
			return;
		}
		// On a real trigger switch, drop the other tab's card at once so the slot
		// shows the loading state rather than the wrong image during the round-trip.
		if (tabChanged) setPreview('');

		timer = setTimeout(async () => {
			try {
				const url = await layoutPreview(store.id, JSON.parse(json));
				if (seq !== previewSeq) {
					URL.revokeObjectURL(url); // superseded by a newer edit / tab / toggle
					return;
				}
				setPreview(url);
			} catch {
				/* best-effort */
			}
		}, 350);
	});

	onDestroy(() => {
		clearTimeout(timer);
		if (liveUrl) URL.revokeObjectURL(liveUrl);
	});

	async function save() {
		if (savePhase === 'saving' || !dirty) return;
		savePhase = 'saving';
		saveErr = '';
		try {
			const saved: SavedCfg = { welcome: fromState(cfg.welcome), goodbye: fromState(cfg.goodbye) };
			await api.saveFeature(store.id, FEATURE, enabled, saved);
			if (store.detail)
				store.detail.features[FEATURE] = { enabled, config: saved as unknown as Record<string, unknown> };
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
		savePhase = 'idle';
		saveErr = '';
	}
	async function sendTest() {
		if (testing) return;
		testing = true;
		testMsg = '';
		try {
			await api.welcomeTest(store.id, tab);
			testMsg = 'Sent';
		} catch (e) {
			testMsg = e instanceof Error ? e.message : 'Failed';
		} finally {
			testing = false;
			setTimeout(() => (testMsg = ''), 4000);
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
			e.preventDefault();
			if (dirty) save();
		}
	}

	// Unsaved-changes guard. In-app navigation is intercepted while dirty and routed
	// through the confirm dialog (Save and leave / Discard / Keep editing); hard
	// closes get the browser's native prompt. While the Card Studio is open it owns
	// its own guard, so we defer to it and let its confirmed leave through.
	let leaveOpen = $state(false);
	let pendingUrl: URL | null = null;
	let bypassNav = false;
	beforeNavigate((nav) => {
		if (bypassNav || nav.type === 'leave' || studioOpen) return;
		if (!dirty || !nav.to) return;
		nav.cancel();
		pendingUrl = nav.to.url;
		leaveOpen = true;
	});
	function keepEditing() {
		pendingUrl = null;
	}
	function discardAndLeave() {
		const url = pendingUrl;
		pendingUrl = null;
		bypassNav = true;
		if (url) goto(url);
	}
	async function saveAndLeave() {
		await save();
		if (savePhase !== 'error') discardAndLeave();
	}
	function onBeforeUnload(e: BeforeUnloadEvent) {
		if (dirty) {
			e.preventDefault();
			e.returnValue = ''; // shows the browser's native "leave site?" prompt
		}
	}

	const tabs = [
		{ k: 'welcome', label: 'Member joins' },
		{ k: 'goodbye', label: 'Member leaves' }
	] as const;
	// Underline subtab entries: icon + label from TRIGGERS, with an on/off pip so
	// you can see at a glance which trigger is live.
	const subTabs = $derived(
		tabs.map((t) => ({
			key: t.k,
			label: TRIGGERS[t.k].label,
			icon: TRIGGERS[t.k].icon,
			dot: cfg[t.k].enabled,
			dotOff: !cfg[t.k].enabled
		}))
	);
</script>

<svelte:head><title>Welcome · {store.name} · Dia</title></svelte:head>
<svelte:window onkeydown={onKeydown} onbeforeunload={onBeforeUnload} />

<div class="relative flex h-full flex-col bg-bg text-ink">
	<!-- ── Slab topbar ──────────────────────────────────────────────────── -->
	<PageTopbar eyebrow="Welcome" subtitle="Greet members the moment they join, and bid them farewell when they leave.">
		{#snippet leading()}
			<div class="grid size-6 place-items-center rounded border border-line bg-surface text-muted">
				<ImageIcon size={13} />
			</div>
		{/snippet}
		{#snippet actions()}
			<a
				href={`${base}/automations/${trigger.builtin}`}
				class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line px-2.5 text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink"
				title="Advanced: see this flow and wire button click actions"
			>
				<Zap size={13} /> <span class="hidden sm:inline">Advanced</span>
				<ExternalLink size={11} class="text-faint" />
			</a>
			<label class="ml-1 flex items-center gap-2 text-[12px]">
				<span class="hidden text-muted sm:inline">{enabled ? 'On' : 'Off'}</span>
				<Toggle bind:checked={enabled} label="Welcome system" />
			</label>
		{/snippet}
	</PageTopbar>

	<!-- ── Trigger switch: shared underline subtab strip (matches the safety pages) ── -->
	<SubTabs tabs={subTabs} bind:active={tab}>
		{#snippet actions()}
			{#if testMsg}
				<span class="text-[11.5px] {testMsg === 'Sent' ? 'text-success' : 'text-danger'}">{testMsg}</span>
			{/if}
			<button
				type="button"
				onclick={sendTest}
				disabled={testing}
				class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line-strong px-2.5 text-[12px] font-medium text-ink transition-colors hover:bg-surface disabled:opacity-50"
			>
				<Send size={13} /> {testing ? 'Sending…' : 'Send test'}
			</button>
		{/snippet}
	</SubTabs>

	<!-- ── Body: one live message you edit in place ─────────────────────── -->
	<div class="relative min-h-0 flex-1 overflow-y-auto bg-bg pb-20">
		{#if !loaded}
			<div class="p-6">
				<div class="skeleton mb-3 h-6 w-40 rounded"></div>
				<div class="skeleton h-72 w-full rounded"></div>
			</div>
		{:else}
			{@const TIcon = trigger.icon}
			<TabSwipe key={tab} index={tabs.findIndex((t) => t.k === tab)}>
				<div class="grid border-b border-line/60 lg:grid-cols-2 lg:divide-x lg:divide-line/60">
					<!-- ── Left column: Delivery (trigger · channel · DM) ─── -->
					<div class="min-w-0">
						<SectionBar label="Delivery" />
						<div class="px-5 py-5">
							{#if !enabled}
								<div class="mb-4 flex items-center gap-2 border-b border-line/60 pb-4 text-[12px] text-muted">
									<span class="size-1.5 shrink-0 rounded-full bg-faint/40"></span>
									The welcome system is off. Turn it on, top-right, to send anything.
								</div>
							{/if}

							<!-- Per-trigger enable + what fires this, a flat hairline row (no rose box). -->
							<div class="flex items-center justify-between gap-3 border-b border-line/60 pb-4">
								<div class="flex min-w-0 items-center gap-2.5">
									<span class="grid size-6 shrink-0 place-items-center rounded border border-line bg-surface text-muted">
										<TIcon size={13} />
									</span>
									<div class="min-w-0">
										<div class="flex items-center gap-1.5">
											<span class="font-mono text-[10px] uppercase tracking-[0.14em] text-faint">When</span>
											<span class="font-mono text-[9.5px] text-faint">{trigger.key}</span>
										</div>
										<div class="truncate text-[12.5px] font-medium text-ink">A member {trigger.verb}, send this</div>
									</div>
								</div>
								<label class="flex shrink-0 items-center gap-2 text-[12px]">
									<span class="hidden text-muted sm:inline">{cfg[tab].enabled ? 'On' : 'Off'}</span>
									<Toggle bind:checked={cfg[tab].enabled} label="Send on {trigger.key}" />
								</label>
							</div>

							<div class="transition-opacity {cfg[tab].enabled ? '' : 'opacity-60'}">
								<!-- Channel: the message's destination -->
								<div class="mt-4 flex flex-wrap items-center gap-2 text-[12.5px] text-muted">
									<Hash size={14} class="text-faint" />
									<span>Posts in</span>
									<div class="min-w-[200px] max-w-xs flex-1"><ChannelSelect bind:value={cfg[tab].channel_id} placeholder="Channel to post the welcome in" /></div>
								</div>

								<!-- DM: a second, private message to the member -->
								<div class="mt-5 border-t border-line/60 pt-5">
									<div class="mb-2 flex items-center justify-between gap-3">
										<div class="flex min-w-0 items-center gap-2">
											<Mail size={14} class="text-faint" />
											<span class="text-[12.5px] font-medium text-ink">Private DM</span>
											<span class="hidden truncate text-[11.5px] text-muted sm:inline">also message the member directly</span>
										</div>
										<Toggle bind:checked={cfg[tab].dm.enabled} label="Send a DM" />
									</div>
									{#if cfg[tab].dm.enabled}
										<!-- The DM can attach the same card image the channel message
										     renders; there is no separate DM card design. -->
										<MessageEditor
											step={cfg[tab].dm.step}
											embeds
											components
											clickPaths={false}
											card
											cardEnabled={cfg[tab].dm.attach_card && cfg[tab].card.enabled && !!cfg[tab].card.layout}
											cardUrl={previewUrl}
											cardAspect={cardAspect}
											onCardToggle={(on) => (cfg[tab].dm.attach_card = on)}
											onCardEdit={openStudio}
										/>
										<p class="mt-1.5 text-[11px] text-faint">Uses the same card as the channel message.</p>
									{:else}
										<button
											type="button"
											onclick={() => (cfg[tab].dm.enabled = true)}
											class="flex w-full items-center justify-center gap-2 rounded-lg border border-dashed border-line px-4 py-4 text-[12.5px] font-medium text-faint transition-colors hover:border-line-strong hover:text-muted"
										>
											<Mail size={14} /> Add a private DM
										</button>
									{/if}
								</div>
							</div>
						</div>
					</div>

					<!-- ── Right column: Message ─────────────────────────── -->
					<div class="min-w-0">
						<SectionBar label="Message" />
						<div class="px-5 py-5">
							<div class="transition-opacity {cfg[tab].enabled ? '' : 'opacity-60'}">
								<!-- The message itself: content, embeds, the card image, buttons /
								     selects, all edited right on the Discord surface. -->
								<MessageEditor
									step={cfg[tab].step}
									embeds
									components
									clickPaths={false}
									card
									cardEnabled={cfg[tab].card.enabled && !!cfg[tab].card.layout}
									cardUrl={previewUrl}
									cardAspect={cardAspect}
									onCardToggle={toggleCard}
									onCardEdit={openStudio}
								/>
							</div>
						</div>
					</div>
				</div>
			</TabSwipe>
		{/if}
	</div>

	<!-- Release dock — the saving experience -->
	{#if loaded}
		<ReleaseDock {dirty} phase={savePhase} error={saveErr} onsave={save} onreset={reset} />
	{/if}

	{#if studioOpen && studioLayout}
		<CardStudioModal
			layout={studioLayout}
			guildId={store.id}
			context="welcome"
			onApply={(l) => {
				cfg[tab].card.layout = l;
				cfg[tab].card.enabled = true;
			}}
			onClose={() => (studioOpen = false)}
		/>
	{/if}
</div>

<ConfirmDialog
	bind:open={leaveOpen}
	title="Unsaved changes"
	description="You have changes on this page that haven't been saved yet. What would you like to do?"
	cancelLabel="Keep editing"
	discardLabel="Discard"
	confirmLabel="Save and leave"
	oncancel={keepEditing}
	ondiscard={discardAndLeave}
	onconfirm={saveAndLeave}
/>
