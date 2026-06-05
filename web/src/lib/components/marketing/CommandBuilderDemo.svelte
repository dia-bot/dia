<script lang="ts">
	import DiscordWindow from './DiscordWindow.svelte';
	import DiscordMessage from './DiscordMessage.svelte';
	import DiscordEmbed from './DiscordEmbed.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ColorField from '$lib/components/ColorField.svelte';
	import CornerDownRight from 'lucide-svelte/icons/corner-down-right';

	// The dashboard's custom-command builder, wired to a live Discord preview.
	// Edit on the left; the registered slash command and its reply update instantly.
	const NAME_RULE = /^[a-z0-9_-]{0,32}$/;

	let name = $state('rules');
	let description = $state('Show the server rules');
	let content = $state('Welcome {user} 👋 — please read these before posting:');
	let ephemeral = $state(false);
	let useEmbed = $state(true);
	let embedTitle = $state('📜 Aurora — Server Rules');
	let embedColor = $state('#B244FC');
	let embedDescription = $state(
		'**1.** Be kind — treat everyone with respect.\n**2.** No spam, ads, or self-promo.\n**3.** Keep it SFW.\n**4.** Use channels for their topic.'
	);

	const nameValid = $derived(NAME_RULE.test(name));
	const cleanName = $derived(name || 'command');
	const sub = (s: string) => (s ?? '').replaceAll('{user}', 'maya').replaceAll('{server}', 'Aurora');

	function onName(e: Event) {
		const v = (e.target as HTMLInputElement).value.toLowerCase().replace(/[^a-z0-9_-]/g, '');
		name = v.slice(0, 32);
	}
</script>

<div class="grid items-start gap-5 lg:grid-cols-2">
	<!-- builder -->
	<div class="card p-5">
		<div class="mb-4 flex items-center justify-between">
			<h3 class="text-[15px] font-semibold">New command</h3>
			<span class="rounded-full bg-blush px-2 py-0.5 text-[11px] font-semibold text-accent-ink"
				>Builder</span
			>
		</div>

		<div class="grid gap-4 sm:grid-cols-2">
			<div>
				<span class="label">Name</span>
				<div class="flex items-center gap-1.5">
					<span class="text-muted">/</span>
					<input
						class="input"
						maxlength="32"
						value={name}
						oninput={onName}
						placeholder="rules"
						aria-label="Command name"
						aria-invalid={!!name && !nameValid}
						aria-describedby="cb-name-err"
					/>
				</div>
				{#if name && !nameValid}
					<p id="cb-name-err" role="alert" class="hint" style="color: var(--color-danger)">
						a–z, 0–9, - and _ only.
					</p>
				{/if}
			</div>
			<div>
				<span class="label">Description</span>
				<input class="input" aria-label="Command description" bind:value={description} placeholder="Show the server rules" />
			</div>
		</div>

		<div class="mt-4">
			<span class="label">Response</span>
			<textarea class="input" rows="2" bind:value={content} placeholder="Hey {'{user}'}…"></textarea>
		</div>

		<div class="mt-3 flex items-center gap-3">
			<Toggle bind:checked={ephemeral} label="Ephemeral reply" />
			<span class="text-sm">Ephemeral — only the user sees the reply</span>
		</div>

		<div class="mt-4 rounded-xl border border-line p-4">
			<div class="flex items-center justify-between gap-3">
				<span class="text-sm font-medium">Attach an embed</span>
				<Toggle bind:checked={useEmbed} label="Attach an embed" />
			</div>
			{#if useEmbed}
				<div class="mt-4 space-y-3">
					<div class="grid gap-3 sm:grid-cols-[1fr_auto]">
						<div>
							<span class="label">Title</span>
							<input class="input" bind:value={embedTitle} />
						</div>
						<ColorField label="Color" bind:value={embedColor} />
					</div>
					<div>
						<span class="label">Description</span>
						<textarea class="input" rows="4" bind:value={embedDescription}></textarea>
					</div>
				</div>
			{/if}
		</div>
	</div>

	<!-- live preview -->
	<DiscordWindow channel="commands" title="Aurora" topic="Slash commands you built">
		<!-- command picker -->
		<div class="overflow-hidden rounded-lg bg-[#2b2d31] ring-1 ring-black/30">
			<div class="border-b border-[#1f2023] px-3 py-1.5 text-[11px] font-semibold uppercase tracking-wide text-[#949ba4]">
				/{cleanName}
			</div>
			<div class="flex items-center gap-2 bg-[#35373c] px-3 py-2">
				<span
					class="grid h-5 w-5 place-items-center rounded bg-[#b244fc] text-[12px] font-bold text-white"
					>/</span
				>
				<span class="text-[14px] font-medium text-[#f2f3f5]">/{cleanName}</span>
				<span class="min-w-0 flex-1 truncate text-[13px] text-[#949ba4]">{description}</span>
				<span class="rounded-[3px] bg-[#5865f2] px-1 py-px text-[10px] font-semibold uppercase text-white"
					>Dia</span
				>
			</div>
		</div>

		<!-- invocation + reply -->
		<div class="flex items-center gap-1.5 pl-1 text-[12px] text-[#949ba4]">
			<CornerDownRight size={13} class="text-[#80848e]" />
			<span class="grid h-4 w-4 place-items-center rounded-full bg-[#1aa179] text-[9px] font-semibold text-white">M</span>
			<span><span class="font-medium text-[#dbdee1]">maya</span> used <span class="font-medium text-[#c79bff]">/{cleanName}</span></span>
		</div>

		<DiscordMessage brand author="Dia" time="Today at 2:30 PM">
			{#if content}<p class="mb-1 whitespace-pre-line">{sub(content)}</p>{/if}
			{#if useEmbed}
				<DiscordEmbed color={embedColor} title={sub(embedTitle)} description={sub(embedDescription)} />
			{/if}
			{#if ephemeral}
				<div class="mt-1.5 text-[12px] italic text-[#949ba4]">Only you can see this · Dismiss message</div>
			{/if}
		</DiscordMessage>
	</DiscordWindow>
</div>
