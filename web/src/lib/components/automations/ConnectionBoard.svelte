<script lang="ts">
	// A derived connection board: "which saved automations run for this thing".
	// A connection IS the automation's own trigger scoping (this target's id in
	// trigger_config[filterKey]); connect/disconnect edit that automation
	// directly and apply instantly, so exactly one runner (the automations
	// dispatcher) ever fires. Generic over the scoping filter so social
	// accounts, milestones and schedules all share one board.
	import type { Snippet } from 'svelte';
	import { api } from '$lib/api';
	import type { AutomationSummary, TriggerConfig } from '$lib/automations/types';
	import { Popover } from '$lib/components/ui';

	import Zap from 'lucide-svelte/icons/zap';
	import X from 'lucide-svelte/icons/x';
	import Plus from 'lucide-svelte/icons/plus';
	import ArrowUpRight from 'lucide-svelte/icons/arrow-up-right';
	import Loader2 from 'lucide-svelte/icons/loader-2';
	import TriangleAlert from 'lucide-svelte/icons/triangle-alert';

	type ScopeKey = 'subscriptions' | 'milestones' | 'schedules';

	let {
		guildId,
		triggerType,
		filterKey,
		targetId,
		eventLabel,
		anyLabel,
		newName,
		unsavedHint = 'Save first, then connect automations to it here.',
		icon
	}: {
		guildId: string;
		triggerType: string;
		filterKey: ScopeKey;
		// Empty while the parent object is unsaved; the board shows a hint.
		targetId: string;
		eventLabel: string;
		// Chip shown on unscoped automations that fire for every target.
		anyLabel: string;
		// Name given to an automation minted from "New automation for this event".
		newName: string;
		unsavedHint?: string;
		icon?: Snippet;
	} = $props();

	let autos = $state<AutomationSummary[]>([]);
	$effect(() => {
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

	function scoped(a: AutomationSummary): string[] {
		return (a.trigger_config?.[filterKey] ?? []) as string[];
	}
	const matching = $derived(autos.filter((a) => a.trigger_type === triggerType));
	const connections = $derived(targetId ? matching.filter((a) => scoped(a).includes(targetId)) : []);
	// Unscoped automations fire for every target, this one included; shown as
	// ambient rows so the board never lies by omission.
	const anyTarget = $derived(matching.filter((a) => !scoped(a).length));
	const connectable = $derived.by(() => {
		const connected = new Set(connections.map((a) => a.id));
		const ambient = new Set(anyTarget.map((a) => a.id));
		const q = connectQuery.trim().toLowerCase();
		return matching.filter(
			(a) => !connected.has(a.id) && !ambient.has(a.id) && (!q || a.name.toLowerCase().includes(q))
		);
	});

	// saveScope rewrites one automation's trigger scoping (fetch full, patch
	// config, upsert) and mirrors the change into the local catalogue.
	async function saveScope(id: string, patch: (cfg: TriggerConfig) => void, extra?: Record<string, unknown>) {
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
		if (!targetId) return;
		const tid = targetId;
		await saveScope(id, (cfg) => {
			cfg[filterKey] = [...new Set([...((cfg[filterKey] ?? []) as string[]), tid])];
		});
		connectOpen = false;
		connectQuery = '';
	}

	async function disconnect(id: string) {
		if (!targetId) return;
		const tid = targetId;
		await saveScope(id, (cfg) => {
			const rest = ((cfg[filterKey] ?? []) as string[]).filter((x) => x !== tid);
			cfg[filterKey] = rest.length ? rest : undefined;
		});
		// An automation whose only target was this one would silently broaden to
		// every target once unscoped; park it disabled instead.
		const a = autos.find((x) => x.id === id);
		if (a && !scoped(a).length && a.enabled) {
			await saveScope(id, () => {}, { enabled: false });
		}
	}

	// createAndConnect mints a draft automation pre-scoped to this target;
	// building the flow happens on the canvas (the row links there).
	async function createAndConnect() {
		if (creatingAuto || !targetId) return;
		creatingAuto = true;
		connectErr = '';
		try {
			const cfg: TriggerConfig = { [filterKey]: [targetId] };
			const r = await api.upsertAutomation(guildId, {
				name: newName,
				description: 'Created from a connection board.',
				enabled: false,
				status: 'draft',
				trigger_type: triggerType,
				trigger_config: cfg,
				definition: { steps: [] }
			});
			autos = [
				...autos,
				{ id: r.id, name: newName, description: '', enabled: false, status: 'draft', trigger_type: triggerType, trigger_config: cfg }
			];
			connectOpen = false;
		} catch (e) {
			connectErr = e instanceof Error ? e.message : 'Could not create the automation';
		} finally {
			creatingAuto = false;
		}
	}
</script>

<div class="mb-2 flex items-center gap-1.5">
	<Zap size={11} class="text-faint" />
	<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Connections</span>
	<span class="text-[11px] text-faint">· flows scoped to this event; applies instantly</span>
</div>

<!-- Event node -->
<div class="flex items-center gap-2">
	<span class="grid size-6 shrink-0 place-items-center rounded-md border border-line bg-bg text-accent-ink">
		{#if icon}{@render icon()}{:else}<Zap size={12} />{/if}
	</span>
	<span class="text-[12px] font-medium text-ink">{eventLabel}</span>
	<span class="font-mono text-[9.5px] uppercase tracking-[0.12em] text-faint">event</span>
</div>

<!-- Connection tree -->
<div class="ml-3 border-l border-line pl-0">
	{#each connections as a (a.id)}
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
	{#each anyTarget as a (a.id)}
		<div class="relative flex items-center gap-2 py-1.5 pl-4 opacity-70">
			<span class="absolute left-0 top-1/2 h-px w-3 bg-line"></span>
			<span class="size-1.5 shrink-0 rounded-full {a.enabled ? 'bg-success' : 'bg-faint/40'}"></span>
			<span class="min-w-0 truncate text-[12px] text-muted">{a.name}</span>
			<span class="shrink-0 font-mono text-[9px] uppercase tracking-[0.12em] text-faint">{anyLabel}</span>
			<a
				href={`/servers/${guildId}/automations/${a.id}`}
				target="_blank"
				rel="noreferrer"
				class="ml-auto grid size-6 shrink-0 place-items-center rounded-md text-muted hover:bg-bg hover:text-ink"
				title="Fires for every event of this kind. Open the canvas to rescope it."
			>
				<ArrowUpRight size={12} />
			</a>
		</div>
	{/each}

	<!-- Connect -->
	<div class="relative py-1.5 pl-4">
		<span class="absolute left-0 top-1/2 h-px w-3 bg-line"></span>
		{#if !targetId}
			<p class="text-[11px] text-faint">{unsavedHint}</p>
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
							<p class="px-2 py-3 text-center text-[11.5px] text-muted-foreground">No saved automations yet.</p>
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
