<script lang="ts">
	// Direction-aware panel transition for in-page tabs: the incoming panel
	// slides in horizontally from the side you're moving toward, so a tab
	// switch reads as movement instead of a same-looking fade.
	//
	//   <TabSwipe key={tab} index={TABS.findIndex((t) => t.k === tab)}>
	//     ...panel...
	//   </TabSwipe>
	import type { Snippet } from 'svelte';
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';

	let { key, index, children }: { key: string | number; index: number; children: Snippet } =
		$props();

	// Compare the incoming tab index against the one rendered last. `prev` is
	// deliberately non-reactive: the derived reads `index` (its only reactive
	// dependency) and advances `prev` as it recomputes, so the travel direction
	// is ready within the same render that re-keys the panel.
	// svelte-ignore state_referenced_locally
	let prev = index;
	const dir = $derived.by(() => {
		const d = index === prev ? 0 : index > prev ? 1 : -1;
		prev = index;
		return d;
	});
</script>

{#key key}
	<div in:fly={{ x: 36 * (dir || 1), duration: dur(220), easing: cubicOut }}>
		{@render children()}
	</div>
{/key}
