<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { fly, fade } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api, ApiError } from '$lib/api';
	import type { CustomBotState, BotPresence } from '$lib/types';
	import { dur } from '$lib/motion';
	import Field from '$lib/components/Field.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';

	import Bot from 'lucide-svelte/icons/bot';
	import ExternalLink from 'lucide-svelte/icons/external-link';
	import Check from 'lucide-svelte/icons/check';
	import CircleCheck from 'lucide-svelte/icons/circle-check';
	import CircleAlert from 'lucide-svelte/icons/circle-alert';
	import Loader from 'lucide-svelte/icons/loader';
	import KeyRound from 'lucide-svelte/icons/key-round';
	import UserPlus from 'lucide-svelte/icons/user-plus';
	import Sparkles from 'lucide-svelte/icons/sparkles';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import RefreshCw from 'lucide-svelte/icons/refresh-cw';
	import Power from 'lucide-svelte/icons/power';

	const store = getContext<GuildStore>(GUILD_CTX);
	const DEV_PORTAL = 'https://discord.com/developers/applications';

	let botState = $state<CustomBotState | null>(null);
	let loaded = $state(false);
	let loadError = $state('');

	async function load() {
		loaded = false;
		loadError = '';
		try {
			botState = await api.customBot(store.id);
			// Seed the wizard's presence editor from the saved bot (or defaults).
			presence = botState.presence ?? { ...defaultPresence };
			loaded = true;
		} catch (e) {
			loadError = e instanceof Error ? e.message : 'Could not load the custom bot.';
		}
	}
	onMount(load);

	// Poll while the bot is connecting so the state badge settles without a manual
	// refresh (the gateway reports ready/error asynchronously).
	let poll: ReturnType<typeof setInterval> | undefined;
	$effect(() => {
		const connecting = botState?.enabled && botState?.state === 'connecting';
		if (connecting && !poll) {
			poll = setInterval(refreshQuiet, 2500);
		} else if (!connecting && poll) {
			clearInterval(poll);
			poll = undefined;
		}
		return () => {
			if (poll) clearInterval(poll);
			poll = undefined;
		};
	});
	async function refreshQuiet() {
		try {
			const next = await api.customBot(store.id);
			botState = next;
		} catch {
			/* keep last */
		}
	}

	// ── Wizard state ────────────────────────────────────────────────────────
	const defaultPresence: BotPresence = {
		status: 'online',
		activity_type: -1,
		activity_text: '',
		activity_url: ''
	};
	let presence = $state<BotPresence>({ ...defaultPresence });

	let token = $state('');
	let validating = $state(false);
	let validateErr = $state('');
	let preview = $state<{ application_id: string; username: string; avatar_url: string } | null>(
		null
	);
	let saving = $state(false);
	let busy = $state(''); // which action button is spinning
	let invited = $state(false);

	async function validate() {
		if (!token.trim()) return;
		validating = true;
		validateErr = '';
		preview = null;
		try {
			preview = await api.validateCustomBot(store.id, token.trim());
		} catch (e) {
			validateErr = e instanceof ApiError ? e.message : 'Could not validate the token.';
		} finally {
			validating = false;
		}
	}

	async function saveAndContinue() {
		saving = true;
		try {
			botState = await api.saveCustomBot(store.id, { token: token.trim(), presence });
			token = '';
			preview = null;
		} catch (e) {
			validateErr = e instanceof ApiError ? e.message : 'Could not save.';
		} finally {
			saving = false;
		}
	}

	async function enable() {
		busy = 'enable';
		try {
			await api.enableCustomBot(store.id);
			await refreshQuiet();
		} finally {
			busy = '';
		}
	}
	async function disable() {
		busy = 'disable';
		try {
			await api.disableCustomBot(store.id);
			await refreshQuiet();
		} finally {
			busy = '';
		}
	}
	async function savePresence() {
		busy = 'presence';
		try {
			await api.customBotPresence(store.id, presence);
			await refreshQuiet();
		} finally {
			busy = '';
		}
	}

	let confirmRemove = $state(false);
	async function remove() {
		busy = 'remove';
		try {
			await api.deleteCustomBot(store.id);
			await load();
		} finally {
			busy = '';
		}
	}

	function openInvite() {
		if (botState?.invite_url) {
			window.open(botState.invite_url, '_blank', 'noopener');
			invited = true;
		}
	}

	// ── Derived ─────────────────────────────────────────────────────────────
	const activityKinds = [
		{ v: -1, label: 'No activity' },
		{ v: 0, label: 'Playing' },
		{ v: 2, label: 'Listening to' },
		{ v: 3, label: 'Watching' },
		{ v: 5, label: 'Competing in' },
		{ v: 1, label: 'Streaming' }
	];
	const statuses = [
		{ v: 'online', label: 'Online', dot: 'bg-success' },
		{ v: 'idle', label: 'Idle', dot: 'bg-[#e0a33a]' },
		{ v: 'dnd', label: 'Do Not Disturb', dot: 'bg-danger' },
		{ v: 'invisible', label: 'Invisible', dot: 'bg-faint' }
	] as const;

	const stateBadge = $derived.by(() => {
		switch (botState?.state) {
			case 'ready':
				return { label: 'Online', cls: 'text-success', dot: 'bg-success', Icon: CircleCheck };
			case 'connecting':
				return { label: 'Connecting…', cls: 'text-muted', dot: 'bg-[#e0a33a]', Icon: Loader };
			case 'error':
				return { label: 'Error', cls: 'text-danger', dot: 'bg-danger', Icon: CircleAlert };
			default:
				return { label: 'Offline', cls: 'text-faint', dot: 'bg-faint', Icon: Power };
		}
	});

	// Activity preview string, mirroring how Discord renders it.
	const activityPreview = $derived.by(() => {
		const k = activityKinds.find((a) => a.v === presence.activity_type);
		if (!k || presence.activity_type < 0 || !presence.activity_text.trim()) return '';
		return `${k.label} ${presence.activity_text.trim()}`;
	});
