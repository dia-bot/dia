<script lang="ts">
	import { onMount, onDestroy, getContext, setContext } from 'svelte';
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
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
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import CardStudioModal from '$lib/components/editor/CardStudioModal.svelte';
	import {
		Send,
		Wand2,
		MessageSquare,
		Mail,
		Image as ImageIcon,
		Frame,
		UserPlus,
		UserMinus,
		Zap,
		ExternalLink,
		Loader2,
		Check,
		CircleAlert
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
	type SavedMsg = {
		enabled: boolean;
		channel_id: string;
		content: string;
		ping_user: boolean;
		embeds: Embed[];
		card: Card;
		dm: { enabled: boolean; content: string };
	};
	type SavedCfg = { welcome: SavedMsg; goodbye: SavedMsg };

	// ── In-memory editor shape — the message + dm are real `send_message` /
	// `send_dm` Steps so the slash-command MessageEditor can edit them in place.
	type MsgState = {
		enabled: boolean;
		channel_id: string;
		step: Step;
		card: Card;
		dm: { enabled: boolean; step: Step };
	};
	type CfgState = { welcome: MsgState; goodbye: MsgState };

	function savedDefaults(): SavedCfg {
		return {
			welcome: {
				enabled: true,
				channel_id: '',
				content: 'Hey {{ .User.Mention }}, welcome to **{{ .Guild.Name }}**! 🎉',
				ping_user: true,
				embeds: [],
				card: { enabled: true, layout: templateLayout('aurora') },
				dm: { enabled: false, content: '' }
			},
			goodbye: {
				enabled: false,
				channel_id: '',
				content: '**{{ .User.GlobalName }}** just left. We are now {{ .Count }} members.',
				ping_user: false,
				embeds: [],
				card: { enabled: false, layout: templateLayout('midnight') },
				dm: { enabled: false, content: '' }
			}
		};
	}

	// Strip welcome's per-embed `enabled` flag; MessageEditor's EmbedSpec has the
	// same field names otherwise, so the rest passes straight through.
	function toSpecEmbed(e: Embed): Record<string, unknown> {
		const { enabled: _enabled, ...rest } = e;
		return rest;
	}
	function toSavedEmbed(e: Record<string, unknown>): Embed {
		const f = (e.fields as Field[]) ?? [];
		return {
			enabled: true,
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
		// undefined allowed_mentions = members ping (the safe default). Only encode
		// the suppressed case, matching MessageEditor's convention.
		if (m.ping_user === false) spec.allowed_mentions = { users: false, roles: false, everyone: false };
		return {
			enabled: m.enabled,
			channel_id: m.channel_id ?? '',
			step: { id: `${id}-msg`, kind: 'send_message', spec },
			card: { enabled: m.card?.enabled ?? false, layout: m.card?.layout },
			dm: { enabled: m.dm?.enabled ?? false, step: { id: `${id}-dm`, kind: 'send_dm', spec: { content: m.dm?.content ?? '' } } }
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
			card: { enabled: st.card.enabled, layout: st.card.layout },
			dm: { enabled: st.dm.enabled, content: (dmSpec.content as string) ?? '' }
		};
	}

	function mergeMsg(d: SavedMsg, c?: Partial<SavedMsg>): SavedMsg {
		if (!c) return d;
		return {
			...d,
			...c,
			embeds: c.embeds ?? d.embeds,
			card: c.card ? { enabled: c.card.enabled ?? d.card.enabled, layout: c.card.layout } : d.card,
			dm: { ...d.dm, ...(c.dm ?? {}) }
		};
	}

	let enabled = $state(false);
	let cfg = $state<CfgState>({ welcome: toState('w', savedDefaults().welcome), goodbye: toState('g', savedDefaults().goodbye) });
	let tab = $state<'welcome' | 'goodbye'>('welcome');
	let selected = $state<string>('message');
	let loaded = $state(false);
	let testing = $state(false);
	let testMsg = $state('');
	let baseline = $state('');
	let previewUrl = $state('');
	let studioOpen = $state(false);

	let savePhase = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
	let saveErr = $state('');

	function serialize() {
		return JSON.stringify({ enabled, cfg });
	}
	const dirty = $derived(loaded && serialize() !== baseline);
	const dockVisible = $derived(dirty || savePhase !== 'idle');

	// The two triggers this built-in automation listens on (member_join / member_leave).
	const TRIGGERS = {
		welcome: { label: 'Member joins', key: 'member_join', verb: 'joins', icon: UserPlus, builtin: 'welcome.join' },
		goodbye: { label: 'Member leaves', key: 'member_leave', verb: 'leaves', icon: UserMinus, builtin: 'welcome.leave' }
	} as const;
	const trigger = $derived(TRIGGERS[tab]);

	function stepContent(st: Step): string {
		return (((st.spec ?? {}) as Record<string, unknown>).content as string)?.trim() ?? '';
	}

	// Ordered flow steps, as the left-rail nodes. Summaries are live.
	const steps = $derived([
		{ key: 'message', icon: MessageSquare, kind: 'send_message', title: 'Message', summary: stepContent(cfg[tab].step) || 'No message yet', on: cfg[tab].enabled },
		{ key: 'card', icon: Frame, kind: 'image.render', title: 'Card image', summary: cfg[tab].card.enabled ? 'Attached' : 'Off', on: cfg[tab].card.enabled },
		{ key: 'dm', icon: Mail, kind: 'send_dm', title: 'Direct message', summary: cfg[tab].dm.enabled ? stepContent(cfg[tab].dm.step) || 'Empty' : 'Off', on: cfg[tab].dm.enabled }
	]);
	const sel = $derived(steps.find((s) => s.key === selected));

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
		if (!cfg[tab].card.layout) cfg[tab].card.layout = templateLayout('aurora');
		studioOpen = true;
	}

	// live card preview (debounced) — renders the layout through the real engine.
	let timer: ReturnType<typeof setTimeout>;
	$effect(() => {
		const card = cfg[tab].card;
		if (!loaded || !card.enabled || !card.layout) {
			previewUrl = '';
			return;
		}
		const json = JSON.stringify(card.layout);
		clearTimeout(timer);
		timer = setTimeout(async () => {
			try {
				const url = await layoutPreview(store.id, JSON.parse(json));
				if (previewUrl) URL.revokeObjectURL(previewUrl);
				previewUrl = url;
			} catch {
				/* best-effort */
			}
		}, 350);
	});

	onDestroy(() => {
		clearTimeout(timer);
		if (previewUrl) URL.revokeObjectURL(previewUrl);
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

	const tabs = [
		{ k: 'welcome', label: 'Member joins' },
		{ k: 'goodbye', label: 'Member leaves' }
	] as const;
</script>

<svelte:head><title>Welcome · {store.name} · Dia</title></svelte:head>
<svelte:window onkeydown={onKeydown} />

<div class="flex h-full flex-col bg-bg text-ink">
	<!-- ── Slab topbar ──────────────────────────────────────────────────── -->
	<PageTopbar eyebrow="Welcome" subtitle="A built-in automation that greets members and bids them farewell.">
		{#snippet leading()}
			<div class="grid size-6 place-items-center rounded border border-line bg-surface text-accent-ink">
				<ImageIcon size={13} />
			</div>
		{/snippet}
		{#snippet actions()}
			<a
				href={`${base}/automations/${trigger.builtin}`}
				class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line px-2.5 text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink"
				title="See this flow (read-only) in Automations"
			>
				<Zap size={13} /> <span class="hidden sm:inline">View in Automations</span>
				<ExternalLink size={11} class="text-faint" />
			</a>
			<label class="ml-1 flex items-center gap-2 text-[12px]">
				<span class="hidden text-muted sm:inline">{enabled ? 'On' : 'Off'}</span>
				<Toggle bind:checked={enabled} label="Welcome system" />
			</label>
		{/snippet}
	</PageTopbar>

	<!-- ── Trigger switch ───────────────────────────────────────────────── -->
	<div class="flex min-h-10 shrink-0 flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-line/60 bg-bg px-5 py-1.5 md:flex-nowrap">
		<span class="hidden font-mono text-[10px] uppercase tracking-[0.14em] text-faint sm:inline">Trigger</span>
		<div class="flex items-center gap-1 rounded-lg border border-line bg-ink-2 p-0.5">
			{#each tabs as t (t.k)}
				{@const Icon = TRIGGERS[t.k].icon}
				<button
					type="button"
					onclick={() => (tab = t.k)}
					class="flex items-center gap-1.5 rounded-md px-2.5 py-1 text-[12.5px] font-medium transition-colors {tab ===
					t.k
						? 'bg-surface text-ink shadow-[inset_0_1px_0_rgba(255,255,255,0.05)]'
						: 'text-muted hover:text-ink'}"
				>
					<span class="size-1.5 shrink-0 rounded-full {cfg[t.k].enabled ? 'bg-success' : 'bg-faint/40'}" title={cfg[t.k].enabled ? 'Active' : 'Off'}></span>
					<Icon size={14} class={tab === t.k ? 'text-accent-ink' : ''} />
					<span>{TRIGGERS[t.k].label}</span>
				</button>
			{/each}
		</div>

		<div class="ml-auto flex items-center gap-2.5">
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
		</div>
	</div>

	<!-- ── Body: flow rail + full-width editor surface + save dock ──────── -->
	<div class="relative min-h-0 flex-1 overflow-hidden bg-bg">
		{#if !loaded}
			<div class="flex h-full">
				<div class="hidden w-[300px] shrink-0 space-y-3 border-r border-line p-4 md:block">
					<div class="skeleton h-14 w-full rounded-xl"></div>
					<div class="skeleton h-16 w-full rounded-xl"></div>
					<div class="skeleton h-16 w-full rounded-xl"></div>
				</div>
				<div class="flex-1 p-6"><div class="skeleton mx-auto h-80 w-full max-w-2xl rounded-xl"></div></div>
			</div>
		{:else}
			{@const TIcon = trigger.icon}
			<div class="flex h-full flex-col md:flex-row">
				<!-- Flow rail (navigator) -->
				<aside class="shrink-0 overflow-y-auto border-b border-line p-4 md:w-[300px] md:border-b-0 md:border-r">
					{#if !enabled}
						<div class="mb-3 flex items-center gap-2 rounded-lg border border-line bg-ink-2 px-3 py-2 text-[12px] text-muted">
							<span class="size-1.5 shrink-0 rounded-full bg-faint/40"></span>
							System is off — turn it on, top-right.
						</div>
					{/if}

					<!-- Trigger entry node -->
					<div class="rounded-xl border border-accent/25 bg-accent/[0.06] px-3.5 py-2.5">
						<div class="flex items-center gap-2.5">
							<span class="grid size-7 shrink-0 place-items-center rounded-lg border border-accent/30 bg-accent/10 text-accent-ink">
								<TIcon size={15} />
							</span>
							<div class="min-w-0 flex-1">
								<div class="flex items-center gap-1.5">
									<span class="font-mono text-[9px] uppercase tracking-[0.16em] text-accent-ink/80">Trigger</span>
									<span class="font-mono text-[9.5px] text-faint">{trigger.key}</span>
								</div>
								<div class="truncate text-[12.5px] font-medium text-ink">When a member {trigger.verb}</div>
							</div>
							<Toggle bind:checked={cfg[tab].enabled} label="Run this flow" />
						</div>
					</div>

					<!-- Step nodes -->
					<div class="transition-opacity {cfg[tab].enabled ? '' : 'opacity-60'}">
						{#each steps as s (s.key)}
							{@const Icon = s.icon}
							<div class="ml-[26px] h-4 w-px bg-line-strong/70"></div>
							<button
								type="button"
								onclick={() => (selected = s.key)}
								class="block w-full rounded-xl border bg-card text-left transition-all duration-200 {selected ===
								s.key
									? 'border-foreground/40 shadow-[0_0_0_3px_hsl(var(--foreground)/0.08),0_12px_32px_-12px_rgba(0,0,0,0.5)]'
									: 'border-border/60 shadow-[0_1px_2px_rgba(0,0,0,0.3)] hover:border-foreground/25 hover:shadow-[0_4px_16px_-4px_rgba(0,0,0,0.45)]'}"
							>
								<div class="flex items-center gap-2.5 rounded-t-xl border-b border-border/50 bg-gradient-to-r from-foreground/[0.05] to-transparent px-3 py-2">
									<span class="grid size-6 shrink-0 place-items-center rounded-md bg-foreground/[0.07] text-foreground/80 ring-1 ring-border/70">
										<Icon size={13} />
									</span>
									<span class="min-w-0 flex-1 truncate text-[12.5px] font-semibold text-foreground">{s.title}</span>
									<span class="size-1.5 shrink-0 rounded-full {s.on ? 'bg-success' : 'bg-faint/40'}"></span>
								</div>
								<div class="px-3 py-2">
									<div class="font-mono text-[9px] uppercase tracking-[0.12em] text-muted-foreground/60">{s.kind}</div>
									<div class="mt-0.5 truncate text-[11.5px] text-muted-foreground">{s.summary}</div>
								</div>
							</button>
						{/each}
					</div>
				</aside>

				<!-- Editor surface -->
				<div class="min-w-0 flex-1 overflow-y-auto px-6 py-6">
					{#key tab + selected}
						<div class="mx-auto w-full max-w-2xl" in:fly={{ y: 8, duration: 160, easing: cubicOut }}>
							{#if selected === 'message'}
								<header class="mb-4">
									<h2 class="text-[15px] font-semibold text-ink">Message</h2>
									<p class="mt-0.5 text-[12.5px] text-muted">Posted in a channel when a member {trigger.verb}. Edit it right in the preview.</p>
								</header>
								<div class="mb-4 max-w-sm">
									<div class="label">Channel</div>
									<ChannelSelect bind:value={cfg[tab].channel_id} />
								</div>
								<MessageEditor step={cfg[tab].step} embeds />
							{:else if selected === 'card'}
								<header class="mb-4 flex items-start justify-between gap-4">
									<div>
										<h2 class="text-[15px] font-semibold text-ink">Card image</h2>
										<p class="mt-0.5 text-[12.5px] text-muted">A rendered welcome card, attached beneath the message.</p>
									</div>
									<Toggle bind:checked={cfg[tab].card.enabled} />
								</header>
								{#if cfg[tab].card.enabled}
									<div class="overflow-hidden rounded-xl border border-line bg-ink-2">
										{#if previewUrl}
											<img src={previewUrl} alt="Welcome card preview" class="block w-full" />
										{:else}
											<div class="grid aspect-[1024/450] place-items-center text-faint"><Loader2 size={20} class="animate-spin" /></div>
										{/if}
									</div>
									<button
										type="button"
										onclick={openStudio}
										class="mt-4 flex w-full items-center justify-center gap-2 rounded-lg border border-line-strong bg-ink-2 py-2.5 text-[13px] font-medium text-ink transition-colors hover:bg-surface"
									>
										<Wand2 size={15} class="text-accent-ink" /> Edit image
									</button>
								{:else}
									<div class="rounded-xl border border-dashed border-line px-4 py-10 text-center text-[12.5px] text-faint">
										No card image. Turn it on to attach a rendered card.
									</div>
								{/if}
							{:else if selected === 'dm'}
								<header class="mb-4 flex items-start justify-between gap-4">
									<div>
										<h2 class="text-[15px] font-semibold text-ink">Direct message</h2>
										<p class="mt-0.5 text-[12.5px] text-muted">Also send the member a private DM when they {trigger.verb}.</p>
									</div>
									<Toggle bind:checked={cfg[tab].dm.enabled} />
								</header>
								{#if cfg[tab].dm.enabled}
									<MessageEditor step={cfg[tab].dm.step} embeds={false} />
								{:else}
									<div class="rounded-xl border border-dashed border-line px-4 py-10 text-center text-[12.5px] text-faint">
										No DM. Turn it on to greet the member privately.
									</div>
								{/if}
							{/if}
						</div>
					{/key}
				</div>
			</div>
		{/if}

		<!-- Release dock — the saving experience -->
		{#if loaded && dockVisible}
			<div class="pointer-events-none absolute inset-x-4 bottom-4 z-40 flex justify-center" transition:fly={{ y: 14, duration: 180, easing: cubicOut }}>
				<div
					class="pointer-events-auto relative flex h-11 items-center gap-2.5 overflow-hidden rounded-[14px] border bg-surface/95 px-3.5 shadow-[0_16px_40px_-12px_rgba(0,0,0,0.7)] backdrop-blur-md {savePhase ===
					'error'
						? 'dock-shake border-danger/40'
						: 'border-line'}"
				>
					{#if savePhase === 'saving'}
						<span class="dock-beam-sweep pointer-events-none absolute inset-y-0 left-0 w-1/3 bg-gradient-to-r from-transparent via-accent/30 to-transparent"></span>
						<Loader2 size={15} class="animate-spin text-muted" />
						<span class="text-[12.5px] text-muted">Saving…</span>
					{:else if savePhase === 'saved'}
						<span class="grid size-4 place-items-center rounded-full bg-success/15 text-success"><Check size={11} /></span>
						<span class="text-[12.5px] text-ink">Saved</span>
					{:else if savePhase === 'error'}
						<CircleAlert size={15} class="text-danger" />
						<span class="max-w-[16rem] truncate text-[12.5px] text-ink" title={saveErr}>{saveErr || "Couldn't save"}</span>
						<button type="button" onclick={save} class="ml-1 inline-flex h-7 items-center rounded-md bg-ink px-2.5 text-[12px] font-medium text-bg transition-opacity hover:opacity-90">Retry</button>
					{:else}
						<span class="size-1.5 animate-pulse rounded-full bg-accent"></span>
						<span class="text-[12.5px] text-muted">Unsaved changes</span>
						<div class="ml-1 flex items-center gap-1.5">
							<button type="button" onclick={reset} class="inline-flex h-7 items-center rounded-md border border-line-strong px-2.5 text-[12px] font-medium text-muted transition-colors hover:text-ink">Discard</button>
							<button type="button" onclick={save} class="inline-flex h-7 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-medium text-bg transition-opacity hover:opacity-90">
								Save <kbd class="hidden font-mono text-[10px] text-bg/60 sm:inline">⌘S</kbd>
							</button>
						</div>
					{/if}
				</div>
			</div>
		{/if}
	</div>

	{#if studioOpen}
		<CardStudioModal
			layout={cfg[tab].card.layout ?? templateLayout('aurora')}
			guildId={store.id}
			onApply={(l) => (cfg[tab].card.layout = l)}
			onClose={() => (studioOpen = false)}
		/>
	{/if}
</div>
