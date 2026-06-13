<script lang="ts" module>
	export type DockState =
		| 'hidden'
		| 'resting'
		| 'dirty'
		| 'saving'
		| 'publishing'
		| 'saved'
		| 'published'
		| 'error';
</script>

<script lang="ts">
	// The release dock: every way a command leaves the editor (save, publish,
	// discard) lives in this floating capsule over the canvas. One element
	// morphs through the whole lifecycle: resting draft, unsaved changes,
	// in-flight with a progress beam, a settled check, or a shaken error.
	// The header above stays identity-only.
	import { fade, fly, draw } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { Tween } from 'svelte/motion';
	import { dur } from '$lib/motion';
	import { Popover } from '$lib/components/ui';
	import PreflightIssues from './PreflightIssues.svelte';
	import type { ValidationIssue } from '$lib/commands/types';

	import Send from 'lucide-svelte/icons/send';
	import CircleAlert from 'lucide-svelte/icons/circle-alert';
	import Undo2 from 'lucide-svelte/icons/undo-2';
	import X from 'lucide-svelte/icons/x';

	let {
		dock,
		status,
		enabled,
		version,
		validating = false,
		hasValidation = false,
		errorCount = 0,
		issues = [],
		requiresDefer = false,
		error = '',
		errorAction = 'save',
		pillVisible = false,
		drawerOpen = false,
		drawerWide = false,
		onSave,
		onPublish,
		onDiscard,
		onRetry,
		onDismissError,
		onJumpToIssue
	}: {
		dock: DockState;
		status: string;
		enabled: boolean;
		version: number;
		validating?: boolean;
		hasValidation?: boolean;
		errorCount?: number;
		issues?: ValidationIssue[];
		requiresDefer?: boolean;
		error?: string;
		errorAction?: 'save' | 'publish';
		// Raised by the page when Cmd/Ctrl+S lands on a clean editor; the
		// shortcut always answers, even when there is nothing to save.
		pillVisible?: boolean;
		drawerOpen?: boolean;
		drawerWide?: boolean;
		onSave: () => void;
		onPublish: () => void;
		onDiscard: () => void;
		onRetry: () => void;
		onDismissError: () => void;
		onJumpToIssue: (issue: ValidationIssue) => void;
	} = $props();

	const isMac =
		typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform ?? '');
	const kbd = isMac ? '⌘S' : 'Ctrl S';

	const inFlight = $derived(dock === 'saving' || dock === 'publishing');
	const settled = $derived(dock === 'saved' || dock === 'published');

	const visible = $derived(dock !== 'hidden' || pillVisible);

	// Status announcements live in persistent sr-only regions below: a live
	// region must already exist in the accessibility tree when its text
	// changes, so the visual spans (recreated per state) cannot carry
	// aria-live themselves.
	const liveMessage = $derived.by(() => {
		if (dock === 'saving') return 'Saving';
		if (dock === 'publishing') return 'Publishing to Discord';
		if (dock === 'published') return `Published version ${version} on Discord`;
		if (dock === 'saved') return enabled ? 'Saved, changes are live' : 'Saved';
		if (pillVisible) return 'Everything is saved';
		return '';
	});

	// Width morph: the capsule physically resizes between states. The active
	// content row reports its natural width through an action; the shell
	// tweens to it. First measurement lands instantly so the dock never grows
	// in from nothing.
	const shellW = new Tween(0, { duration: 240, easing: cubicOut });
	let measured = $state(false);
	function reportWidth(w: number) {
		if (w <= 0) return;
		if (!measured) {
			measured = true;
			void shellW.set(w, { duration: 0 });
		} else {
			void shellW.set(w, { duration: dur(240) });
		}
	}
	function measure(el: HTMLElement) {
		const ro = new ResizeObserver(() => reportWidth(el.offsetWidth));
		ro.observe(el);
		reportWidth(el.offsetWidth);
		return {
			destroy: () => ro.disconnect()
		};
	}
	$effect(() => {
		if (!visible) measured = false;
	});

	// Settle beam: on saved/published the in-flight sweep becomes a full
	// fill that fades right out. Leaving the settled state by any route (a
	// new edit, navigation) clears the fill so it can't stick around.
	let beamDone = $state(false);
	let beamTimer: ReturnType<typeof setTimeout> | null = null;
	$effect(() => {
		if (!settled) {
			beamDone = false;
			return;
		}
		beamDone = true;
		if (beamTimer) clearTimeout(beamTimer);
		beamTimer = setTimeout(() => (beamDone = false), 450);
		return () => {
			if (beamTimer) clearTimeout(beamTimer);
		};
	});

	const publishSlot = $derived.by(() => {
		if (validating || !hasValidation) return 'checking';
		if (errorCount > 0) return 'blocked';
		return 'ready';
	});

	let preflightOpen = $state(false);

	function jump(issue: ValidationIssue) {
		preflightOpen = false;
		onJumpToIssue(issue);
	}
