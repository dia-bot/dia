<script lang="ts">
	// A modal that shows an admin exactly what a member WITHOUT a manager role
	// sees for this feature: the tab renders locked in the sidebar (a click pops
	// a "locked" notice) and opening it directly lands on the 403 panel. It's a
	// faithful, self-contained replica of servers/[id]/+layout.svelte's locked
	// nav item, lock popup and 403 view — no navigation or permission change.
	import { fade, scale } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';
	import { Lock, ShieldX, Gift, X } from 'lucide-svelte';

	let {
		open = $bindable(false),
		featureName = 'Giveaways'
	}: { open?: boolean; featureName?: string } = $props();

	let popup = $state(false);
	function close() {
		open = false;
		popup = false;
	}
</script>

{#if open}
	<div class="fixed inset-0 z-[70] grid place-items-center p-4">
		<button
			type="button"
			class="absolute inset-0 h-full w-full cursor-default bg-black/40"
			aria-label="Dismiss"
			onclick={close}
			transition:fade={{ duration: dur(150) }}
		></button>

		<div
			class="relative w-full max-w-2xl overflow-hidden rounded-xl border border-line bg-surface shadow-2xl"
			transition:scale={{ duration: dur(200), start: 0.95, opacity: 0, easing: cubicOut }}
		>
			<div class="flex items-center gap-2 border-b border-line px-4 py-3">
				<Lock size={13} class="text-muted" />
				<span class="text-[13px] font-semibold text-ink">Restricted view</span>
				<span class="text-[12px] text-muted">— what members without a manager role see</span>
				<button
					type="button"
					onclick={close}
					class="ml-auto grid size-7 place-items-center rounded-md text-muted hover:bg-ink-2 hover:text-ink"
					aria-label="Close"
				>
					<X size={14} />
				</button>
			</div>

			<div class="grid gap-0 sm:grid-cols-[180px_1fr]">
				<!-- Mock sidebar: the feature tab renders locked. -->
				<div class="border-b border-line bg-bg p-3 sm:border-b-0 sm:border-r">
					<div class="mb-2 font-mono text-[10px] uppercase tracking-[0.14em] text-faint">Sidebar</div>
					<div class="flex flex-col gap-1">
						<div class="flex items-center gap-2 rounded-md px-2 py-1.5 text-[12px] text-muted">
							<Gift size={13} class="text-faint" /> Overview
						</div>
						<button
							type="button"
							onclick={() => (popup = true)}
							class="flex items-center gap-2 rounded-md px-2 py-1.5 text-[12px] text-faint/80 hover:bg-ink-2"
							title="You don't have access to this section"
						>
							<Gift size={13} class="text-faint/70" />
							{featureName}
							<Lock size={11} class="ml-auto shrink-0 text-faint/60" />
						</button>
					</div>
				</div>

				<!-- Mock content: the 403 panel. -->
				<div class="relative grid min-h-[220px] place-items-center px-6 py-10">
					<div class="flex max-w-xs flex-col items-center gap-3 text-center">
						<span class="grid size-11 place-items-center rounded-full border border-line bg-surface text-muted">
							<ShieldX size={20} />
						</span>
						<div>
							<p class="text-[14px] font-semibold text-ink">You don't have access</p>
							<p class="mt-1 text-[12px] text-muted">
								This section is restricted. Ask a server admin to grant your role access.
							</p>
						</div>
						<span class="mt-1 inline-flex h-8 items-center rounded-md bg-ink px-3 text-[12px] font-semibold text-bg">
							Back to overview
						</span>
					</div>

					{#if popup}
						<!-- The lock popup, shown when the locked tab is clicked. -->
						<div class="absolute inset-0 grid place-items-center p-4">
							<button
								type="button"
								class="absolute inset-0 cursor-default bg-black/30"
								aria-label="Dismiss"
								onclick={() => (popup = false)}
								transition:fade={{ duration: dur(120) }}
							></button>
							<div
								class="relative flex max-w-xs flex-col items-center gap-2.5 rounded-xl border border-line bg-surface p-5 text-center shadow-2xl"
								transition:scale={{ duration: dur(180), start: 0.94, opacity: 0, easing: cubicOut }}
							>
								<span class="grid size-9 place-items-center rounded-full border border-line bg-bg text-muted">
									<Lock size={16} />
								</span>
								<p class="text-[13px] font-semibold text-ink">{featureName} is locked</p>
								<p class="text-[12px] text-muted">
									You don't have access to this section. Ask a server admin to grant your role access.
								</p>
								<button
									type="button"
									onclick={() => (popup = false)}
									class="mt-1 h-8 rounded-md bg-ink px-3 text-[12px] font-semibold text-bg hover:bg-ink/90"
								>
									Got it
								</button>
							</div>
						</div>
					{/if}
				</div>
			</div>

			<div class="border-t border-line px-4 py-2.5 text-[11px] text-muted">
				Click the locked <span class="font-medium text-ink">{featureName}</span> tab above to see the popup members get.
			</div>
		</div>
	</div>
{/if}
