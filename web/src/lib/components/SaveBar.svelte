<script lang="ts">
	let {
		dirty = false,
		saving = false,
		onsave,
		onreset
	}: { dirty?: boolean; saving?: boolean; onsave?: () => void; onreset?: () => void } = $props();
</script>

{#if dirty}
	<div
		class="sticky bottom-4 z-20 mt-6 flex items-center justify-between gap-4 rounded-xl border border-[var(--color-line-strong)] bg-white/95 px-4 py-3 shadow-lg backdrop-blur"
	>
		<span class="text-sm text-muted">You have unsaved changes</span>
		<div class="flex items-center gap-2">
			<button class="btn btn-ghost" onclick={onreset} disabled={saving}>Reset</button>
			<button class="btn btn-accent" onclick={onsave} disabled={saving}>
				{saving ? 'Saving…' : 'Save changes'}
			</button>
		</div>
	</div>
{/if}
