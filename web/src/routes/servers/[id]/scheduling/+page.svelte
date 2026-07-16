<script lang="ts">
	import { onMount, getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import { cadenceLabel, type ScheduledMessage } from '$lib/schedules';
	import Toggle from '$lib/components/Toggle.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import PageTopbar from '$lib/components/page/PageTopbar.svelte';
	import SectionBar from '$lib/components/page/SectionBar.svelte';
	import EmptyBlock from '$lib/components/page/EmptyBlock.svelte';
	import ScheduleEditor from '$lib/components/scheduling/ScheduleEditor.svelte';
	import { CalendarClock, Plus, Pencil, Trash2, Send, Zap, Loader2 } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const FEATURE = 'scheduler';

	let enabled = $state(false);
	let schedules = $state<ScheduledMessage[]>([]);
	let loaded = $state(false);
	let loadErr = $state('');

	onMount(async () => {
		try {
			const [f, list] = await Promise.all([api.feature(store.id, FEATURE), api.schedules(store.id)]);
			enabled = f.enabled;
			schedules = list.schedules ?? [];
			loaded = true;
		} catch (e) {
			loadErr = e instanceof Error ? e.message : 'Failed to load schedules';
		}
	});

	async function toggleFeature(v: boolean) {
		try {
			await api.saveFeature(store.id, FEATURE, v, {});
		} catch {
			enabled = !v;
		}
	}

	let editorOpen = $state(false);
	let editorSched = $state<ScheduledMessage | null>(null);
	function openAdd() {
		editorSched = null;
		editorOpen = true;
	}
	function openEdit(s: ScheduledMessage) {
		editorSched = s;
		editorOpen = true;
	}
	function onCreated(s: ScheduledMessage) {
		schedules = [...schedules, s];
		if (!enabled && schedules.length === 1) enabled = true;
	}
	function onSaved(s: ScheduledMessage) {
		schedules = schedules.map((x) => (x.id === s.id ? s : x));
	}

	async function toggleSched(s: ScheduledMessage, v: boolean) {
		const prev = s.enabled;
		s.enabled = v;
		try {
			const r = await api.updateSchedule(store.id, s.id, { enabled: v });
			schedules = schedules.map((x) => (x.id === r.schedule.id ? r.schedule : x));
		} catch {
			s.enabled = prev;
		}
	}

	let sendingId = $state('');
	let sentId = $state('');
	async function sendNow(s: ScheduledMessage) {
		if (sendingId) return;
		sendingId = s.id;
		sentId = '';
		try {
			await api.sendSchedule(store.id, s.id);
			sentId = s.id;
			setTimeout(() => (sentId = ''), 2000);
		} catch {
			/* the channel simply didn't get a message */
		} finally {
			sendingId = '';
		}
	}

	let confirmDelete = $state<ScheduledMessage | null>(null);
	async function doDelete() {
		const s = confirmDelete;
		if (!s) return;
		try {
			await api.deleteSchedule(store.id, s.id);
			schedules = schedules.filter((x) => x.id !== s.id);
		} catch {
			/* keep the row on failure */
		}
	}

	function channelName(id: string): string {
		return store.channels.find((c) => c.id === id)?.name ?? id;
	}
	function nextLabel(s: ScheduledMessage): string {
		if (!s.enabled) return 'paused';
		if (!s.next_run_at) return 'finished';
		const diff = s.next_run_at - Date.now();
		if (diff <= 0) return 'due now';
		const m = Math.round(diff / 60000);
		if (m < 60) return `in ${m}m`;
		if (m < 1440) return `in ${Math.round(m / 60)}h`;
		return `in ${Math.round(m / 1440)}d`;
	}
</script>

<svelte:head><title>Scheduling · {store.name} · Dia</title></svelte:head>

<div class="relative flex h-full flex-col bg-bg text-ink">
	<PageTopbar eyebrow="Scheduling" subtitle="Post composed messages on a schedule: announcements, reminders, recurring events.">
		{#snippet leading()}
			<div class="grid size-6 place-items-center rounded border border-line bg-surface text-muted">
				<CalendarClock size={13} />
			</div>
		{/snippet}
		{#snippet actions()}
			<a
				href={`/servers/${store.id}/automations/scheduler.sent`}
				class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line bg-bg px-2.5 text-[12px] font-medium text-muted transition-colors hover:border-line-strong hover:text-ink"
				title="Open the built-in post flow on the automations canvas"
			>
				<Zap size={13} /> Advanced
			</a>
			<button
				type="button"
				onclick={openAdd}
				class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90"
			>
				<Plus size={13} /> New schedule
			</button>
			<label class="ml-1 flex items-center gap-2 text-[12px]">
				<span class="hidden text-muted sm:inline">{enabled ? 'On' : 'Off'}</span>
				<Toggle bind:checked={enabled} label="Scheduling" onchange={toggleFeature} />
			</label>
		{/snippet}
	</PageTopbar>

	<div class="relative min-h-0 flex-1 overflow-y-auto bg-bg pb-16">
		{#if loadErr}
			<div class="px-5 py-8 text-[12.5px] text-danger">{loadErr}</div>
		{:else if !loaded}
			<div class="p-6"><div class="skeleton h-40 w-full rounded"></div></div>
		{:else}
			<SectionBar label="Schedules" count={`${schedules.length}`} />
			{#if !enabled && schedules.length}
				<div class="flex items-center gap-2 border-b border-line/60 px-5 py-3 text-[12px] text-muted">
					<span class="size-1.5 shrink-0 rounded-full bg-faint/40"></span>
					Scheduling is off. Turn it on, top-right, to start posting.
				</div>
			{/if}
			{#if schedules.length === 0}
				<EmptyBlock
					title="Nothing scheduled yet"
					body="Compose a message once and Dia posts it on time: weekly events, daily reminders, one-off announcements."
				>
					{#snippet cta()}
						<button
							type="button"
							onclick={openAdd}
							class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90"
						>
							<Plus size={13} /> Schedule your first message
						</button>
					{/snippet}
				</EmptyBlock>
			{:else}
				{#each schedules as s (s.id)}
					<div class="flex items-center gap-3 border-b border-line/60 px-5 py-3.5 {s.enabled ? '' : 'opacity-55'}">
						<span class="grid size-8 shrink-0 place-items-center rounded-md border border-line bg-surface text-accent-ink">
							<CalendarClock size={15} />
						</span>
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<span class="truncate text-[12.5px] font-medium text-ink">{s.name}</span>
								<span class="shrink-0 font-mono text-[10px] text-faint">{nextLabel(s)}</span>
							</div>
							<div class="mt-0.5 truncate text-[11.5px] text-muted">
								{cadenceLabel(s.schedule)} → #{channelName(s.channel_id)}
							</div>
						</div>
						<button
							type="button"
							onclick={() => sendNow(s)}
							disabled={!!sendingId}
							class="hidden h-7 items-center gap-1.5 rounded-md border border-line bg-bg px-2 text-[11.5px] font-medium text-muted hover:border-line-strong hover:text-ink disabled:opacity-50 sm:inline-flex"
							title="Post it now (the timer is untouched)"
						>
							{#if sendingId === s.id}<Loader2 size={12} class="animate-spin" />{:else}<Send size={12} />{/if}
							{sentId === s.id ? 'Sent' : 'Send now'}
						</button>
						<button
							type="button"
							onclick={() => openEdit(s)}
							class="grid h-7 w-7 place-items-center rounded-md border border-line bg-bg text-muted hover:border-line-strong hover:text-ink"
							aria-label="Edit schedule"
						>
							<Pencil size={12} />
						</button>
						<button
							type="button"
							onclick={() => (confirmDelete = s)}
							class="grid h-7 w-7 place-items-center rounded-md border border-line bg-bg text-muted hover:border-line-strong hover:text-danger"
							aria-label="Delete schedule"
						>
							<Trash2 size={12} />
						</button>
						<Toggle checked={s.enabled} label="Schedule enabled" onchange={(v) => toggleSched(s, v)} />
					</div>
				{/each}
			{/if}
		{/if}
	</div>
</div>

<ScheduleEditor bind:open={editorOpen} guildId={store.id} sched={editorSched} oncreated={onCreated} onsaved={onSaved} />

<ConfirmDialog
	open={!!confirmDelete}
	title="Delete {confirmDelete?.name ?? 'this schedule'}?"
	description="It stops posting immediately. This can't be undone."
	confirmLabel="Delete"
	onconfirm={doDelete}
	oncancel={() => (confirmDelete = null)}
/>
