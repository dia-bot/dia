<script lang="ts">
	import { getContext, onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api, ApiError } from '$lib/api';
	import FlowThumb from '$lib/components/commands/FlowThumb.svelte';
	import { Dialog } from '$lib/components/ui';
	import {
		TRIGGERS,
		TRIGGER_BY_KEY,
		TRIGGER_CATEGORIES,
		type AutomationSummary
	} from '$lib/automations/types';

	import Zap from 'lucide-svelte/icons/zap';
	import Plus from 'lucide-svelte/icons/plus';
	import Lock from 'lucide-svelte/icons/lock';
	import CircleAlert from 'lucide-svelte/icons/circle-alert';
	import ArrowRight from 'lucide-svelte/icons/arrow-right';

	const store = getContext<GuildStore>(GUILD_CTX);

	let automations = $state<AutomationSummary[]>([]);
	let builtins = $state<AutomationSummary[]>([]);
	let loaded = $state(false);
	let loadError = $state('');

	let pickerOpen = $state(false);
	let creating = $state(false);
	let createError = $state('');

	const base = $derived(`/servers/${store.id}`);

	async function reload() {
		const r = await api.automations(store.id);
		automations = r.automations ?? [];
		builtins = r.builtins ?? [];
	}

	onMount(() => {
		(async () => {
			try {
				await reload();
			} catch (e) {
				loadError = e instanceof ApiError ? e.message : String(e);
			} finally {
				loaded = true;
			}
		})();
	});

	function triggerLabel(key: string): string {
		return TRIGGER_BY_KEY.get(key)?.label ?? key;
	}

	async function createFrom(triggerKey: string) {
		if (creating) return;
		creating = true;
		createError = '';
		try {
			const meta = TRIGGER_BY_KEY.get(triggerKey);
			const r = await api.upsertAutomation(store.id, {
				name: meta?.label ?? 'New automation',
				description: meta?.description ?? '',
				enabled: false,
				status: 'draft',
				trigger_type: triggerKey,
				trigger_config: {},
				definition: { steps: [] }
			});
			pickerOpen = false;
			await goto(`${base}/automations/${r.id}`);
		} catch (e) {
			createError = e instanceof ApiError ? e.message : String(e);
		} finally {
			creating = false;
		}
	}

	function relTime(iso?: string | null): string {
		if (!iso) return '';
		const diff = (Date.now() - new Date(iso).getTime()) / 1000;
		if (diff < 60) return `${Math.round(diff)}s`;
		if (diff < 3600) return `${Math.round(diff / 60)}m`;
		if (diff < 86400) return `${Math.round(diff / 3600)}h`;
		return `${Math.round(diff / 86400)}d`;
	}

	// Group the trigger catalogue by category for the picker.
	const triggersByCat = $derived(
		TRIGGER_CATEGORIES.map((cat) => ({
			...cat,
			items: TRIGGERS.filter((t) => t.category === cat.id)
		})).filter((g) => g.items.length > 0)
	);
</script>

<svelte:head>
	<title>Automations · {store.name} · Dia</title>
</svelte:head>

