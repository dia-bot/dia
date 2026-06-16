<script lang="ts">
	import { onMount, onDestroy, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api, layoutPreview } from '$lib/api';
	import type { Layout } from '$lib/layout/schema';
	import { cardTemplates, templateLayout } from '$lib/layout/templates';
	import Toggle from '$lib/components/Toggle.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import SaveBar from '$lib/components/SaveBar.svelte';
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
		ExternalLink
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
	let saving = $state(false);
	let testing = $state(false);
	let testMsg = $state('');
	let baseline = $state('');
	let variables = $state<{ token: string; desc: string }[]>([]);
	let previewUrl = $state('');
	let studioOpen = $state(false);

	const dirty = $derived(loaded && JSON.stringify({ enabled, cfg }) !== baseline);

	// The two triggers this built-in automation listens on — mirrored from the
	// automations catalogue (member_join / member_leave) so the page reads as the
	// configure surface for the welcome.join / welcome.leave built-ins.
	const TRIGGERS = {
		welcome: { label: 'Member joins', key: 'member_join', verb: 'joins', icon: UserPlus, builtin: 'welcome.join' },
		goodbye: { label: 'Member leaves', key: 'member_leave', verb: 'leaves', icon: UserMinus, builtin: 'welcome.leave' }
	} as const;
	const trigger = $derived(TRIGGERS[tab]);

	// Shared mono "kind" chip — the automations technical motif.
	const chip =
		'inline-flex w-fit items-center gap-1 rounded border border-line bg-bg px-1.5 py-0.5 font-mono text-[9.5px] uppercase tracking-[0.1em] text-muted';

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

	// Example {{ }} template snippets (logic + functions), click to insert.
	const templateSnippets = [
		'{{upper .User.Username}}',
		'{{if gt .Guild.MemberCount 100}}🎉{{end}}',
		'{{randInt 1 6}}',
		'{{.Guild.Name}}'
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

	// variable insertion at the cursor of the last focused text field
	let lastField: HTMLInputElement | HTMLTextAreaElement | null = null;
	function trackFocus(e: FocusEvent) {
		const t = e.target;
		if (t instanceof HTMLInputElement || t instanceof HTMLTextAreaElement) lastField = t;
	}
	function insertVar(token: string) {
		const el = lastField;
		if (!el) return;
		const s = el.selectionStart ?? el.value.length;
		const e = el.selectionEnd ?? el.value.length;
		el.value = el.value.slice(0, s) + token + el.value.slice(e);
		el.dispatchEvent(new Event('input', { bubbles: true }));
		el.focus();
		const pos = s + token.length;
		el.setSelectionRange(pos, pos);
	}

	function addEmbed() {
		if (cfg[tab].embeds.length >= 10) return;
		cfg[tab].embeds = [...cfg[tab].embeds, emptyEmbed('#5865F2')];
	}
	function removeEmbed(i: number) {
		cfg[tab].embeds = cfg[tab].embeds.filter((_, idx) => idx !== i);
	}

	// Card design — studio-first.
	function applyTemplate(id: string) {
		cfg[tab].card.layout = templateLayout(id);
	}
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
		saving = true;
		try {
			await api.saveFeature(store.id, FEATURE, enabled, cfg);
			if (store.detail)
				store.detail.features[FEATURE] = { enabled, config: cfg as unknown as Record<string, unknown> };
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

	const tabs = [
		{ k: 'welcome', label: 'Member joins' },
		{ k: 'goodbye', label: 'Member leaves' }
	] as const;
</script>

<svelte:head><title>Welcome · {store.name} · Dia</title></svelte:head>

<div class="flex h-full flex-col bg-bg text-ink">
	<PageTopbar
		eyebrow="Welcome"
		subtitle="A built-in automation that greets members and bids them farewell."
	>
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

	<div
		class="flex min-h-10 shrink-0 flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-line/60 bg-bg px-5 py-1.5 md:flex-nowrap"
	>
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

	<div class="min-h-0 flex-1 overflow-y-auto">
		<div class="mx-auto max-w-[1180px] px-5 py-6">
			{#if !loaded}
				<div class="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(340px,400px)]">
					<div class="space-y-3">
						<div class="skeleton h-16 w-full rounded-xl"></div>
						<div class="skeleton h-44 w-full rounded-xl"></div>
						<div class="skeleton h-28 w-full rounded-xl"></div>
						<div class="skeleton h-28 w-full rounded-xl"></div>
					</div>
					<div class="skeleton h-72 w-full rounded-xl"></div>
				</div>
			{:else}
				{@const TIcon = trigger.icon}
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div
					class="grid items-start gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(340px,400px)]"
					onfocusin={trackFocus}
				>
					<div>
						{#if !enabled}
							<div
								class="mb-3 flex items-center gap-2 rounded-lg border border-line bg-ink-2 px-3.5 py-2 text-[12.5px] text-muted"
							>
								<span class="size-1.5 shrink-0 rounded-full bg-faint/40"></span>
								The welcome system is off. Turn it on, top-right, for these flows to run.
							</div>
						{/if}

						<div
							class="flex items-center gap-3 rounded-xl border border-accent/25 bg-accent/[0.06] px-4 py-3"
						>
							<div
								class="grid size-8 shrink-0 place-items-center rounded-lg border border-accent/30 bg-accent/10 text-accent-ink"
							>
								<TIcon size={16} />
							</div>
							<div class="min-w-0 flex-1">
								<div class="flex flex-wrap items-center gap-x-2 gap-y-1">
									<span class="font-mono text-[9.5px] uppercase tracking-[0.16em] text-accent-ink/80">Trigger</span>
									<span class={chip}><Zap size={9} /> {trigger.key}</span>
								</div>
								<div class="mt-0.5 truncate text-[13.5px] font-medium text-ink">
									When a member {trigger.verb} <span class="text-muted">{store.name}</span>
								</div>
							</div>
							<label class="flex shrink-0 items-center gap-2 text-[12px]">
								<span class="hidden text-muted sm:inline">{cfg[tab].enabled ? 'Active' : 'Off'}</span>
								<Toggle bind:checked={cfg[tab].enabled} label="Run this flow" />
							</label>
						</div>

						<div class="ml-[31px] h-4 w-px bg-line-strong"></div>
						<div class="rounded-xl border border-line bg-surface/40">
							<div class="flex items-center gap-3 px-4 py-3">
								<div class="grid size-8 shrink-0 place-items-center rounded-lg border border-line bg-bg text-accent-ink">
									<MessageSquare size={16} />
								</div>
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-2">
										<h2 class="text-[13.5px] font-semibold text-ink">Post a message</h2>
										<span class={chip}>send_message</span>
									</div>
									<p class="mt-0.5 truncate text-[12px] text-muted">Sent to a channel when the trigger fires.</p>
								</div>
							</div>
							<div class="space-y-4 border-t border-line/60 px-4 py-4">
								<div>
									<div class="label">Channel</div>
									<ChannelSelect bind:value={cfg[tab].channel_id} />
								</div>
								<TemplateField label="Message" placeholder="Plain message text…" guildId={store.id} {variables} bind:value={cfg[tab].content} />
								<div class="flex items-center justify-between gap-4">
									<div>
										<div class="text-[13px] font-medium text-ink">Mention the member</div>
										<p class="text-[12px] text-muted">Pings them so it shows as a notification.</p>
									</div>
									<Toggle bind:checked={cfg[tab].ping_user} />
								</div>
							</div>
						</div>

						<div class="ml-[31px] h-4 w-px bg-line-strong"></div>
						<div class="rounded-xl border border-line bg-surface/40">
							<div class="flex items-center gap-3 px-4 py-3">
								<div class="grid size-8 shrink-0 place-items-center rounded-lg border border-line bg-bg text-accent-ink">
									<Layers size={16} />
								</div>
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-2">
										<h2 class="text-[13.5px] font-semibold text-ink">Embeds</h2>
										<span class={chip}>embed · {cfg[tab].embeds.length}/10</span>
									</div>
									<p class="mt-0.5 truncate text-[12px] text-muted">Rich blocks attached beneath the message.</p>
								</div>
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
								<div class="space-y-3 border-t border-line/60 px-4 py-4">
									{#each cfg[tab].embeds as _e, i (i)}
										<EmbedEditor embed={cfg[tab].embeds[i]} index={i} onRemove={() => removeEmbed(i)} />
									{/each}
								</div>
							{/if}
						</div>

						<div class="ml-[31px] h-4 w-px bg-line-strong"></div>
						<div class="rounded-xl border border-line bg-surface/40">
							<div class="flex items-center gap-3 px-4 py-3">
								<div class="grid size-8 shrink-0 place-items-center rounded-lg border border-line bg-bg text-accent-ink">
									<Frame size={16} />
								</div>
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-2">
										<h2 class="text-[13.5px] font-semibold text-ink">Attach a card image</h2>
										<span class={chip}>image.render</span>
									</div>
									<p class="mt-0.5 truncate text-[12px] text-muted">A rendered welcome card, designed in Card Studio.</p>
								</div>
								<Toggle bind:checked={cfg[tab].card.enabled} />
							</div>
							{#if cfg[tab].card.enabled}
								<div class="space-y-4 border-t border-line/60 px-4 py-4">
									<div>
										<div class="label">Starter</div>
										<div class="flex flex-wrap gap-2">
											{#each cardTemplates as t (t.id)}
												<button
													type="button"
													onclick={() => applyTemplate(t.id)}
													class="rounded-lg border border-line px-3 py-1.5 text-[13px] text-muted transition-colors hover:border-line-strong hover:text-ink"
												>
													{t.name}
												</button>
											{/each}
										</div>
									</div>
									<button
										type="button"
										onclick={openStudio}
										class="flex w-full items-center justify-center gap-2 rounded-lg border border-line-strong bg-ink-2 py-2.5 text-[13px] font-medium text-ink transition-colors hover:bg-surface"
									>
										<Wand2 size={15} class="text-accent-ink" /> Customize in Card Studio
									</button>
									<p class="text-[11.5px] text-faint">The render appears live in the preview, right.</p>
								</div>
							{/if}
						</div>

						<div class="ml-[31px] h-4 w-px bg-line-strong"></div>
						<div class="rounded-xl border border-line bg-surface/40">
							<div class="flex items-center gap-3 px-4 py-3">
								<div class="grid size-8 shrink-0 place-items-center rounded-lg border border-line bg-bg text-accent-ink">
									<Mail size={16} />
								</div>
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-2">
										<h2 class="text-[13.5px] font-semibold text-ink">Send a direct message</h2>
										<span class={chip}>send_dm</span>
									</div>
									<p class="mt-0.5 truncate text-[12px] text-muted">Also send the member a private DM.</p>
								</div>
								<Toggle bind:checked={cfg[tab].dm.enabled} />
							</div>
							{#if cfg[tab].dm.enabled}
								<div class="border-t border-line/60 px-4 py-4">
									<TemplateField placeholder="DM text…" guildId={store.id} {variables} rows={2} bind:value={cfg[tab].dm.content} />
								</div>
							{/if}
						</div>
					</div>

					<div class="space-y-4 lg:sticky lg:top-4">
						<div class="flex items-center justify-between">
							<span class="eyebrow">Preview</span>
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

						<div class="rounded-xl border border-line bg-surface p-3">
							<div class="mb-2 font-mono text-[10px] uppercase tracking-[0.14em] text-faint">Tokens — click to insert</div>
							<div class="flex flex-wrap gap-1.5">
								{#each variables as v (v.token)}
									<button
										type="button"
										title={v.desc}
										onclick={() => insertVar(v.token)}
										class="rounded-md border border-line bg-ink-2 px-1.5 py-1 font-mono text-[11px] text-muted transition-colors hover:border-line-strong hover:text-ink"
									>
										{v.token}
									</button>
								{/each}
							</div>
						</div>

						<div class="rounded-xl border border-line bg-surface p-3">
							<div class="mb-1 flex items-center gap-2 font-mono text-[10px] uppercase tracking-[0.14em] text-faint">
								Logic &amp; functions <span class="normal-case text-faint">{'{{ }}'}</span>
							</div>
							<p class="mb-2 text-[11px] leading-relaxed text-faint">
								Messages run a sandboxed template engine — conditionals, loops and functions
								(<span class="font-mono">if</span>, <span class="font-mono">range</span>,
								<span class="font-mono">upper</span>, <span class="font-mono">randInt</span>). Click to insert:
							</p>
							<div class="flex flex-wrap gap-1.5">
								{#each templateSnippets as t (t)}
									<button
										type="button"
										onclick={() => insertVar(t)}
										class="rounded-md border border-line bg-ink-2 px-1.5 py-1 font-mono text-[11px] text-muted transition-colors hover:border-line-strong hover:text-ink"
									>
										{t}
									</button>
								{/each}
							</div>
						</div>
					</div>
				</div>

				<SaveBar {dirty} {saving} onsave={save} onreset={reset} />
			{/if}
		</div>
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
