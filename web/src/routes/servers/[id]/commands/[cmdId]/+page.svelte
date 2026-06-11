<script lang="ts">
	import { getContext, setContext } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import FlowCanvas from '$lib/components/commands/canvas/FlowCanvas.svelte';
	import { ENTRY_ID, errorRouterOwner } from '$lib/components/commands/canvas/adapter';
	import StepDrawer from '$lib/components/commands/StepDrawer.svelte';
	import SlashCommandPreview from '$lib/components/commands/SlashCommandPreview.svelte';
	import PropertiesEditor from '$lib/components/commands/PropertiesEditor.svelte';
	import FieldSelect from '$lib/components/commands/FieldSelect.svelte';
	import NumberField from '$lib/components/commands/NumberField.svelte';
	import type { Definition, Step, ValidationResult, ValidationIssue } from '$lib/commands/types';
	import { newStep } from '$lib/commands/types';
	import { EXPR_SCOPE_CTX, type ExprScope } from '$lib/commands/expr-meta';

	import { Dialog } from '$lib/components/ui';

	import ChevronLeft from 'lucide-svelte/icons/chevron-left';
	import Send from 'lucide-svelte/icons/send';
	import Settings from 'lucide-svelte/icons/settings';
	import CircleAlert from 'lucide-svelte/icons/circle-alert';
	import Braces from 'lucide-svelte/icons/braces';
	import Plus from 'lucide-svelte/icons/plus';
	import Trash2 from 'lucide-svelte/icons/trash-2';

	const store = getContext<GuildStore>(GUILD_CTX);
	// Reactive: SvelteKit reuses this component on param-only navigations
	// (e.g. multi-step history traversal between two editors), so the id must
	// track the URL and the load below re-keys on it.
	const cmdId = $derived(Number($page.params.cmdId ?? 0));

	type EditCommand = {
		id: number;
		name: string;
		description: string;
		enabled: boolean;
		status: string;
		version: number;
		requires_defer: boolean;
		definition: Definition;
	};

	let cmd = $state<EditCommand | null>(null);
	let baseline = $state('');
	let loaded = $state(false);
	let loadError = $state<string>('');
	let elapsedSec = $state(0);
	let loadTimer: ReturnType<typeof setInterval> | null = null;
	let loadStartMs = $state(0);
	let saving = $state(false);
	let publishing = $state(false);
	let selectedId = $state('');
	let validation = $state<ValidationResult | null>(null);
	let runs = $state<
		{ id: string; status: string; started_at: string; trigger_kind: string; error: string }[]
	>([]);

	let settingsOpen = $state(false);
	let propertiesOpen = $state(false);

	const dirty = $derived(loaded && cmd ? JSON.stringify(cmd) !== baseline : false);
	const selectedStep = $derived.by(() => {
		if (!cmd) return null;
		return (
			findStep(cmd.definition.steps ?? [], selectedId) ??
			(cmd.definition.scratch ?? []).reduce<Step | null>(
				(acc, ch) => acc ?? findStep(ch, selectedId),
				null
			)
		);
	});

	const errorPaths = $derived(buildErrorPaths(validation?.issues ?? []));
	const issueCount = $derived(validation?.issues?.length ?? 0);
	const errorCount = $derived(
		validation?.issues?.filter((i) => i.severity === 'error').length ?? 0
	);

	// Editor-wide expression scope — every <ExprField> reads this through
	// context to populate the in-scope variable picker.
	const exprScope: ExprScope = $state({ options: [], variables: [] });
	setContext(EXPR_SCOPE_CTX, exprScope);
	$effect(() => {
		exprScope.options = cmd?.definition.options ?? [];
		exprScope.variables = cmd?.definition.variables ?? [];
		// Live tree (same proxies) — powers "reference a previous step" pickers.
		exprScope.steps = cmd?.definition.steps ?? [];
	});

	function buildErrorPaths(issues: ValidationIssue[]): Set<string> {
		const set = new Set<string>();
		for (const iss of issues) {
			const parts = iss.path.split('.');
			let acc = '';
			for (let i = 0; i < parts.length; i++) {
				acc = i === 0 ? parts[0] : `${acc}.${parts[i]}`;
				if (parts[i] === 'steps' && i + 1 < parts.length) {
					set.add(`${acc}.${parts[i + 1]}`);
				}
			}
		}
		return set;
	}

	function findStep(steps: Step[], id: string): Step | null {
		for (const s of steps) {
			if (s.id === id) return s;
			const branches: (Step[] | undefined)[] = [s.then, s.else, s.default, s.on_error];
			for (const c of s.cases ?? []) branches.push(c.do);
			for (const ec of s.on_error_cases ?? []) branches.push(ec.do);
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const parBranches = ((s.spec ?? {}) as any).branches ?? [];
			for (const b of [...branches, ...parBranches]) {
				if (!b) continue;
				const found = findStep(b, id);
				if (found) return found;
			}
		}
		return null;
	}

	// Load (and re-load on in-place navigation to another command id).
	$effect(() => {
		void cmdId;
		loaded = false;
		loadError = '';
		cmd = null;
		baseline = '';
		selectedId = '';
		validation = null;
		runs = [];
		(async () => {
			const t0 = performance.now();
			loadStartMs = t0;
			loadTimer = setInterval(() => {
				elapsedSec = Math.floor((performance.now() - loadStartMs) / 1000);
			}, 250);
			try {
				await reload();
			} catch (e) {
				loadError = e instanceof Error ? e.message : String(e);
				console.error('[editor] reload failed', e);
			}
			try {
				await loadRuns();
			} catch (e) {
				console.warn('[editor] loadRuns failed', e);
			}
			if (loadTimer) {
				clearInterval(loadTimer);
				loadTimer = null;
			}
			loaded = true;
		})();
	});

	async function reload() {
		const c = await api.command(store.id, cmdId);
		cmd = {
			id: c.id,
			name: c.name,
			description: c.description,
			enabled: c.enabled,
			status: c.status,
			version: c.version,
			requires_defer: c.requires_defer,
			definition: normaliseDefinition(c.definition ?? {})
		};
		baseline = JSON.stringify(cmd);
		void validate();
	}

	async function loadRuns() {
		const r = await api.commandRuns(store.id, cmdId, 25);
		runs = r.runs ?? [];
	}

	function normaliseDefinition(d: Partial<Definition>): Definition {
		return {
			options: d.options ?? [],
			permissions: d.permissions ?? '',
			cooldown: d.cooldown,
			variables: d.variables ?? [],
			triggers: d.triggers ?? [{ kind: 'slash' }],
			steps: d.steps ?? [],
			scratch: d.scratch ?? [],
			ui_hints: d.ui_hints
		};
	}

	async function validate() {
		if (!cmd) return;
		try {
			const r = await api.validateCommand(store.id, {
				name: cmd.name,
				description: cmd.description,
				definition: cmd.definition
			});
			validation = r.validation;
			if (cmd) cmd.requires_defer = r.validation.requires_defer;
		} catch {
			/* ignore */
		}
	}

	let validateTimer: ReturnType<typeof setTimeout>;
	$effect(() => {
		if (!cmd || !loaded) return;
		void JSON.stringify(cmd.definition);
		clearTimeout(validateTimer);
		validateTimer = setTimeout(validate, 400);
	});

	async function save(thenPublish = false) {
		if (!cmd) return;
		if (thenPublish) publishing = true;
		else saving = true;
		try {
			const r = await api.upsertCommand(store.id, {
				id: cmd.id,
				name: cmd.name,
				description: cmd.description,
				enabled: cmd.enabled,
				status: thenPublish ? 'published' : cmd.status,
				definition: cmd.definition
			});
			validation = r.validation;
			await reload();
		} finally {
			saving = false;
			publishing = false;
		}
	}

	function reset() {
		if (baseline) cmd = JSON.parse(baseline);
	}

	function addAtRoot(kind: string) {
		if (!cmd) return;
		const ns = newStep(kind);
		const steps = (cmd.definition.steps ?? []).slice();
		steps.push(ns);
		cmd.definition.steps = steps;
		selectedId = ns.id;
	}

	// Inserting a branching step into an existing chain must NOT leave the old
	// continuation dangling off a side "after" line — the rest of the chain
	// moves into the natural branch (if → then, switch → default, loop → body)
	// so the flow reads top-to-bottom with only the arms leaving the card.
	function absorbFollowing(ns: Step, following: Step[]): boolean {
		if (following.length === 0) return false;
		// Loops keep their real "after" continuation — the body REPEATS, so
		// absorbing the chain there would change what the flow does.
		if (ns.kind === 'if') ns.then = following;
		else if (ns.kind === 'switch') ns.default = following;
		else return false;
		return true;
	}

	function addFromHandle(sourceNodeId: string, handle: string | null, kind: string) {
		if (!cmd) return;
		const ns = newStep(kind);
		// Dragging out of the synthetic /command entry pill prepends to the root.
		if (sourceNodeId === ENTRY_ID) {
			const rest = (cmd.definition.steps ?? []).slice();
			cmd.definition.steps = absorbFollowing(ns, rest) ? [ns] : [ns, ...rest];
			selectedId = ns.id;
			return;
		}
		// Dragging out of an on-error router's arms: the step starts that
		// arm's recovery chain on the owning step.
		const routerOwner = errorRouterOwner(sourceNodeId);
		if (routerOwner) {
			const owner = locateStep(cmd.definition.steps ?? [], routerOwner);
			if (!owner) return;
			const h = handle ?? 'default';
			if (h.startsWith('arm-')) {
				const ei = Number(h.slice(4));
				const cases = owner.step.on_error_cases ?? [];
				if (!cases[ei]) return;
				cases[ei].do = [...(cases[ei].do ?? []), ns];
				owner.step.on_error_cases = [...cases];
			} else {
				owner.step.on_error = [...(owner.step.on_error ?? []), ns];
			}
			cmd.definition.steps = [...(cmd.definition.steps ?? [])];
			selectedId = ns.id;
			return;
		}
		const located = locateStep(cmd.definition.steps ?? [], sourceNodeId);
		if (!located) return;
		const { branch, index, step: src } = located;
		const h = handle ?? 'out';
		if (h === 'out') {
			const at = insertionIndex(branch, index);
			const following = branch.splice(at);
			if (!absorbFollowing(ns, following)) branch.splice(at, 0, ns, ...following);
			else branch.splice(at, 0, ns);
		} else if (h === 'then' || h === 'body') src.then = [...(src.then ?? []), ns];
		else if (h === 'else') src.else = [...(src.else ?? []), ns];
		else if (h === 'default') src.default = [...(src.default ?? []), ns];
		else if (h.startsWith('case-')) {
			const ci = Number(h.slice(5));
			src.cases = src.cases ?? [];
			if (!src.cases[ci]) src.cases[ci] = { when: { lang: 'tmpl', src: '' }, do: [] };
			src.cases[ci].do.push(ns);
		} else if (h.startsWith('branch-')) {
			const bi = Number(h.slice(7));
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const spec = (src.spec ?? {}) as any;
			spec.branches = spec.branches ?? [];
			if (!spec.branches[bi]) spec.branches[bi] = [];
			spec.branches[bi].push(ns);
			src.spec = spec;
		} else if (h.startsWith('component-')) {
			// Dragging out of a button's dot. ONE hidden listener per message
			// (waits for any of its buttons) + a hidden switch routing by the
			// clicked button's id — so every button can lead somewhere
			// different, and the message's own bottom dot still says what
			// happens after.
			const sfx = h.slice('component-'.length);
			addClickAction(branch, index, sfx, ns);
		} else branch.splice(index + 1, 0, ns);
		cmd.definition.steps = [...(cmd.definition.steps ?? [])];
		selectedId = ns.id;
	}

	// locateAnywhere searches the live tree AND the scratch islands.
	function locateAnywhere(id: string): { branch: Step[]; index: number; step: Step } | null {
		if (!cmd) return null;
		const hit = locateStep(cmd.definition.steps ?? [], id);
		if (hit) return hit;
		for (const ch of cmd.definition.scratch ?? []) {
			const f = locateStep(ch, id);
			if (f) return f;
		}
		return null;
	}

	function locateStep(
		steps: Step[],
		id: string
	): { branch: Step[]; index: number; step: Step } | null {
		for (let i = 0; i < steps.length; i++) {
			const s = steps[i];
			if (s.id === id) return { branch: steps, index: i, step: s };
			const subs: (Step[] | undefined)[] = [s.then, s.else, s.default, s.on_error];
			for (const c of s.cases ?? []) subs.push(c.do);
			for (const ec of s.on_error_cases ?? []) subs.push(ec.do);
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const parBranches = ((s.spec ?? {}) as any).branches ?? [];
			for (const b of parBranches) subs.push(b as Step[]);
			for (const branch of subs) {
				if (!branch) continue;
				const f = locateStep(branch, id);
				if (f) return f;
			}
		}
		return null;
	}

	function deleteStep(id: string) {
		if (!cmd) return;
		const located = locateAnywhere(id);
		if (!located) return;
		located.branch.splice(located.index, 1);
		cmd.definition.scratch = (cmd.definition.scratch ?? []).filter((ch) => ch.length > 0);
		cmd.definition.steps = [...(cmd.definition.steps ?? [])];
		if (selectedId === id) selectedId = '';
	}

	// Detach a line WITHOUT deleting anything: the target step and everything
	// after it in that branch become a disconnected island (kept in scratch).
	function detachToScratch(id: string) {
		if (!cmd) return;
		const located = locateAnywhere(id);
		if (!located) return;
		const chain = located.branch.splice(located.index);
		if (chain.length === 0) return;
		cmd.definition.scratch = [...(cmd.definition.scratch ?? []), chain];
		cmd.definition.steps = [...(cmd.definition.steps ?? [])];
	}

	// Reconnect a scratch island: drag any dot onto the island's first step.
	function attachScratch(sourceNodeId: string, handle: string | null, headId: string) {
		if (!cmd) return;
		const all = cmd.definition.scratch ?? [];
		const idx = all.findIndex((ch) => ch[0]?.id === headId);
		if (idx < 0) return;
		const chain = all[idx];
		cmd.definition.scratch = all.filter((_, i) => i !== idx);
		insertChain(sourceNodeId, handle, chain);
		cmd.definition.steps = [...(cmd.definition.steps ?? [])];
	}

	// isClickWait / isClickSwitch recognise the hidden click-router pair the
	// canvas fuses into "on click" lines.
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function isClickWait(s?: Step): boolean {
		if (!s || s.kind !== 'wait_for') return false;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const sp = (s.spec ?? {}) as any;
		return !sp.custom_id_suffix && (sp.trigger ?? 'component') === 'component' && !!sp.into;
	}
	function isClickSwitch(s: Step | undefined, wait: Step): boolean {
		if (!s || s.kind !== 'switch') return false;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const into = ((wait.spec ?? {}) as any).into;
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		return ((s.spec ?? {}) as any).on?.src === `{{ .Vars.${into}.id }}`;
	}

	// insertionIndex: steps glued to a message (its hidden click cluster)
	// stay glued — insertions from the message's bottom dot land AFTER the
	// cluster, never between the message and its listener.
	function insertionIndex(branch: Step[], index: number): number {
		const next = branch[index + 1];
		const nextNext = branch[index + 2];
		if (isClickWait(next) && isClickSwitch(nextNext, next)) return index + 3;
		return index + 1;
	}

	// addClickAction wires "when <button sfx> is clicked, run ns" onto the
	// message at branch[index], creating the listener + switch if needed.
	function addClickAction(branch: Step[], index: number, sfx: string, ns: Step) {
		const next = branch[index + 1];
		const nextNext = branch[index + 2];
		if (isClickWait(next) && isClickSwitch(nextNext, next)) {
			const sw = nextNext;
			sw.cases = sw.cases ?? [];
			let c = sw.cases.find((cc) => cc.when?.src === sfx);
			if (!c) {
				c = { when: { lang: 'tmpl', src: sfx }, do: [] };
				sw.cases = [...sw.cases, c];
			}
			c.do.push(ns);
			return;
		}
		const wait = newStep('wait_for');
		wait.spec = { trigger: 'component', into: 'click', timeout: '5m' };
		const sw = newStep('switch');
		sw.spec = { on: { lang: 'tmpl', src: '{{ .Vars.click.id }}' } };
		sw.cases = [{ when: { lang: 'tmpl', src: sfx }, do: [ns] }];
		sw.default = [];
		branch.splice(index + 1, 0, wait, sw);
	}

	// insertChain splices a whole chain in at the location a handle points to
	// (same routing rules as addFromHandle).
	function insertChain(sourceNodeId: string, handle: string | null, chain: Step[]) {
		if (!cmd || chain.length === 0) return;
		if (sourceNodeId === ENTRY_ID) {
			cmd.definition.steps = [...chain, ...(cmd.definition.steps ?? [])];
			return;
		}
		const routerOwner = errorRouterOwner(sourceNodeId);
		if (routerOwner) {
			const owner = locateAnywhere(routerOwner);
			if (!owner) return;
			const h = handle ?? 'default';
			if (h.startsWith('arm-')) {
				const ei = Number(h.slice(4));
				const cases = owner.step.on_error_cases ?? [];
				if (!cases[ei]) return;
				cases[ei].do = [...(cases[ei].do ?? []), ...chain];
				owner.step.on_error_cases = [...cases];
			} else {
				owner.step.on_error = [...(owner.step.on_error ?? []), ...chain];
			}
			return;
		}
		const located = locateAnywhere(sourceNodeId);
		if (!located) return;
		const { branch, index, step: src } = located;
		const h = handle ?? 'out';
		if (h.startsWith('component-')) {
			const sfx = h.slice('component-'.length);
			for (const st of chain) addClickAction(branch, index, sfx, st);
			return;
		}
		if (h === 'out') branch.splice(insertionIndex(branch, index), 0, ...chain);
		else if (h === 'then' || h === 'body') src.then = [...(src.then ?? []), ...chain];
		else if (h === 'else') src.else = [...(src.else ?? []), ...chain];
		else if (h === 'default') src.default = [...(src.default ?? []), ...chain];
		else if (h.startsWith('case-')) {
			const ci = Number(h.slice(5));
			src.cases = src.cases ?? [];
			if (!src.cases[ci]) src.cases[ci] = { when: { lang: 'tmpl', src: '' }, do: [] };
			src.cases[ci].do.push(...chain);
		} else if (h.startsWith('branch-')) {
			const bi = Number(h.slice(7));
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const spec = (src.spec ?? {}) as any;
			spec.branches = spec.branches ?? [];
			if (!spec.branches[bi]) spec.branches[bi] = [];
			spec.branches[bi].push(...chain);
			src.spec = spec;
		} else branch.splice(index + 1, 0, ...chain);
	}

	// Spawn the on-error router on a step — no steps are auto-created; the
	// router's case editor + arm dots take it from there.
	function addErrorRouter(id: string) {
		if (!cmd) return;
		const located = locateStep(cmd.definition.steps ?? [], id);
		if (!located) return;
		if (located.step.on_error === undefined && !located.step.on_error_cases?.length) {
			located.step.on_error = [];
		}
		cmd.definition.steps = [...(cmd.definition.steps ?? [])];
	}

	function removeErrorRouter(id: string) {
		if (!cmd) return;
		const located = locateStep(cmd.definition.steps ?? [], id);
		if (!located) return;
		located.step.on_error = undefined;
		located.step.on_error_cases = undefined;
		cmd.definition.steps = [...(cmd.definition.steps ?? [])];
	}

	// Tuck an if/switch's legacy after-chain into one of its branches — the
	// explicit migration for flows built before branching steps absorbed their
	// continuation (offered on the "After every path" line).
	function absorbAfterInto(id: string, which: 'then' | 'else' | 'default') {
		if (!cmd) return;
		const located = locateStep(cmd.definition.steps ?? [], id);
		if (!located) return;
		const following = located.branch.splice(located.index + 1);
		if (following.length === 0) return;
		located.step[which] = [...(located.step[which] ?? []), ...following];
		cmd.definition.steps = [...(cmd.definition.steps ?? [])];
	}

	// Disconnect a sequence line: delete the step it leads to AND everything
	// chained after it in the same branch.
	function truncateChain(id: string) {
		if (!cmd) return;
		const located = locateStep(cmd.definition.steps ?? [], id);
		if (!located) return;
		located.branch.splice(located.index);
		cmd.definition.steps = [...(cmd.definition.steps ?? [])];
		if (selectedId && !findStep(cmd.definition.steps ?? [], selectedId)) selectedId = '';
	}

	function addCase(id: string) {
		if (!cmd) return;
		const located = locateStep(cmd.definition.steps ?? [], id);
		if (!located || located.step.kind !== 'switch') return;
		const ns = newStep('reply');
		located.step.cases = [
			...(located.step.cases ?? []),
			{ when: { lang: 'tmpl', src: '' }, do: [ns] }
		];
		cmd.definition.steps = [...(cmd.definition.steps ?? [])];
		selectedId = ns.id;
	}

	function addParallelBranchSlot(id: string) {
		if (!cmd) return;
		const located = locateStep(cmd.definition.steps ?? [], id);
		if (!located || located.step.kind !== 'parallel') return;
		const ns = newStep('reply');
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const spec = (located.step.spec ?? {}) as any;
		spec.branches = [...(spec.branches ?? []), [ns]];
		located.step.spec = spec;
		cmd.definition.steps = [...(cmd.definition.steps ?? [])];
		selectedId = ns.id;
	}

	function addVariable() {
		if (!cmd) return;
		cmd.definition.variables = [
			...(cmd.definition.variables ?? []),
			{ name: `var${(cmd.definition.variables?.length ?? 0) + 1}`, type: 'string', scope: 'run' }
		];
	}
	function removeVariable(i: number) {
		if (!cmd) return;
		cmd.definition.variables = (cmd.definition.variables ?? []).filter((_, idx) => idx !== i);
	}

	function addTrigger() {
		if (!cmd) return;
		cmd.definition.triggers = [
			...(cmd.definition.triggers ?? []),
			{ kind: 'event', event: 'GUILD_MEMBER_ADD' }
		];
	}
	function removeTrigger(i: number) {
		if (!cmd) return;
		cmd.definition.triggers = (cmd.definition.triggers ?? []).filter((_, idx) => idx !== i);
	}

	function isInTextField(target: EventTarget | null): boolean {
		const el = target as HTMLElement | null;
		if (!el) return false;
		const tag = el.tagName?.toLowerCase();
		return tag === 'input' || tag === 'textarea' || el.isContentEditable;
	}

	$effect(() => {
		if (!loaded) return;
		const onKey = (e: KeyboardEvent) => {
			if (isInTextField(e.target)) return;
			// The canvas consumes Escape first when one of its pickers is open.
			if (e.key === 'Escape' && !e.defaultPrevented) selectedId = '';
		};
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	});

	function statusPipColor(): string {
		if (!cmd) return 'bg-faint';
		if (!cmd.enabled) return 'bg-faint/50';
		if (cmd.status === 'published') return 'bg-success';
		return 'bg-ink/60'; // draft — neutral white, no amber
	}

	function relTime(iso: string): string {
		const d = new Date(iso).getTime();
		const diff = (Date.now() - d) / 1000;
		if (diff < 60) return `${Math.round(diff)}s`;
		if (diff < 3600) return `${Math.round(diff / 60)}m`;
		if (diff < 86400) return `${Math.round(diff / 3600)}h`;
		return `${Math.round(diff / 86400)}d`;
	}
