<script lang="ts">
	import { goto } from '$app/navigation';
	import { Dialog, Command } from 'bits-ui';
	import { manageableGuildsQuery } from '$lib/queries';
	import { Search, CornerDownLeft, Hash, ArrowRight } from 'lucide-svelte';

	type Page = { label: string; path: string };

	let {
		open = $bindable(false),
		serverId,
		pages
	}: { open?: boolean; serverId: string; pages: Page[] } = $props();

	const guilds = manageableGuildsQuery();
	// Other-server picker is for jumping between servers Dia is actually in —
	// listing not-yet-added guilds here would be confusing because picking one
	// just bounces you back to /servers to invite the bot.
	const otherServers = $derived(
		(guilds.data ?? []).filter((g) => g.id !== serverId && g.bot_present)
	);

	function go(href: string) {
		open = false;
		goto(href);
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Portal>
		<Dialog.Overlay class="fixed inset-0 z-[100] bg-black/60 backdrop-blur-sm" />
		<Dialog.Content
			class="dialog-pop fixed left-1/2 top-[16vh] z-[100] w-[92vw] max-w-lg -translate-x-1/2 overflow-hidden rounded-2xl border border-line-strong bg-surface shadow-2xl outline-none"
		>
			<Dialog.Title class="sr-only">Command palette</Dialog.Title>
			<Dialog.Description class="sr-only">Jump to a page or switch servers.</Dialog.Description>

			<Command.Root class="flex flex-col" loop>
				<div class="flex items-center gap-3 border-b border-line px-4">
					<Search size={16} class="shrink-0 text-faint" />
					<Command.Input
						placeholder="Jump to a page or server…"
						class="h-12 w-full bg-transparent text-sm text-ink outline-none placeholder:text-faint"
					/>
					<kbd
						class="hidden shrink-0 rounded border border-line-strong px-1.5 py-0.5 font-mono text-[10px] text-faint sm:block"
					>
						ESC
					</kbd>
				</div>

				<Command.List class="max-h-[330px] overflow-y-auto p-1.5">
					<Command.Empty class="px-3 py-8 text-center text-[13px] text-faint">
						No matches.
					</Command.Empty>

					<Command.Group>
						<Command.GroupHeading
							class="px-3 pb-1 pt-1.5 font-mono text-[10px] uppercase tracking-[0.12em] text-faint"
						>
							Pages
						</Command.GroupHeading>
						{#each pages as p (p.path)}
							<Command.Item
								value={`page:${p.path}`}
								keywords={[p.label]}
								onSelect={() => go(`/servers/${serverId}${p.path ? '/' + p.path : ''}`)}
								class="group flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2 text-left outline-none data-[selected]:bg-ink-2"
							>
								<span class="grid h-7 w-7 shrink-0 place-items-center rounded-md bg-blush text-accent">
									<Hash size={14} />
								</span>
								<span class="flex-1 truncate text-[13px] text-ink">{p.label}</span>
								<CornerDownLeft
									size={13}
									class="shrink-0 text-faint opacity-0 group-data-[selected]:opacity-100"
								/>
							</Command.Item>
						{/each}
					</Command.Group>

					{#if otherServers.length}
						<Command.Separator class="my-1 h-px bg-line" />
						<Command.Group>
							<Command.GroupHeading
								class="px-3 pb-1 pt-1.5 font-mono text-[10px] uppercase tracking-[0.12em] text-faint"
							>
								Switch server
							</Command.GroupHeading>
							{#each otherServers as g (g.id)}
								<Command.Item
									value={`server:${g.id}`}
									keywords={[g.name]}
									onSelect={() => go(`/servers/${g.id}`)}
									class="group flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2 text-left outline-none data-[selected]:bg-ink-2"
								>
									<span class="grid h-7 w-7 shrink-0 place-items-center rounded-md bg-blush text-accent">
										<ArrowRight size={14} />
									</span>
									<span class="flex-1 truncate text-[13px] text-ink">{g.name}</span>
								</Command.Item>
							{/each}
						</Command.Group>
					{/if}
				</Command.List>
			</Command.Root>
		</Dialog.Content>
	</Dialog.Portal>
</Dialog.Root>
