<script lang="ts">
	// Standalone Card Studio: owns an EditorStore, persists to localStorage per
	// guild, and embeds the reusable LayoutEditor chrome.
	import { getContext, setContext, onMount, onDestroy } from 'svelte';
	import { beforeNavigate, goto } from '$app/navigation';
	import { EditorStore, EDITOR_CTX } from '$lib/layout/editor.svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { guildFonts } from '$lib/api';
	import LayoutEditor from '$lib/components/editor/LayoutEditor.svelte';
	import ConfirmDialog from '$lib/components/ConfirmDialog.svelte';
	import { Save, Check } from 'lucide-svelte';

	const guild = getContext<GuildStore>(GUILD_CTX);
	const editor = new EditorStore();
	editor.guildId = guild.id;
	setContext(EDITOR_CTX, editor);

	const storageKey = $derived(`dia:layout:${guild.id}`);
	let saved = $state(false);
	let savedTimer: ReturnType<typeof setTimeout>;
	// Snapshot of the last-saved document; "dirty" when it differs. Computed on
	// demand (only at navigation time) — a $derived here would re-stringify the
	// whole layout on every drag frame and make editing lag.
	let savedJson = $state('');
	function isDirty() {
		return JSON.stringify(editor.toJSON()) !== savedJson;
	}

	onMount(() => {
		if (typeof window === 'undefined') return;
		try {
			const raw = window.localStorage.getItem(storageKey);
			if (raw) editor.layout = JSON.parse(raw);
		} catch {
			/* corrupt or missing — keep the default layout */
		}
		savedJson = JSON.stringify(editor.toJSON()); // baseline: nothing to save yet
		guildFonts(guild.id)
			.then((r) => editor.setFonts(r.fonts, r.premium))
			.catch(() => {});
	});

	function save() {
		try {
			window.localStorage.setItem(storageKey, JSON.stringify(editor.toJSON()));
		} catch {
			/* storage blocked — fail quietly */
		}
		savedJson = JSON.stringify(editor.toJSON());
		saved = true;
		clearTimeout(savedTimer);
		savedTimer = setTimeout(() => (saved = false), 1800);
	}

	// Unsaved-changes guard. In-app navigation (another sidebar tab, the back button…)
	// is intercepted and routed through the polished confirm dialog below; the
	// browser's own tab-close / reload prompt can only ever be the native one.
	let leaveOpen = $state(false);
	let pendingUrl: URL | null = null;
	let bypassGuard = false;

	beforeNavigate((nav) => {
		if (bypassGuard || nav.type === 'leave') return; // a confirmed leave, or browser unload (native prompt)
		if (!isDirty() || !nav.to) return;
		nav.cancel();
		pendingUrl = nav.to.url;
		leaveOpen = true;
	});
	function leaveWithoutSaving() {
		const url = pendingUrl;
		pendingUrl = null;
		bypassGuard = true;
		if (url) goto(url);
	}
	function saveAndLeave() {
		save();
		leaveWithoutSaving();
	}
	function keepEditing() {
		pendingUrl = null;
	}

	function onBeforeUnload(e: BeforeUnloadEvent) {
		if (isDirty()) {
			e.preventDefault();
			e.returnValue = ''; // shows the browser's native "leave site?" prompt
		}
	}

	function onKeydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
			e.preventDefault();
			save();
		}
	}
	onDestroy(() => clearTimeout(savedTimer));
</script>

<svelte:head><title>Card Studio · {guild.name} · Dia</title></svelte:head>
<svelte:window onkeydown={onKeydown} onbeforeunload={onBeforeUnload} />

<!-- Break out of the dashboard column for an edge-to-edge editor. -->
<div class="-m-6 -my-7 h-[calc(100vh-3.5rem-1px)]">
	<LayoutEditor guildId={guild.id}>
		{#snippet actions()}
			{#if saved}
				<span class="flex items-center gap-1 text-[12px] font-medium text-success">
					<Check size={13} /> Saved
				</span>
			{/if}
			<button
				type="button"
				onclick={save}
				class="flex h-7 items-center gap-1.5 rounded-md bg-ink px-2.5 text-[12px] font-medium text-bg transition-opacity hover:opacity-90"
			>
				<Save size={13} /> Save
			</button>
		{/snippet}
	</LayoutEditor>
</div>

<ConfirmDialog
	bind:open={leaveOpen}
	title="Unsaved changes"
	description="You’ve made changes to this card that haven’t been saved yet. What would you like to do?"
	cancelLabel="Keep editing"
	discardLabel="Discard"
	confirmLabel="Save & leave"
	oncancel={keepEditing}
	ondiscard={leaveWithoutSaving}
	onconfirm={saveAndLeave}
/>
