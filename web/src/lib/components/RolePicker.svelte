<script lang="ts">
	// The one role picker, used everywhere a role (or a list of roles) is chosen.
	// Searchable dropdown over the live guild store with color dots and chips for
	// multi-select. Mirrors ChannelPicker.
	//
	// Single:  <RolePicker value={id}    onChange={(v) => …} />
	// Multi:   <RolePicker multiple value={ids} onChange={(v) => …} />
	import { getContext } from 'svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { Popover } from '$lib/components/ui';
	import { ChevronDown, Check, X, Search, Plus } from 'lucide-svelte';

	let {
		value,
		multiple = false,
		includeManaged = false,
		placeholder = '',
		onChange,
		class: cls
	}: {
		value?: string | string[];
		multiple?: boolean;
		includeManaged?: boolean;
		placeholder?: string;
		onChange?: (v: string | string[]) => void;
		class?: string;
	} = $props();

	const store = getContext<GuildStore | undefined>(GUILD_CTX);

	const selected = $derived(Array.isArray(value) ? value : value ? [value] : []);
	const selectedSet = $derived(new Set(selected));

	// id -> role, so chips/labels resolve even for a stale id (then show the id).
	const byId = $derived(new Map((store?.roles ?? []).map((r) => [r.id, r])));
	const labelOf = (id: string) => byId.get(id)?.name ?? id;
	const colorOf = (id: string) => {
		const c = byId.get(id)?.color ?? 0;
		return c ? '#' + c.toString(16).padStart(6, '0') : 'var(--color-faint)';
	};

	type Item = { value: string; label: string; color: number };
	// Assignable roles: drop @everyone and (by default) managed integration roles,
	// ordered by Discord position (highest first), like store.roleOptions().
	const roles = $derived.by<Item[]>(() =>
		(store?.roles ?? [])
			.filter((r) => r.id !== store?.id && (includeManaged || !r.managed))
			.sort((a, b) => b.position - a.position)
			.map((r) => ({ value: r.id, label: r.name, color: r.color }))
	);

	let query = $state('');
	const filtered = $derived.by<Item[]>(() => {
		const q = query.trim().toLowerCase();
		return q ? roles.filter((r) => r.label.toLowerCase().includes(q)) : roles;
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
	const singlePlaceholder = $derived(placeholder || 'Select a role…');
	const addPlaceholder = $derived(selected.length ? 'Add another role…' : placeholder || 'Add roles…');
</script>

<div class={cls}>
	{#if multiple && selected.length}
		<div class="mb-2 flex flex-wrap gap-1.5">
			{#each selected as id (id)}
				<span
					class="inline-flex items-center gap-1 rounded-full border border-line bg-surface py-1 pl-2 pr-1 text-xs font-medium text-ink"
				>
					<span class="size-2 shrink-0 rounded-full" style="background:{colorOf(id)}"></span>
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
			{:else}
				<span
					class="size-2.5 shrink-0 rounded-full"
					style="background:{singleLabel ? colorOf(selected[0]) : 'var(--color-faint)'}"
				></span>
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
					placeholder="Search roles…"
					class="h-5 w-full bg-transparent text-[12.5px] text-ink outline-none placeholder:text-faint"
					autocomplete="off"
					spellcheck="false"
				/>
			</div>
			<div class="max-h-72 overflow-y-auto p-1.5">
				{#each filtered as r (r.value)}
					<button
						type="button"
						class="flex w-full cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-left text-sm text-ink outline-none transition-colors hover:bg-ink-2"
						onclick={() => toggle(r.value)}
					>
						<span
							class="size-2.5 shrink-0 rounded-full"
							style="background:{r.color ? '#' + r.color.toString(16).padStart(6, '0') : 'var(--color-faint)'}"
						></span>
						<span class="flex-1 truncate">{r.label}</span>
						{#if selectedSet.has(r.value)}<Check size={14} class="shrink-0 text-muted" />{/if}
					</button>
				{:else}
					<div class="px-2 py-6 text-center text-xs text-faint">
						{query ? 'No roles match.' : 'No roles.'}
					</div>
				{/each}
			</div>
		</Popover.Content>
	</Popover.Root>
</div>