</script>

<span class="sr-only" role="status">{liveMessage}</span>
<span class="sr-only" role="alert">
	{dock === 'error' ? `${errorAction === 'publish' ? 'Publish' : 'Save'} failed: ${error}` : ''}
</span>

{#if visible}
	<div
		class="pointer-events-none absolute inset-x-4 bottom-4 z-40 flex justify-center @container {drawerOpen
			? `max-md:hidden ${drawerWide ? 'md:pr-[36rem] xl:pr-[42rem]' : 'md:pr-[28rem]'}`
			: ''}"
	>
		{#if pillVisible && (dock === 'hidden' || dock === 'resting')}
			<div
				in:fly|global={{ y: 8, duration: dur(200), easing: cubicOut }}
				out:fade|global={{ duration: dur(150) }}
				class="pointer-events-auto flex h-8 items-center gap-1.5 rounded-full border border-line bg-surface/95 px-3 font-mono text-[11px] text-muted shadow-[0_12px_32px_-12px_rgba(0,0,0,0.7)] backdrop-blur-md @max-[340px]:hidden"
			>
				<svg viewBox="0 0 24 24" class="size-3 text-success" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
					<path d="M4 12l5 5L20 7" />
				</svg>
				Everything is saved
			</div>
		{:else}
			<div
				in:fly|global={{ y: 12, duration: dur(260), easing: cubicOut }}
				out:fly|global={{ y: 8, duration: dur(180), easing: cubicOut }}
				class="pointer-events-auto relative max-w-full overflow-hidden rounded-[14px] border bg-surface/95 shadow-[0_16px_40px_-12px_rgba(0,0,0,0.7)] backdrop-blur-md transition-[border-color] duration-150 @max-[340px]:hidden {dock ===
				'error'
					? 'dock-shake border-danger/40'
					: 'border-line'}"
				style={measured ? `width: ${Math.ceil(shellW.current) + 2}px` : ''}
			>
				<!-- Progress beam along the top hairline -->
				{#if inFlight || beamDone}
					<div class="absolute inset-x-0 top-0 h-[1.5px] overflow-hidden rounded-t-[14px]">
						{#if beamDone}
							<div
								out:fade={{ duration: dur(240) }}
								class="h-full w-full {dock === 'published' ? 'bg-accent' : 'bg-ink/60'}"
							></div>
						{:else}
							<div
								class="dock-beam-sweep h-full w-[30%] {dock === 'publishing'
									? 'bg-accent'
									: 'bg-ink/60'}"
							></div>
						{/if}
					</div>
				{/if}

				<div class="grid">
					{#key dock}
						<div
							use:measure
							in:fade={{ duration: dur(160), delay: dur(60) }}
							out:fade={{ duration: dur(120) }}
							class="col-start-1 row-start-1 flex h-11 w-max max-w-full items-center gap-2.5 justify-self-start whitespace-nowrap px-3.5"
						>
							{#if dock === 'resting'}
								<span class="flex items-center gap-2">
									<span class="size-1.5 rounded-full border border-faint"></span>
									<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-muted">
										Draft
									</span>
								</span>
								{@render publishArea()}
							{:else if dock === 'dirty'}
								<span class="flex items-center gap-2">
									<span class="size-1.5 animate-pulse rounded-full bg-ink/70"></span>
									<span class="text-[12px] font-medium text-ink">Unsaved changes</span>
								</span>
								<div class="flex items-center gap-1.5">
									<button
										type="button"
										class="inline-flex h-8 items-center gap-1.5 rounded-md px-2.5 text-[12.5px] font-medium text-muted transition-colors hover:text-ink"
										onclick={onDiscard}
										title="Restore last saved version"
									>
										<Undo2 size={12} class="hidden @max-[480px]:block" />
										<span class="@max-[480px]:sr-only">Discard</span>
									</button>
									{#if status === 'draft'}
										<button
											type="button"
											class="inline-flex h-8 items-center gap-1.5 rounded-md border border-line bg-bg px-3 text-[12.5px] font-medium text-ink transition-colors hover:border-line-strong"
											onclick={onSave}
										>
											<span class="@max-[480px]:hidden">Save draft</span>
											<span class="hidden @max-[480px]:inline">Save</span>
											<kbd class="font-mono text-[9px] text-faint @max-[480px]:hidden">{kbd}</kbd>
										</button>
										{@render publishArea()}
									{:else}
										<button
											type="button"
											class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3.5 text-[12.5px] font-medium text-bg transition-opacity hover:opacity-90"
											onclick={onSave}
											title="Members get your changes as soon as you save"
										>
											Save
											<kbd class="font-mono text-[9px] text-bg/50 @max-[480px]:hidden">{kbd}</kbd>
										</button>
									{/if}
								</div>
							{:else if dock === 'saving' || dock === 'publishing'}
								<span class="flex items-center gap-2.5">
									<span class="dots-loader" aria-hidden="true"><span></span><span></span><span></span></span>
									<span class="text-[12.5px] font-medium text-ink">
										{dock === 'saving' ? 'Saving' : 'Publishing to Discord'}
									</span>
								</span>
							{:else if dock === 'saved' || dock === 'published'}
								<span class="flex items-center gap-2">
									<svg viewBox="0 0 24 24" class="size-3.5 text-success" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
										<path in:draw={{ duration: dur(280), easing: cubicOut }} d="M4 12l5 5L20 7" />
									</svg>
									<span class="text-[12.5px] font-medium text-ink">
										{#if dock === 'published'}
											Published · v{version} on Discord
										{:else if enabled}
											Saved · changes are live
										{:else}
											Saved
										{/if}
									</span>
								</span>
							{:else if dock === 'error'}
								<span class="flex min-w-0 items-center gap-2">
									<CircleAlert size={13} class="shrink-0 text-danger" />
									<span class="max-w-64 truncate text-[12.5px] font-medium text-danger" title="{errorAction === 'publish' ? 'Publish' : 'Save'} failed: {error}">
										{errorAction === 'publish' ? 'Publish' : 'Save'} failed: {error}
									</span>
								</span>
								<div class="flex items-center gap-0.5">
									<button
										type="button"
										class="inline-flex h-8 items-center rounded-md px-2.5 text-[12.5px] font-medium text-ink transition-colors hover:bg-bg/60"
										onclick={onRetry}
									>
										Retry
									</button>
									<button
										type="button"
										class="grid size-7 place-items-center rounded text-faint transition-colors hover:bg-bg/60 hover:text-ink"
										onclick={onDismissError}
										aria-label="Dismiss"
									>
										<X size={12} />
									</button>
								</div>
							{/if}
						</div>
					{/key}
				</div>
			</div>
		{/if}
	</div>
{/if}

{#snippet publishArea()}
	{#if publishSlot === 'checking'}
		<span
			class="inline-flex h-8 items-center gap-2 rounded-md border border-line px-3 font-mono text-[11px] text-muted"
			aria-disabled="true"
		>
			<span class="dots-loader" aria-hidden="true"><span></span><span></span><span></span></span>
			Checking
		</span>
	{:else if publishSlot === 'blocked'}
		<Popover.Root bind:open={preflightOpen}>
			<Popover.Trigger
				class="inline-flex h-8 items-center gap-1.5 rounded-md border border-danger/30 bg-danger/5 px-3 font-mono text-[11px] text-danger transition-colors hover:border-danger/50"
				title="{errorCount} validation {errorCount === 1 ? 'error blocks' : 'errors block'} publishing"
			>
				<CircleAlert size={12} />
				{errorCount}
				{errorCount === 1 ? 'error' : 'errors'}
			</Popover.Trigger>
			<Popover.Content class="w-[340px] p-1.5" side="top" align="end" sideOffset={10}>
				<PreflightIssues {issues} {requiresDefer} onJump={jump} />
			</Popover.Content>
		</Popover.Root>
	{:else}
		<button
			type="button"
			class="inline-flex h-8 items-center gap-1.5 rounded-md bg-ink px-3.5 text-[12.5px] font-medium text-bg transition-opacity hover:opacity-90"
			onclick={onPublish}
			title="Saves your changes and publishes v{version + 1} to Discord"
		>
			<Send size={11} />
			Publish
		</button>
	{/if}
{/snippet}
