<script lang="ts">
	// The large editing surface for one ticket type: a near-fullscreen modal
	// hosting the complete CategoryEditor (opening message, closed card, close
	// request, form, transcript, feedback, auto-close, automations). Edits apply
	// to the bound category object live; the host page's save dock persists
	// them. Modeled on the TemplateGuide modal shell.
	import { fade, scale } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';
	import type { CategoryConfig } from '$lib/tickets/types';
	import CategoryEditor from '$lib/components/tickets/CategoryEditor.svelte';
	import { Ticket, X, Trash2 } from 'lucide-svelte';

	let {
		open = $bindable(false),
		category,
		guildId,
		onRemove
	}: {
		open?: boolean;
		category: CategoryConfig | null;
		guildId: string;
		// Deletes the ticket type (the host closes the modal after).
		onRemove?: () => void;
	} = $props();

	function close() {
		open = false;
	}
	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && open) {
			e.stopPropagation();
			close();
		}
	}
</script>

<svelte:window onkeydown={onKeydown} />

{#if open && category}
	<div class="fixed inset-0 z-[70] grid place-items-center p-3 sm:p-6">
		<button
			type="button"
			class="absolute inset-0 h-full w-full cursor-default bg-black/40"
			aria-label="Dismiss"
			onclick={close}
			transition:fade={{ duration: dur(150) }}
		></button>

		<div
			class="relative flex max-h-[92vh] w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-line bg-surface shadow-2xl"
			transition:scale={{ duration: dur(200), start: 0.97, opacity: 0, easing: cubicOut }}
			role="dialog"
			aria-label="Edit ticket type"
		>
			<div class="flex shrink-0 items-center gap-2 border-b border-line px-4 py-3">
				<Ticket size={13} class="text-accent-ink" />
				<span class="text-lg leading-none">{category.emoji || '🎫'}</span>
				<span class="truncate text-[13px] font-semibold text-ink">{category.label || 'Untitled ticket type'}</span>
				<span class="hidden font-mono text-[10px] uppercase tracking-[0.14em] text-muted sm:inline">
					{category.open_mode === 'thread' ? 'private thread' : 'private channel'}
				</span>
				<div class="ml-auto flex items-center gap-1.5">
					{#if onRemove}
						<button
							type="button"
							onclick={onRemove}
							class="grid size-7 place-items-center rounded-md border border-line text-muted hover:border-line-strong hover:text-danger"
							aria-label="Delete ticket type"
						>
							<Trash2 size={13} />
						</button>
					{/if}
					<button
						type="button"
						onclick={close}
						class="inline-flex h-7 items-center gap-1.5 rounded-md bg-ink px-2.5 text-[12px] font-semibold text-bg hover:bg-ink/90"
					>
						Done
					</button>
					<button
						type="button"
						onclick={close}
						class="grid size-7 place-items-center rounded-md text-muted hover:bg-ink-2 hover:text-ink"
						aria-label="Close"
					>
						<X size={14} />
					</button>
				</div>
			</div>

			<div class="min-h-0 flex-1 overflow-y-auto px-4 py-4 sm:px-5">
				<CategoryEditor {category} {guildId} />
			</div>
		</div>
	</div>
{/if}
