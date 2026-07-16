<script lang="ts">
	// First-run onboarding for Social Alerts: Overview → Platform → Connect.
	//
	// Walks a fresh server through following its first creator in one popup, then
	// creates the subscription atomically (the same api.createSocial the inline
	// editor uses) and hands the new row back to the page. Modeled on the custom
	// command NewCommandWizard.
	import { api } from '$lib/api';
	import type { SocialCapability, SocialSubscription } from '$lib/social';
	import { providerIcon, providerColor } from './providers';
	import Field from '$lib/components/Field.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import { Dialog } from '$lib/components/ui';
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';

	import Megaphone from 'lucide-svelte/icons/megaphone';
	import Radio from 'lucide-svelte/icons/radio';
	import Bell from 'lucide-svelte/icons/bell';
	import Check from 'lucide-svelte/icons/check';
	import ChevronLeft from 'lucide-svelte/icons/chevron-left';
	import ChevronRight from 'lucide-svelte/icons/chevron-right';
	import Lock from 'lucide-svelte/icons/lock';
	import Sparkles from 'lucide-svelte/icons/sparkles';
	import Loader2 from 'lucide-svelte/icons/loader-2';
	import TriangleAlert from 'lucide-svelte/icons/triangle-alert';

	let {
		open = $bindable(false),
		guildId,
		caps = [],
		atLimit = false,
		oncreated
	}: {
		open?: boolean;
		guildId: string;
		caps?: SocialCapability[];
		atLimit?: boolean;
		oncreated?: (sub: SocialSubscription) => void;
	} = $props();

	type WizardStep = 0 | 1 | 2;
	const STEPS: { key: WizardStep; label: string; icon: typeof Megaphone }[] = [
		{ key: 0, label: 'Overview', icon: Megaphone },
		{ key: 1, label: 'Platform', icon: Radio },
		{ key: 2, label: 'Connect', icon: Bell }
	];

	let step = $state<WizardStep>(0);
	let provider = $state('');
	let account = $state('');
	let channel = $state('');
	let pingRole = $state('');
	let creating = $state(false);
	let createError = $state('');

	const available = $derived(caps.filter((c) => c.status === 'available'));
	const comingSoon = $derived(caps.filter((c) => c.status === 'coming_soon'));
	const selectedCap = $derived(caps.find((c) => c.provider === provider));

	// Reset every time the wizard (re)opens.
	$effect(() => {
		if (!open) return;
		step = 0;
		provider = '';
		account = '';
		channel = '';
		pingRole = '';
		creating = false;
		createError = '';
	});

	const canAdvance = $derived.by(() => {
		if (step === 0) return true;
		if (step === 1) return provider !== '';
		return true;
	});

	function tryGo(target: WizardStep) {
		if (target <= step || canAdvance) step = target;
	}

	function pickPlatform(p: string) {
		provider = p;
		step = 2;
	}

	async function create() {
		if (creating) return;
		createError = '';
		if (!provider) {
			step = 1;
			return;
		}
		if (!account.trim()) {
			createError = 'Enter the account to follow.';
			return;
		}
		if (!channel) {
			createError = 'Pick a channel to announce in.';
			return;
		}
		creating = true;
		try {
			const res = await api.createSocial(guildId, {
				provider,
				account: account.trim(),
				channel_id: channel,
				ping_role_id: pingRole,
				template: '',
				embed: true
			});
			oncreated?.(res.subscription);
			open = false;
		} catch (e) {
			createError = e instanceof Error ? e.message : String(e);
		} finally {
			creating = false;
		}
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-w-[640px] gap-0 overflow-hidden p-0">
		<Dialog.Title class="sr-only">Set up Social Alerts</Dialog.Title>

		<!-- Header: eyebrow · current step · progress -->
		<div class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-4">
			<div class="grid size-5 place-items-center rounded border border-line bg-surface text-accent-ink">
				<Megaphone size={11} />
			</div>
			<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
				Social Alerts
			</span>
			<div class="h-4 w-px bg-line"></div>
			<span class="text-[12.5px] font-medium text-ink">{STEPS[step].label}</span>
			<span class="font-mono text-[10.5px] tabular-nums text-faint">
				{step + 1} / {STEPS.length}
			</span>
		</div>

		<!-- Step rail -->
		<div class="flex h-9 shrink-0 items-center gap-1 border-b border-line/60 px-3">
			{#each STEPS as s, i (s.key)}
				{@const active = s.key === step}
				{@const done = s.key < step}
				<button
					type="button"
					onclick={() => tryGo(s.key)}
					class="inline-flex h-6 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium transition-colors {active
						? 'bg-surface text-ink'
						: done
							? 'text-muted hover:bg-surface/60 hover:text-ink'
							: 'text-faint hover:text-muted'}"
				>
					{#if done}
						<Check size={11} />
					{:else}
						<s.icon size={11} />
					{/if}
					{s.label}
					{#if i < STEPS.length - 1}
						<span class="ml-1 text-faint/60">›</span>
					{/if}
				</button>
			{/each}
		</div>

		<!-- Body -->
		<div class="max-h-[60vh] min-h-72 overflow-y-auto px-5 py-5">
			{#key step}
				<div in:fly={{ x: 14, duration: dur(220), easing: cubicOut }}>
					{#if step === 0}
						<div class="space-y-4">
							<div>
								<h2 class="text-[15px] font-semibold text-ink">Never miss a go-live</h2>
								<p class="mt-1 text-[12.5px] leading-relaxed text-muted">
									Follow creators on Twitch, YouTube, Kick, Bluesky and RSS. Dia posts to your
									server the moment they go live, upload, or post, so nobody has to keep checking.
								</p>
							</div>
							<div class="grid gap-2">
								{#each [{ icon: Radio, t: 'Live streams', b: 'Announce when a channel goes live, with the title and game.' }, { icon: Sparkles, t: 'New uploads & posts', b: 'Videos, Bluesky posts and RSS entries land in your channel.' }, { icon: Bell, t: 'Optional role pings', b: 'Mention a role so the right people get notified.' }] as f (f.t)}
									<div class="flex items-start gap-2.5 rounded-lg border border-line bg-surface/40 px-3 py-2.5">
										<f.icon size={14} class="mt-0.5 shrink-0 text-accent-ink" />
										<div class="min-w-0">
											<div class="text-[12.5px] font-medium text-ink">{f.t}</div>
											<div class="mt-0.5 text-[11.5px] leading-snug text-muted">{f.b}</div>
										</div>
									</div>
								{/each}
							</div>
						</div>
					{:else if step === 1}
						<div class="space-y-3">
							<p class="text-[12.5px] text-muted">Which platform is the creator on?</p>
							{#if available.length === 0}
								<div class="rounded-lg border border-dashed border-line bg-surface/30 px-4 py-8 text-center text-[12px] text-muted">
									No platforms are available on this deployment yet. Ask your host to add the
									provider credentials.
								</div>
							{:else}
								<div class="grid grid-cols-2 gap-2 sm:grid-cols-3">
									{#each available as cap (cap.provider)}
										{@const Icon = providerIcon(cap.provider)}
										{@const selected = provider === cap.provider}
										<button
											type="button"
											onclick={() => pickPlatform(cap.provider)}
											class="group flex items-center gap-2.5 rounded-lg border px-3 py-3 text-left transition-colors {selected
												? 'border-line-strong bg-surface'
												: 'border-line bg-surface/40 hover:border-line-strong'}"
										>
											<span
												class="grid size-8 shrink-0 place-items-center rounded-md border border-line bg-bg"
												style="color: {providerColor(cap.provider)}"
											>
												<Icon size={16} />
											</span>
											<span class="min-w-0">
												<span class="block truncate text-[12.5px] font-medium text-ink">{cap.name}</span>
												<span class="block truncate text-[10.5px] text-muted">{cap.input}</span>
											</span>
										</button>
									{/each}
								</div>
							{/if}
							{#if comingSoon.length}
								<div class="flex flex-wrap items-center gap-1.5 pt-1">
									<span class="font-mono text-[9.5px] uppercase tracking-[0.14em] text-faint">Soon</span>
									{#each comingSoon as cap (cap.provider)}
										<span
											class="inline-flex items-center gap-1 rounded border border-dashed border-line bg-bg px-1.5 py-0.5 text-[10.5px] text-faint"
										>
											<Lock size={9} />
											{cap.name}
										</span>
									{/each}
								</div>
							{/if}
						</div>
					{:else}
						<div class="space-y-1">
							{#if selectedCap}
								{@const Icon = providerIcon(selectedCap.provider)}
								<div class="mb-3 inline-flex items-center gap-2 rounded-md border border-line bg-surface/50 px-2.5 py-1.5">
									<span style="color: {providerColor(selectedCap.provider)}">
										<Icon size={14} />
									</span>
									<span class="text-[12px] font-medium text-ink">{selectedCap.name}</span>
									<button
										type="button"
										class="ml-1 font-mono text-[10px] text-faint hover:text-ink"
										onclick={() => (step = 1)}
									>
										change
									</button>
								</div>
							{/if}
							<Field label={selectedCap?.input ?? 'Account'} hint="Resolved and validated when you finish.">
								<!-- svelte-ignore a11y_autofocus -->
								<input
									type="text"
									bind:value={account}
									placeholder={selectedCap?.input ?? ''}
									class="input w-full"
									autofocus
									onkeydown={(e) => e.key === 'Enter' && create()}
								/>
							</Field>
							<Field label="Announce in" hint="New activity posts to this channel.">
								<ChannelPicker value={channel} onChange={(v) => (channel = v as string)} />
							</Field>
							<Field label="Ping role" hint="Optional role mentioned with each announcement.">
								<RolePicker
									includeManaged
									value={pingRole}
									onChange={(v) => (pingRole = v as string)}
									placeholder="No ping"
								/>
							</Field>
							<div class="flex items-start gap-2.5 rounded-lg border border-line bg-ink-2 px-3 py-2.5">
								<Sparkles size={13} class="mt-0.5 shrink-0 text-faint" />
								<p class="text-[11.5px] leading-relaxed text-muted">
									Posts a rich embed by default. You can edit the message, toggle the embed, and
									follow more accounts from the page after this.
								</p>
							</div>
						</div>
					{/if}
				</div>
			{/key}
		</div>

		<!-- Footer -->
		<div class="flex h-12 shrink-0 items-center gap-1.5 border-t border-line px-3">
			{#if step > 0}
				<button
					type="button"
					class="inline-flex h-7 items-center gap-1 rounded-md px-2 text-[12px] font-medium text-muted transition-colors hover:bg-surface hover:text-ink"
					onclick={() => (step = (step - 1) as WizardStep)}
				>
					<ChevronLeft size={12} />
					Back
				</button>
			{:else}
				<button
					type="button"
					class="inline-flex h-7 items-center rounded-md px-2 text-[12px] font-medium text-faint transition-colors hover:text-ink"
					onclick={() => (open = false)}
				>
					Skip for now
				</button>
			{/if}
			{#if createError}
				<span class="ml-1 inline-flex min-w-0 items-center gap-1 truncate text-[11px] text-danger" title={createError}>
					<TriangleAlert size={12} class="shrink-0" />
					{createError}
				</span>
			{/if}
			<div class="ml-auto flex items-center gap-1.5">
				{#if step < STEPS.length - 1}
					<button
						type="button"
						class="inline-flex h-7 items-center gap-1 rounded-lg bg-ink px-2.5 text-[12px] font-medium text-bg transition-opacity hover:opacity-90 disabled:opacity-40"
						disabled={!canAdvance}
						onclick={() => (step = (step + 1) as WizardStep)}
					>
						Continue
						<ChevronRight size={12} />
					</button>
				{:else}
					<button
						type="button"
						class="inline-flex h-7 items-center gap-1.5 rounded-lg bg-ink px-3 text-[12px] font-medium text-bg transition-opacity hover:opacity-90 disabled:opacity-50"
						disabled={creating || atLimit}
						onclick={create}
					>
						{#if creating}<Loader2 size={12} class="animate-spin" />{/if}
						{atLimit ? 'Limit reached' : 'Follow account'}
					</button>
				{/if}
			</div>
		</div>
	</Dialog.Content>
</Dialog.Root>
