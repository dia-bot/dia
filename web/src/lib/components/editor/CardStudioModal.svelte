<script lang="ts">
	// The Card Studio launched in-context (e.g. from the welcome card section). It
	// fills the dashboard content area (the positioned <main>), not the whole
	// viewport, so the nav/header stay visible. The parent mounts it only while
	// open ({#if}), so a fresh EditorStore is seeded from a COPY of the incoming
	// layout each time — Cancel always discards, Apply commits via onApply.
	import { setContext, untrack, onMount } from 'svelte';
	import { beforeNavigate, goto } from '$app/navigation';
	import { fade } from 'svelte/transition';
	import { EditorStore, EDITOR_CTX } from '$lib/layout/editor.svelte';
	import type { Layout } from '$lib/layout/schema';
	import { guildFonts } from '$lib/api';
	import LayoutEditor from '$lib/components/editor/LayoutEditor.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
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

	// Unsaved-changes guard. "Dirty" = the studio's layout differs from what we were
	// seeded with. Cancelling, or navigating away (switching tabs) while dirty, opens
	// the confirm dialog instead of silently discarding. Computed on demand (never in
	// a $derived) so it can't re-stringify the layout on every drag frame.
	const baseline = untrack(() => JSON.stringify($state.snapshot(layout) as Layout));
	function isDirty(): boolean {
		return JSON.stringify(store.toJSON()) !== baseline;
	}

	let confirmOpen = $state(false);
	let proceed: (() => void) | null = null; // what "leave" does once confirmed
	let bypassGuard = false;

	function requestClose() {
		if (!isDirty()) {
			onClose();
			return;
		}
		proceed = onClose;
		confirmOpen = true;
	}
	beforeNavigate((nav) => {
		if (bypassGuard || nav.type === 'leave') return; // confirmed leave, or browser unload
		if (!isDirty() || !nav.to) return;
		const url = nav.to.url;
		nav.cancel();
		proceed = () => {
			bypassGuard = true;
			goto(url);
		};
		confirmOpen = true;
	});
	function keepEditing() {
		proceed = null;
	}
	function discardAndLeave() {
		const go = proceed;
		proceed = null;
		go?.();
	}
	function applyAndLeave() {
		onApply(store.toJSON());
		const go = proceed;
		proceed = null;
		if (go) go();
		else onClose();
	}
</script>

<svelte:window
	onkeydown={(e) => {
		if (e.key === 'Escape' && !confirmOpen) requestClose();
	}}
/>

<!-- A large, obviously-floating popup over a dimmed, blurred backdrop. Clicking the
     margin (backdrop) or pressing Esc closes it — guarded by the unsaved check. -->
<button
	type="button"
	aria-label="Close Card Studio"
	onclick={requestClose}
	transition:fade={{ duration: 120 }}
	class="fixed inset-0 z-40 cursor-default bg-black/65 backdrop-blur-sm"
></button>
<div
	transition:fade={{ duration: 120 }}
	class="fixed inset-3 z-50 overflow-hidden rounded-2xl border border-line-strong bg-bg shadow-2xl md:inset-6 lg:inset-8"
>
	<LayoutEditor {guildId} {extraVars} title="Card Studio">
		{#snippet actions()}
			<button
				type="button"
				onclick={requestClose}
				class="inline-flex h-8 items-center justify-center gap-1.5 rounded-md border border-line-strong px-2.5 text-xs font-medium text-ink transition-colors hover:bg-ink-2 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-line-strong disabled:pointer-events-none disabled:opacity-40"
			>
				<X size={13} /> Cancel
			</button>
			<button
				type="button"
				onclick={apply}
				class="inline-flex h-8 items-center justify-center gap-1.5 rounded-md bg-ink px-2.5 text-xs font-medium text-bg transition-colors hover:opacity-90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-line-strong disabled:pointer-events-none disabled:opacity-40"
			>
				<Check size={13} /> Apply to card
			</button>
		{/snippet}
	</LayoutEditor>
</div>

<ConfirmDialog
	bind:open={confirmOpen}
	title="Unsaved changes"
	description="You’ve made changes in the Card Studio that haven’t been applied yet. What would you like to do?"
	cancelLabel="Keep editing"
	discardLabel="Discard"
	confirmLabel="Apply to card"
	oncancel={keepEditing}
	ondiscard={discardAndLeave}
	onconfirm={applyAndLeave}
/>
