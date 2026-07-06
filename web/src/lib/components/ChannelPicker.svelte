<script lang="ts">
	// The one channel picker, used everywhere a channel (or a list of channels)
	// is chosen. A searchable, Discord-grouped dropdown over the live guild store
	// (channels stay current via realtime), with chips for multi-select. No more
	// pasting ids or #mentions.
	//
	// Single:  <ChannelPicker value={id}    onChange={(v) => …} />
	// Multi:   <ChannelPicker multiple value={ids} onChange={(v) => …} />
	// Voice:   add kind="voice" (or "all" for every channel type).
	import { getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { CHANNEL_TEXT, CHANNEL_ANNOUNCEMENT, CHANNEL_CATEGORY, CHANNEL_VOICE } from '$lib/types';
	import { Popover } from '$lib/components/ui';
	import { cn } from '$lib/utils';
	import { Hash, Volume2, ChevronDown, Check, X, Search, Plus } from 'lucide-svelte';

	const STAGE = 13; // stage channels (no exported constant)

	let {
		value,
		multiple = false,
		kind = 'text',
		placeholder = '',
		onChange,
		class: cls
	}: {
		value?: string | string[];
		multiple?: boolean;
		kind?: 'text' | 'voice' | 'all';
		placeholder?: string;
		onChange?: (v: string | string[]) => void;
		class?: string;
	} = $props();

	const store = getContext<GuildStore | undefined>(GUILD_CTX);

	const selected = $derived(Array.isArray(value) ? value : value ? [value] : []);
	const selectedSet = $derived(new Set(selected));

	const isVoiceType = (t: number) => t === CHANNEL_VOICE || t === STAGE;
	function typeOk(t: number): boolean {
		if (kind === 'voice') return isVoiceType(t);
		if (kind === 'all') return t !== CHANNEL_CATEGORY;
		return t === CHANNEL_TEXT || t === CHANNEL_ANNOUNCEMENT;
	}

	// id -> channel, so chips and the single-value label resolve even for a stale
	// id that is no longer in the guild (we then show the raw id rather than lose it).
	const byId = $derived(new Map((store?.channels ?? []).map((c) => [c.id, c])));
	const labelOf = (id: string) => byId.get(id)?.name ?? id;
	const isVoice = (id: string) => isVoiceType(byId.get(id)?.type ?? -1);

	type Item = { value: string; label: string; voice: boolean };
	type Group = { id: string; name: string; channels: Item[] };

	// Discord layout: uncategorised first, then each category with its channels,
	// all ordered by position.
	const groups = $derived.by<Group[]>(() => {
		const all = store?.channels ?? [];
		const matching = all.filter((c) => typeOk(c.type)).sort((a, b) => a.position - b.position);
		const inCat = (cat: string): Item[] =>
			matching
				.filter((c) => (c.parent_id ?? '') === cat)
				.map((c) => ({ value: c.id, label: c.name, voice: isVoiceType(c.type) }));
		const out: Group[] = [];
		const top = inCat('');
		if (top.length) out.push({ id: '', name: '', channels: top });
		for (const cat of all.filter((c) => c.type === CHANNEL_CATEGORY).sort((a, b) => a.position - b.position)) {
			const channels = inCat(cat.id);
			if (channels.length) out.push({ id: cat.id, name: cat.name, channels });
		}
		return out;
	});

	let query = $state('');
	const filteredGroups = $derived.by<Group[]>(() => {
		const q = query.trim().toLowerCase();
		if (!q) return groups;
		return groups
			.map((g) => ({ ...g, channels: g.channels.filter((c) => c.label.toLowerCase().includes(q)) }))
			.filter((g) => g.channels.length);
	});

	let open = $state(false);
	let searchEl = $state<HTMLInputElement | null>(null);
	$effect(() => {
		if (open) queueMicrotask(() => searchEl?.focus());
		else query = '';
	});

	function toggle(id: string) {
		if (multiple) {
			const cur = Array.isArray(value) ? value : [];
			onChange?.(cur.includes(id) ? cur.filter((x) => x !== id) : [...cur, id]);
		} else {
			onChange?.(id);
			open = false;
		}
	}
	function removeChip(id: string) {
		if (multiple) onChange?.((Array.isArray(value) ? value : []).filter((x) => x !== id));
		else onChange?.('');
	}

	const triggerClass =
		'flex h-9 w-full items-center gap-2 rounded-lg border border-line-strong bg-ink-2 px-3 text-sm outline-none transition-colors hover:border-faint focus:border-accent data-[state=open]:border-accent';
	const singleLabel = $derived(selected.length ? labelOf(selected[0]) : '');
	const singlePlaceholder = $derived(placeholder || 'Select a channel…');
	const addPlaceholder = $derived(selected.length ? 'Add another channel…' : placeholder || 'Add channels…');
</script>

<div class={cls}>
	{#if multiple && selected.length}
		<div class="mb-2 flex flex-wrap gap-1.5">
			{#each selected as id (id)}
				<span
					class="inline-flex items-center gap-1 rounded-full border border-line bg-surface py-1 pl-2 pr-1 text-xs font-medium text-ink"
				>
					{#if isVoice(id)}<Volume2 size={11} class="opacity-70" />{:else}<Hash
							size={11}
							class="opacity-70"
						/>{/if}
					<span class="max-w-[12rem] truncate">{labelOf(id)}</span>
					<button
						type="button"
						class="grid size-4 place-items-center rounded-full opacity-70 transition hover:bg-ink-2 hover:opacity-100"
						onclick={() => removeChip(id)}
						aria-label="Remove {labelOf(id)}"
					>
						<X size={12} />
					</button>
				</span>
			{/each}
		</div>
	{/if}

	<Popover.Root bind:open>
		<Popover.Trigger class={triggerClass}>
			{#if multiple}
				<Plus size={14} class="shrink-0 text-faint" />
				<span class="flex-1 truncate text-left text-faint">{addPlaceholder}</span>
			{:else if kind === 'voice'}
				<Volume2 size={14} class="shrink-0 text-faint" />
				<span class="flex-1 truncate text-left {singleLabel ? 'text-ink' : 'text-faint'}"
					>{singleLabel || singlePlaceholder}</span
				>
			{:else}
				<Hash size={14} class="shrink-0 text-faint" />
				<span class="flex-1 truncate text-left {singleLabel ? 'text-ink' : 'text-faint'}"
					>{singleLabel || singlePlaceholder}</span
				>
			{/if}
			<ChevronDown size={15} class="shrink-0 text-faint" />
		</Popover.Trigger>
		<Popover.Content
			align="start"
			sideOffset={6}
			class="z-50 w-[min(22rem,90vw)] overflow-hidden rounded-xl border border-line-strong bg-surface p-0 shadow-2xl"
		>
			<div class="flex items-center gap-1.5 border-b border-line/60 px-2.5 py-2">
				<Search size={13} class="shrink-0 text-faint" />
				<input
					bind:this={searchEl}
					bind:value={query}
					placeholder="Search channels…"
					class="h-5 w-full bg-transparent text-[12.5px] text-ink outline-none placeholder:text-faint"
					autocomplete="off"
					spellcheck="false"
				/>
			</div>
			<div class="max-h-72 overflow-y-auto p-1.5">
				{#each filteredGroups as g (g.id)}
					{#if g.name}
						<div
							class="truncate px-2 pb-1 pt-2 font-mono text-[10px] font-medium uppercase tracking-[0.1em] text-faint"
						>
							{g.name}
						</div>
					{/if}
					{#each g.channels as ch (ch.value)}
						<button
							type="button"
							class="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm text-ink outline-none transition-colors hover:bg-ink-2"
							onclick={() => toggle(ch.value)}
						>
							{#if ch.voice}<Volume2 size={14} class="shrink-0 text-faint" />{:else}<Hash
									size={14}
									class="shrink-0 text-faint"
								/>{/if}
							<span class="flex-1 truncate">{ch.label}</span>
							{#if selectedSet.has(ch.value)}<Check size={14} class="shrink-0 text-muted" />{/if}
						</button>
					{/each}
				{:else}
					<div class="px-2 py-6 text-center text-xs text-faint">
						{query ? 'No channels match.' : 'No channels.'}
					</div>
				{/each}
			</div>
		</Popover.Content>
	</Popover.Root>
</div>