<div class="flex h-full flex-col bg-bg text-ink">
	<!-- Topbar -->
	<header class="flex h-12 shrink-0 items-center gap-3 border-b border-line bg-bg px-4">
		<div class="grid size-6 place-items-center rounded border border-line bg-surface text-accent-ink">
			<Zap size={13} />
		</div>
		<span class="font-mono text-[10px] uppercase tracking-[0.16em] text-faint">Automations</span>
		<div class="h-3.5 w-px bg-line"></div>
		<p class="hidden font-mono text-[11px] text-muted sm:block">
			When something happens on your server, run a flow.
		</p>
		<button
			type="button"
			class="ml-auto inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-medium text-bg transition-opacity hover:opacity-90"
			onclick={() => (pickerOpen = true)}
		>
			<Plus size={14} /> New automation
		</button>
	</header>

	<div class="min-h-0 flex-1 overflow-y-auto px-4 py-5">
		<div class="mx-auto max-w-[1100px]">
			{#if !loaded}
				<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
					{#each Array(6) as _, i (i)}
						<div class="skeleton h-[150px] rounded-lg"></div>
					{/each}
				</div>
			{:else if loadError}
				<div class="flex flex-col items-center justify-center py-20 text-center">
					<CircleAlert size={20} class="mb-2 text-danger" />
					<p class="text-[13px] text-ink">{loadError}</p>
				</div>
			{:else}
				<!-- Your automations -->
				<section>
					<div class="mb-2.5 flex items-baseline gap-2">
						<h2 class="font-mono text-[10px] uppercase tracking-[0.16em] text-faint">
							Your automations
						</h2>
						<span class="font-mono text-[10px] tabular-nums text-faint">{automations.length}</span>
					</div>

					{#if automations.length === 0}
						<button
							type="button"
							onclick={() => (pickerOpen = true)}
							class="flex w-full flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-line bg-surface/30 py-12 text-center transition-colors hover:border-line-strong"
						>
							<div class="grid size-9 place-items-center rounded-full border border-line bg-bg text-accent-ink">
								<Zap size={16} />
							</div>
							<p class="text-[13px] font-medium text-ink">Create your first automation</p>
							<p class="max-w-sm font-mono text-[10.5px] text-faint">
								Greet boosters, auto-thread support posts, log deletions, reaction roles, and more.
							</p>
						</button>
					{:else}
						<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
							{#each automations as a (a.id)}
								<a
									href={`${base}/automations/${a.id}`}
									class="group relative flex flex-col gap-2 overflow-hidden rounded-lg border border-line bg-surface/40 p-3 transition-colors hover:border-line-strong"
								>
									<div class="flex items-center gap-2">
										<span
											class="size-1.5 shrink-0 rounded-full {a.enabled
												? 'bg-success'
												: 'bg-faint/40'}"
											title={a.enabled ? 'Enabled' : 'Disabled'}
										></span>
										<span class="truncate text-[13px] font-medium text-ink">{a.name}</span>
										{#if a.status === 'draft'}
											<span class="ml-auto font-mono text-[9px] uppercase tracking-[0.12em] text-faint">
												draft
											</span>
										{/if}
									</div>
									<div
										class="inline-flex w-fit items-center gap-1 rounded border border-line bg-bg px-1.5 py-0.5 font-mono text-[9.5px] uppercase tracking-[0.1em] text-muted"
									>
										<Zap size={9} /> {triggerLabel(a.trigger_type)}
									</div>
									<div class="mt-1 h-[88px] overflow-hidden rounded border border-line/60 bg-bg">
										<FlowThumb shape={a.flow_shape} name={a.name} more={a.shape_more ?? 0} />
									</div>
									<div class="flex items-center gap-2 font-mono text-[9.5px] text-faint">
										<span>{a.step_count ?? 0} steps</span>
										{#if a.runs_24h !== undefined}
											<span>· {a.runs_24h} runs/24h</span>
										{/if}
										{#if a.last_run_at}
											<span class="ml-auto">{relTime(a.last_run_at)}</span>
										{/if}
									</div>
								</a>
							{/each}
						</div>
					{/if}
				</section>

				<!-- Built-in (managed) -->
				{#if builtins.length > 0}
					<section class="mt-8">
						<div class="mb-1 flex items-baseline gap-2">
							<h2 class="font-mono text-[10px] uppercase tracking-[0.16em] text-faint">
								Built-in
							</h2>
							<Lock size={10} class="text-faint" />
						</div>
						<p class="mb-2.5 font-mono text-[10.5px] text-faint">
							Dia ships these. View how they work here, configure them on their own tab.
						</p>
						<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
							{#each builtins as b (b.id)}
								<a
									href={`${base}/automations/${b.id}`}
									class="group relative flex flex-col gap-2 overflow-hidden rounded-lg border border-line bg-ink-2/30 p-3 transition-colors hover:border-line-strong"
								>
									<div class="flex items-center gap-2">
										<span
											class="size-1.5 shrink-0 rounded-full {b.enabled
												? 'bg-success'
												: 'bg-faint/40'}"
											title={b.enabled ? 'On' : 'Off'}
										></span>
										<span class="truncate text-[13px] font-medium text-ink">{b.name}</span>
										<span class="ml-auto inline-flex items-center gap-1 font-mono text-[9px] uppercase tracking-[0.12em] text-faint">
											<Lock size={9} /> managed
										</span>
									</div>
									<div
										class="inline-flex w-fit items-center gap-1 rounded border border-line bg-bg px-1.5 py-0.5 font-mono text-[9.5px] uppercase tracking-[0.1em] text-muted"
									>
										<Zap size={9} /> {triggerLabel(b.trigger_type)}
									</div>
									<div class="mt-1 h-[88px] overflow-hidden rounded border border-line/60 bg-bg">
										<FlowThumb shape={b.flow_shape} name={b.name} more={b.shape_more ?? 0} />
									</div>
									<div class="flex items-center gap-1 font-mono text-[9.5px] text-accent-ink">
										Configure in {b.feature_name}
										<ArrowRight size={10} />
									</div>
								</a>
							{/each}
						</div>
					</section>
				{/if}
			{/if}
		</div>
	</div>
</div>

<!-- Trigger picker dialog -->
<Dialog.Root bind:open={pickerOpen}>
	<Dialog.Content class="flex h-[min(640px,88vh)] max-w-2xl flex-col gap-0 overflow-hidden p-0">
		<Dialog.Title class="sr-only">Choose a trigger</Dialog.Title>
		<div class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-4">
			<div class="grid size-5 place-items-center rounded border border-line bg-surface text-accent-ink">
				<Zap size={11} />
			</div>
			<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
				When this happens…
			</span>
		</div>
		<div class="min-h-0 flex-1 overflow-y-auto p-3">
			{#if createError}
				<p class="mb-2 rounded border border-danger/30 bg-danger/5 px-2 py-1 font-mono text-[11px] text-danger">
					{createError}
				</p>
			{/if}
			{#each triggersByCat as group (group.id)}
				<div class="mb-4">
					<div class="mb-1.5 font-mono text-[9.5px] uppercase tracking-[0.16em] text-faint">
						{group.label}
					</div>
					<div class="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
						{#each group.items as t (t.key)}
							<button
								type="button"
								disabled={creating}
								onclick={() => createFrom(t.key)}
								class="flex flex-col items-start gap-0.5 rounded-md border border-line bg-surface/40 p-2.5 text-left transition-colors hover:border-line-strong hover:bg-surface disabled:opacity-50"
							>
								<span class="text-[12.5px] font-medium text-ink">{t.label}</span>
								<span class="font-mono text-[10px] leading-snug text-faint">{t.description}</span>
							</button>
						{/each}
					</div>
				</div>
			{/each}
		</div>
	</Dialog.Content>
</Dialog.Root>
