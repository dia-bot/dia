<script lang="ts">
	import { onMount, onDestroy, getContext } from 'svelte';
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api, layoutPreview } from '$lib/api';
	import type { Layout } from '$lib/layout/schema';
	import { templateLayout } from '$lib/layout/templates';
	import Toggle from '$lib/components/Toggle.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import DiscordMessagePreview from '$lib/components/dashboard/DiscordMessagePreview.svelte';
	import EmbedEditor from '$lib/components/dashboard/EmbedEditor.svelte';
	import CardStudioModal from '$lib/components/editor/CardStudioModal.svelte';
	import TemplateField from '$lib/components/TemplateField.svelte';
	import {
		Plus,
		Send,
		Wand2,
		MessageSquare,
		Mail,
		Image as ImageIcon,
		Frame,
		Layers,
		UserPlus,
		UserMinus,
		Zap,
		ExternalLink,
		X,
		Check,
		Loader2,
		CircleAlert
	} from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'welcome';
	const base = $derived(`/servers/${store.id}`);

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
	type Msg = {
		enabled: boolean;
		channel_id: string;
		content: string;
		ping_user: boolean;
		embeds: Embed[];
		card: Card;
		dm: { enabled: boolean; content: string };
	};
	type Cfg = { welcome: Msg; goodbye: Msg };

	function emptyEmbed(color: string): Embed {
		return {
			enabled: true,
			color,
			author_name: '',
			author_icon: '',
			title: '',
			url: '',
			description: '',
			fields: [],
			thumbnail: '',
			image_url: '',
			footer_text: '',
			footer_icon: '',
			timestamp: false
		};
	}
	function defaults(): Cfg {
		return {
			welcome: {
				enabled: true,
				channel_id: '',
				content: 'Hey {user.mention}, welcome to **{server}**! 🎉',
				ping_user: true,
				embeds: [],
				card: { enabled: true, layout: templateLayout('aurora') },
				dm: { enabled: false, content: '' }
			},
			goodbye: {
				enabled: false,
				channel_id: '',
				content: '**{user.name}** just left. We are now {count} members.',
				ping_user: false,
				embeds: [],
				card: { enabled: false, layout: templateLayout('midnight') },
				dm: { enabled: false, content: '' }
			}
		};
	}

	let enabled = $state(false);
	let cfg = $state<Cfg>(defaults());
	let tab = $state<'welcome' | 'goodbye'>('welcome');
	let loaded = $state(false);
	let testing = $state(false);
	let testMsg = $state('');
	let baseline = $state('');
	let variables = $state<{ token: string; desc: string }[]>([]);
	let previewUrl = $state('');
	let studioOpen = $state(false);

	// Which step node is open in the inspector drawer (null = nothing selected).
	let selected = $state<string | null>(null);

	// Save lifecycle for the release dock.
	let savePhase = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
	let saveErr = $state('');

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	const TRIGGERS = {
		welcome: { label: 'Member joins', key: 'member_join', verb: 'joins', icon: UserPlus, builtin: 'welcome.join' },
		goodbye: { label: 'Member leaves', key: 'member_leave', verb: 'leaves', icon: UserMinus, builtin: 'welcome.leave' }
	} as const;
	const trigger = $derived(TRIGGERS[tab]);

	const steps = $derived([
		{ key: 'message', icon: MessageSquare, kind: 'send_message', title: 'Post a message', summary: cfg[tab].content?.trim() || 'No message text', on: cfg[tab].enabled },
		{ key: 'embeds', icon: Layers, kind: 'embed', title: 'Embeds', summary: cfg[tab].embeds.length ? `${cfg[tab].embeds.length} embed${cfg[tab].embeds.length > 1 ? 's' : ''}` : 'No embeds', on: cfg[tab].embeds.length > 0 },
		{ key: 'card', icon: Frame, kind: 'image.render', title: 'Card image', summary: cfg[tab].card.enabled ? 'Card attached' : 'No card', on: cfg[tab].card.enabled },
		{ key: 'dm', icon: Mail, kind: 'send_dm', title: 'Direct message', summary: cfg[tab].dm.enabled ? cfg[tab].dm.content?.trim() || 'Empty message' : 'No DM', on: cfg[tab].dm.enabled }
	]);
	const sel = $derived(steps.find((s) => s.key === selected));

	const fallbackVars = [
		{ token: '{user}', desc: "Member's display name" },
		{ token: '{user.mention}', desc: 'Pings the member' },
		{ token: '{user.name}', desc: 'Username' },
		{ token: '{user.id}', desc: 'Member ID' },
		{ token: '{user.avatar}', desc: 'Avatar image URL' },
		{ token: '{server}', desc: 'Server name' },
		{ token: '{count}', desc: 'Member count' },
		{ token: '{count.ordinal}', desc: 'Member count, like 1,024th' }
	];

	function mergeMsg(d: Msg, c?: Partial<Msg>): Msg {
		if (!c) return d;
		return {
			...d,
			...c,
			embeds: c.embeds ?? d.embeds,
			card: c.card ? { enabled: c.card.enabled ?? d.card.enabled, layout: c.card.layout } : d.card,
			dm: { ...d.dm, ...(c.dm ?? {}) }
		};
	}

	onMount(async () => {
		const [f, v] = await Promise.all([
			api.feature(store.id, FEATURE),
			api.welcomeVariables(store.id).catch(() => ({ variables: [] }))
		]);
		const d = defaults();
		const c = (f.config ?? {}) as Partial<Cfg>;
		cfg = { welcome: mergeMsg(d.welcome, c.welcome), goodbye: mergeMsg(d.goodbye, c.goodbye) };
		enabled = f.enabled;
		variables = v.variables?.length ? v.variables : fallbackVars;
		loaded = true;
		baseline = JSON.stringify({ enabled, cfg });
	});

	function selectStep(key: string) {
		selected = selected === key ? null : key;
	}

	function addEmbed() {
		if (cfg[tab].embeds.length >= 10) return;
		cfg[tab].embeds = [...cfg[tab].embeds, emptyEmbed('#5865F2')];
	}
	function removeEmbed(i: number) {
		cfg[tab].embeds = cfg[tab].embeds.filter((_, idx) => idx !== i);
	}

	function openStudio() {
		if (!cfg[tab].card.layout) cfg[tab].card.layout = templateLayout('aurora');
		studioOpen = true;
	}

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
			await api.saveFeature(store.id, FEATURE, enabled, cfg);
			if (store.detail)
				store.detail.features[FEATURE] = { enabled, config: cfg as unknown as Record<string, unknown> };
			baseline = JSON.stringify({ enabled, cfg });
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
		if (e.key === 'Escape' && selected && !e.defaultPrevented) {
			selected = null;
			return;
		}
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
			e.preventDefault();
			if (dirty) save();
		}
	}

	const tabs = [
		{ k: 'welcome', label: 'Member joins' },
		{ k: 'goodbye', label: 'Member leaves' }
	] as const;

	const dockVisible = $derived(dirty || savePhase !== 'idle');
