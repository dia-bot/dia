<script lang="ts">
	// The milestone editor popup: one member-count milestone, its cadence, and
	// the automations connected to it. Recurring ("every 100 members") or a
	// one-time target ("at 10,000 members"); each crossing fires a
	// member_milestone event carrying this milestone's id, and the connection
	// board scopes flows to exactly it (mirrors the social editor's board).
	import { getContext } from 'svelte';
	import { fade, scale } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { newMilestoneId, type StatsMilestone, type MilestoneKind } from '$lib/stats';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ConnectionBoard from '$lib/components/automations/ConnectionBoard.svelte';

	import Flag from 'lucide-svelte/icons/flag';
	import X from 'lucide-svelte/icons/x';
	import Zap from 'lucide-svelte/icons/zap';
	import Loader2 from 'lucide-svelte/icons/loader-2';
	import TriangleAlert from 'lucide-svelte/icons/triangle-alert';

	const store = getContext<GuildStore>(GUILD_CTX);

	let {
		open = $bindable(false),
		guildId,
		milestone = null,
		onsave
	}: {
		open?: boolean;
		guildId: string;
		// null = creating a new milestone.
		milestone?: StatsMilestone | null;
		onsave: (m: StatsMilestone) => Promise<void>;
	} = $props();

	const creating = $derived(!milestone);

	let kind = $state<MilestoneKind>('every');
	let value = $state(100);
	let enabled = $state(true);
	let saving = $state(false);
	let saveErr = $state('');
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
		kind = milestone?.kind ?? 'every';
		value = milestone?.value ?? 100;
		enabled = milestone?.enabled ?? true;
		saving = false;
		saveErr = '';
		confirmOpen = false;
		baseline = serialize();
	});

	function serialize(): string {
		return JSON.stringify({ kind, value, enabled });
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

	// A plain sentence describing exactly when this milestone fires, rendered
	// against the server's real member count.
	const explain = $derived.by(() => {
		const v = Math.max(0, Math.floor(value || 0));
		if (v <= 0) return 'Set a member count above zero.';
		const members = store.memberCount;
		if (kind === 'at') {
			if (members >= v) return `Fires once, at ${v.toLocaleString()} members. Already reached (${members.toLocaleString()} now).`;
			return `Fires once, when the server reaches ${v.toLocaleString()} members (${(v - members).toLocaleString()} to go).`;
		}
		const next = (Math.floor(members / v) + 1) * v;
		return `Fires every ${v.toLocaleString()} members: next at ${next.toLocaleString()} (${members.toLocaleString()} now).`;
	});

	async function save() {
		if (saving) return;
		saveErr = '';
		const v = Math.floor(value || 0);
		if (v <= 0) {
			saveErr = 'The member count must be above zero.';
			return;
		}
		saving = true;
		try {
			await onsave({ id: milestone?.id ?? newMilestoneId(), kind, value: v, enabled });
			baseline = serialize();
			doClose();
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Could not save';
		} finally {
			saving = false;
		}
	}

	const eventLabel = $derived.by(() => {
		const v = Math.max(0, Math.floor(value || 0)).toLocaleString();
		return kind === 'at' ? `Reached ${v} members` : `Every ${v} members`;
	});
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
			aria-label={creating ? 'New milestone' : 'Edit milestone'}
		>
			<!-- Header -->
			<div class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-4">
				<span class="grid size-5 place-items-center rounded border border-line bg-bg text-muted">
					<Flag size={11} />
				</span>
				<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Milestone</span>
				<div class="h-4 w-px bg-line"></div>
				<span class="min-w-0 truncate text-[12.5px] font-medium text-ink">
					{creating ? 'New milestone' : eventLabel}
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
				<!-- Cadence -->
				<div class="mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Fires</div>
				<div class="flex flex-wrap items-center gap-2">
					<div class="inline-flex overflow-hidden rounded-md border border-line">
						<button
							type="button"
							aria-pressed={kind === 'every'}
							onclick={() => (kind = 'every')}
							class="h-8 px-3 text-[12px] font-medium transition-colors {kind === 'every' ? 'bg-ink text-bg' : 'bg-bg text-muted hover:text-ink'}"
						>
							Every
						</button>
						<button
							type="button"
							aria-pressed={kind === 'at'}
							onclick={() => (kind = 'at')}
							class="h-8 border-l border-line px-3 text-[12px] font-medium transition-colors {kind === 'at' ? 'bg-ink text-bg' : 'bg-bg text-muted hover:text-ink'}"
						>
							At exactly
						</button>
					</div>
					<input type="number" min="1" step={kind === 'every' ? 50 : 100} bind:value class="input w-28 text-center font-mono text-[12px]" />
					<span class="text-[12.5px] text-ink">members</span>
				</div>
				<p class="mt-2 text-[11.5px] leading-relaxed text-muted">{explain}</p>

				<div class="mt-4 flex items-center justify-between gap-3 border-t border-line/60 pt-3">
					<div class="min-w-0">
						<div class="text-[12.5px] font-medium text-ink">Milestone enabled</div>
						<div class="mt-0.5 text-[11.5px] text-muted">Off silences it; connected flows stop firing for it.</div>
					</div>
					<Toggle bind:checked={enabled} label="Milestone enabled" />
				</div>

				<!-- Connections -->
				<div class="mt-4 border-t border-line/60 pt-3">
					<ConnectionBoard
						{guildId}
						triggerType="member_milestone"
						filterKey="milestones"
						targetId={milestone?.id ?? ''}
						{eventLabel}
						anyLabel="any milestone"
						newName={`Celebrate · ${eventLabel}`}
						unsavedHint="Save the milestone first, then connect automations to it here."
					>
						{#snippet icon()}<Flag size={12} />{/snippet}
					</ConnectionBoard>
					<p class="mt-2 flex items-center gap-1.5 text-[11px] text-faint">
						<Zap size={11} class="shrink-0" />
						The built-in Member milestone flow also runs for every milestone.
						<a href={`/servers/${guildId}/automations/stats.milestone`} target="_blank" rel="noreferrer" class="text-accent-ink hover:underline">
							Open it →
						</a>
					</p>
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
						{creating ? 'Add milestone' : 'Save changes'}
					</button>
				</div>
			</div>
		</div>
	</div>
{/if}

<ConfirmDialog
	bind:open={confirmOpen}
	title="Discard changes?"
	description="You have unsaved changes to this milestone. Discard them, or keep editing?"
	confirmLabel="Discard"
	cancelLabel="Keep editing"
	onconfirm={doClose}
	oncancel={() => (confirmOpen = false)}
/>
