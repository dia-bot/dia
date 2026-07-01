<script lang="ts">
	// A chip-list text input: type a term, press Enter or comma to add it. Used by
	// every `words` field (blocked words, patterns, domains, allow-lists). Mirrors
	// the banned-words pattern from the original automod page.
	import { X } from 'lucide-svelte';

	let {
		value = $bindable<string[]>([]),
		placeholder = 'Add a term…',
		lower = false
	}: { value?: string[]; placeholder?: string; lower?: boolean } = $props();

	let draft = $state('');

	function add() {
		let w = draft.trim();
		if (lower) w = w.toLowerCase();
		draft = '';
		if (!w || value.includes(w)) return;
		value = [...value, w];
	}

	function keydown(e: KeyboardEvent) {
		if (e.key === 'Enter' || e.key === ',') {
			e.preventDefault();
			add();
		} else if (e.key === 'Backspace' && !draft && value.length) {
			value = value.slice(0, -1);
		}
	}

	function remove(w: string) {
		value = value.filter((x) => x !== w);
	}
</script>

<div
	class="flex flex-wrap items-center gap-1.5 rounded-xl border border-line-strong bg-ink-2 px-2 py-2 transition-colors focus-within:border-accent"
>
	{#each value as w (w)}
		<span
			class="inline-flex items-center gap-1 rounded-full border border-line bg-ink-2 py-1 pl-2.5 pr-1 text-xs font-medium text-ink"
		>
			<span class="max-w-[14rem] truncate font-mono">{w}</span>
			<button
				type="button"
				class="grid size-4 place-items-center rounded-full opacity-70 transition hover:bg-accent/20 hover:opacity-100"
				onclick={() => remove(w)}
				aria-label="Remove {w}"
			>
				<X size={12} />
			</button>
		</span>
	{/each}
	<input
		class="h-6 min-w-[8rem] flex-1 bg-transparent px-1 text-sm text-ink outline-none placeholder:text-faint"
		{placeholder}
		bind:value={draft}
		onkeydown={keydown}
		onblur={add}
		autocomplete="off"
		spellcheck="false"
	/>
</div>
