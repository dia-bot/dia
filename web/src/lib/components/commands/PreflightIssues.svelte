<script lang="ts">
	// The preflight list — what stands between a draft and Discord. Shared by
	// the release dock's blocked Publish chip and the header's issues chip.
	// Clicking a row jumps to the offending step (or opens Settings when the
	// issue isn't about a step).
	import type { ValidationIssue } from '$lib/commands/types';

	import CircleAlert from 'lucide-svelte/icons/circle-alert';
	import TriangleAlert from 'lucide-svelte/icons/triangle-alert';

	let {
		issues,
		requiresDefer = false,
		onJump
	}: {
		issues: ValidationIssue[];
		requiresDefer?: boolean;
		onJump: (issue: ValidationIssue) => void;
	} = $props();

	const shown = $derived(issues.slice(0, 8));
</script>

<div class="px-2.5 pb-1 pt-2 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
	Before you publish
</div>

<div class="flex flex-col">
	{#each shown as iss (iss.path + iss.code)}
		<button
			type="button"
			class="flex items-start gap-2 rounded-md px-2.5 py-1.5 text-left transition-colors hover:bg-secondary"
			onclick={() => onJump(iss)}
		>
			{#if iss.severity === 'error'}
				<CircleAlert size={12} class="mt-0.5 shrink-0 text-danger" />
			{:else}
				<TriangleAlert size={12} class="mt-0.5 shrink-0 text-muted-foreground" />
			{/if}
			<span class="min-w-0">
				<span class="block text-[12px] leading-snug text-foreground">{iss.message}</span>
				<span class="block truncate font-mono text-[10px] text-muted-foreground/70">{iss.path}</span>
			</span>
		</button>
	{/each}
	{#if issues.length > shown.length}
		<p class="px-2.5 py-1 font-mono text-[10px] text-muted-foreground/70">
			+{issues.length - shown.length} more
		</p>
	{/if}
</div>

<div class="mt-1 border-t border-border/60 px-2.5 py-2">
	<p class="font-mono text-[10px] leading-relaxed text-muted-foreground/80">
		Errors block publishing. Warnings do not.
	</p>
	{#if requiresDefer}
		<p class="mt-0.5 font-mono text-[10px] leading-relaxed text-muted-foreground/80">
			This command auto-defers (worst case over 3 seconds)
		</p>
	{/if}
</div>
