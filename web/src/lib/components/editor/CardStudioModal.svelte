<script lang="ts">
	// The Card Studio launched in-context (e.g. from the welcome card section). It
	// fills the dashboard content area (the positioned <main>), not the whole
	// viewport, so the nav/header stay visible. The parent mounts it only while
	// open ({#if}), so a fresh EditorStore is seeded from a COPY of the incoming
	// layout each time — Cancel always discards, Apply commits via onApply.
	import { setContext, untrack, onMount } from 'svelte';
	import { fade } from 'svelte/transition';
	import { EditorStore, EDITOR_CTX } from '$lib/layout/editor.svelte';
	import type { Layout } from '$lib/layout/schema';
	import { guildFonts } from '$lib/api';
	import LayoutEditor from '$lib/components/editor/LayoutEditor.svelte';
	import { Check, X } from 'lucide-svelte';

	let {
		layout,
		guildId,
		extraVars,
		onApply,
		onClose
	}: {
		layout: Layout;
		guildId: string;
		extraVars?: Record<string, string>;
		onApply: (l: Layout) => void;
		onClose: () => void;
	} = $props();

	// Seed a fresh store from a copy of the layout as it is right now. This is a
	// deliberate one-time read (the parent re-mounts us per open), so untrack.
	const store = new EditorStore(untrack(() => $state.snapshot(layout) as Layout));
	store.guildId = untrack(() => guildId);
	setContext(EDITOR_CTX, store);

	onMount(() => {
		guildFonts(untrack(() => guildId))
			.then((r) => store.setFonts(r.fonts, r.premium))
			.catch(() => {});
	});

	function apply() {
		onApply(store.toJSON());
		onClose();
	}
</script>

<!-- Fixed to the dashboard work area (below the 3.5rem header, right of the 260px
     sidebar) so it stays put regardless of page scroll — never anchored to the
     scrolled content. Full-bleed on mobile where the sidebar is off-canvas. -->
<div
	transition:fade={{ duration: 120 }}
	class="fixed inset-x-0 bottom-0 top-14 z-40 overflow-hidden bg-bg md:left-[260px] md:rounded-tl-2xl"
>
	<LayoutEditor {guildId} {extraVars} title="Card Studio">
		{#snippet actions()}
			<button
				type="button"
				onclick={onClose}
				class="flex h-7 items-center gap-1.5 rounded-md border border-line-strong px-2.5 text-[12px] font-medium text-muted transition-colors hover:bg-surface hover:text-ink"
			>
				<X size={13} /> Cancel
			</button>
			<button
				type="button"
				onclick={apply}
				class="flex h-7 items-center gap-1.5 rounded-md bg-ink px-2.5 text-[12px] font-medium text-bg transition-opacity hover:opacity-90"
			>
				<Check size={13} /> Apply to card
			</button>
		{/snippet}
	</LayoutEditor>
</div>
