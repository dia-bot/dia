<script lang="ts">
	// Editor for an on-error router. Flat by design: each case is a section
	// separated by a hairline — pattern chips inline, and the error-kind
	// picker floats in a popover instead of nesting boxes. Cases are checked
	// top to bottom; the first match runs; "else" catches the rest. Mutations
	// go straight into the owning step (the canvas re-derives the lines).
	import { ERROR_GROUPS, ERROR_KINDS } from '$lib/commands/expr-meta';
	import type { Step } from '$lib/commands/types';
	import Toggle from '$lib/components/Toggle.svelte';
	import { Popover } from '$lib/components/ui';
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';

	import X from 'lucide-svelte/icons/x';
	import Plus from 'lucide-svelte/icons/plus';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import ShieldAlert from 'lucide-svelte/icons/shield-alert';
	import Check from 'lucide-svelte/icons/check';

	let {
		step,
		onClose
	}: {
		step: Step;
		onClose: () => void;
	} = $props();

	const cases = $derived(step.on_error_cases ?? []);
	const hasElse = $derived(step.on_error !== undefined);

	let pickerGroup = $state<string>('discord');
	let custom = $state('');

	function touch() {
		// New array identity so the deep-tracking rebuild effect fires.
		if (step.on_error_cases) step.on_error_cases = [...step.on_error_cases];
	}

	function togglePattern(ei: number, pat: string) {
		const ec = cases[ei];
		if (!ec) return;
		const i = ec.when.indexOf(pat);
		if (i >= 0) ec.when.splice(i, 1);
		else {
			// "Anything" is a placeholder — replace it with the first real pick.
			if (ec.when.length === 1 && ec.when[0] === '*') ec.when.length = 0;
			ec.when.push(pat);
		}
		touch();
	}

	function addCustom(ei: number) {
		const pat = custom.trim();
		if (!pat) return;
		togglePattern(ei, pat);
		custom = '';
	}

	function addCase() {
		step.on_error_cases = [...cases, { when: ['*'], do: [] }];
	}

	function removeCase(ei: number) {
		step.on_error_cases = cases.filter((_, i) => i !== ei);
		if (step.on_error_cases.length === 0) step.on_error_cases = undefined;
	}

	function toggleElse() {
		if (hasElse) step.on_error = undefined;
		else step.on_error = [];
	}

	function removeAll() {
		step.on_error = undefined;
		step.on_error_cases = undefined;
		onClose();
	}
</script>

<div
	in:fly={{ y: -8, duration: dur(200), easing: cubicOut }}
	out:fly={{ y: -6, duration: dur(140), easing: cubicOut }}
	class="absolute right-3 top-3 z-30 flex max-h-[calc(100%-1.5rem)] w-[320px] max-w-[calc(100%-1.5rem)] flex-col rounded-lg border border-border bg-popover text-popover-foreground shadow-[0_16px_40px_-12px_rgba(0,0,0,0.7)]"
