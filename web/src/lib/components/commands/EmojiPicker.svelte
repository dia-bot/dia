<script lang="ts">
	// MEE6-style emoji picker — the SERVER's own custom emojis first, then
	// every standard Discord emoji grouped by the official Unicode categories,
	// with search across both. The unicode dataset (~1.9k) and the guild's
	// emoji list are lazy-loaded the first time the picker opens. Custom
	// server emojis are stored as "name:id" ("a:name:id" when animated) —
	// exactly what the runtime parses into Discord's emoji shape.
	import { page } from '$app/stores';
	import { api } from '$lib/api';
	import { Popover } from '$lib/components/ui';

	import Smile from 'lucide-svelte/icons/smile';
	import Search from 'lucide-svelte/icons/search';
	import X from 'lucide-svelte/icons/x';

	let {
		value = '',
		onChange,
		returnFocusOnPick = true,
		class: cls = ''
	}: {
		value?: string;
		onChange: (v: string) => void;
		// Inserter usages (the composer toolbar) keep the caret in the text
		// surface they insert into: after a pick the popover must NOT pull
		// focus back to its trigger button. Dismissing without picking still
		// returns focus to the trigger.
		returnFocusOnPick?: boolean;
		class?: string;
	} = $props();

	type Emoji = { emoji: string; name: string; slug: string };
	type Group = { name: string; slug: string; emojis: Emoji[] };
	type ServerEmoji = { id: string; name: string; animated: boolean };

	let open = $state(false);
	let groups = $state.raw<Group[]>([]);
	let serverEmojis = $state.raw<ServerEmoji[]>([]);
	let loading = $state(false);
	let query = $state('');
	// -1 = the Server tab; 0.. = unicode category index.
	let activeTab = $state(0);
	let hovered = $state<{ emoji?: string; image?: string; label: string } | null>(null);
	let custom = $state('');

	// Each category tab is its own first emoji — instantly recognizable.
	const GROUP_ICONS: Record<string, string> = {
		smileys_emotion: '😀',
		people_body: '👋',
		animals_nature: '🐻',
		food_drink: '🍔',
		travel_places: '✈️',
		activities: '⚽',
		objects: '💡',
		symbols: '🔣',
		flags: '🏁'
	};

	$effect(() => {
		if (open && groups.length === 0 && !loading) void load();
	});

	async function load() {
		loading = true;
		// Guild emojis are best-effort — the picker works without them.
		const guildId = $page.params.id ?? '';
		const [mod, srv] = await Promise.all([
			import('unicode-emoji-json/data-by-group.json'),
			guildId
				? api.emojis(guildId).catch(() => ({ emojis: [] as ServerEmoji[] }))
				: Promise.resolve({ emojis: [] as ServerEmoji[] })
		]);
		groups = (mod.default ?? mod) as unknown as Group[];
		serverEmojis = srv.emojis ?? [];
		if (serverEmojis.length > 0) activeTab = -1;
		loading = false;
	}

	function cdn(e: ServerEmoji): string {
		return `https://cdn.discordapp.com/emojis/${e.id}.${e.animated ? 'gif' : 'png'}?size=64`;
	}
	function serverToken(e: ServerEmoji): string {
		return e.animated ? `a:${e.name}:${e.id}` : `${e.name}:${e.id}`;
	}

	// The current value, decoded for the trigger: a custom "name:id" renders
	// as its CDN image; anything else renders as text.
	const valueImage = $derived.by(() => {
		const m = /^(a:)?([\w~-]+):(\d{15,21})$/.exec((value ?? '').trim().replace(/^<|>$/g, ''));
		if (!m) return null;
		return `https://cdn.discordapp.com/emojis/${m[3]}.${m[1] ? 'gif' : 'png'}?size=64`;
	});

	const q = $derived(query.trim().toLowerCase());

	const serverShown = $derived.by(() => {
		if (!serverEmojis.length) return [] as ServerEmoji[];
		if (q) return serverEmojis.filter((e) => e.name.toLowerCase().includes(q));
		return activeTab === -1 ? serverEmojis : [];
	});

	const unicodeShown = $derived.by(() => {
		if (!groups.length) return [] as Emoji[];
		if (q) {
			const slugQ = q.replaceAll(' ', '_');
			const out: Emoji[] = [];
			for (const g of groups) {
				for (const e of g.emojis) {
					if (e.slug.includes(slugQ) || e.name.includes(q)) {
						out.push(e);
						if (out.length >= 240) return out;
					}
				}
			}
			return out;
		}
		return activeTab >= 0 ? (groups[activeTab]?.emojis ?? []) : [];
	});

	let justPicked = false;

	function pick(emoji: string) {
		justPicked = true;
		onChange(emoji);
		open = false;
		query = '';
		custom = '';
	}

	function onCloseAutoFocus(e: Event) {
		if (!returnFocusOnPick && justPicked) e.preventDefault();
		justPicked = false;
	}
</script>

