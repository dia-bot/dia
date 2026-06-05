<script lang="ts">
	import { onMount } from 'svelte';
	import DiscordWindow from './DiscordWindow.svelte';
	import DiscordMessage from './DiscordMessage.svelte';
	import DiscordEmbed from './DiscordEmbed.svelte';
	import ShieldAlert from 'lucide-svelte/icons/shield-alert';

	// Auto-playing AutoMod demo: a rule-breaking message is posted, flagged,
	// removed, and written to the case log — cycling through the real rule types.
	// AutoMod applies one server-wide action to every violation (it deletes, then
	// the configured warn/timeout). This server is set to delete + timeout, so the
	// action is the same across rules — only the rule that tripped changes.
	const ACTION = 'Deleted · 10m timeout';
	const SCN = [
		{ user: 'nitro_deals', color: '#e0853a', text: '🎁 FREE NITRO for the first 50 people — claim now at discord.gg/fr33-nitro', rule: 'Discord invite' },
		{ user: 'promo_alt', color: '#5865f2', text: 'grow your server fast — buy members here → sketchy-promo.link/cheap', rule: 'Link filter' },
		{ user: 'raid_alt', color: '#f23f43', text: '@everyone @here @everyone free stuff go go go read this now!!!', rule: 'Mention spam (8 ▸ max 5)' },
		{ user: 'rudeguy', color: '#9b59b6', text: "honestly that's such a ******* take", rule: 'Banned word' }
	];

	let el: HTMLElement;
	let idx = $state(0);
	let step = $state(0); // 0 posted · 1 flagged · 2 removed + logged
	const scn = $derived(SCN[idx]);
	const caseNo = $derived(142 + idx);

	let timers: ReturnType<typeof setTimeout>[] = [];
	const clearTimers = () => {
		timers.forEach(clearTimeout);
		timers = [];
	};

	function cycle() {
		clearTimers();
		step = 0;
		timers.push(setTimeout(() => (step = 1), 1400));
		timers.push(setTimeout(() => (step = 2), 2400));
		timers.push(
			setTimeout(() => {
				idx = (idx + 1) % SCN.length;
				cycle();
			}, 5200)
		);
	}

	onMount(() => {
		const reduce = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
		if (reduce || typeof IntersectionObserver === 'undefined') {
			step = 2;
			return;
		}
		// Only animate while on screen — pause (and reset on return) otherwise.
		const io = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) cycle();
				else clearTimers();
			},
			{ threshold: 0.3 }
		);
		io.observe(el);
		return () => {
			io.disconnect();
			clearTimers();
		};
	});
</script>

<div bind:this={el}>
<DiscordWindow channel="general" title="Aurora" topic="AutoMod is watching" members="1,284 online">
	<DiscordMessage author="maya" color="#1aa179" time="Today at 9:14 AM">
		anyone around for the raid tonight? 👀
	</DiscordMessage>

	{#key idx}
		<div class="anim">
			{#if step < 2}
				<div
					class="-mx-2 flex gap-3 rounded-md px-2 py-1 transition-colors duration-300 {step === 1
						? 'bg-[#f23f43]/10'
						: ''}"
				>
					<div
						class="grid h-10 w-10 shrink-0 place-items-center rounded-full text-[13px] font-semibold text-white"
						style="background: {scn.color};"
					>
						{scn.user.slice(0, 2).toUpperCase()}
					</div>
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-2 leading-none">
							<span class="text-[15px] font-medium text-[#f2f3f5]">{scn.user}</span>
							<span class="text-[11px] text-[#949ba4]">Today at 9:15 AM</span>
							{#if step === 1}
								<span
									class="inline-flex items-center gap-1 rounded-[3px] bg-[#f23f43]/20 px-1.5 py-px text-[10px] font-semibold uppercase text-[#ff707a]"
								>
									<ShieldAlert size={11} /> removing…
								</span>
							{/if}
						</div>
						<div
							class="mt-0.5 text-[14.5px] leading-[1.4] transition-all duration-300 {step === 1
								? 'text-[#a3666a] line-through opacity-60'
								: 'text-[#dbdee1]'}"
						>
							{scn.text}
						</div>
					</div>
				</div>
			{:else}
				<div class="flex items-center gap-2 text-[13.5px] text-[#949ba4]">
					<ShieldAlert size={15} class="shrink-0 text-[#f23f43]" />
					<span>
						Dia removed a message from <span class="font-medium text-[#dbdee1]">@{scn.user}</span> —
						<span class="text-[#dbdee1]">{scn.rule}</span>
					</span>
				</div>
				<div class="mt-2.5">
					<DiscordMessage brand author="Dia" time="Today at 9:15 AM">
						<DiscordEmbed
							color="#f23f43"
							title={`Case #${caseNo} · AutoMod`}
							fields={[
								{ name: 'Member', value: `@${scn.user}` },
								{ name: 'Rule broken', value: scn.rule },
								{ name: 'Action', value: ACTION },
								{ name: 'Channel', value: '#general' }
							]}
							footer="AutoMod · just now"
							footerIcon
						/>
					</DiscordMessage>
				</div>
			{/if}
		</div>
	{/key}
</DiscordWindow>
</div>

<style>
	.anim {
		animation: rise 0.4s cubic-bezier(0.16, 1, 0.3, 1) both;
	}
	@keyframes rise {
		from {
			opacity: 0;
			transform: translateY(8px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.anim {
			animation: none;
		}
	}
</style>