</script>

<svelte:head><title>Custom Bot · {store.name} · Dia</title></svelte:head>

<div class="flex h-full flex-col bg-bg text-ink">
	<!-- Slab header (matches the other settings surfaces). -->
	<header class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line bg-bg px-4 sm:px-5">
		<span class="grid size-6 shrink-0 place-items-center rounded border border-line bg-surface text-muted">
			<Bot size={13} />
		</span>
		<span class="text-[13px] font-semibold tracking-tight text-ink">Custom Bot</span>
		<div class="hidden h-3.5 w-px bg-line sm:block"></div>
		<span class="hidden min-w-0 truncate text-[12px] text-muted sm:block">
			Run Dia under your own bot's name, avatar and status.
		</span>
		{#if loaded && botState?.configured}
			{@const B = stateBadge}
			<span class="ml-auto inline-flex items-center gap-1.5 text-[12px] font-medium {B.cls}">
				<span class="inline-block size-1.5 rounded-full {B.dot}"></span>
				{B.label}
			</span>
		{/if}
	</header>

	<div class="min-h-0 flex-1 overflow-y-auto">
		{#if !loaded}
			<div class="mx-auto max-w-3xl px-4 py-8 sm:px-6">
				<div class="skeleton h-5 w-40 rounded"></div>
				<div class="skeleton mt-5 h-32 w-full rounded-xl"></div>
				<div class="skeleton mt-4 h-32 w-full rounded-xl"></div>
			</div>
		{:else if loadError}
			<div class="flex min-h-full items-center justify-center px-6 py-16">
				<div class="flex max-w-md flex-col items-center gap-3 text-center">
					<span class="grid size-10 place-items-center rounded-full border border-line bg-surface text-danger">
						<CircleAlert size={18} />
					</span>
					<p class="text-[13px] text-muted">{loadError}</p>
					<button type="button" onclick={load} class="btn btn-sm">Try again</button>
				</div>
			</div>
		{:else if botState && !botState.available}
			<!-- Feature not enabled on this deployment. -->
			<div class="mx-auto max-w-2xl px-4 py-16 text-center sm:px-6">
				<span class="mx-auto grid size-12 place-items-center rounded-full border border-line bg-surface text-faint">
					<KeyRound size={20} />
				</span>
				<h2 class="mt-4 text-[15px] font-semibold text-ink">Custom bots aren't enabled here</h2>
				<p class="mx-auto mt-2 max-w-md text-[13px] leading-relaxed text-muted">
					This Dia instance hasn't set an encryption key for storing bot tokens. Ask the
					administrator to set <code class="rounded bg-ink-2 px-1 py-0.5 font-mono text-[11px] text-accent-ink">CUSTOM_BOT_ENC_KEY</code>
					to turn on the personalizer.
				</p>
			</div>
		{:else if botState && !botState.configured}
			<!-- ── Onboarding wizard ── -->
			{@render wizard()}
		{:else if botState}
			<!-- ── Management ── -->
			{@render manage()}
		{/if}
	</div>
</div>

{#snippet stepHeader(n: number, title: string, sub: string, Icon: typeof Bot, done: boolean)}
	<div class="flex items-start gap-3">
		<span
			class="mt-0.5 grid size-7 shrink-0 place-items-center rounded-full border text-[12px] font-semibold {done
				? 'border-success/40 bg-success/10 text-success'
				: 'border-line bg-surface text-muted'}"
		>
			{#if done}<Check size={14} />{:else}{n}{/if}
		</span>
		<div class="min-w-0">
			<div class="flex items-center gap-1.5 text-[13.5px] font-semibold text-ink">
				<Icon size={14} class="text-faint" />
				{title}
			</div>
			<p class="mt-0.5 text-[12.5px] leading-relaxed text-muted">{sub}</p>
		</div>
	</div>
{/snippet}

{#snippet presenceEditor()}
	<div class="space-y-4">
		<Field label="Status">
			<div class="flex flex-wrap gap-1.5">
				{#each statuses as s (s.v)}
					<button
						type="button"
						onclick={() => (presence.status = s.v)}
						class="inline-flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-[12.5px] transition-colors {presence.status ===
						s.v
							? 'border-line-strong bg-ink-2 text-ink'
							: 'border-line text-muted hover:bg-ink-2/50'}"
					>
						<span class="inline-block size-1.5 rounded-full {s.dot}"></span>
						{s.label}
					</button>
				{/each}
			</div>
		</Field>

		<div class="grid gap-3 sm:grid-cols-[minmax(0,10rem)_1fr]">
			<Field label="Activity">
				<select
					bind:value={presence.activity_type}
					class="input w-full"
				>
					{#each activityKinds as a (a.v)}
						<option value={a.v}>{a.label}</option>
					{/each}
				</select>
			</Field>
			{#if presence.activity_type >= 0}
				<Field label={presence.activity_type === 1 ? 'Stream title' : 'Activity text'}>
					<input
						type="text"
						class="input w-full"
						maxlength={128}
						placeholder={presence.activity_type === 3 ? 'the server' : 'your custom text'}
						bind:value={presence.activity_text}
					/>
				</Field>
			{/if}
		</div>
		{#if presence.activity_type === 1}
			<Field label="Twitch / YouTube URL" hint="Streaming shows a live badge; needs a valid stream URL.">
				<input type="url" class="input w-full" placeholder="https://twitch.tv/…" bind:value={presence.activity_url} />
			</Field>
		{/if}

		{#if activityPreview}
			<div class="rounded-lg border border-line bg-ink-2 px-3 py-2 text-[12px] text-muted">
				Preview: <span class="text-ink">{activityPreview}</span>
			</div>
		{/if}
	</div>
{/snippet}

{#snippet wizard()}
	<div class="mx-auto max-w-2xl px-4 py-8 sm:px-6" in:fade={{ duration: dur(200) }}>
		<div class="rounded-xl border border-line bg-gradient-to-b from-surface to-bg p-5">
			<div class="flex items-center gap-2 text-[13px] font-semibold text-ink">
				<Sparkles size={15} class="text-accent-ink" /> Set up your own bot
			</div>
			<p class="mt-1.5 text-[12.5px] leading-relaxed text-muted">
				Create a Discord application, hand Dia the bot token, and every feature runs under your
				bot's name, avatar and status. It stays in your control: revoke the token any time.
			</p>
		</div>

		<ol class="mt-6 space-y-6">
			<!-- Step 1 -->
			<li class="border-l border-line pl-5">
				{@render stepHeader(1, 'Create your application', 'In the Discord Developer Portal: New Application, then open the Bot tab and turn on all three Privileged Gateway Intents (Presence, Server Members, Message Content).', KeyRound, false)}
				<a href={DEV_PORTAL} target="_blank" rel="noopener" class="mt-2.5 ml-10 inline-flex items-center gap-1.5 rounded-md border border-line-strong px-3 py-1.5 text-[12px] font-medium text-ink hover:bg-ink-2">
					Open Developer Portal <ExternalLink size={12} />
				</a>
			</li>

			<!-- Step 2 -->
			<li class="border-l border-line pl-5">
				{@render stepHeader(2, 'Paste the bot token', "On the Bot tab, press Reset Token and copy it. We encrypt it before storing, and it's never shown again.", KeyRound, !!preview)}
				<div class="mt-3 ml-10 space-y-2">
					<div class="flex gap-2">
						<input
							type="password"
							class="input w-full font-mono text-[12px]"
							placeholder="Bot token"
							bind:value={token}
							autocomplete="off"
							spellcheck="false"
						/>
						<button type="button" onclick={validate} disabled={validating || !token.trim()} class="btn btn-sm shrink-0">
							{#if validating}<Loader size={13} class="animate-spin" />{/if}
							Validate
						</button>
					</div>
					{#if validateErr}
						<p class="flex items-start gap-1.5 text-[12px] text-danger" in:fly={{ y: -4, duration: dur(150) }}>
							<CircleAlert size={13} class="mt-px shrink-0" />{validateErr}
						</p>
					{/if}
					{#if preview}
						<div class="flex items-center gap-3 rounded-lg border border-success/30 bg-success/5 p-2.5" in:fly={{ y: -4, duration: dur(180), easing: cubicOut }}>
							{#if preview.avatar_url}
								<img src={preview.avatar_url} alt="" class="size-9 rounded-full" />
							{:else}
								<span class="grid size-9 place-items-center rounded-full bg-elevated text-faint"><Bot size={18} /></span>
							{/if}
							<div class="min-w-0">
								<div class="truncate text-[13px] font-semibold text-ink">{preview.username}</div>
								<div class="font-mono text-[11px] text-faint">ID {preview.application_id}</div>
							</div>
							<CircleCheck size={16} class="ml-auto text-success" />
						</div>
					{/if}
				</div>
			</li>

			<!-- Step 3 -->
			<li class="border-l border-line pl-5">
				{@render stepHeader(3, 'Save & personalize', 'Store the bot and set its status. You can change the status any time.', Sparkles, false)}
				{#if preview}
					<div class="mt-3 ml-10 space-y-4" in:fly={{ y: -4, duration: dur(180) }}>
						{@render presenceEditor()}
						<button type="button" onclick={saveAndContinue} disabled={saving} class="btn btn-sm btn-primary">
							{#if saving}<Loader size={13} class="animate-spin" />{/if}
							Save bot
						</button>
					</div>
				{:else}
					<p class="mt-2 ml-10 text-[12px] text-faint">Validate a token above to continue.</p>
				{/if}
			</li>
		</ol>
	</div>
{/snippet}

{#snippet manage()}
	{#if botState}
		<div class="mx-auto max-w-2xl px-4 py-8 sm:px-6" in:fade={{ duration: dur(200) }}>
			<!-- Identity card -->
			<div class="rounded-xl border border-line bg-surface p-4">
				<div class="flex items-center gap-3.5">
					{#if botState.avatar_url}
						<img src={botState.avatar_url} alt="" class="size-14 rounded-full" />
					{:else}
						<span class="grid size-14 place-items-center rounded-full bg-elevated text-faint"><Bot size={26} /></span>
					{/if}
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-2">
							<span class="truncate text-[15px] font-semibold text-ink">{botState.username}</span>
							<span class="shrink-0 rounded bg-accent/15 px-1 py-px font-mono text-[9px] font-semibold uppercase tracking-wide text-accent-ink">App</span>
						</div>
						<div class="mt-0.5 font-mono text-[11px] text-faint">ID {botState.application_id}</div>
						{#if activityPreview}
							<div class="mt-1 text-[12px] text-muted">{activityPreview}</div>
						{/if}
					</div>
					<!-- Enable toggle -->
					<button
						type="button"
						onclick={() => (botState?.enabled ? disable() : enable())}
						disabled={busy === 'enable' || busy === 'disable'}
						class="inline-flex items-center gap-1.5 rounded-lg border px-3 py-1.5 text-[12.5px] font-medium transition-colors {botState.enabled
							? 'border-success/40 bg-success/10 text-success'
							: 'border-line-strong text-ink hover:bg-ink-2'}"
					>
						{#if busy === 'enable' || busy === 'disable'}
							<Loader size={13} class="animate-spin" />
						{:else}
							<Power size={13} />
						{/if}
						{botState.enabled ? 'Running' : 'Turn on'}
					</button>
				</div>

				{#if botState.enabled && botState.state === 'error' && botState.last_error}
					<div class="mt-3 flex items-start gap-2 rounded-lg border border-danger/30 bg-danger/5 px-3 py-2 text-[12px] text-danger" in:fly={{ y: -4, duration: dur(150) }}>
						<CircleAlert size={14} class="mt-px shrink-0" />
						<div>
							<div class="font-medium">The bot couldn't connect.</div>
							<div class="mt-0.5 text-danger/80">{botState.last_error}</div>
							<div class="mt-1 text-danger/70">Most often this means a privileged intent isn't enabled in the Developer Portal, or the token was reset.</div>
						</div>
					</div>
				{:else if botState.enabled && botState.state === 'ready'}
					<div class="mt-3 flex items-center gap-2 text-[12px] text-muted">
						<CircleCheck size={14} class="text-success" />
						Connected{botState.commands_synced ? ' and commands registered' : '; registering commands…'}.
					</div>
				{/if}
			</div>

			<!-- Invite -->
			<div class="mt-4 flex flex-wrap items-center gap-3 rounded-xl border border-line bg-surface p-4">
				<span class="grid size-9 shrink-0 place-items-center rounded-lg border border-line bg-ink-2 text-muted"><UserPlus size={16} /></span>
				<div class="min-w-0 flex-1">
					<div class="text-[13px] font-medium text-ink">Add the bot to this server</div>
					<div class="text-[12px] text-muted">Authorize your bot with the commands scope so it can post and respond here.</div>
				</div>
				<button type="button" onclick={openInvite} class="btn btn-sm shrink-0">
					<UserPlus size={13} /> Invite {invited ? 'again' : 'bot'}
				</button>
			</div>

			<!-- Presence editor -->
			<div class="mt-4 rounded-xl border border-line bg-surface p-4">
				<div class="mb-3 text-[13px] font-semibold text-ink">Status & activity</div>
				{@render presenceEditor()}
				<button type="button" onclick={savePresence} disabled={busy === 'presence'} class="btn btn-sm btn-primary mt-4">
					{#if busy === 'presence'}<Loader size={13} class="animate-spin" />{/if}
					Save status
				</button>
			</div>

			<!-- Update token -->
			<div class="mt-4 rounded-xl border border-line bg-surface p-4">
				<div class="text-[13px] font-semibold text-ink">Replace the token</div>
				<p class="mt-1 text-[12px] text-muted">Reset the token in the Developer Portal and paste the new one to rotate credentials.</p>
				<div class="mt-3 flex gap-2">
					<input type="password" class="input w-full font-mono text-[12px]" placeholder="New bot token" bind:value={token} autocomplete="off" spellcheck="false" />
					<button type="button" onclick={saveAndContinue} disabled={saving || !token.trim()} class="btn btn-sm shrink-0">
						{#if saving}<Loader size={13} class="animate-spin" />{:else}<RefreshCw size={13} />{/if}
						Update
					</button>
				</div>
				{#if validateErr}<p class="mt-2 text-[12px] text-danger">{validateErr}</p>{/if}
			</div>

			<!-- Danger zone -->
			<div class="mt-4 rounded-xl border border-danger/30 bg-danger/5 p-4">
				<div class="flex items-center justify-between gap-3">
					<div>
						<div class="text-[13px] font-semibold text-ink">Remove custom bot</div>
						<div class="text-[12px] text-muted">Stops the bot and deletes the stored token. Dia's shared bot takes over again.</div>
					</div>
					<button type="button" onclick={() => (confirmRemove = true)} disabled={busy === 'remove'} class="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-danger/40 px-3 py-1.5 text-[12.5px] font-medium text-danger hover:bg-danger/10">
						<Trash2 size={13} /> Remove
					</button>
				</div>
			</div>
		</div>
	{/if}
{/snippet}

<ConfirmDialog
	bind:open={confirmRemove}
	title="Remove custom bot?"
	description="This stops your bot and permanently deletes the stored token. You can set it up again later."
	confirmLabel="Remove bot"
	cancelLabel="Keep it"
	onconfirm={remove}
	oncancel={() => (confirmRemove = false)}
/>
