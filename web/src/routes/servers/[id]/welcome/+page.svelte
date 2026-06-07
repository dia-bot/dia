<script lang="ts">
	import { onMount, onDestroy, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api, layoutPreview } from '$lib/api';
	import type { Layout } from '$lib/layout/schema';
	import { cardTemplates, templateLayout } from '$lib/layout/templates';
	import Toggle from '$lib/components/Toggle.svelte';
	import ChannelSelect from '$lib/components/ChannelSelect.svelte';
	import SaveBar from '$lib/components/SaveBar.svelte';
	import DiscordMessagePreview from '$lib/components/dashboard/DiscordMessagePreview.svelte';
	import EmbedEditor from '$lib/components/dashboard/EmbedEditor.svelte';
	import CardStudioModal from '$lib/components/editor/CardStudioModal.svelte';
	import TemplateField from '$lib/components/TemplateField.svelte';
	import { Plus, Send, Wand2 } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'welcome';

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
			if (store.detail) store.detail.features[FEATURE] = { enabled, config: cfg as unknown as Record<string, unknown> };
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
		{ k: 'welcome', label: 'Welcome' },
		{ k: 'goodbye', label: 'Goodbye' }
	] as const;
</script>

<svelte:head><title>Welcome · {store.name} · Dia</title></svelte:head>

<header class="mb-6 flex items-start justify-between gap-4">
	<div>
		<h1 class="text-xl font-semibold tracking-tight text-ink">Welcome &amp; Goodbye</h1>
		<p class="mt-1 text-sm text-muted">Greet members when they join, and say farewell when they leave.</p>
	</div>
	<label class="flex shrink-0 items-center gap-2.5 text-sm">
		<span class="text-muted">{enabled ? 'Enabled' : 'Disabled'}</span>
		<Toggle bind:checked={enabled} />
	</label>
</header>