</script>

<svelte:head><title>Welcome · {store.name} · Dia</title></svelte:head>
<svelte:window onkeydown={onKeydown} />

<div class="flex h-full flex-col bg-bg text-ink">
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

	<div class="flex min-h-10 shrink-0 flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-line/60 bg-bg px-5 py-1.5 md:flex-nowrap">
		<span class="hidden font-mono text-[10px] uppercase tracking-[0.14em] text-faint sm:inline">Trigger</span>
		<div class="flex items-center gap-1 rounded-lg border border-line bg-ink-2 p-0.5">
			{#each tabs as t (t.k)}
				{@const Icon = TRIGGERS[t.k].icon}
				<button
					type="button"
					onclick={() => {
						tab = t.k;
						selected = null;
					}}
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

	<div class="relative min-h-0 flex-1 overflow-hidden bg-bg">
		<div
			class="h-full overflow-y-auto"
			style="background-image: radial-gradient(rgba(255,255,255,0.045) 1px, transparent 1px); background-size: 22px 22px;"
		>
			{#if !loaded}
				<div class="mx-auto flex max-w-5xl flex-col gap-10 px-6 py-12 lg:flex-row lg:justify-center lg:gap-14">
					<div class="w-full space-y-3 lg:w-[320px]">
						<div class="skeleton h-14 w-full rounded-xl"></div>
						<div class="skeleton h-20 w-full rounded-xl"></div>
						<div class="skeleton h-20 w-full rounded-xl"></div>
						<div class="skeleton h-20 w-full rounded-xl"></div>
					</div>
					<div class="skeleton h-72 w-full rounded-xl lg:max-w-[440px]"></div>
				</div>
			{:else}
				{@const TIcon = trigger.icon}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="mx-auto flex min-h-full max-w-5xl flex-col gap-10 px-6 py-12 lg:flex-row lg:items-start lg:justify-center lg:gap-14 {selected
						? 'xl:pr-[24rem]'
						: ''}"
				>
					<div class="mx-auto w-full max-w-sm lg:mx-0 lg:w-[320px] lg:shrink-0">
						{#if !enabled}
							<div class="mb-3 flex items-center gap-2 rounded-lg border border-line bg-ink-2 px-3 py-2 text-[12px] text-muted">
								<span class="size-1.5 shrink-0 rounded-full bg-faint/40"></span>
								System is off — turn it on, top-right.
							</div>
						{/if}

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

						<div class="transition-opacity {cfg[tab].enabled ? '' : 'opacity-60'}">
							{#each steps as s (s.key)}
								{@const Icon = s.icon}
								<div class="ml-[26px] h-4 w-px bg-line-strong/70"></div>
								<button
									type="button"
									onclick={() => selectStep(s.key)}
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

						<p class="mt-4 text-center font-mono text-[10.5px] text-faint">Click a step to edit it.</p>
					</div>

					<div class="mx-auto w-full max-w-md lg:mx-0 lg:sticky lg:top-2 lg:max-w-[440px]">
						<div class="mb-2.5 flex items-center justify-between">
							<span class="eyebrow">Live preview</span>
							<span class="font-mono text-[10.5px] text-faint">what members see</span>
						</div>
						<div class="overflow-hidden rounded-xl border border-line">
							<DiscordMessagePreview
								content={cfg[tab].content}
								embeds={cfg[tab].embeds}
								cardEnabled={cfg[tab].card.enabled && !!cfg[tab].card.layout}
								cardUrl={previewUrl}
								serverName={store.name}
								serverId={store.id}
							/>
						</div>
					</div>
				</div>
			{/if}
		</div>

		{#if loaded && selected && sel}
			{@const SIcon = sel.icon}
			<div
				class="absolute inset-y-0 right-0 z-20 flex w-full flex-col border-l border-line bg-bg shadow-[0_0_48px_-12px_rgba(0,0,0,0.7)] {selected ===
				'embeds'
					? 'max-w-md md:max-w-xl'
					: 'max-w-sm md:max-w-md'}"
				transition:fly={{ x: 24, duration: 200, easing: cubicOut }}
			>
				<div class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-3">
					<span class="grid size-6 place-items-center rounded-md bg-foreground/[0.07] text-foreground/80 ring-1 ring-border/70">
						<SIcon size={13} />
					</span>
					<div class="min-w-0 flex-1 truncate text-[13px] font-semibold text-ink">{sel.title}</div>
					<span class="inline-flex items-center rounded border border-line bg-surface/40 px-1.5 py-0.5 font-mono text-[9.5px] uppercase tracking-[0.1em] text-muted">{sel.kind}</span>
					<button
						type="button"
						onclick={() => (selected = null)}
						class="grid size-7 place-items-center rounded text-muted transition-colors hover:bg-surface hover:text-ink"
						aria-label="Close"
					>
						<X size={15} />
					</button>
				</div>

				<div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
					{#if selected === 'message'}
						<div>
							<div class="label">Channel</div>
							<ChannelSelect bind:value={cfg[tab].channel_id} />
						</div>
						<TemplateField label="Message" placeholder="Plain message text…" guildId={store.id} {variables} bind:value={cfg[tab].content} />
						<label class="flex items-center justify-between gap-4">
							<div>
								<div class="text-[13px] font-medium text-ink">Mention the member</div>
								<p class="text-[12px] text-muted">Pings them so it shows as a notification.</p>
							</div>
							<Toggle bind:checked={cfg[tab].ping_user} />
						</label>
					{:else if selected === 'embeds'}
						<div class="flex items-center justify-between gap-3">
							<p class="text-[12px] text-muted">Rich blocks beneath the message — {cfg[tab].embeds.length}/10.</p>
							<button
								type="button"
								onclick={addEmbed}
								disabled={cfg[tab].embeds.length >= 10}
								class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border border-line-strong px-2.5 text-[12px] font-medium text-ink transition-colors hover:bg-ink-2 disabled:opacity-40"
							>
								<Plus size={13} /> Add embed
							</button>
						</div>
						{#if cfg[tab].embeds.length}
							<div class="space-y-3">
								{#each cfg[tab].embeds as _e, i (i)}
									<EmbedEditor embed={cfg[tab].embeds[i]} index={i} onRemove={() => removeEmbed(i)} />
								{/each}
							</div>
						{:else}
							<div class="rounded-lg border border-dashed border-line px-4 py-8 text-center text-[12px] text-faint">
								No embeds yet. Add one to attach a rich block.
							</div>
						{/if}
					{:else if selected === 'card'}
						<label class="flex items-center justify-between gap-4">
							<div>
								<div class="text-[13px] font-medium text-ink">Attach a card image</div>
								<p class="text-[12px] text-muted">A rendered welcome card, designed in Card Studio.</p>
							</div>
							<Toggle bind:checked={cfg[tab].card.enabled} />
						</label>
						{#if cfg[tab].card.enabled}
							<div class="overflow-hidden rounded-xl border border-line bg-ink-2">
								{#if previewUrl}
									<img src={previewUrl} alt="Welcome card preview" class="block w-full" />
								{:else}
									<div class="grid aspect-[1024/450] place-items-center text-faint">
										<Loader2 size={18} class="animate-spin" />
									</div>
								{/if}
							</div>
							<button
								type="button"
								onclick={openStudio}
								class="flex w-full items-center justify-center gap-2 rounded-lg border border-line-strong bg-ink-2 py-2.5 text-[13px] font-medium text-ink transition-colors hover:bg-surface"
							>
								<Wand2 size={15} class="text-accent-ink" /> Edit image
							</button>
						{/if}
					{:else if selected === 'dm'}
						<label class="flex items-center justify-between gap-4">
							<div>
								<div class="text-[13px] font-medium text-ink">Send a direct message</div>
								<p class="text-[12px] text-muted">Also send the member a private DM.</p>
							</div>
							<Toggle bind:checked={cfg[tab].dm.enabled} />
						</label>
						{#if cfg[tab].dm.enabled}
							<TemplateField label="DM text" placeholder="DM text…" guildId={store.id} {variables} rows={3} bind:value={cfg[tab].dm.content} />
						{/if}
					{/if}
				</div>
			</div>
		{/if}

		{#if loaded && dockVisible}
			<div class="pointer-events-none absolute inset-x-4 bottom-4 z-40 flex justify-center {selected ? 'md:pr-[24rem] xl:pr-[28rem]' : ''}" transition:fly={{ y: 14, duration: 180, easing: cubicOut }}>
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
