<script lang="ts">
	// The subscription editor: one large popup that owns everything about a
	// followed account. Delivery (account / channel / ping) up top, then a
	// per-event-kind section: each kind the provider emits gets its own announce
	// toggle, a fully composed message on the shared WYSIWYG MessageEditor (the
	// editor doubles as the live Discord preview), and an optional saved
	// automation to run when the event fires. Follows the ticket type modal
	// shell and the menu editor's unsaved-changes guard.
	import { setContext } from 'svelte';
	import { fade, scale, fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';
	import { api } from '$lib/api';
	import {
		SOCIAL_KINDS,
		SOCIAL_TEMPLATE_VARS,
		defaultSocialMessage,
		emptySocialMessageSpec,
		type SocialCapability,
		type SocialKindConfig,
		type SocialMessageSpec,
		type SocialSubSpec,
		type SocialSubscription
	} from '$lib/social';
	import { providerIcon, providerColor } from './providers';
	import type { Step } from '$lib/commands/types';
	import type { AutomationSummary } from '$lib/automations/types';
	import { AUTOMATION_CTX, EXPR_SCOPE_CTX, type ExprScope } from '$lib/commands/expr-meta';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import AutomationPicker from '$lib/components/commands/AutomationPicker.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import { Popover } from '$lib/components/ui';

	import Megaphone from 'lucide-svelte/icons/megaphone';
	import X from 'lucide-svelte/icons/x';
	import Zap from 'lucide-svelte/icons/zap';
	import Plus from 'lucide-svelte/icons/plus';
	import ArrowUpRight from 'lucide-svelte/icons/arrow-up-right';
	import Loader2 from 'lucide-svelte/icons/loader-2';
	import TriangleAlert from 'lucide-svelte/icons/triangle-alert';
	import Sparkles from 'lucide-svelte/icons/sparkles';

	let {
		open = $bindable(false),
		guildId,
		cap,
		sub = null,
		oncreated,
		onsaved
	}: {
		open?: boolean;
		guildId: string;
		// The provider capability (labels + which kinds it emits).
		cap?: SocialCapability;
		// null = following a new account on cap.provider.
		sub?: SocialSubscription | null;
		oncreated?: (s: SocialSubscription) => void;
		onsaved?: (s: SocialSubscription) => void;
	} = $props();

	// The message editor and its pickers read these once at init.
	setContext(AUTOMATION_CTX, false);
	setContext(EXPR_SCOPE_CTX, {
		options: [],
		variables: [],
		steps: [],
		extraVars: SOCIAL_TEMPLATE_VARS
	} satisfies ExprScope);

	const provider = $derived(sub?.provider ?? cap?.provider ?? '');
	const kindList = $derived(cap?.kinds?.length ? cap.kinds : ['new_post']);
	const creating = $derived(!sub);

	type KindState = {
		enabled: boolean;
		step: Step;
		buttonActions: Record<string, string>;
	};

	let account = $state('');
	let channel = $state('');
	let pingRole = $state('');
	let legacyEmbed = $state(true);
	let kinds = $state<Record<string, KindState>>({});
	let activeKind = $state('');
	let saving = $state(false);
	let saveErr = $state('');
	let baseline = $state('');
	let confirmOpen = $state(false);

	function isEmptyMessage(msg?: SocialMessageSpec): boolean {
		return !msg || (!msg.content?.trim() && !msg.embeds?.length && !msg.components?.length);
	}

	// defaultMessage is what the bot sends for a kind with nothing custom
	// stored; the editor seeds from it so the preview is never blank.
	function defaultMessage(k: string): SocialMessageSpec {
		return defaultSocialMessage(k, {
			platform: cap?.name ?? provider,
			color: providerColor(provider),
			embed: legacyEmbed
		});
	}

	function seedKind(k: string, kc?: SocialKindConfig): KindState {
		const stored = structuredClone($state.snapshot(kc?.message) ?? undefined);
		const msg: SocialMessageSpec = isEmptyMessage(stored)
			? defaultMessage(k)
			: { ...emptySocialMessageSpec(), ...stored };
		return {
			enabled: kc ? !kc.disabled : (SOCIAL_KINDS[k]?.defaultOn ?? true),
			step: {
				id: 'social-' + k,
				kind: 'send_message',
				spec: {
					content: msg.content ?? '',
					embeds: msg.embeds ?? [],
					components: msg.components ?? []
				}
			},
			buttonActions: { ...(msg.button_actions ?? {}) }
		};
	}

	// resetKind puts the active kind's message back to the bot default.
	function resetKind() {
		const st = kinds[activeKind];
		if (!st) return;
		const msg = defaultMessage(activeKind);
		st.step.spec = {
			content: msg.content ?? '',
			embeds: msg.embeds ?? [],
			components: msg.components ?? []
		};
		st.buttonActions = {};
	}

	// Seed the whole draft each time the popup (re)opens.
	let seeded = false;
	$effect(() => {
		if (!open) {
			seeded = false;
			return;
		}
		if (seeded) return;
		seeded = true;
		account = '';
		channel = sub?.channel_id ?? '';
		pingRole = sub?.ping_role_id ?? '';
		legacyEmbed = sub?.embed ?? true;
		const next: Record<string, KindState> = {};
		for (const k of kindList) next[k] = seedKind(k, sub?.spec?.kinds?.[k]);
		kinds = next;
		activeKind = kindList[0];
		saving = false;
		saveErr = '';
		confirmOpen = false;
		baseline = serialize();
	});

	// cleanMessage extracts the composed message from a kind's editor state,
	// pruning wholly empty parts and button actions whose button is gone.
	function cleanMessage(st: KindState): SocialMessageSpec | undefined {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const s = $state.snapshot(st.step.spec) as any;
		const content: string = s?.content ?? '';
		const embeds = (s?.embeds ?? []) as SocialMessageSpec['embeds'];
		const components = (s?.components ?? []) as NonNullable<SocialMessageSpec['components']>;
		if (!content.trim() && !embeds?.length && !components.length) return undefined;
		const suffixes = new Set(
			components.flatMap((row) => row.components.map((c) => c.custom_id_suffix).filter(Boolean))
		);
		const actions: Record<string, string> = {};
		for (const [suffix, id] of Object.entries(st.buttonActions)) {
			if (id && suffixes.has(suffix)) actions[suffix] = id;
		}
		const msg: SocialMessageSpec = { content, embeds, components };
		if (Object.keys(actions).length) msg.button_actions = actions;
		return msg;
	}

	function buildSpec(): SocialSubSpec {
		const out: Record<string, SocialKindConfig> = {};
		for (const k of kindList) {
			const st = kinds[k];
			if (!st) continue;
			const kc: SocialKindConfig = { disabled: !st.enabled };
			const msg = cleanMessage(st);
			if (msg) kc.message = msg;
			out[k] = kc;
		}
		return { kinds: out };
	}

	function serialize(): string {
		return JSON.stringify({ account, channel, pingRole, legacyEmbed, spec: buildSpec() });
	}
	function isDirty(): boolean {
		return serialize() !== baseline;
	}

	// Controlled close: a dirty draft asks before discarding.
	function guardClose() {
		if (!isDirty()) {
			doClose();
			return;
		}
		confirmOpen = true;
	}
	function doClose() {
		confirmOpen = false;
		open = false;
	}
	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && open && !confirmOpen) {
			e.stopPropagation();
			guardClose();
		}
	}

	async function save() {
		if (saving) return;
		saveErr = '';
		if (creating && !account.trim()) {
			saveErr = 'Enter the account to follow.';
			return;
		}
		if (!channel) {
			saveErr = 'Pick a channel to announce in.';
			return;
		}
		saving = true;
		try {
			if (creating) {
				const res = await api.createSocial(guildId, {
					provider,
					account: account.trim(),
					channel_id: channel,
					ping_role_id: pingRole,
					template: '',
					embed: legacyEmbed,
					spec: buildSpec()
				});
				baseline = serialize();
				oncreated?.(res.subscription);
			} else if (sub) {
				const res = await api.updateSocial(guildId, sub.id, {
					channel_id: channel,
					ping_role_id: pingRole,
					template: sub.template,
					embed: legacyEmbed,
					spec: buildSpec(),
					enabled: sub.enabled
				});
				baseline = serialize();
				onsaved?.(res.subscription);
			}
			doClose();
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Something went wrong';
		} finally {
			saving = false;
		}
	}

	// ── Connected automations ───────────────────────────────────────────────────
	// The saved-automations catalogue backing the per-event connection board.
	let autos = $state<AutomationSummary[]>([]);
	let autosLoaded = $state(false);
	$effect(() => {
		if (!open) {
			autosLoaded = false;
			return;
		}
		if (autosLoaded) return;
		autosLoaded = true;
		api
			.automations(guildId)
			.then((r) => (autos = r.automations ?? []))
			.catch(() => {});
	});
	let connectOpen = $state(false);
	let connectQuery = $state('');
	let creatingAuto = $state(false);
	let connectBusy = $state('');
	let connectErr = $state('');

	// A connection IS the automation's own trigger scoping: this subscription's
	// id in trigger_config.subscriptions (plus the kind, when kind-scoped).
	// Connect/disconnect edit that automation directly and apply instantly, so
	// exactly one runner (the automations dispatcher) ever fires — never a
	// duplicate side channel.
	const socialAutos = $derived(autos.filter((a) => a.trigger_type === 'social_update'));
	const connections = $derived(
		sub ? socialAutos.filter((a) => (a.trigger_config?.subscriptions ?? []).includes(sub.id)) : []
	);
	// Unscoped social automations fire for every followed account, this one
	// included; shown as ambient rows so the board never lies by omission.
	const anyAccount = $derived(
		socialAutos.filter((a) => {
			const cfg = a.trigger_config ?? {};
			if ((cfg.subscriptions ?? []).length) return false;
			const ks = cfg.kinds ?? [];
			return !ks.length || ks.includes(activeKind);
		})
	);
	const connectable = $derived.by(() => {
		const connected = new Set(connections.map((a) => a.id));
		const q = connectQuery.trim().toLowerCase();
		return socialAutos.filter(
			(a) =>
				!connected.has(a.id) &&
				(a.trigger_config?.subscriptions ?? []).length >= 0 &&
				!anyAccount.some((x) => x.id === a.id) &&
				(!q || a.name.toLowerCase().includes(q))
		);
	});

	// saveScope rewrites one automation's trigger scoping (fetch full, patch
	// config, upsert) and mirrors the change into the local catalogue.
	async function saveScope(id: string, patch: (cfg: { subscriptions?: string[]; kinds?: string[] }) => void, extra?: Record<string, unknown>) {
		connectBusy = id;
		connectErr = '';
		try {
			const a = await api.automation(guildId, id);
			const cfg = { ...(a.trigger_config ?? {}) };
			patch(cfg);
			await api.upsertAutomation(guildId, { ...a, ...extra, trigger_config: cfg });
			autos = autos.map((x) => (x.id === id ? { ...x, ...extra, trigger_config: cfg } : x));
		} catch (e) {
			connectErr = e instanceof Error ? e.message : 'Could not update the automation';
		} finally {
			connectBusy = '';
		}
	}

	async function connect(id: string) {
		if (!sub) return;
		const sid = sub.id;
		await saveScope(id, (cfg) => {
			cfg.subscriptions = [...new Set([...(cfg.subscriptions ?? []), sid])];
		});
		connectOpen = false;
		connectQuery = '';
	}

	async function disconnect(id: string) {
		if (!sub) return;
		const sid = sub.id;
		await saveScope(id, (cfg) => {
			const rest = (cfg.subscriptions ?? []).filter((x) => x !== sid);
			cfg.subscriptions = rest.length ? rest : undefined;
		});
		// An automation whose only account was this one would silently broaden
		// to every account once unscoped; park it disabled instead.
		const a = autos.find((x) => x.id === id);
		if (a && !(a.trigger_config?.subscriptions ?? []).length && a.enabled) {
			await saveScope(id, () => {}, { enabled: false });
		}
	}

	// createAndConnect mints a draft automation pre-scoped to this account and
	// event kind; building the flow happens on the canvas (the row links there).
	async function createAndConnect() {
		if (creatingAuto || !sub) return;
		creatingAuto = true;
		connectErr = '';
		try {
			const name = `${SOCIAL_KINDS[activeKind]?.label ?? activeKind} · ${sub.account_name}`;
			const cfg = { subscriptions: [sub.id], kinds: [activeKind] };
			const r = await api.upsertAutomation(guildId, {
				name,
				description: 'Created from the Social Alerts editor.',
				enabled: false,
				status: 'draft',
				trigger_type: 'social_update',
				trigger_config: cfg,
				definition: { steps: [] }
			});
			autos = [
				...autos,
				{ id: r.id, name, description: '', enabled: false, status: 'draft', trigger_type: 'social_update', trigger_config: cfg }
			];
			connectOpen = false;
		} catch (e) {
			connectErr = e instanceof Error ? e.message : 'Could not create the automation';
		} finally {
			creatingAuto = false;
		}
	}

	const active = $derived(kinds[activeKind]);
	// A live "is the composed message empty" read so the default-announcement
	// hint appears/disappears as you type.
	const activeEmpty = $derived.by(() => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const s = active?.step.spec as any;
		return !s?.content?.trim() && !(s?.embeds ?? []).length && !(s?.components ?? []).length;
	});
	const Icon = $derived(providerIcon(provider));
