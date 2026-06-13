<script lang="ts">
	// "What you can use after this step" — the plain-language teaching panel.
	// For any step that saves a value, it shows the LIVE variable name the admin
	// chose plus every field they can pull out of it, as click-to-copy template
	// snippets. The shapes come from stepProducedVar(), which mirrors the Go
	// runtime exactly, so what we show is what actually exists at run time.
	import type { ProducedVar } from '$lib/commands/expr-meta';
	import Check from 'lucide-svelte/icons/check';
	import Copy from 'lucide-svelte/icons/copy';

	let { produced }: { produced: ProducedVar } = $props();

	let copied = $state('');
	let timer: ReturnType<typeof setTimeout> | null = null;
	function copy(text: string) {
		try {
			navigator.clipboard?.writeText(text);
		} catch {
			/* clipboard may be unavailable; the snippet is still visible to copy by hand */
		}
		copied = text;
		if (timer) clearTimeout(timer);
		timer = setTimeout(() => (copied = ''), 1200);
	}

	const base = $derived(`{{ .Vars.${produced.name} }}`);
	const idxToken = $derived(produced.indexVar ? `{{ .Vars.${produced.indexVar} }}` : '');
</script>

{#snippet tag(token: string, placeholder: boolean)}
	{#if placeholder}
		<code
			class="rounded border border-dashed border-line bg-bg px-1 py-0.5 font-mono text-[10.5px] text-muted"
			>{token}</code
		>
	{:else}
		<button
			type="button"
			class="group inline-flex items-center gap-1 rounded border border-line bg-bg px-1 py-0.5 font-mono text-[10.5px] text-accent-ink transition-colors hover:border-line-strong"
			title="Click to copy"
			onclick={() => copy(token)}
		>
			<span>{token}</span>
			{#if copied === token}
				<Check size={10} class="text-success" />
			{:else}
				<Copy size={10} class="text-faint" />
			{/if}
		</button>
	{/if}
{/snippet}

{#if produced.named}
	<div class="rounded-md border border-line bg-surface/30 p-3">
		<div class="eyebrow mb-1.5 text-faint">Use this in later steps</div>
		<p class="mb-2 text-[11.5px] leading-snug text-muted">
			This step saves {produced.summary} as
			{@render tag(base, false)}{#if produced.fields.length}. Pull out a part of it with:{:else}, which you can drop into any later step.{/if}
		</p>

		{#if produced.fields.length}
			<ul class="space-y-1">
				{#each produced.fields as f (f.key)}
					<li class="flex flex-wrap items-center gap-x-2 gap-y-1">
						{@render tag(`{{ .Vars.${produced.name}.${f.key} }}`, f.key.includes('<'))}
						<span class="text-[11px] text-muted">{f.short}</span>
					</li>
				{/each}
			</ul>
		{/if}

		{#if produced.indexVar}
			<p class="mt-2 flex flex-wrap items-center gap-2 text-[11px] text-muted">
				Loop position: {@render tag(idxToken, false)}
			</p>
		{/if}

		<p class="mt-2 text-[10.5px] text-faint">Click a tag to copy it.</p>
	</div>
{:else}
	<div class="rounded-md border border-dashed border-line bg-surface/20 p-3">
		<p class="text-[11.5px] leading-snug text-muted">
			Give this step a name in <span class="text-ink">“{produced.nameLabel}”</span> above, then you
			can use {produced.summary} in any later step.
		</p>
	</div>
{/if}
