<script lang="ts">
	// Standalone Card Studio: owns an EditorStore, persists to localStorage per
	// guild, and embeds the reusable LayoutEditor chrome.
	import { getContext, setContext, onMount, onDestroy } from 'svelte';
	import { EditorStore, EDITOR_CTX } from '$lib/layout/editor.svelte';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import LayoutEditor from '$lib/components/editor/LayoutEditor.svelte';
	import { Save, Check } from 'lucide-svelte';

	const guild = getContext<GuildStore>(GUILD_CTX);
	const editor = new EditorStore();
	editor.guildId = guild.id;
	setContext(EDITOR_CTX, editor);

	const storageKey = $derived(`dia:layout:${guild.id}`);
	let saved = $state(false);
	let savedTimer: ReturnType<typeof setTimeout>;

	onMount(() => {
		if (typeof window === 'undefined') return;
		try {
			const raw = window.localStorage.getItem(storageKey);
			if (raw) editor.layout = JSON.parse(raw);
		} catch {
			/* corrupt or missing — keep the default layout */
		}
	});

	function save() {
		try {
			window.localStorage.setItem(storageKey, JSON.stringify(editor.toJSON()));
		} catch {
			/* storage blocked — fail quietly */
		}
		saved = true;
		clearTimeout(savedTimer);
		savedTimer = setTimeout(() => (saved = false), 1800);
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
<svelte:window onkeydown={onKeydown} />

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
