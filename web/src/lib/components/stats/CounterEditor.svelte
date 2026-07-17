<script lang="ts">
	// The counter editor popup: everything about one stats channel in one place.
	// A live channel-row preview up top (rendered against the server's real
	// numbers), the name template with presets and variable chips, then where it
	// lives. Follows the subscription editor's modal shell and unsaved-changes
	// guard; saving is delegated to the page via onsave.
	import { getContext } from 'svelte';
	import { fade, scale } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import { COUNTER_PRESETS, STATS_TEMPLATE_VARS, newCounterId, type StatsCounter } from '$lib/stats';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import Field from '$lib/components/Field.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';

	import BarChart3 from 'lucide-svelte/icons/bar-chart-3';
	import Volume2 from 'lucide-svelte/icons/volume-2';
	import X from 'lucide-svelte/icons/x';
	import Plus from 'lucide-svelte/icons/plus';
	import Loader2 from 'lucide-svelte/icons/loader-2';
	import TriangleAlert from 'lucide-svelte/icons/triangle-alert';

	const store = getContext<GuildStore>(GUILD_CTX);

	let {
		open = $bindable(false),
		guildId,
		counter = null,
		milestones = { last: 0, next: 0 },
		onsave
	}: {
		open?: boolean;
		guildId: string;
		// null = creating a new counter.
		counter?: StatsCounter | null;
		// Real milestone window for the preview ({{ .Milestone }} / {{ .NextMilestone }}).
		milestones?: { last: number; next: number };
		onsave: (c: StatsCounter) => Promise<void>;
	} = $props();

	const creating = $derived(!counter);

	let template = $state('');
	let channel = $state('');
	let enabled = $state(true);
	let saving = $state(false);
	let saveErr = $state('');
	let creatingChannel = $state(false);
	let baseline = $state('');
	let confirmOpen = $state(false);

	let seeded = false;
	$effect(() => {
		if (!open) {
			seeded = false;
			return;
		}
		if (seeded) return;
		seeded = true;
		template = counter?.template ?? COUNTER_PRESETS[0].template;
		channel = counter?.channel_id ?? '';
		enabled = counter?.enabled ?? true;
		saving = false;
		saveErr = '';
		confirmOpen = false;
		baseline = serialize();
	});

	function serialize(): string {
		return JSON.stringify({ template, channel, enabled });
	}
	function guardClose() {
		if (serialize() === baseline) {
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

	// preview renders the template against the server's real numbers so the
	// channel row reads exactly like it will in Discord; the worker renders the
	// real template.
	const preview = $derived.by(() => {
		const out = template
			.replace(/\{\{\s*\.Members\s*\}\}/g, store.memberCount.toLocaleString())
			.replace(/\{\{\s*\.Channels\s*\}\}/g, String(store.channels.length))
			.replace(/\{\{\s*\.Roles\s*\}\}/g, String(store.roles.length))
			.replace(/\{\{\s*\.Milestone\s*\}\}/g, milestones.last.toLocaleString())
			.replace(/\{\{\s*\.NextMilestone\s*\}\}/g, milestones.next.toLocaleString())
			.replace(/\{\{\s*\.Guild\.Name\s*\}\}/g, store.name);
		return out.slice(0, 100);
	});

	function insertVar(path: string) {
		template = template + `{{ ${path} }}`;
	}

	async function createChannel() {
		if (creatingChannel) return;
		creatingChannel = true;
		saveErr = '';
		try {
			const r = await api.createStatsChannel(guildId, preview || 'server stats');
			channel = r.channel_id;
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Could not create the channel';
		} finally {
			creatingChannel = false;
		}
	}

	async function save() {
		if (saving) return;
		saveErr = '';
		if (!template.trim()) {
			saveErr = 'Write a channel name template.';
			return;
		}
		if (!channel) {
			saveErr = 'Pick or create the channel to rename.';
			return;
		}
		saving = true;
		try {
			await onsave({
				id: counter?.id ?? newCounterId(),
				channel_id: channel,
				template: template.trim(),
				enabled
			});
			baseline = serialize();
			doClose();
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Could not save';
		} finally {
			saving = false;
		}
	}
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
			class="relative flex max-h-[92vh] w-full max-w-xl flex-col overflow-hidden rounded-xl border border-line bg-surface shadow-2xl"
			transition:scale={{ duration: dur(200), start: 0.97, opacity: 0, easing: cubicOut }}
			role="dialog"
			aria-label={creating ? 'New counter' : 'Edit counter'}
		>
			<!-- Header -->
			<div class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-4">
				<span class="grid size-5 place-items-center rounded border border-line bg-bg text-muted">
					<BarChart3 size={11} />
				</span>
				<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Counter</span>
				<div class="h-4 w-px bg-line"></div>
				<span class="min-w-0 truncate text-[12.5px] font-medium text-ink">
					{creating ? 'New counter' : 'Edit counter'}
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
				<!-- Live preview, styled like the Discord channel row it becomes -->
				<div class="mb-4 rounded-lg border border-line bg-ink-2 px-3 py-2.5">
					<div class="mb-1.5 font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-faint">
						Live preview
					</div>
					<div class="flex items-center gap-2 rounded-md px-1 py-0.5">
						<Volume2 size={15} class="shrink-0 text-muted" />
						<span class="min-w-0 truncate text-[13.5px] font-medium text-ink">
							{preview || 'Channel name'}
						</span>
					</div>
				</div>

				<Field label="Channel name template" hint="Rendered live; renames apply at most twice per 10 minutes (Discord limit).">
					<input type="text" bind:value={template} class="input w-full font-mono text-[12px]" placeholder={COUNTER_PRESETS[0].template} />
				</Field>
				<div class="-mt-2 mb-4 flex flex-wrap items-center gap-1">
					{#each COUNTER_PRESETS as p (p.label)}
						<button
							type="button"
							onclick={() => (template = p.template)}
							class="inline-flex h-6 items-center rounded-md border border-line bg-bg px-2 text-[11px] font-medium text-muted hover:border-line-strong hover:text-ink"
						>
							{p.label}
						</button>
					{/each}
					<span class="mx-1 h-3.5 w-px bg-line"></span>
					{#each STATS_TEMPLATE_VARS as tv (tv.path)}
						<button
							type="button"
							onclick={() => insertVar(tv.path)}
							class="inline-flex h-6 items-center rounded border border-line bg-surface px-1.5 font-mono text-[10px] text-muted hover:border-line-strong hover:text-ink"
							title={tv.short}
						>
							{`{{ ${tv.path} }}`}
						</button>
					{/each}
				</div>

				<Field label="Channel" hint="Dia renames this channel; a locked voice channel works best.">
					<div class="flex items-center gap-2">
						<div class="min-w-0 flex-1">
							<ChannelPicker kind="all" value={channel} onChange={(v) => (channel = v as string)} placeholder="Pick a channel" />
						</div>
						<button
							type="button"
							onclick={createChannel}
							disabled={creatingChannel}
							class="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md border border-line bg-bg px-2.5 text-[11.5px] font-medium text-muted hover:border-line-strong hover:text-ink disabled:opacity-50"
							title="Create a locked voice channel for this counter"
						>
							{#if creatingChannel}<Loader2 size={12} class="animate-spin" />{:else}<Plus size={12} />{/if}
							Create for me
						</button>
					</div>
				</Field>

				<div class="flex items-center justify-between gap-3 border-t border-line/60 pt-3">
					<div class="min-w-0">
						<div class="text-[12.5px] font-medium text-ink">Counter enabled</div>
						<div class="mt-0.5 text-[11.5px] text-muted">Off pauses renames; the channel keeps its last name.</div>
					</div>
					<Toggle bind:checked={enabled} label="Counter enabled" />
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
						{creating ? 'Add counter' : 'Save changes'}
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

<ConfirmDialog
	bind:open={confirmOpen}
	title="Discard changes?"
	description="You have unsaved changes to this counter. Discard them, or keep editing?"
	confirmLabel="Discard"
	cancelLabel="Keep editing"
	onconfirm={doClose}
	oncancel={() => (confirmOpen = false)}
/>