<div class="mb-7 flex items-center gap-6 border-b border-line">
	{#each tabs as t (t.k)}
		<button
			type="button"
			onclick={() => (tab = t.k)}
			class="-mb-px flex items-center gap-2 border-b-2 pb-2.5 text-sm font-medium transition-colors {tab === t.k
				? 'border-ink text-ink'
				: 'border-transparent text-muted hover:text-ink'}"
		>
			{t.label}
			{#if cfg[t.k].enabled}<span class="h-1.5 w-1.5 rounded-full bg-success"></span>{/if}
		</button>
	{/each}
</div>

{#if !loaded}
	<div class="grid gap-8 lg:grid-cols-[1fr_minmax(360px,440px)]">
		<div class="skeleton h-[28rem] w-full rounded-card"></div>
		<div class="skeleton h-72 w-full rounded-card"></div>
	</div>
{:else}
	<!-- svelte-ignore a11y_no_static_element_interactions -->
	<div class="grid items-start gap-8 lg:grid-cols-[1fr_minmax(360px,440px)]" onfocusin={trackFocus}>
		<!-- form: one panel, hairline-divided sections -->
		<div class="card divide-y divide-line">
			<!-- message -->
			<section class="p-5">
				<div class="flex items-center justify-between gap-4">
					<div>
						<h2 class="text-sm font-semibold text-ink">Message</h2>
						<p class="mt-0.5 text-xs text-muted">
							Posted in the channel when a member {tab === 'welcome' ? 'joins' : 'leaves'}.
						</p>
					</div>
					<Toggle bind:checked={cfg[tab].enabled} />
				</div>
				<div class="mt-5 space-y-4">
					<div>
						<div class="label">Channel</div>
						<ChannelSelect bind:value={cfg[tab].channel_id} />
					</div>
					<div>
						<TemplateField
							label="Text"
							placeholder="Plain message text…"
							guildId={store.id}
							{variables}
							bind:value={cfg[tab].content}
						/>
					</div>
					<div class="flex items-center justify-between gap-4">
						<div>
							<div class="text-sm font-medium text-ink">Mention the member</div>
							<p class="text-xs text-muted">Pings them so it shows as a notification.</p>
						</div>
						<Toggle bind:checked={cfg[tab].ping_user} />
					</div>
				</div>
			</section>

			<!-- embeds -->
			<section class="p-5">
				<div class="flex items-center justify-between gap-4">
					<div>
						<h2 class="text-sm font-semibold text-ink">Embeds</h2>
						<p class="mt-0.5 text-xs text-muted">Rich blocks under the message — {cfg[tab].embeds.length}/10.</p>
					</div>
					<button
						type="button"
						onclick={addEmbed}
						disabled={cfg[tab].embeds.length >= 10}
						class="flex items-center gap-1.5 rounded-lg border border-line-strong px-2.5 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-ink-2 disabled:opacity-40"
					>
						<Plus size={13} /> Add embed
					</button>
				</div>
				{#if cfg[tab].embeds.length}
					<div class="mt-4 space-y-3">
						{#each cfg[tab].embeds as _e, i (i)}
							<EmbedEditor embed={cfg[tab].embeds[i]} index={i} onRemove={() => removeEmbed(i)} />
						{/each}
					</div>
				{/if}
			</section>

			<!-- card -->
			<section class="p-5">
				<div class="flex items-center justify-between gap-4">
					<div>
						<h2 class="text-sm font-semibold text-ink">Card image</h2>
						<p class="mt-0.5 text-xs text-muted">A generated image, designed in Card Studio.</p>
					</div>
					<Toggle bind:checked={cfg[tab].card.enabled} />
				</div>
				{#if cfg[tab].card.enabled}
					<div class="mt-5 space-y-4">
						<div class="overflow-hidden rounded-xl border border-line bg-ink-2">
							{#if previewUrl}
								<img src={previewUrl} alt="Card preview" class="block w-full" />
							{:else}
								<div class="grid aspect-[1024/450] place-items-center text-sm text-faint">
									{cfg[tab].card.layout ? 'Rendering…' : 'Pick a starter to begin'}
								</div>
							{/if}
						</div>
						<div>
							<div class="label">Starter</div>
							<div class="flex flex-wrap gap-2">
								{#each cardTemplates as t (t.id)}
									<button
										type="button"
										onclick={() => applyTemplate(t.id)}
										class="rounded-lg border border-line px-3 py-1.5 text-sm text-muted transition-colors hover:border-line-strong hover:text-ink"
									>
										{t.name}
									</button>
								{/each}
							</div>
						</div>
						<button
							type="button"
							onclick={openStudio}
							class="flex w-full items-center justify-center gap-2 rounded-lg border border-line-strong bg-ink-2 py-2.5 text-sm font-medium text-ink transition-colors hover:bg-surface"
						>
							<Wand2 size={15} class="text-accent-ink" /> Customize in Card Studio
						</button>
					</div>
				{/if}
			</section>

			<!-- dm -->
			<section class="p-5">
				<div class="flex items-center justify-between gap-4">
					<div>
						<h2 class="text-sm font-semibold text-ink">Direct message</h2>
						<p class="mt-0.5 text-xs text-muted">Also send the member a private DM.</p>
					</div>
					<Toggle bind:checked={cfg[tab].dm.enabled} />
				</div>
				{#if cfg[tab].dm.enabled}
					<textarea class="input mt-5" rows="2" placeholder="DM text…" bind:value={cfg[tab].dm.content}></textarea>
				{/if}
			</section>
		</div>

		<!-- preview rail -->
		<div class="space-y-4 lg:sticky lg:top-4">
			<div class="flex items-center justify-between">
				<span class="eyebrow">Preview</span>
				<div class="flex items-center gap-3">
					{#if testMsg}<span class="text-xs {testMsg === 'Sent' ? 'text-success' : 'text-danger'}">{testMsg}</span>{/if}
					<button
						type="button"
						onclick={sendTest}
						disabled={testing}
						class="flex items-center gap-1.5 rounded-lg border border-line-strong px-2.5 py-1.5 text-xs font-medium text-ink transition-colors hover:bg-ink-2 disabled:opacity-50"
					>
						<Send size={13} /> {testing ? 'Sending…' : 'Send test'}
					</button>
				</div>
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
				<div class="mb-2 text-xs font-medium text-muted">Variables — click to insert at your cursor</div>
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
				<div class="mb-1 flex items-center gap-2 text-xs font-medium text-muted">
					Logic &amp; functions <span class="font-mono text-[11px] text-faint">{'{{ }}'}</span>
				</div>
				<p class="mb-2 text-[11px] leading-relaxed text-faint">
					Beyond tokens, messages run a sandboxed template engine — conditionals, loops and functions
					(<span class="font-mono">if</span>, <span class="font-mono">range</span>,
					<span class="font-mono">upper</span>, <span class="font-mono">randInt</span>,
					<span class="font-mono">default</span>…). Click to insert:
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

	{#if studioOpen}
		<CardStudioModal
			layout={cfg[tab].card.layout ?? templateLayout('aurora')}
			guildId={store.id}
			onApply={(l) => (cfg[tab].card.layout = l)}
			onClose={() => (studioOpen = false)}
		/>
	{/if}
{/if}
