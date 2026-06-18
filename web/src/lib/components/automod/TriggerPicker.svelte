<script lang="ts">
	// "Add rule" picker: a dialog listing every trigger grouped by category, each
	// with its icon, label and one-line summary. Choosing one emits its key.
	import { Dialog } from '$lib/components/ui';
	import { TRIGGERS, type TriggerKey, type TriggerSpec } from '$lib/moderation/automod';
	import { iconFor } from '$lib/commands/icons';

	let {
		open = $bindable(false),
		onpick
	}: { open?: boolean; onpick?: (key: TriggerKey) => void } = $props();

	const ORDER = ['Content', 'Spam & flood', 'Mentions', 'Formatting', 'Members'] as const;

	const grouped = $derived.by(() => {
		const map = new Map<string, TriggerSpec[]>();
		for (const t of TRIGGERS) {
			if (!map.has(t.category)) map.set(t.category, []);
			map.get(t.category)!.push(t);
		}
		return ORDER.filter((c) => map.has(c)).map((c) => ({ category: c, items: map.get(c)! }));
	});

	function choose(key: TriggerKey) {
		open = false;
		onpick?.(key);
	}
</script>

<Dialog.Root bind:open>
	<Dialog.Content class="max-w-2xl bg-surface p-0">
		<div class="border-b border-line px-5 py-4">
			<span class="eyebrow">New rule</span>
			<h2 class="mt-1 text-base font-semibold text-ink">Pick a trigger</h2>
			<p class="mt-0.5 text-xs text-muted">
				A trigger decides what trips the rule. You add actions next.
			</p>
		</div>
		<div class="max-h-[65vh] overflow-y-auto px-5 py-4">
			<div class="space-y-6">
				{#each grouped as g (g.category)}
					<div>
						<div class="eyebrow mb-2">{g.category}</div>
						<div class="grid gap-2 sm:grid-cols-2">
							{#each g.items as t (t.key)}
								{@const Icon = iconFor(t.icon)}
								<button
									type="button"
									onclick={() => choose(t.key)}
									class="group flex items-start gap-3 rounded-xl border border-line bg-ink-2/40 p-3 text-left transition-colors hover:border-line-strong hover:bg-ink-2"
								>
									<span
										class="grid size-8 shrink-0 place-items-center rounded-lg border border-line bg-surface text-muted transition-colors group-hover:text-accent-ink"
									>
										<Icon size={16} />
									</span>
									<span class="min-w-0">
										<span class="block text-sm font-medium text-ink">{t.label}</span>
										<span class="mt-0.5 block text-xs leading-snug text-muted">{t.short}</span>
									</span>
								</button>
							{/each}
						</div>
					</div>
				{/each}
			</div>
		</div>
	</Dialog.Content>
</Dialog.Root>
