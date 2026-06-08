<script lang="ts">
	// A polished, accessible confirm dialog (bits-ui AlertDialog → focus-trap, escape
	// handling, portal, role="alertdialog"). Motion is CSS-driven via the data-state
	// attribute (see .dialog-overlay / .dialog-content in app.css), so it animates in
	// AND out. Up to three outcomes: cancel (safe), an optional discard, and confirm.
	import { AlertDialog } from 'bits-ui';
	import { TriangleAlert } from 'lucide-svelte';

	type Props = {
		open?: boolean;
		title: string;
		description: string;
		confirmLabel?: string;
		cancelLabel?: string;
		discardLabel?: string; // optional middle action (e.g. "Discard changes")
		onconfirm?: () => void;
		oncancel?: () => void;
		ondiscard?: () => void;
	};
	let {
		open = $bindable(false),
		title,
		description,
		confirmLabel = 'Confirm',
		cancelLabel = 'Cancel',
		discardLabel,
		onconfirm,
		oncancel,
		ondiscard
	}: Props = $props();

	function choose(fn?: () => void) {
		open = false;
		fn?.();
	}
</script>

<AlertDialog.Root bind:open>
	<AlertDialog.Portal>
		<AlertDialog.Overlay
			class="dialog-overlay fixed inset-0 z-[70] bg-ink/60 backdrop-blur-[3px]"
		/>
		<AlertDialog.Content
			class="dialog-content fixed left-1/2 top-1/2 z-[71] w-[min(92vw,26rem)] -translate-x-1/2 -translate-y-1/2 rounded-2xl border border-line-strong bg-surface p-5 shadow-2xl outline-none"
		>
			<div class="flex gap-3.5">
				<div
					class="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-accent/12 text-accent-ink"
				>
					<TriangleAlert size={18} strokeWidth={2} />
				</div>
				<div class="min-w-0 flex-1 pt-0.5">
					<AlertDialog.Title class="text-[15px] font-semibold text-ink">{title}</AlertDialog.Title>
					<AlertDialog.Description class="mt-1 text-[13px] leading-relaxed text-muted">
						{description}
					</AlertDialog.Description>
				</div>
			</div>
			<div class="mt-5 flex items-center justify-end gap-2">
				<button
					type="button"
					onclick={() => choose(oncancel)}
					class="h-8 rounded-lg px-3 text-[13px] font-medium text-muted transition-colors hover:bg-ink-2 hover:text-ink"
				>
					{cancelLabel}
				</button>
				{#if discardLabel}
					<button
						type="button"
						onclick={() => choose(ondiscard)}
						class="h-8 rounded-lg px-3 text-[13px] font-medium text-danger transition-colors hover:bg-danger/12"
					>
						{discardLabel}
					</button>
				{/if}
				<button
					type="button"
					onclick={() => choose(onconfirm)}
					class="h-8 rounded-lg bg-ink px-3.5 text-[13px] font-medium text-bg transition-opacity hover:opacity-90"
				>
					{confirmLabel}
				</button>
			</div>
		</AlertDialog.Content>
	</AlertDialog.Portal>
</AlertDialog.Root>