</script>

<svelte:head>
	<title>{cmd ? `/${cmd.name}` : 'Command'} · {store.name} · Dia</title>
</svelte:head>

{#if loadError}
	<div class="flex h-full flex-col bg-bg">
		<div class="flex h-12 shrink-0 items-center gap-3 border-b border-line bg-bg px-3">
			<button
				type="button"
				class="grid size-8 place-items-center rounded text-muted hover:bg-surface hover:text-ink"
				onclick={() => goto(`/servers/${store.id}/commands`)}
				title="Back"
			>
				<ChevronLeft size={14} />
			</button>
			<span class="font-mono text-[10px] uppercase tracking-[0.14em] text-faint">Commands</span>
			<div class="h-3.5 w-px bg-line"></div>
			<span class="font-mono text-[12px] text-danger">Could not load command</span>
		</div>
		<div class="flex flex-1 items-center justify-center px-5">
			<div class="text-center">
				<CircleAlert size={18} class="mx-auto mb-2 text-danger" />
				<p class="text-[12.5px] text-ink">{loadError}</p>
				<p class="mt-1 font-mono text-[10.5px] text-faint">command #{cmdId}</p>
				<div class="mt-4 flex justify-center gap-2">
					<button
						type="button"
						class="inline-flex h-7 items-center rounded-md border border-line bg-bg px-2.5 text-[12px] font-medium text-ink hover:border-line-strong"
						onclick={() => {
							loadError = '';
							loaded = false;
							reload()
								.catch((e) => {
									loadError = e instanceof Error ? e.message : String(e);
								})
								.finally(() => {
									loaded = true;
								});
						}}
					>
						Retry
					</button>
					<button
						type="button"
						class="inline-flex h-7 items-center rounded-md bg-ink px-2.5 text-[12px] font-medium text-bg"
						onclick={() => goto(`/servers/${store.id}/commands`)}
					>
						Back
					</button>
				</div>
			</div>
		</div>
	</div>
{:else if !loaded || !cmd}
	<!-- Loading -->
	<div class="flex h-full flex-col bg-bg">
		<div class="flex h-12 shrink-0 items-center gap-3 border-b border-line bg-bg px-3">
			<div class="skeleton size-7 rounded"></div>
			<span class="font-mono text-[10px] uppercase tracking-[0.14em] text-faint">Commands</span>
			<div class="h-3.5 w-px bg-line"></div>
			<div class="skeleton h-3 w-24 rounded"></div>
			<div class="ml-auto flex items-center gap-2 font-mono text-[10.5px] text-faint">
				<span class="dots-loader" aria-hidden="true"><span></span><span></span><span></span></span>
				Loading{#if elapsedSec > 0} · {elapsedSec}s{/if}
			</div>
		</div>
		<div class="flex flex-1 items-center justify-center">
			<div class="text-center">
				<div class="mx-auto mb-3 grid size-8 place-items-center rounded-full border border-dashed border-line">
					<span class="dots-loader" aria-hidden="true"><span></span><span></span><span></span></span>
				</div>
				<p class="font-mono text-[10.5px] text-faint">
					Fetching command #{cmdId}
				</p>
			</div>
		</div>
	</div>
{:else}
	<div class="flex h-full flex-col bg-bg text-ink fade-in">
		<!-- ── Slim topbar: back / /name / status / version / unsaved / actions ── -->
		<header class="flex h-12 shrink-0 items-center gap-2 border-b border-line bg-bg px-3">
			<button
				type="button"
				class="grid size-8 place-items-center rounded text-muted transition-colors hover:bg-surface hover:text-ink"
				onclick={() => goto(`/servers/${store.id}/commands`)}
				title="Back to commands"
			>
				<ChevronLeft size={14} />
			</button>

			<!-- Status pip + name -->
			<span class="size-1.5 shrink-0 rounded-full {statusPipColor()}" title={cmd.status}></span>
			<span class="select-none text-[13px] text-faint">/</span>
			<input
				class="min-w-0 rounded border border-transparent bg-transparent px-1 py-0.5 font-mono text-[13px] font-medium text-ink focus:border-line focus:outline-none"
				style="width: {Math.max(6, (cmd.name?.length ?? 0) + 2)}ch"
				bind:value={cmd.name}
			/>
			<span class="shrink-0 font-mono text-[10px] tabular-nums text-faint">v{cmd.version}</span>
			<span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.12em] text-faint">
				{cmd.status}
			</span>

			{#if cmd.requires_defer}
				<span
					class="shrink-0 font-mono text-[10px] uppercase tracking-[0.12em] text-faint"
					title="Auto-defers (worst-case path > 3s)"
				>
					defers
				</span>
			{/if}

			<!-- Validation indicator -->
			{#if validation}
				{#if errorCount > 0}
					<span
						class="inline-flex shrink-0 items-center gap-1 rounded border border-danger/30 bg-danger/5 px-1.5 font-mono text-[10px] text-danger"
						title="{errorCount} {errorCount === 1 ? 'error' : 'errors'}"
					>
						<CircleAlert size={10} />
						{errorCount}
					</span>
				{:else if issueCount > 0}
					<span
						class="inline-flex shrink-0 items-center gap-1 rounded border border-line-strong bg-surface/40 px-1.5 font-mono text-[10px] text-muted"
						title="{issueCount} warnings"
					>
						{issueCount}
					</span>
				{/if}
			{/if}

			{#if dirty}
				<span class="inline-flex shrink-0 items-center gap-1 font-mono text-[10px] uppercase tracking-[0.12em] text-muted">
					<span class="size-1 rounded-full bg-ink/70"></span> unsaved
				</span>
			{/if}

			<div class="ml-auto flex items-center gap-1">
				<!-- Properties — the /command <property> inputs -->
				<button
					type="button"
					class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[12px] font-medium text-muted transition-colors hover:bg-surface hover:text-ink"
					onclick={() => (propertiesOpen = true)}
					title="Slash properties members fill in"
				>
					<Braces size={12} />
					<span class="hidden sm:inline">Properties</span>
					<span class="font-mono text-[10px] tabular-nums text-faint">
						{cmd.definition.options?.length ?? 0}
					</span>
				</button>

				<!-- Settings -->
				<button
					type="button"
					class="grid size-7 place-items-center rounded text-faint transition-colors hover:bg-surface hover:text-ink"
					onclick={() => (settingsOpen = true)}
					title="Command settings"
				>
					<Settings size={13} />
				</button>

				<div class="mx-1 h-3.5 w-px bg-line"></div>

				<!-- On/Off pill -->
				<button
					type="button"
					class="inline-flex h-6 items-center gap-1.5 rounded border border-line bg-bg/40 px-1.5 font-mono text-[10px] uppercase tracking-[0.12em] transition-colors hover:border-line-strong"
					onclick={() => cmd && (cmd.enabled = !cmd.enabled)}
					title={cmd.enabled ? 'Disable' : 'Enable'}
				>
					<span class="size-1.5 rounded-full {cmd.enabled ? 'bg-success' : 'bg-faint/40'}"></span>
					{cmd.enabled ? 'On' : 'Off'}
				</button>

				{#if dirty}
					<button
						type="button"
						class="inline-flex h-7 items-center rounded-md px-2 text-[13px] font-medium text-muted hover:text-ink"
						onclick={reset}
						disabled={saving || publishing}
					>
						Reset
					</button>
				{/if}
				<button
					type="button"
					class="inline-flex h-7 items-center rounded-md border border-line bg-bg px-2.5 text-[13px] font-medium text-ink transition-colors hover:border-line-strong disabled:opacity-50"
					onclick={() => save(false)}
					disabled={saving || publishing || !dirty}
				>
					{saving ? 'Saving' : 'Save'}
				</button>
				<button
					type="button"
					class="inline-flex h-7 items-center gap-1.5 rounded-md bg-ink px-2.5 text-[13px] font-medium text-bg transition-opacity hover:opacity-90 disabled:opacity-40"
					onclick={() => save(true)}
					disabled={publishing || (validation && !validation.ok) || false}
				>
					<Send size={11} />
					{publishing ? 'Publishing' : 'Publish'}
				</button>
			</div>
		</header>

		<!-- ── The canvas, full-bleed. Click a step → drawer. ── -->
		<div class="relative min-h-0 flex-1 overflow-hidden bg-bg">
			<FlowCanvas
				steps={cmd.definition.steps as Step[]}
				scratch={cmd.definition.scratch ?? []}
				commandName={cmd.name}
				commandId={cmd.id}
				bind:selectedId
				{errorPaths}
				onAddAtRoot={addAtRoot}
				onAddFromHandle={addFromHandle}
				onDeleteStep={deleteStep}
				onDetach={detachToScratch}
				onAttachScratch={attachScratch}
				onAddErrorRouter={addErrorRouter}
				onRemoveErrorRouter={removeErrorRouter}
				onTruncateChain={truncateChain}
				onAbsorbAfter={absorbAfterInto}
				onAddCase={addCase}
				onAddParallelBranch={addParallelBranchSlot}
			/>

			{#if selectedStep}
				<StepDrawer
					step={selectedStep as Step}
					onClose={() => (selectedId = '')}
					onDelete={(id) => deleteStep(id)}
				/>
			{/if}
		</div>
	</div>

	<!-- ── Properties dialog: the /command <property> builder ── -->
	<Dialog.Root bind:open={propertiesOpen}>
		<Dialog.Content class="max-w-[760px] gap-0 overflow-hidden p-0">
			<Dialog.Title class="sr-only">Properties</Dialog.Title>
			<div class="flex h-12 shrink-0 items-center gap-2.5 border-b border-line px-4">
				<div class="grid size-5 place-items-center rounded border border-line bg-surface text-muted">
					<Braces size={11} />
				</div>
				<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
					Properties
				</span>
				<div class="h-4 w-px bg-line"></div>
				<span class="font-mono text-[12.5px] font-medium text-ink">/{cmd.name}</span>
				<span class="ml-auto mr-6 font-mono text-[10px] tabular-nums text-faint">
					{cmd.definition.options?.length ?? 0}/25
				</span>
			</div>
			<!-- Live Discord preview pinned up top; the list scrolls underneath it,
			     so what members will see stays in sight while editing. -->
			<div class="shrink-0 border-b border-line bg-ink-2/40 px-5 pb-4 pt-3.5">
				<div class="mb-2 flex items-baseline justify-between">
					<span class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
						How it looks in Discord
					</span>
					<span class="text-[11px] text-muted">
						read any property as
						<code class="rounded bg-surface px-1 font-mono text-[10.5px] text-ink"
							>{'{{ .Input.name }}'}</code
						>
					</span>
				</div>
				<SlashCommandPreview
					name={cmd.name}
					description={cmd.description}
					options={cmd.definition.options ?? []}
				/>
			</div>
			<div class="max-h-[56vh] overflow-y-auto px-5 py-4">
				<PropertiesEditor
					options={cmd.definition.options ?? []}
					onChange={(next) => {
						if (!cmd) return;
						cmd.definition.options = next;
					}}
				/>
			</div>
		</Dialog.Content>
	</Dialog.Root>

	<!-- ── Settings dialog ── -->
	<Dialog.Root bind:open={settingsOpen}>
		<Dialog.Content class="max-w-3xl">
			<Dialog.Header>
				<Dialog.Title>Command settings</Dialog.Title>
				<Dialog.Description>
					Description, cooldown, permissions, triggers, variables, runs.
				</Dialog.Description>
			</Dialog.Header>

			<div class="grid max-h-[65vh] gap-5 overflow-y-auto pr-1">
				<!-- Description -->
				<section>
					<div class="mb-1.5 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
						Description
					</div>
					<input
						class="h-7 w-full rounded-md border border-line bg-bg px-2 text-[12px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
						bind:value={cmd.description}
						maxlength="100"
						placeholder="A short, one-line description"
					/>
				</section>

				<!-- Permissions + Cooldown -->
				<section class="grid grid-cols-3 gap-3">
					<div class="col-span-1">
						<div class="mb-1.5 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
							Permissions
						</div>
						<input
							class="h-7 w-full rounded-md border border-line bg-bg px-2 text-[12px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
							placeholder="(Admin)"
							bind:value={cmd.definition.permissions}
						/>
					</div>
					<div class="col-span-1">
						<div class="mb-1.5 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
							Cooldown scope
						</div>
						<FieldSelect
							value={cmd.definition.cooldown?.scope ?? 'none'}
							onChange={(v) => {
								if (!cmd) return;
								if (v === 'none') cmd.definition.cooldown = undefined;
								else
									cmd.definition.cooldown = {
										scope: v,
										seconds: cmd.definition.cooldown?.seconds ?? 30
									};
							}}
							options={[
								{ value: 'none', label: 'None' },
								{ value: 'user', label: 'Per user' },
								{ value: 'channel', label: 'Per channel' },
								{ value: 'guild', label: 'Per guild' }
							]}
						/>
					</div>
					<div class="col-span-1">
						<div class="mb-1.5 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
							Seconds
						</div>
						<NumberField
							min={0}
							suffix="s"
							value={cmd.definition.cooldown?.seconds ?? 0}
							disabled={!cmd.definition.cooldown}
							onChange={(n) => {
								if (!cmd || !cmd.definition.cooldown) return;
								cmd.definition.cooldown = { ...cmd.definition.cooldown, seconds: n ?? 0 };
							}}
						/>
					</div>
				</section>

				<!-- Triggers -->
				<section>
					<div class="mb-1.5 flex items-center justify-between">
						<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
							Triggers
						</div>
						<button
							type="button"
							onclick={addTrigger}
							class="inline-flex h-6 items-center gap-1 rounded border border-line bg-bg px-1.5 text-[11px] font-medium text-muted hover:border-line-strong hover:text-ink"
						>
							<Plus size={11} /> Add
						</button>
					</div>
					<p class="mb-1.5 font-mono text-[10.5px] text-faint">
						Slash is always active.
					</p>
					{#if (cmd.definition.triggers?.length ?? 0) === 0}
						<p class="font-mono text-[10.5px] text-faint">No extra triggers.</p>
					{:else}
						<div class="space-y-1">
							{#each cmd.definition.triggers ?? [] as t, i (i)}
								<div class="grid items-center gap-1.5 sm:grid-cols-[6rem_1fr_1fr_1.5rem]">
									<FieldSelect
										bind:value={t.kind}
										options={[
											{ value: 'slash', label: 'slash' },
											{ value: 'component', label: 'component' },
											{ value: 'modal', label: 'modal' },
											{ value: 'event', label: 'event' },
											{ value: 'schedule', label: 'schedule' }
										]}
									/>
									{#if t.kind === 'event'}
										<input
											class="h-7 rounded-md border border-line bg-bg px-2 text-[11.5px] focus:border-line-strong focus:outline-none sm:col-span-2"
											placeholder="GUILD_MEMBER_ADD"
											bind:value={t.event}
										/>
									{:else if t.kind === 'schedule'}
										<input
											class="h-7 rounded-md border border-line bg-bg px-2 text-[11.5px] focus:border-line-strong focus:outline-none"
											placeholder="0 9 * * *"
											bind:value={t.cron}
										/>
										<input
											class="h-7 rounded-md border border-line bg-bg px-2 text-[11.5px] focus:border-line-strong focus:outline-none"
											placeholder="UTC"
											bind:value={t.timezone}
										/>
									{:else if t.kind === 'component' || t.kind === 'modal'}
										<input
											class="h-7 rounded-md border border-line bg-bg px-2 text-[11.5px] focus:border-line-strong focus:outline-none sm:col-span-2"
											placeholder="custom_id prefix"
											bind:value={t.prefix}
										/>
									{:else}
										<div class="sm:col-span-2"></div>
									{/if}
									<button
										type="button"
										class="grid size-7 place-items-center rounded text-faint hover:bg-surface hover:text-danger"
										onclick={() => removeTrigger(i)}
										aria-label="Remove"
									>
										<Trash2 size={11} />
									</button>
								</div>
							{/each}
						</div>
					{/if}
				</section>

				<!-- Variables -->
				<section>
					<div class="mb-1.5 flex items-center justify-between">
						<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
							Variables
						</div>
						<button
							type="button"
							onclick={addVariable}
							class="inline-flex h-6 items-center gap-1 rounded border border-line bg-bg px-1.5 text-[11px] font-medium text-muted hover:border-line-strong hover:text-ink"
						>
							<Plus size={11} /> Add
						</button>
					</div>
					{#if (cmd.definition.variables?.length ?? 0) === 0}
						<p class="font-mono text-[10.5px] text-faint">No variables.</p>
					{:else}
						<div class="space-y-1">
							{#each cmd.definition.variables ?? [] as v, i (i)}
								<div class="grid items-center gap-1.5 sm:grid-cols-[1fr_6rem_1fr_1.5rem]">
									<input
										class="h-7 rounded-md border border-line bg-bg px-2 text-[11.5px] focus:border-line-strong focus:outline-none"
										placeholder="name"
										bind:value={v.name}
									/>
									<FieldSelect
										bind:value={v.type}
										options={['string', 'int', 'float', 'bool', 'list', 'object'].map(
											(t) => ({ value: t, label: t })
										)}
									/>
									<input
										class="h-7 rounded-md border border-line bg-bg px-2 font-mono text-[11px] focus:border-line-strong focus:outline-none"
										placeholder="default (JSON)"
										value={v.default !== undefined ? JSON.stringify(v.default) : ''}
										oninput={(e) => {
											const txt = (e.currentTarget as HTMLInputElement).value;
											try {
												v.default = txt === '' ? undefined : JSON.parse(txt);
											} catch {
												/* keep typing */
											}
										}}
									/>
									<button
										type="button"
										class="grid size-7 place-items-center rounded text-faint hover:bg-surface hover:text-danger"
										onclick={() => removeVariable(i)}
										aria-label="Remove"
									>
										<Trash2 size={11} />
									</button>
								</div>
							{/each}
						</div>
					{/if}
				</section>

				<!-- Recent runs -->
				<section>
					<div class="mb-1.5 flex items-center justify-between">
						<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">
							Recent runs
						</div>
						<button
							type="button"
							onclick={loadRuns}
							class="inline-flex h-6 items-center gap-1 rounded border border-line bg-bg px-1.5 text-[11px] font-medium text-muted hover:border-line-strong hover:text-ink"
						>
							Refresh
						</button>
					</div>
					{#if runs.length === 0}
						<p class="font-mono text-[10.5px] text-faint">No runs yet.</p>
					{:else}
						<div class="overflow-hidden rounded-md border border-line">
							<div class="divide-y divide-line/60">
								{#each runs.slice(0, 10) as r (r.id)}
									<div class="flex h-8 items-center gap-2 px-2.5">
										<span
											class="size-1.5 rounded-full {r.status === 'done'
												? 'bg-success'
												: r.status === 'failed'
													? 'bg-danger'
													: r.status === 'waiting'
														? 'bg-ink/60'
														: 'bg-faint/40'}"
										></span>
										<code class="font-mono text-[10.5px] text-ink">{r.id.slice(0, 10)}</code>
										<span class="font-mono text-[10px] text-muted">{r.trigger_kind}</span>
										{#if r.error}
											<span class="truncate font-mono text-[10px] text-danger" title={r.error}>{r.error}</span>
										{/if}
										<span class="ml-auto font-mono text-[10px] tabular-nums text-faint">{relTime(r.started_at)}</span>
									</div>
								{/each}
							</div>
						</div>
					{/if}
				</section>

				{#if validation && validation.issues.length > 0}
					<section>
						<div class="mb-1.5 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-danger">
							Issues
						</div>
						<ul class="space-y-0.5">
							{#each validation.issues.slice(0, 10) as iss (iss.path + iss.code)}
								<li class="font-mono text-[10.5px] {iss.severity === 'error' ? 'text-danger' : 'text-muted'}">
									<code class="opacity-70">{iss.path}</code> — {iss.message}
								</li>
							{/each}
						</ul>
					</section>
				{/if}
			</div>
		</Dialog.Content>
	</Dialog.Root>
{/if}

<style>
	@keyframes editor-fade-in {
		from {
			opacity: 0;
			transform: translateY(2px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}
	.fade-in {
		animation: editor-fade-in 240ms cubic-bezier(0.16, 1, 0.3, 1);
	}
	@media (prefers-reduced-motion: reduce) {
		.fade-in {
			animation: none;
		}
	}
</style>
