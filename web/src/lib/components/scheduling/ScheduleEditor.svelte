<script lang="ts">
	// The schedule editor: one large popup owning a scheduled message end to
	// end. Name + channel + cadence up top, then the composed message on the
	// shared WYSIWYG MessageEditor (the editor doubles as the live preview).
	// Buttons can run saved automations on click. Same shell, animations and
	// unsaved-changes guard as the social subscription editor.
	import { setContext } from 'svelte';
	import { fade, scale } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';
	import { api } from '$lib/api';
	import {
		SCHED_TEMPLATE_VARS,
		WEEKDAYS,
		type SchedMessageSpec,
		type ScheduleDef,
		type ScheduledMessage
	} from '$lib/schedules';
	import type { Step } from '$lib/commands/types';
	import { AUTOMATION_CTX, EXPR_SCOPE_CTX, type ExprScope } from '$lib/commands/expr-meta';
	import MessageEditor from '$lib/components/commands/MessageEditor.svelte';
	import AutomationPicker from '$lib/components/commands/AutomationPicker.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import Field from '$lib/components/Field.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import FieldSelect from '$lib/components/commands/FieldSelect.svelte';

	import CalendarClock from 'lucide-svelte/icons/calendar-clock';
	import X from 'lucide-svelte/icons/x';
	import Loader2 from 'lucide-svelte/icons/loader-2';
	import TriangleAlert from 'lucide-svelte/icons/triangle-alert';

	let {
		open = $bindable(false),
		guildId,
		sched = null,
		oncreated,
		onsaved
	}: {
		open?: boolean;
		guildId: string;
		sched?: ScheduledMessage | null; // null = creating
		oncreated?: (s: ScheduledMessage) => void;
		onsaved?: (s: ScheduledMessage) => void;
	} = $props();

	setContext(AUTOMATION_CTX, false);
	setContext(EXPR_SCOPE_CTX, {
		options: [],
		variables: [],
		steps: [],
		extraVars: SCHED_TEMPLATE_VARS
	} satisfies ExprScope);

	const creating = $derived(!sched);

	let name = $state('');
	let channel = $state('');
	let kind = $state<ScheduleDef['kind']>('weekly');
	let at = $state(''); // datetime-local value (once)
	let everyMinutes = $state(60);
	let timeOfDay = $state('18:00');
	let weekdays = $state<number[]>([5]);
	let step = $state<Step>({ id: 'sched-msg', kind: 'send_message', spec: { content: '' } });
	let buttonActions = $state<Record<string, string>>({});
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
		name = sched?.name ?? '';
		channel = sched?.channel_id ?? '';
		const def = sched?.schedule;
		kind = def?.kind ?? 'weekly';
		at = def?.at ? toLocalInput(def.at) : '';
		everyMinutes = def?.every_minutes ?? 60;
		timeOfDay = def?.time ?? '18:00';
		weekdays = [...(def?.weekdays ?? [5])];
		const msg = structuredClone($state.snapshot(sched?.spec) ?? {}) as SchedMessageSpec;
		step = {
			id: 'sched-msg',
			kind: 'send_message',
			spec: { content: msg.content ?? '', embeds: msg.embeds ?? [], components: msg.components ?? [] }
		};
		buttonActions = { ...(msg.button_actions ?? {}) };
		saving = false;
		saveErr = '';
		confirmOpen = false;
		baseline = serialize();
	});

	function toLocalInput(rfc: string): string {
		const d = new Date(rfc);
		if (isNaN(d.getTime())) return '';
		const pad = (n: number) => String(n).padStart(2, '0');
		return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
	}

	function buildSchedule(): ScheduleDef {
		switch (kind) {
			case 'once':
				return { kind, at: at ? new Date(at).toISOString() : '' };
			case 'every':
				return { kind, every_minutes: everyMinutes };
			case 'daily':
				return { kind, time: timeOfDay };
			default:
				return { kind: 'weekly', time: timeOfDay, weekdays: [...weekdays].sort() };
		}
	}

	function buildSpec(): SchedMessageSpec {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const s = $state.snapshot(step.spec) as any;
		const components = (s?.components ?? []) as NonNullable<SchedMessageSpec['components']>;
		const suffixes = new Set(
			components.flatMap((row) => row.components.map((c) => c.custom_id_suffix).filter(Boolean))
		);
		const actions: Record<string, string> = {};
		for (const [suffix, id] of Object.entries(buttonActions)) {
			if (id && suffixes.has(suffix)) actions[suffix] = id;
		}
		const msg: SchedMessageSpec = {
			content: s?.content ?? '',
			embeds: s?.embeds ?? [],
			components
		};
		if (Object.keys(actions).length) msg.button_actions = actions;
		return msg;
	}

	function serialize(): string {
		return JSON.stringify({ name, channel, schedule: buildSchedule(), spec: buildSpec() });
	}
	function isDirty(): boolean {
		return serialize() !== baseline;
	}
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

	function toggleWeekday(w: number) {
		weekdays = weekdays.includes(w) ? weekdays.filter((x) => x !== w) : [...weekdays, w];
	}

	async function save() {
		if (saving) return;
		saveErr = '';
		if (!channel) {
			saveErr = 'Pick a channel to post in.';
			return;
		}
		const spec = buildSpec();
		if (!spec.content?.trim() && !spec.embeds?.length && !spec.components?.length) {
			saveErr = 'Compose the message to post.';
			return;
		}
		if (kind === 'once' && !at) {
			saveErr = 'Pick a date and time.';
			return;
		}
		if (kind === 'weekly' && weekdays.length === 0) {
			saveErr = 'Pick at least one weekday.';
			return;
		}
		saving = true;
		try {
			const body = {
				name: name.trim() || 'Scheduled message',
				channel_id: channel,
				spec,
				schedule: buildSchedule()
			};
			if (creating) {
				const r = await api.createSchedule(guildId, body);
				baseline = serialize();
				oncreated?.(r.schedule);
			} else if (sched) {
				const r = await api.updateSchedule(guildId, sched.id, { ...body, enabled: sched.enabled });
				baseline = serialize();
				onsaved?.(r.schedule);
			}
			doClose();
		} catch (e) {
			saveErr = e instanceof Error ? e.message : 'Something went wrong';
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
			class="relative flex max-h-[92vh] w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-line bg-surface shadow-2xl"
			transition:scale={{ duration: dur(200), start: 0.97, opacity: 0, easing: cubicOut }}
			role="dialog"
			aria-label={creating ? 'New scheduled message' : 'Edit scheduled message'}
		>
			<div class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-4">
				<span class="grid size-5 place-items-center rounded border border-line bg-bg text-accent-ink">
					<CalendarClock size={11} />
				</span>
				<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
					Scheduling
				</span>
				<div class="h-4 w-px bg-line"></div>
				<span class="min-w-0 truncate text-[12.5px] font-medium text-ink">
					{creating ? 'New scheduled message' : sched?.name}
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

			<div class="min-h-0 flex-1 overflow-y-auto px-4 py-4 sm:px-5">
				<div class="grid gap-x-8 lg:grid-cols-2">
					<div class="min-w-0">
						<Field label="Name" hint="For you; also in scope as {'{{ .Name }}'}.">
							<input type="text" bind:value={name} placeholder="Weekly event reminder" class="input w-full" />
						</Field>
						<Field label="Post in" hint="The message posts to this channel.">
							<ChannelPicker value={channel} onChange={(v) => (channel = v as string)} />
						</Field>
					</div>
					<div class="min-w-0">
						<Field label="Repeats" hint="All times are UTC.">
							<FieldSelect
								value={kind}
								onChange={(v) => (kind = v as ScheduleDef['kind'])}
								options={[
									{ value: 'once', label: 'Once' },
									{ value: 'every', label: 'Every N minutes' },
									{ value: 'daily', label: 'Daily' },
									{ value: 'weekly', label: 'Weekly' }
								]}
							/>
						</Field>
						{#if kind === 'once'}
							<Field label="When" hint="Your local time; stored as UTC.">
								<input type="datetime-local" bind:value={at} class="input w-full font-mono text-[12px]" />
							</Field>
						{:else if kind === 'every'}
							<Field label="Interval (minutes)" hint="Minimum 5.">
								<input type="number" min="5" step="5" bind:value={everyMinutes} class="input w-32 text-center font-mono text-[12px]" />
							</Field>
						{:else}
							<Field label="Time of day (UTC)" hint="24-hour clock.">
								<input type="time" bind:value={timeOfDay} class="input w-32 font-mono text-[12px]" />
							</Field>
							{#if kind === 'weekly'}
								<div class="mb-4 flex flex-wrap gap-1">
									{#each WEEKDAYS as w, i (w)}
										{@const on = weekdays.includes(i)}
										<button
											type="button"
											aria-pressed={on}
											onclick={() => toggleWeekday(i)}
											class="inline-flex h-6 items-center rounded-md border px-2 text-[11px] font-medium transition-colors {on
												? 'border-line-strong bg-bg text-ink'
												: 'border-line bg-surface text-muted hover:border-line-strong hover:text-ink'}"
										>
											{w}
										</button>
									{/each}
								</div>
							{/if}
						{/if}
					</div>
				</div>

				<div class="mt-2 border-t border-line/60 pt-4">
					<div class="mb-2 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
						Message
					</div>
					<MessageEditor {step} embeds components clickPaths={false} buttonExtras={buttonAction} />
				</div>
			</div>

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
						{creating ? 'Create schedule' : 'Save changes'}
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
			<span class="font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-faint">On click</span>
			<AutomationPicker
				value={buttonActions[suffix] ?? ''}
				onChange={(v) => (buttonActions[suffix] = v)}
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
	description="You have unsaved changes to this schedule. Discard them, or keep editing?"
	confirmLabel="Discard"
	cancelLabel="Keep editing"
	onconfirm={doClose}
	oncancel={() => (confirmOpen = false)}
/>