</script>

<svelte:window onkeydown={onKeydown} />

{#if open}
	<div class="fixed inset-0 z-[70] grid place-items-center p-3 sm:p-6">
		<button
			type="button"
			class="absolute inset-0 h-full w-full cursor-default bg-black/40"
			onclick={guardClose}
			transition:fade={{ duration: dur(150) }}
			aria-label="Close"
		></button>
		<div
			class="relative flex max-h-[92vh] w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-line bg-surface shadow-2xl"
			transition:scale={{ duration: dur(200), start: 0.97, opacity: 0, easing: cubicOut }}
			role="dialog"
			aria-label={creating ? 'Follow an account' : 'Edit subscription'}
		>
			<!-- Header -->
			<div class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-4">
				<span
					class="grid size-5 place-items-center rounded border border-line bg-bg"
					style="color: {providerColor(provider)}"
				>
					<Icon size={11} />
				</span>
				<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
					{cap?.name ?? provider}
				</span>
				<div class="h-4 w-px bg-line"></div>
				<span class="min-w-0 truncate text-[12.5px] font-medium text-ink">
					{creating ? 'Follow an account' : sub?.account_name}
				</span>
				<button
					type="button"
					onclick={guardClose}
					class="ml-auto grid size-7 place-items-center rounded-md text-muted transition-colors hover:bg-bg hover:text-ink"
					aria-label="Close"
				>
					<X size={14} />
				</button>
			</div>

			<!-- Body -->
			<div class="min-h-0 flex-1 overflow-y-auto px-4 py-4 sm:px-5">
				<!-- Delivery -->
				<div class="grid gap-x-8 lg:grid-cols-2">
					<div class="min-w-0">
						{#if creating}
							<Field label={cap?.input ?? 'Account'} hint="Resolved and validated when you save.">
								<!-- svelte-ignore a11y_autofocus -->
								<input type="text" bind:value={account} placeholder={cap?.input ?? ''} class="input w-full" autofocus />
							</Field>
						{/if}
						<Field label="Announce in" hint="New activity posts to this channel.">
							<ChannelPicker value={channel} onChange={(v) => (channel = v as string)} />
						</Field>
					</div>
					<div class="min-w-0">
						<Field label="Ping role" hint="Optional role mentioned with each announcement.">
							<RolePicker
								includeManaged
								value={pingRole}
								onChange={(v) => (pingRole = v as string)}
								placeholder="No ping"
							/>
						</Field>
					</div>
				</div>

				<!-- Events -->
				<div class="mt-2 border-t border-line/60 pt-4">
					<div class="mb-3 flex flex-wrap items-center gap-1">
						<span class="mr-2 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
							Events
						</span>
						{#each kindList as k (k)}
							{@const st = kinds[k]}
							<button
								type="button"
								onclick={() => (activeKind = k)}
								class="inline-flex h-6 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium transition-colors {activeKind === k
									? 'bg-bg text-ink'
									: 'text-muted hover:bg-bg/60 hover:text-ink'}"
							>
								<span class="size-1.5 rounded-full {st?.enabled ? 'bg-success' : 'bg-faint/40'}"></span>
								{SOCIAL_KINDS[k]?.label ?? k}
							</button>
						{/each}
					</div>

					{#if active}
						{#key activeKind}
							<div in:fly={{ x: 14, duration: dur(200), easing: cubicOut }}>
								<div class="flex items-center justify-between gap-4 border-b border-line/60 pb-3">
									<div class="min-w-0">
										<div class="text-[12.5px] font-medium text-ink">
											Announce · {SOCIAL_KINDS[activeKind]?.label ?? activeKind}
										</div>
										<div class="mt-0.5 text-[12px] text-muted">
											{SOCIAL_KINDS[activeKind]?.hint ?? ''}
										</div>
									</div>
									<div class="flex shrink-0 items-center gap-3">
										<button
											type="button"
											onclick={resetKind}
											class="font-mono text-[10px] text-faint transition-colors hover:text-ink"
											title="Put this event's message back to the standard announcement"
										>
											Reset to default
										</button>
										<Toggle bind:checked={active.enabled} label="Announce this event" />
									</div>
								</div>

								<div class="pt-3">
									{#if !active.enabled}
										<div class="mb-2 flex items-center gap-2 rounded-md border border-line bg-bg px-3 py-2 text-[11.5px] text-muted">
											<span class="size-1.5 shrink-0 rounded-full bg-faint/40"></span>
											Announcements for this event are off. The message saves either way, so you can
											prepare it and flip the toggle when ready.
										</div>
									{/if}
									<MessageEditor step={active.step} embeds components clickPaths={false} buttonExtras={buttonAction} />
									{#if activeEmpty}
										<div class="mt-2 flex items-start gap-2 rounded-lg border border-line bg-bg px-3 py-2">
											<Sparkles size={12} class="mt-0.5 shrink-0 text-faint" />
											<p class="text-[11.5px] leading-relaxed text-muted">
												Left empty, Dia posts the standard announcement.
												{#if legacyEmbed}With a rich card.{:else}As a bare link.{/if}
												<button
													type="button"
													class="text-accent-ink hover:underline"
													onclick={() => (legacyEmbed = !legacyEmbed)}
												>
													Switch to {legacyEmbed ? 'bare link' : 'rich card'}
												</button>
											</p>
										</div>
									{/if}
								</div>

								<div class="mt-4 border-t border-line/60 pt-3">
									<div class="mb-2 flex items-center gap-1.5">
										<Zap size={11} class="text-faint" />
										<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
											Connections
										</span>
										<span class="text-[11px] text-faint">
											· flows scoped to this account's events; applies instantly
										</span>
									</div>

									<!-- Event node -->
									<div class="flex items-center gap-2">
										<span
											class="grid size-6 shrink-0 place-items-center rounded-md border border-line bg-bg"
											style="color: {providerColor(provider)}"
										>
											<Icon size={12} />
										</span>
										<span class="text-[12px] font-medium text-ink">
											{SOCIAL_KINDS[activeKind]?.label ?? activeKind}
										</span>
										<span class="font-mono text-[9.5px] uppercase tracking-[0.12em] text-faint">event</span>
									</div>

									<!-- Connection tree -->
									<div class="ml-3 border-l border-line pl-0">
										{#each connections as a (a.id)}
											{@const ks = a.trigger_config?.kinds ?? []}
											<div class="group/conn relative flex items-center gap-2 py-1.5 pl-4">
												<span class="absolute left-0 top-1/2 h-px w-3 bg-line"></span>
												<span
													class="size-1.5 shrink-0 rounded-full {a.enabled ? 'bg-success' : 'bg-faint/40'}"
													title={a.enabled ? 'Enabled' : a.status === 'draft' ? 'Draft, finish it on the canvas' : 'Disabled, it will not run'}
												></span>
												<span class="min-w-0 truncate text-[12px] text-ink">{a.name}</span>
												{#if a.status === 'draft'}
													<span class="font-mono text-[9px] uppercase tracking-[0.12em] text-faint">draft</span>
												{:else if !a.enabled}
													<span class="font-mono text-[9px] uppercase tracking-[0.12em] text-faint">off</span>
												{/if}
												{#if ks.length}
													<span class="shrink-0 font-mono text-[9px] text-faint">
														{ks.map((k) => SOCIAL_KINDS[k]?.label ?? k).join(' · ')}
													</span>
												{/if}
												<span class="ml-auto flex shrink-0 items-center gap-0.5 opacity-0 transition-opacity group-hover/conn:opacity-100">
													<a
														href={`/servers/${guildId}/automations/${a.id}`}
														target="_blank"
														rel="noreferrer"
														class="grid size-6 place-items-center rounded-md text-muted hover:bg-bg hover:text-ink"
														title="Open on the automations canvas (new tab)"
													>
														<ArrowUpRight size={12} />
													</a>
													<button
														type="button"
														disabled={connectBusy === a.id}
														onclick={() => disconnect(a.id)}
														class="grid size-6 place-items-center rounded-md text-muted hover:bg-bg hover:text-danger disabled:opacity-50"
														aria-label="Disconnect automation"
													>
														{#if connectBusy === a.id}<Loader2 size={12} class="animate-spin" />{:else}<X size={12} />{/if}
													</button>
												</span>
											</div>
										{/each}
										{#each anyAccount as a (a.id)}
											<div class="relative flex items-center gap-2 py-1.5 pl-4 opacity-70">
												<span class="absolute left-0 top-1/2 h-px w-3 bg-line"></span>
												<span class="size-1.5 shrink-0 rounded-full {a.enabled ? 'bg-success' : 'bg-faint/40'}"></span>
												<span class="min-w-0 truncate text-[12px] text-muted">{a.name}</span>
												<span class="shrink-0 font-mono text-[9px] uppercase tracking-[0.12em] text-faint">any account</span>
												<a
													href={`/servers/${guildId}/automations/${a.id}`}
													target="_blank"
													rel="noreferrer"
													class="ml-auto grid size-6 shrink-0 place-items-center rounded-md text-muted hover:bg-bg hover:text-ink"
													title="Applies to every followed account. Open the canvas to rescope it."
												>
													<ArrowUpRight size={12} />
												</a>
											</div>
										{/each}

										<!-- Connect -->
										<div class="relative py-1.5 pl-4">
											<span class="absolute left-0 top-1/2 h-px w-3 bg-line"></span>
											{#if !sub}
												<p class="text-[11px] text-faint">
													Follow the account first, then connect automations to its events here.
												</p>
											{:else}
											<Popover.Root bind:open={connectOpen}>
												<Popover.Trigger
													class="inline-flex h-6 items-center gap-1.5 rounded-md border border-dashed border-line px-2 text-[11.5px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink"
												>
													<Plus size={11} /> Connect automation
												</Popover.Trigger>
												<Popover.Content class="w-80 p-0" align="start">
													<div class="border-b border-border p-2">
														<input
															type="text"
															bind:value={connectQuery}
															placeholder="Search automations…"
															class="h-7 w-full rounded-md border border-input bg-background px-2 text-[12px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
														/>
													</div>
													<div class="max-h-56 overflow-y-auto p-1">
														{#if !autos.length}
															<p class="px-2 py-3 text-center text-[11.5px] text-muted-foreground">
																No saved automations yet.
															</p>
														{:else if !connectable.length}
															<p class="px-2 py-3 text-center text-[11.5px] text-muted-foreground">
																{connectQuery ? 'No matches.' : 'Everything is already connected.'}
															</p>
														{:else}
															{#each connectable as a (a.id)}
																<button
																	type="button"
																	onclick={() => connect(a.id)}
																	class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-secondary"
																>
																	<span class="size-1.5 shrink-0 rounded-full {a.enabled ? 'bg-success' : 'bg-faint/40'}"></span>
																	<span class="min-w-0 flex-1 truncate text-[12px] text-foreground">{a.name}</span>
																	<span class="shrink-0 font-mono text-[9px] uppercase tracking-[0.1em] text-muted-foreground">
																		{a.status === 'draft' ? 'draft' : a.enabled ? 'on' : 'off'}
																	</span>
																</button>
															{/each}
														{/if}
													</div>
													<div class="border-t border-border p-1">
														<button
															type="button"
															disabled={creatingAuto}
															onclick={createAndConnect}
															class="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-[12px] font-medium text-foreground transition-colors hover:bg-secondary disabled:opacity-50"
														>
															{#if creatingAuto}<Loader2 size={12} class="animate-spin" />{:else}<Plus size={12} />{/if}
															New automation for this event
														</button>
													</div>
												</Popover.Content>
											</Popover.Root>
											{/if}
											{#if connectErr}
												<p class="mt-1 flex items-center gap-1 text-[10.5px] text-danger">
													<TriangleAlert size={11} />
													{connectErr}
												</p>
											{/if}
										</div>
									</div>
								</div>
							</div>
						{/key}
					{/if}
				</div>
			</div>

			<!-- Footer -->
			<div class="flex h-12 shrink-0 items-center gap-1.5 border-t border-line px-3">
				{#if saveErr}
					<span class="inline-flex min-w-0 items-center gap-1 truncate text-[11px] text-danger" title={saveErr}>
						<TriangleAlert size={12} class="shrink-0" />
						{saveErr}
					</span>
				{/if}
				<div class="ml-auto flex items-center gap-1.5">
					<button
						type="button"
						onclick={guardClose}
						class="h-7 rounded-md border border-line bg-bg px-2.5 text-[12px] font-medium text-ink hover:border-line-strong"
					>
						Cancel
					</button>
					<button
						type="button"
						onclick={save}
						disabled={saving}
						class="inline-flex h-7 items-center gap-1.5 rounded-lg bg-ink px-3 text-[12px] font-medium text-bg transition-opacity hover:opacity-90 disabled:opacity-50"
					>
						{#if saving}<Loader2 size={12} class="animate-spin" />{/if}
						{creating ? 'Follow account' : 'Save changes'}
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

{#snippet buttonAction({ component }: { component: { custom_id_suffix?: string; style?: string; url?: string }; ri: number; ci: number })}
	{@const suffix = component.custom_id_suffix}
	{#if suffix && component.style !== 'link' && !component.url}
		<div class="mt-2 space-y-1.5">
			<span class="font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-faint">
				On click
			</span>
			<AutomationPicker
				value={kinds[activeKind]?.buttonActions[suffix] ?? ''}
				onChange={(v) => {
					const st = kinds[activeKind];
					if (st) st.buttonActions[suffix] = v;
				}}
			/>
			<p class="text-[10.5px] leading-snug text-faint">
				Runs a saved automation. Make it a Link-style button instead to open a URL.
			</p>
		</div>
	{/if}
{/snippet}

<ConfirmDialog
	bind:open={confirmOpen}
	title="Discard changes?"
	description="You have unsaved changes to this subscription. Discard them, or keep editing?"
	confirmLabel="Discard"
	cancelLabel="Keep editing"
	onconfirm={doClose}
	oncancel={() => (confirmOpen = false)}
/>
