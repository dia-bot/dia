<script lang="ts">
	// Connection editor — click a line on the canvas to see exactly when that
	// path runs and edit it in place: the if condition, a switch case's match
	// value. Lines can also be cancelled from here (delete the case / branch,
	// or the step the line leads to). Error lines open the on-error panel
	// instead (FlowInner routes them there).
	import type { EdgeInfo } from './adapter';
	import { fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';

	import X from 'lucide-svelte/icons/x';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import Unlink from 'lucide-svelte/icons/unlink';
	import Toggle from '$lib/components/Toggle.svelte';

	let {
		info,
		onClose,
		onDeleteStep,
		onTruncateChain,
		onAbsorbAfter,
		onDetach
	}: {
		info: EdgeInfo;
		onClose: () => void;
		onDeleteStep: (id: string) => void;
		onTruncateChain?: (id: string) => void;
		// Tuck an if/switch's legacy after-chain into one of its branches.
		onAbsorbAfter?: (id: string, which: 'then' | 'else' | 'default') => void;
		// Detach the line: the downstream chain becomes a disconnected island
		// (steps survive; reconnect by dragging a dot onto the island's head).
		onDetach?: (id: string) => void;
	} = $props();

	const step = $derived(info.sourceStep);
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const spec = $derived((step?.spec ?? {}) as any);

	const title = $derived.by(() => {
		switch (info.branch) {
			case 'then':
				return 'True path';
			case 'else':
				return 'False path';
			case 'case':
				return `Case ${(info.caseIndex ?? 0) + 1}`;
			case 'default':
				return 'Default path';
			case 'body':
				return 'Loop body';
			case 'parallel':
				return `Branch ${(info.caseIndex ?? 0) + 1}`;
			case 'click':
				return 'On click';
			case 'on_error':
				if (info.rail) return 'Error rail';
				return info.caseIndex !== undefined ? `Error case ${(info.caseIndex ?? 0) + 1}` : 'On error';
			default:
				if (step?.kind === 'if' || step?.kind === 'switch') return 'After every path';
				return 'Sequence link';
		}
	});

	const isAfterBranching = $derived(
		!info.branch && !info.rail && (step?.kind === 'if' || step?.kind === 'switch')
	);

	// Click paths: the line owns a hidden wait_for; edit it here.
	const wait = $derived(info.clickWait ?? null);
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const waitSpec = $derived((wait?.spec ?? {}) as any);
	// Legacy definitions stored the brace shorthand; the editor only writes
	// Go template syntax now.
	const invokerOnly = $derived(
		['{{ .User.ID }}', '{user.id}'].includes((waitSpec.from_user?.src ?? '').trim())
	);

	// How the bot answers THIS button's click on Discord. One listener serves
	// the whole message, so per-button modes live in a suffix-keyed map on the
	// wait spec; the plain `response` field is the listener-wide default.
	const RESPONSE_MODES = [
		{
			value: 'reply',
			label: 'Replies',
			hint: "Shows the bot thinking until the flow's first Message step answers."
		},
		{
			value: 'update',
			label: 'Updates this message',
			hint: "The flow's first Message step rewrites the clicked message in place."
		},
		{
			value: 'silent',
			label: 'Just acknowledges',
			hint: 'Nothing shows at the click. Any later Message step posts a fresh, separate message.'
		}
	];
	const clickSuffix = $derived.by(() => {
		if (info.clickSwitch && info.caseIndex !== undefined) {
			return info.clickSwitch.cases?.[info.caseIndex]?.when?.src ?? '';
		}
		return '';
	});
	const clickResponse = $derived.by(() => {
		if (clickSuffix && waitSpec.responses?.[clickSuffix]) return waitSpec.responses[clickSuffix];
		return waitSpec.response || 'reply';
	});
	function setClickResponse(mode: string) {
		if (!wait) return;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const sp = { ...((wait.spec ?? {}) as any) };
		if (clickSuffix) {
			const rs = { ...(sp.responses ?? {}) };
			// Dropping back to the default only works when no listener-wide
			// override would shadow it.
			if (mode === 'reply' && !sp.response) delete rs[clickSuffix];
			else rs[clickSuffix] = mode;
			if (Object.keys(rs).length === 0) delete sp.responses;
			else sp.responses = rs;
		} else if (mode === 'reply') {
			delete sp.response;
		} else {
			sp.response = mode;
		}
		wait.spec = sp;
	}

	// Removing a click path: routed (switch) arms drop their case; the
	// listener + switch are deleted once the last case goes.
	function removeClickArm(keepSteps: boolean) {
		const sw = info.clickSwitch;
		if (keepSteps) onDetach?.(info.targetId);
		else onTruncateChain?.(info.targetId);
		if (sw && info.caseIndex !== undefined) {
			sw.cases = (sw.cases ?? []).filter((_, i) => i !== info.caseIndex);
			if ((sw.cases ?? []).length === 0) {
				onDeleteStep(sw.id);
				if (wait) onDeleteStep(wait.id);
			}
		} else if (wait) {
			onDeleteStep(wait.id);
		}
		onClose();
	}

	function patchWait(field: string, value: unknown) {
		if (!wait) return;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const sp = { ...((wait.spec ?? {}) as any) };
		if (value === '' || value === undefined) delete sp[field];
		else sp[field] = value;
		wait.spec = sp;
	}

	function setCond(src: string) {
		if (!step) return;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const s = (step.spec ?? {}) as any;
		s.cond = { lang: 'tmpl', src };
		step.spec = { ...s };
	}

	function setCaseWhen(src: string) {
		if (!step || info.caseIndex === undefined) return;
		const cases = step.cases ?? [];
		if (!cases[info.caseIndex]) return;
		cases[info.caseIndex].when = { lang: 'tmpl', src };
	}

	function deleteCase() {
		if (!step || info.caseIndex === undefined) return;
		step.cases?.splice(info.caseIndex, 1);
		onClose();
	}

	function clearArm(which: 'then' | 'else' | 'default') {
		if (!step) return;
		step[which] = [];
		onClose();
	}

	function deleteParallelBranch() {
		if (!step || info.caseIndex === undefined) return;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const s = (step.spec ?? {}) as any;
		(s.branches ?? []).splice(info.caseIndex, 1);
		step.spec = { ...s };
		onClose();
	}

	const armCount = $derived.by(() => {
		if (!step) return 0;
		if (info.branch === 'then') return step.then?.length ?? 0;
		if (info.branch === 'else') return step.else?.length ?? 0;
		if (info.branch === 'default') return step.default?.length ?? 0;
		return 0;
	});
</script>

<div
	in:fly={{ y: -8, duration: dur(200), easing: cubicOut }}
	out:fly={{ y: -6, duration: dur(140), easing: cubicOut }}
	class="absolute right-3 top-3 z-30 w-[300px] max-w-[calc(100%-1.5rem)] rounded-lg border border-border bg-popover p-3 text-popover-foreground shadow-[0_16px_40px_-12px_rgba(0,0,0,0.7)]"
>
	<div class="mb-2 flex items-center justify-between gap-2">
		<span class="truncate font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
			{title}
		</span>
		<button
			type="button"
			class="grid size-6 shrink-0 place-items-center rounded text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
			onclick={onClose}
			aria-label="Close"
		>
			<X class="size-3.5" />
		</button>
	</div>

	{#if info.branch === 'then' || info.branch === 'else'}
		<p class="mb-2 text-[11.5px] leading-relaxed text-muted-foreground">
			Runs when the condition is
			<span class="font-medium text-foreground">{info.branch === 'then' ? 'true' : 'false'}</span>.
		</p>
		<span class="mb-1 block font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
			Condition
		</span>
		<input
			class="mb-2 h-7 w-full rounded-md border border-input bg-background px-2 font-mono text-[11.5px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
			placeholder={'{{ gt .Input.amount 100 }}'}
			value={spec.cond?.src ?? ''}
			oninput={(e) => setCond((e.currentTarget as HTMLInputElement).value)}
		/>
		<p class="text-[10.5px] leading-relaxed text-muted-foreground">
			Truthy = non-empty / non-zero. Both paths continue independently — chain
			anything after either one.
		</p>
		{#if armCount > 0}
			<div class="mt-2 flex flex-col items-start gap-0.5">
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-foreground transition-colors hover:bg-secondary"
				onclick={() => {
					onDetach?.(info.targetId);
					onClose();
				}}
				title="Break the line; the steps survive as a disconnected island"
			>
				<Unlink class="size-3" />
				Disconnect the line (keep the steps)
			</button>
				<button
					type="button"
					class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-destructive transition-colors hover:bg-destructive/10"
					onclick={() => clearArm(info.branch === 'then' ? 'then' : 'else')}
					title="Remove every step on this path"
				>
					<Trash2 class="size-3" />
					Clear this path ({armCount} step{armCount === 1 ? '' : 's'})
				</button>
			</div>
		{/if}
	{:else if info.branch === 'case'}
		<p class="mb-2 text-[11.5px] leading-relaxed text-muted-foreground">
			Taken when the switch value
			<span class="font-mono text-foreground">{spec.on?.src || '(set on the switch)'}</span>
			equals:
		</p>
		<input
			class="mb-2 h-7 w-full rounded-md border border-input bg-background px-2 font-mono text-[11.5px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
			placeholder={'"mod" — a value or template'}
			value={step?.cases?.[info.caseIndex ?? -1]?.when?.src ?? ''}
			oninput={(e) => setCaseWhen((e.currentTarget as HTMLInputElement).value)}
		/>
		<p class="text-[10.5px] leading-relaxed text-muted-foreground">
			Cases are checked top to bottom; the first match wins. No match → the
			default path.
		</p>
		<div class="mt-2 flex flex-col items-start gap-0.5">
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-foreground transition-colors hover:bg-secondary"
				onclick={() => {
					onDetach?.(info.targetId);
					onClose();
				}}
				title="Break the line; the steps survive as a disconnected island"
			>
				<Unlink class="size-3" />
				Disconnect the line (keep the steps)
			</button>
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-destructive transition-colors hover:bg-destructive/10"
				onclick={deleteCase}
				title="Remove this case and its steps"
			>
				<Trash2 class="size-3" />
				Delete case
			</button>
		</div>
	{:else if info.branch === 'default'}
		<p class="text-[11.5px] leading-relaxed text-muted-foreground">
			Runs when no case matched.
		</p>
		{#if armCount > 0}
			<div class="mt-2 flex flex-col items-start gap-0.5">
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-foreground transition-colors hover:bg-secondary"
				onclick={() => {
					onDetach?.(info.targetId);
					onClose();
				}}
				title="Break the line; the steps survive as a disconnected island"
			>
				<Unlink class="size-3" />
				Disconnect the line (keep the steps)
			</button>
				<button
					type="button"
					class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-destructive transition-colors hover:bg-destructive/10"
					onclick={() => clearArm('default')}
				>
					<Trash2 class="size-3" />
					Clear this path ({armCount} step{armCount === 1 ? '' : 's'})
				</button>
			</div>
		{/if}
	{:else if info.branch === 'body'}
		<p class="text-[11.5px] leading-relaxed text-muted-foreground">
			Runs once per item. Click the Loop card to change what it iterates over
			and the item variable name.
		</p>
	{:else if info.branch === 'parallel'}
		<p class="text-[11.5px] leading-relaxed text-muted-foreground">
			Runs concurrently with the other branches.
		</p>
		<button
			type="button"
			class="mt-2 inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-destructive transition-colors hover:bg-destructive/10"
			onclick={deleteParallelBranch}
			title="Remove this branch and its steps"
		>
			<Trash2 class="size-3" />
			Delete branch
		</button>
	{:else if info.branch === 'click' && wait}
		<p class="mb-2 text-[11.5px] leading-relaxed text-muted-foreground">
			Runs when the button is clicked. The run pauses on this line until
			someone presses it (or the timeout passes).
		</p>
		{#if info.targetKind === 'modal_open'}
			<p class="mb-2 rounded-md border border-input bg-secondary/40 px-2 py-1.5 text-[10.5px] leading-snug text-muted-foreground">
				This path opens a form, and the form itself answers the click. Discord
				requires the form to be the first response, so no other click response
				applies here.
			</p>
		{:else}
		<span class="mb-1 block font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
			On click, the bot
		</span>
		<div class="mb-2 flex flex-col gap-1" role="radiogroup" aria-label="Click response">
			{#each RESPONSE_MODES as m (m.value)}
				<button
					type="button"
					role="radio"
					aria-checked={clickResponse === m.value}
					class="rounded-md border px-2 py-1.5 text-left transition-colors {clickResponse === m.value
						? 'border-foreground/40 bg-secondary/60'
						: 'border-input hover:border-foreground/25'}"
					onclick={() => setClickResponse(m.value)}
				>
					<span class="flex items-center gap-1.5">
						<span
							class="size-1.5 rounded-full {clickResponse === m.value
								? 'bg-foreground'
								: 'border border-muted-foreground/60'}"
						></span>
						<span class="text-[11.5px] font-medium text-foreground">{m.label}</span>
					</span>
					<span class="mt-0.5 block pl-3 text-[10.5px] leading-snug text-muted-foreground">
						{m.hint}
					</span>
				</button>
			{/each}
		</div>
		{/if}
		<label class="mb-2 flex items-center justify-between gap-2">
			<span class="text-[11.5px] text-muted-foreground">
				Only the <span class="font-medium text-foreground">command's invoker</span> can click
			</span>
			<Toggle
				checked={invokerOnly}
				onchange={(v) => patchWait('from_user', v ? { lang: 'tmpl', src: '{{ .User.ID }}' } : undefined)}
			/>
		</label>
		<div class="mb-2 grid grid-cols-2 gap-1.5">
			<div>
				<span class="mb-1 block font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
					Timeout
				</span>
				<input
					class="h-7 w-full rounded-md border border-input bg-background px-2 font-mono text-[11.5px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
					placeholder="10m"
					value={waitSpec.timeout ?? ''}
					oninput={(e) => patchWait('timeout', (e.currentTarget as HTMLInputElement).value)}
				/>
			</div>
			<div>
				<span class="mb-1 block font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground">
					Save click to
				</span>
				<input
					class="h-7 w-full rounded-md border border-input bg-background px-2 font-mono text-[11.5px] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
					placeholder="click"
					value={waitSpec.into ?? ''}
					oninput={(e) => patchWait('into', (e.currentTarget as HTMLInputElement).value)}
				/>
			</div>
		</div>
		<p class="text-[10.5px] leading-relaxed text-muted-foreground">
			Branch on the clicker later: {'{{ .Vars.' + (waitSpec.into || 'click') + '.user_id }}'}
		</p>
		<div class="mt-2 flex flex-col items-start gap-0.5">
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-foreground transition-colors hover:bg-secondary"
				onclick={() => removeClickArm(true)}
				title="Break the click path; the steps survive as a disconnected island"
			>
				<Unlink class="size-3" />
				Disconnect the line (keep the steps)
			</button>
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-destructive transition-colors hover:bg-destructive/10"
				onclick={() => removeClickArm(false)}
				title="Delete the click path and everything on it"
			>
				<Trash2 class="size-3" />
				Delete the click path
			</button>
		</div>
	{:else if isAfterBranching && step}
		<p class="mb-2 text-[11.5px] leading-relaxed text-muted-foreground">
			These steps run <span class="font-medium text-foreground">after either path
			finishes</span> — left over from an older layout. Tuck them into a branch so
			the flow reads as pure {step.kind === 'if' ? 'then / else' : 'cases'}:
		</p>
		<div class="flex flex-col items-start gap-0.5">
			{#if step.kind === 'if'}
				<button
					type="button"
					class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-foreground transition-colors hover:bg-secondary"
					onclick={() => {
						onAbsorbAfter?.(step.id, 'then');
						onClose();
					}}
				>
					→ Move into the <span class="font-mono text-[10.5px]">then</span> path
				</button>
				<button
					type="button"
					class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-foreground transition-colors hover:bg-secondary"
					onclick={() => {
						onAbsorbAfter?.(step.id, 'else');
						onClose();
					}}
				>
					→ Move into the <span class="font-mono text-[10.5px]">else</span> path
				</button>
			{:else}
				<button
					type="button"
					class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-foreground transition-colors hover:bg-secondary"
					onclick={() => {
						onAbsorbAfter?.(step.id, 'default');
						onClose();
					}}
				>
					→ Move into the <span class="font-mono text-[10.5px]">default</span> path
				</button>
			{/if}
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-destructive transition-colors hover:bg-destructive/10"
				onclick={() => {
					onTruncateChain?.(info.targetId);
					onClose();
				}}
				title="Disconnect here — delete the chain entirely"
			>
				<Trash2 class="size-3" />
				Disconnect — delete from here down
			</button>
		</div>
	{:else}
		<p class="text-[11.5px] leading-relaxed text-muted-foreground">
			Sequence link — the next step runs after the previous one finishes.
		</p>
		<div class="mt-2 flex flex-col items-start gap-0.5">
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-foreground transition-colors hover:bg-secondary"
				onclick={() => {
					onDetach?.(info.targetId);
					onClose();
				}}
				title="Break the line; the steps survive as a disconnected island"
			>
				<Unlink class="size-3" />
				Disconnect the line (keep the steps)
			</button>
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-destructive transition-colors hover:bg-destructive/10"
				onclick={() => {
					onDeleteStep(info.targetId);
					onClose();
				}}
				title="Remove the next step; the rest of the chain reconnects here"
			>
				<Trash2 class="size-3" />
				Remove the next step, keep the chain
			</button>
			<button
				type="button"
				class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[11.5px] font-medium text-destructive transition-colors hover:bg-destructive/10"
				onclick={() => {
					onTruncateChain?.(info.targetId);
					onClose();
				}}
				title="Disconnect here — delete the next step and everything after it"
			>
				<Trash2 class="size-3" />
				Disconnect — delete from here down
			</button>
		</div>
	{/if}
</div>