<Popover.Root bind:open>
	<Popover.Trigger
		class={cls ||
			'grid h-7 w-9 shrink-0 place-items-center rounded-md border border-input bg-background text-[15px] transition-colors hover:border-ring/60 data-[state=open]:border-ring'}
	>
		{#if valueImage}
			<img src={valueImage} alt={value} class="size-4 object-contain" />
		{:else if value}
			<span class="max-w-full truncate px-0.5 leading-none">{value}</span>
		{:else}
			<Smile class="size-3.5 text-muted-foreground" />
		{/if}
	</Popover.Trigger>

	<Popover.Content class="w-[324px] p-0" align="start" {onCloseAutoFocus}>
		<!-- Search -->
		<div class="flex items-center gap-2 border-b border-border/60 px-2.5">
			<Search class="size-3.5 shrink-0 text-muted-foreground" />
			<!-- svelte-ignore a11y_autofocus -->
			<input
				class="h-9 min-w-0 flex-1 bg-transparent text-[12.5px] text-foreground placeholder:text-muted-foreground focus:outline-none"
				placeholder="Search emojis…"
				autofocus
				bind:value={query}
			/>
			{#if value}
				<button
					type="button"
					class="inline-flex h-5 shrink-0 items-center gap-1 rounded px-1.5 font-mono text-[9.5px] uppercase tracking-[0.08em] text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
					onclick={() => pick('')}
					title="Remove emoji"
				>
					<X class="size-2.5" />
					clear
				</button>
			{/if}
		</div>

		<!-- Tabs: the server first, then the unicode categories -->
		{#if !q && groups.length > 0}
			<div class="flex items-center justify-between border-b border-border/60 px-1.5 py-1">
				{#if serverEmojis.length > 0}
					<button
						type="button"
						class="grid size-7 place-items-center rounded-md transition-all {activeTab === -1
							? 'bg-secondary'
							: 'opacity-45 hover:opacity-100'}"
						title="This server's emojis"
						onclick={() => (activeTab = -1)}
					>
						<img src={cdn(serverEmojis[0])} alt="Server" class="size-4 object-contain" />
					</button>
				{/if}
				{#each groups as g, i (g.slug)}
					<button
						type="button"
						class="grid size-7 place-items-center rounded-md text-[15px] leading-none transition-all {activeTab ===
						i
							? 'bg-secondary'
							: 'opacity-45 hover:opacity-100'}"
						title={g.name}
						onclick={() => (activeTab = i)}
					>
						{GROUP_ICONS[g.slug] ?? g.emojis[0]?.emoji}
					</button>
				{/each}
			</div>
		{/if}

		<!-- Grid -->
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="h-56 overflow-y-auto p-1.5" onmouseleave={() => (hovered = null)}>
			{#if loading || groups.length === 0}
				<div class="grid h-full place-items-center font-mono text-[10.5px] text-muted-foreground">
					Loading emojis…
				</div>
			{:else if serverShown.length === 0 && unicodeShown.length === 0}
				<div class="grid h-full place-items-center font-mono text-[10.5px] text-muted-foreground">
					No match for “{query}”
				</div>
			{:else}
				{#if serverShown.length > 0}
					{#if q}
						<div class="px-1 pb-0.5 pt-1 font-mono text-[9px] font-medium uppercase tracking-[0.14em] text-muted-foreground/60">
							This server
						</div>
					{/if}
					<div class="grid grid-cols-8">
						{#each serverShown as e (e.id)}
							<button
								type="button"
								class="grid size-9 place-items-center rounded-md transition-colors duration-75 hover:bg-secondary"
								onclick={() => pick(serverToken(e))}
								onmouseenter={() => (hovered = { image: cdn(e), label: e.name })}
								aria-label={e.name}
							>
								<img src={cdn(e)} alt={e.name} class="size-6 object-contain" loading="lazy" />
							</button>
						{/each}
					</div>
				{/if}
				{#if unicodeShown.length > 0}
					{#if q && serverShown.length > 0}
						<div class="px-1 pb-0.5 pt-2 font-mono text-[9px] font-medium uppercase tracking-[0.14em] text-muted-foreground/60">
							Standard
						</div>
					{/if}
					<div class="grid grid-cols-8">
						{#each unicodeShown as e (e.slug)}
							<button
								type="button"
								class="grid size-9 place-items-center rounded-md text-[20px] leading-none transition-colors duration-75 hover:bg-secondary"
								onclick={() => pick(e.emoji)}
								onmouseenter={() => (hovered = { emoji: e.emoji, label: e.slug })}
								aria-label={e.name}
							>
								{e.emoji}
							</button>
						{/each}
					</div>
				{/if}
			{/if}
		</div>

		<!-- Footer: hovered name + custom entry -->
		<div class="border-t border-border/60 px-2.5 py-1.5">
			<div class="flex h-5 items-center gap-1.5 overflow-hidden">
				{#if hovered}
					{#if hovered.image}
						<img src={hovered.image} alt="" class="size-4 object-contain" />
					{:else}
						<span class="text-[15px] leading-none">{hovered.emoji}</span>
					{/if}
					<span class="truncate font-mono text-[10.5px] text-muted-foreground">:{hovered.label}:</span>
				{:else}
					<span class="font-mono text-[10px] text-muted-foreground/60">
						other server's emoji? paste it as name:id
					</span>
				{/if}
			</div>
			<form
				class="mt-1 flex items-center gap-1.5"
				onsubmit={(e) => {
					e.preventDefault();
					if (custom.trim()) pick(custom.trim());
				}}
			>
				<input
					class="h-6 min-w-0 flex-1 rounded border border-input bg-background px-1.5 font-mono text-[10.5px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
					placeholder="partyparrot:1123581321345589"
					bind:value={custom}
				/>
				<button
					type="submit"
					class="h-6 shrink-0 rounded bg-foreground px-2 text-[10.5px] font-medium text-background transition-opacity hover:opacity-90 disabled:opacity-40"
					disabled={!custom.trim()}
				>
					Use
				</button>
			</form>
		</div>
	</Popover.Content>
</Popover.Root>