>
	<div class="flex h-11 shrink-0 items-center gap-2 border-b border-border/60 px-3.5">
		<ShieldAlert class="size-3.5 shrink-0 text-destructive" />
		<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
			On error
		</span>
		<span class="text-[10.5px] text-muted-foreground/60">first match runs</span>
		<button
			type="button"
			class="ml-auto grid size-6 shrink-0 place-items-center rounded text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
			onclick={onClose}
			aria-label="Close"
		>
			<X class="size-3.5" />
		</button>
	</div>

	<div class="min-h-0 flex-1 overflow-y-auto">
		{#each cases as ec, ei (ei)}
			<div class="group/case border-b border-border/40 px-3.5 py-2.5">
				<div class="mb-1.5 flex items-center gap-2">
					<span class="font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
						Case {ei + 1}
					</span>
					<span class="font-mono text-[9.5px] tabular-nums text-muted-foreground/50">
						{ec.do?.length ?? 0} step{(ec.do?.length ?? 0) === 1 ? '' : 's'}
					</span>
					<button
						type="button"
						class="ml-auto grid size-5 place-items-center rounded text-muted-foreground/40 opacity-0 transition-all hover:text-destructive group-hover/case:opacity-100"
						onclick={() => removeCase(ei)}
						title="Delete this case and its steps"
						aria-label="Delete case"
					>
						<Trash2 class="size-3" />
					</button>
				</div>
				<div class="flex flex-wrap items-center gap-1">
					{#each ec.when as pat (pat)}
						<button
							type="button"
							class="inline-flex h-6 items-center gap-1 rounded-md bg-secondary px-2 font-mono text-[10px] text-foreground transition-colors hover:bg-secondary/70"
							onclick={() => togglePattern(ei, pat)}
							title="Remove"
						>
							{pat === '*' ? 'anything' : pat}
							<X class="size-2.5 opacity-50" />
						</button>
					{/each}
					<Popover.Root>
						<Popover.Trigger
							class="grid size-6 place-items-center rounded-md text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
						>
							<Plus class="size-3" />
						</Popover.Trigger>
						<Popover.Content class="w-72 p-0" align="start">
							<div class="flex flex-wrap items-center gap-0.5 border-b border-border/60 px-2 py-1.5">
								{#each ERROR_GROUPS as g (g.id)}
									<button
										type="button"
										class="rounded px-1.5 py-0.5 font-mono text-[9.5px] font-medium uppercase tracking-[0.1em] transition-colors {pickerGroup === g.id
											? 'bg-secondary text-foreground'
											: 'text-muted-foreground hover:text-foreground'}"
										onclick={() => (pickerGroup = g.id)}
									>
										{g.label}
									</button>
								{/each}
							</div>
							<div class="max-h-44 overflow-y-auto p-1">
								<button
									type="button"
									class="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left transition-colors hover:bg-secondary"
									onclick={() => togglePattern(ei, `${pickerGroup}.*`)}
								>
									<span class="grid size-3.5 shrink-0 place-items-center">
										{#if ec.when.includes(`${pickerGroup}.*`)}<Check class="size-3" />{/if}
									</span>
									<code class="shrink-0 font-mono text-[10.5px] font-medium text-foreground">{pickerGroup}.*</code>
									<span class="truncate text-[10px] text-muted-foreground">whole group</span>
								</button>
								{#each ERROR_KINDS.filter((k) => k.group === pickerGroup) as k (k.id)}
									<button
										type="button"
										class="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left transition-colors hover:bg-secondary"
										onclick={() => togglePattern(ei, k.id)}
									>
										<span class="grid size-3.5 shrink-0 place-items-center">
											{#if ec.when.includes(k.id)}<Check class="size-3" />{/if}
										</span>
										<code class="shrink-0 font-mono text-[10.5px] text-foreground">{k.id}</code>
										<span class="min-w-0 flex-1 truncate text-[10px] text-muted-foreground">{k.short}</span>
									</button>
								{/each}
							</div>
							<form
								class="flex items-center gap-1.5 border-t border-border/60 p-2"
								onsubmit={(e) => {
									e.preventDefault();
									addCustom(ei);
								}}
							>
								<input
									class="h-6 min-w-0 flex-1 rounded border border-input bg-background px-1.5 font-mono text-[10.5px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
									placeholder="custom: *.timeout"
									bind:value={custom}
								/>
								<button
									type="submit"
									class="h-6 shrink-0 rounded bg-foreground px-2 text-[10.5px] font-medium text-background transition-opacity hover:opacity-90 disabled:opacity-40"
									disabled={!custom.trim()}
								>
									Add
								</button>
							</form>
						</Popover.Content>
					</Popover.Root>
				</div>
			</div>
		{/each}

		<button
			type="button"
			class="flex h-9 w-full items-center gap-1.5 border-b border-border/40 px-3.5 text-[11.5px] font-medium text-muted-foreground transition-colors hover:bg-secondary/40 hover:text-foreground"
			onclick={addCase}
		>
			<Plus class="size-3" />
			Add error case
		</button>

		<label class="flex h-10 items-center justify-between gap-2 px-3.5">
			<span class="text-[11.5px] text-muted-foreground">
				<span class="font-medium text-foreground">Else</span> — catch everything left
			</span>
			<Toggle checked={hasElse} onchange={toggleElse} />
		</label>
	</div>

	<div class="shrink-0 border-t border-border/60">
		<button
			type="button"
			class="flex h-9 w-full items-center justify-center gap-1.5 rounded-b-lg text-[11.5px] font-medium text-destructive transition-colors hover:bg-destructive/10"
			onclick={removeAll}
			title="Remove all error handling (and its steps) from this step"
		>
			<Trash2 class="size-3" />
			Remove error handling
		</button>
	</div>
</div>
