<script lang="ts">
	import DiscordWindow from './DiscordWindow.svelte';
	import DiscordMessage from './DiscordMessage.svelte';
	import DiscordEmbed from './DiscordEmbed.svelte';
	import Check from 'lucide-svelte/icons/check';

	// Interactive reaction-roles demo. Visitors click the role buttons and watch
	// assignment update live — and can switch between the three real menu modes.
	type Role = { id: string; emoji: string; label: string };
	const ROLES: Role[] = [
		{ id: 'gamer', emoji: '🎮', label: 'Gamer' },
		{ id: 'artist', emoji: '🎨', label: 'Artist' },
		{ id: 'music', emoji: '🎵', label: 'Music' },
		{ id: 'news', emoji: '📣', label: 'Announcements' },
		{ id: 'beta', emoji: '🧪', label: 'Beta Tester' }
	];

	const MODES = [
		{ id: 'toggle', label: 'Toggle', note: 'Add or remove any roles you like.' },
		{ id: 'unique', label: 'Unique', note: 'Pick one — choosing another swaps it. Perfect for colour roles.' },
		{ id: 'verify', label: 'Verify', note: 'A single-use opt-in. One click unlocks the server.' }
	];

	let mode = $state<'toggle' | 'unique' | 'verify'>('toggle');
	let selected = $state<string[]>(['gamer']);
	let verified = $state(false);

	const selectedRoles = $derived(ROLES.filter((r) => selected.includes(r.id)));

	function setMode(m: 'toggle' | 'unique' | 'verify') {
		mode = m;
		verified = false;
		selected = m === 'unique' ? ['gamer'] : m === 'toggle' ? ['gamer'] : [];
	}

	function click(id: string) {
		if (mode === 'toggle') {
			selected = selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id];
		} else if (mode === 'unique') {
			selected = selected.includes(id) ? [] : [id];
		}
	}
</script>

<div>
	<!-- mode control -->
	<div class="mb-3 flex flex-wrap items-center gap-3">
		<div class="inline-flex rounded-lg border border-line-strong p-0.5">
			{#each MODES as m (m.id)}
				<button
					type="button"
					onclick={() => setMode(m.id as 'toggle' | 'unique' | 'verify')}
					aria-pressed={mode === m.id}
					class="rounded-md px-3 py-1 text-sm font-medium transition-colors {mode === m.id
						? 'bg-ink text-bg'
						: 'text-muted hover:text-ink'}"
				>
					{m.label}
				</button>
			{/each}
		</div>
		<span class="text-sm text-muted">{MODES.find((m) => m.id === mode)?.note}</span>
	</div>

	<DiscordWindow channel="roles" topic="Choose what you want to see" title="Aurora">
		<DiscordMessage brand author="Dia" time="Today at 11:02 AM">
			{#if mode === 'verify'}
				<DiscordEmbed
					color="#23a55a"
					title="🔒 Verify to unlock Aurora"
					description="Tap the button below to confirm you've read the rules and gain access to the server."
				/>
				<div class="mt-2 flex flex-wrap gap-2">
					{#if verified}
						<span
							class="inline-flex items-center gap-1.5 rounded-[3px] bg-[#248046] px-3 py-1.5 text-[14px] font-medium text-white"
						>
							<Check size={15} /> Verified
						</span>
					{:else}
						<button
							type="button"
							onclick={() => (verified = true)}
							class="inline-flex items-center gap-1.5 rounded-[3px] bg-[#248046] px-3 py-1.5 text-[14px] font-medium text-white transition-colors hover:bg-[#1a6334]"
						>
							✓ Verify me
						</button>
					{/if}
				</div>
				{#if verified}
					<div class="mt-2 text-[13px] text-[#949ba4]">
						You've been given the <span
							class="rounded bg-[#b244fc]/25 px-1 font-medium text-[#d9b8ff]">@Member</span
						> role. Welcome in! 🎉
					</div>
				{/if}
			{:else}
				<DiscordEmbed
					color="#B244FC"
					title="🎭 Pick your roles"
					description="Click a button to assign yourself a role. Click again to remove it."
				/>
				<div class="mt-2 flex flex-wrap gap-2">
					{#each ROLES as r (r.id)}
						{@const on = selected.includes(r.id)}
						<button
							type="button"
							onclick={() => click(r.id)}
							aria-pressed={on}
							class="inline-flex items-center gap-1.5 rounded-[3px] px-3 py-1.5 text-[14px] font-medium transition-colors {on
								? 'bg-[#b244fc] text-white hover:bg-[#9d34e6]'
								: 'bg-[#4e5058] text-[#dbdee1] hover:bg-[#6d6f78]'}"
						>
							<span>{r.emoji}</span>
							{r.label}
							{#if on}<Check size={14} />{/if}
						</button>
					{/each}
				</div>
				<div class="mt-2.5 flex flex-wrap items-center gap-1.5 text-[13px] text-[#949ba4]" aria-live="polite">
					{#if selectedRoles.length}
						<span>Your roles:</span>
						{#each selectedRoles as r (r.id)}
							<span class="rounded bg-[#b244fc]/25 px-1.5 py-0.5 font-medium text-[#d9b8ff]"
								>@{r.label}</span
							>
						{/each}
					{:else}
						<span>You have no self-roles yet — click one above.</span>
					{/if}
				</div>
			{/if}
		</DiscordMessage>
	</DiscordWindow>
</div>
