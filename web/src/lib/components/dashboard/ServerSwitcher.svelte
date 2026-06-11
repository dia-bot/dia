<script lang="ts">
	import { getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { Popover, Command } from 'bits-ui';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { manageableGuildsQuery } from '$lib/queries';
	import Avatar from '$lib/components/ui/Avatar.svelte';
	import { ChevronsUpDown, Check, Search, Plus, ServerOff } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const guilds = manageableGuildsQuery();

	let open = $state(false);

	const icon = $derived(store.detail?.guild.icon ?? '');
	const iconUrl = $derived(
		icon ? `https://cdn.discordapp.com/icons/${store.id}/${icon}.png?size=64` : undefined
	);

	// Group present vs not-added so the dropdown is scannable even when
	// the listing has many "Add" candidates. Inside each group: alpha sort.
	const presentGuilds = $derived(
		[...(guilds.data ?? [])]
			.filter((g) => g.bot_present)
			.sort((a, b) => a.name.localeCompare(b.name))
	);
	const otherGuilds = $derived(
		[...(guilds.data ?? [])]
			.filter((g) => !g.bot_present)
			.sort((a, b) => a.name.localeCompare(b.name))
	);

	function pick(id: string, present: boolean) {
		open = false;
		if (!present) {
			// Bot isn't in this server — route to the index so the user can hit
			// the invite button instead of landing on a "Dia is not in this
			// server" 404 inside the layout.
			goto(`/servers#g${id}`);
			return;
		}
		if (id !== store.id) goto(`/servers/${id}`);
	}
</script>

<Popover.Root bind:open>
	<Popover.Trigger
		class="group flex h-8 items-center gap-2 rounded-lg px-1.5 pr-2 text-left transition-colors hover:bg-surface data-[state=open]:bg-surface"
	>
		{#if store.detail}
			<Avatar
				src={iconUrl}
				class="h-5 w-5 rounded-md"
				fallback={(store.name || '?').charAt(0).toUpperCase()}
				fallbackClass="text-[10px]"
			/>
			<span class="max-w-[180px] truncate text-[13px] font-medium text-ink">{store.name}</span>
		{:else if store.id}
			<span class="skeleton h-5 w-5 rounded-md"></span>
			<span class="skeleton h-3.5 w-24 rounded"></span>
		{:else}
			<span class="grid h-5 w-5 place-items-center rounded-md bg-ink-2 text-faint">
				<ServerOff size={11} />
			</span>
			<span class="text-[13px] font-medium text-muted">Select a server</span>
		{/if}
		<ChevronsUpDown size={13} class="shrink-0 text-faint transition-colors group-hover:text-muted" />
	</Popover.Trigger>

	<Popover.Portal>
		<Popover.Content
			sideOffset={6}
			align="start"
			class="menu-pop z-50 w-80 max-w-[calc(100vw-2rem)] overflow-hidden rounded-xl border border-line-strong bg-surface shadow-2xl outline-none"
		>
			<Command.Root class="flex flex-col">
				<div class="flex items-center gap-2 border-b border-line px-3">
					<Search size={14} class="shrink-0 text-faint" />
					<Command.Input
						placeholder="Switch server…"
						class="h-10 w-full bg-transparent text-[13px] text-ink outline-none placeholder:text-faint"
					/>
				</div>
				<Command.List class="max-h-80 overflow-y-auto p-1.5">
					{#if guilds.isLoading}
						<div class="space-y-1 p-1">
							{#each [0, 1, 2] as i (i)}
								<div class="flex items-center gap-2.5 px-2 py-1.5">
									<span class="skeleton h-6 w-6 rounded-md"></span>
									<span class="skeleton h-3.5 flex-1 rounded"></span>
								</div>
							{/each}
						</div>
					{:else if guilds.isError}
						<div class="px-3 py-6 text-center text-[12px] text-danger">
							Couldn't load your servers.
						</div>
					{:else}
						<Command.Empty class="px-2 py-6 text-center text-[12px] text-faint">
							No servers match.
						</Command.Empty>

						{#if presentGuilds.length > 0}
							<Command.Group>
								<Command.GroupHeading
									class="mt-0.5 px-2 pb-1 pt-0.5 font-mono text-[10px] uppercase tracking-[0.14em] text-faint"
								>
									Your servers · {presentGuilds.length}
								</Command.GroupHeading>
								{#each presentGuilds as g (g.id)}
									<Command.Item
										value={`server:${g.id}`}
										keywords={[g.name]}
										onSelect={() => pick(g.id, true)}
										class="flex cursor-pointer items-center gap-2.5 rounded-lg px-2 py-1.5 text-[13px] text-ink outline-none data-[selected]:bg-ink-2"
									>
										<Avatar
											src={g.icon_url}
											class="h-6 w-6 rounded-md"
											fallback={g.name.charAt(0).toUpperCase()}
											fallbackClass="text-[11px]"
										/>
										<span class="flex-1 truncate">{g.name}</span>
										{#if g.id === store.id}
											<span
												class="grid size-4 shrink-0 place-items-center rounded-full bg-ink text-bg"
												title="Current server"
											>
												<Check size={11} strokeWidth={2.5} />
											</span>
										{/if}
									</Command.Item>
								{/each}
							</Command.Group>
						{/if}

						{#if otherGuilds.length > 0}
							{#if presentGuilds.length > 0}
								<Command.Separator class="my-1 h-px bg-line" />
							{/if}
							<Command.Group>
								<Command.GroupHeading
									class="mt-0.5 px-2 pb-1 pt-0.5 font-mono text-[10px] uppercase tracking-[0.14em] text-faint"
								>
									Add Dia · {otherGuilds.length}
								</Command.GroupHeading>
								{#each otherGuilds as g (g.id)}
									<Command.Item
										value={`add:${g.id}`}
										keywords={[g.name]}
										onSelect={() => pick(g.id, false)}
										class="flex cursor-pointer items-center gap-2.5 rounded-lg px-2 py-1.5 text-[13px] text-muted outline-none data-[selected]:bg-ink-2"
									>
										<Avatar
											src={g.icon_url}
											class="h-6 w-6 rounded-md opacity-60"
											fallback={g.name.charAt(0).toUpperCase()}
											fallbackClass="text-[11px]"
										/>
										<span class="flex-1 truncate">{g.name}</span>
										<span
											class="shrink-0 rounded-md border border-line bg-ink-2 px-1.5 py-px font-mono text-[9.5px] uppercase tracking-wider text-muted"
											title="Dia hasn't been added to this server yet"
										>
											Add
										</span>
									</Command.Item>
								{/each}
							</Command.Group>
						{/if}
					{/if}
				</Command.List>
				<button
					type="button"
					onclick={() => {
						open = false;
						goto('/servers');
					}}
					class="flex w-full items-center gap-2 border-t border-line px-3 py-2.5 text-[12.5px] text-muted transition-colors hover:bg-ink-2 hover:text-ink"
				>
					<Plus size={14} class="text-faint" /> All servers
				</button>
			</Command.Root>
		</Popover.Content>
	</Popover.Portal>
</Popover.Root>
