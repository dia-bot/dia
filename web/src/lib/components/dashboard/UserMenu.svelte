<script lang="ts">
	import { goto } from '$app/navigation';
	import { DropdownMenu } from 'bits-ui';
	import { api } from '$lib/api';
	import Avatar from '$lib/components/ui/Avatar.svelte';
	import type { User } from '$lib/types';
	import { LogOut, ArrowLeft } from 'lucide-svelte';

	let { user }: { user: User } = $props();

	let signingOut = $state(false);
	const name = $derived(user.global_name || user.username);

	async function logout() {
		if (signingOut) return;
		signingOut = true;
		try {
			await api.logout();
		} finally {
			location.href = '/';
		}
	}
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		class="flex w-full items-center gap-2.5 rounded-lg px-2 py-2 text-left transition-colors hover:bg-surface data-[state=open]:bg-surface"
	>
		<Avatar
			src={user.avatar_url}
			class="h-7 w-7 rounded-full ring-1 ring-line-strong"
			fallback={(name || '?').charAt(0).toUpperCase()}
			fallbackClass="text-[12px]"
		/>
		<div class="min-w-0 flex-1">
			<div class="truncate text-[13px] font-medium text-ink">{name}</div>
			<div class="truncate font-mono text-[10.5px] text-faint">@{user.username}</div>
		</div>
		<span class="h-1.5 w-1.5 shrink-0 rounded-full bg-success" title="Online"></span>
	</DropdownMenu.Trigger>

	<DropdownMenu.Portal>
		<DropdownMenu.Content
			side="top"
			align="start"
			sideOffset={6}
			class="menu-pop z-50 min-w-[240px] max-w-[calc(100vw-2rem)] overflow-hidden rounded-xl border border-line-strong bg-surface p-1.5 shadow-2xl outline-none"
		>
			<div class="-mx-1.5 -mt-1.5 mb-1.5 border-b border-line px-3 py-2.5">
				<div class="truncate text-[12.5px] font-medium text-ink">{name}</div>
				<div class="truncate font-mono text-[11px] text-faint">@{user.username}</div>
			</div>
			<DropdownMenu.Item
				onSelect={() => goto('/servers')}
				class="flex cursor-pointer items-center gap-2.5 rounded-lg px-2 py-1.5 text-[13px] text-muted outline-none transition-colors data-[highlighted]:bg-ink-2 data-[highlighted]:text-ink"
			>
				<ArrowLeft size={14} class="text-faint" /> All servers
			</DropdownMenu.Item>
			<DropdownMenu.Item
				onSelect={logout}
				closeOnSelect={false}
				class="flex cursor-pointer items-center gap-2.5 rounded-lg px-2 py-1.5 text-[13px] text-danger outline-none transition-colors data-[highlighted]:bg-ink-2"
			>
				<LogOut size={14} /> {signingOut ? 'Signing out…' : 'Log out'}
			</DropdownMenu.Item>
		</DropdownMenu.Content>
	</DropdownMenu.Portal>
</DropdownMenu.Root>
