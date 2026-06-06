<script lang="ts">
	import { getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { Popover, Command } from 'bits-ui';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { presentGuildsQuery } from '$lib/queries';
	import Avatar from '$lib/components/ui/Avatar.svelte';
	import { ChevronsUpDown, Check, Search, Plus } from 'lucide-svelte';

	const store = getContext<GuildStore>(GUILD_CTX);
	const guilds = presentGuildsQuery();

	let open = $state(false);

	const icon = $derived(store.detail?.guild.icon ?? '');
	const iconUrl = $derived(
		icon ? `https://cdn.discordapp.com/icons/${store.id}/${icon}.png?size=64` : undefined
	);

	function pick(id: string) {
		open = false;
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
		{:else}
			<span class="skeleton h-5 w-5 rounded-md"></span>
			<span class="skeleton h-3.5 w-24 rounded"></span>
		{/if}
		<ChevronsUpDown size={13} class="shrink-0 text-faint transition-colors group-hover:text-muted" />
	</Popover.Trigger>

	<Popover.Portal>
		<Popover.Content
			sideOffset={6}
			align="start"
			class="menu-pop z-50 w-72 overflow-hidden rounded-xl border border-line-strong bg-surface shadow-2xl outline-none"
		>
			<Command.Root class="flex flex-col">
				<div class="flex items-center gap-2 border-b border-line px-3">
					<Search size={14} class="shrink-0 text-faint" />
					<Command.Input
						placeholder="Switch server…"
						class="h-10 w-full bg-transparent text-[13px] text-ink outline-none placeholder:text-faint"
					/>
				</div>
				<Command.List class="max-h-72 overflow-y-auto p-1.5">
					<Command.Empty class="px-2 py-6 text-center text-[12px] text-faint">
						No servers found.
					</Command.Empty>
					<Command.Group>
						{#each guilds.data ?? [] as g (g.id)}
							<Command.Item
								value={`server:${g.id}`}
								keywords={[g.name]}
								onSelect={() => pick(g.id)}
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
									<Check size={14} class="shrink-0 text-accent-ink" />
								{/if}
							</Command.Item>
						{/each}
					</Command.Group>
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
