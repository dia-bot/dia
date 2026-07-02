<script lang="ts">
	import { getContext, setContext } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { GuildStore, GUILD_CTX } from '$lib/guild.svelte';
	import { api } from '$lib/api';
	import FlowCanvas from '$lib/components/commands/canvas/FlowCanvas.svelte';
	import { ENTRY_ID, errorRouterOwner } from '$lib/components/commands/canvas/adapter';
	import StepDrawer from '$lib/components/commands/StepDrawer.svelte';
	import FieldSelect from '$lib/components/commands/FieldSelect.svelte';
	import NumberField from '$lib/components/commands/NumberField.svelte';
	import ChannelPicker from '$lib/components/ChannelPicker.svelte';
	import RolePicker from '$lib/components/RolePicker.svelte';
	import ReleaseDock, { type DockState } from '$lib/components/commands/ReleaseDock.svelte';
	import PreflightIssues from '$lib/components/commands/PreflightIssues.svelte';
	import type { Definition, Step, ValidationResult, ValidationIssue, StepKindMeta } from '$lib/commands/types';
	import { newStep, STEP_KINDS, STEP_KIND_BY_KIND } from '$lib/commands/types';
	import { EXPR_SCOPE_CTX, AUTOMATION_CTX, type ExprScope } from '$lib/commands/expr-meta';
	import { TRIGGERS, TRIGGER_BY_KEY, triggerEventVars, type TriggerConfig } from '$lib/automations/types';

	import { Dialog, Popover } from '$lib/components/ui';
	import { fade, fly } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';

	import ChevronLeft from 'lucide-svelte/icons/chevron-left';
	import Settings from 'lucide-svelte/icons/settings';
	import CircleAlert from 'lucide-svelte/icons/circle-alert';
	import Plus from 'lucide-svelte/icons/plus';
	import Trash2 from 'lucide-svelte/icons/trash-2';
	import Power from 'lucide-svelte/icons/power';
	import Zap from 'lucide-svelte/icons/zap';
	import Lock from 'lucide-svelte/icons/lock';
	import ArrowRight from 'lucide-svelte/icons/arrow-right';
	import Loader2 from 'lucide-svelte/icons/loader-2';
	import Check from 'lucide-svelte/icons/check';
	import MousePointerClick from 'lucide-svelte/icons/mouse-pointer-click';

	const store = getContext<GuildStore>(GUILD_CTX);
	const autoId = $derived($page.params.autoId ?? '');

	type EditAutomation = {
		id: string;
		name: string;
		description: string;
		enabled: boolean;
		status: string;
		version: number;
		trigger_type: string;
		trigger_config: TriggerConfig;
		definition: Definition;
		builtin: boolean;
		feature_name?: string;
		feature_tab?: string;
	};

	let auto = $state<EditAutomation | null>(null);
	let baseline = $state('');
	let loaded = $state(false);
	let loadError = $state('');
	let selectedId = $state('');
	let validation = $state<ValidationResult | null>(null);
	let runs = $state<{ id: string; status: string; started_at: string; trigger_kind: string; error: string }[]>([]);

	let settingsOpen = $state(false);
	let triggerOpen = $state(false);

	type DockPhase = 'idle' | 'saving' | 'publishing' | 'saved' | 'published' | 'error';
	let phase = $state<DockPhase>('idle');
	let dockError = $state('');
	let dockErrorAction = $state<'save' | 'publish'>('save');
	let dockTimer: ReturnType<typeof setTimeout> | null = null;
	let validating = $state(false);
	let pillVisible = $state(false);
	let pillTimer: ReturnType<typeof setTimeout> | null = null;
	let loadGen = 0;
	let validateGen = 0;

	const readonly = $derived(!!auto?.builtin);
	const dirty = $derived(loaded && auto && !readonly ? JSON.stringify(auto) !== baseline : false);

	// Some built-ins are read-only except for their per-button click actions and
	// post-message tail: you wire them by dragging button dots here, and they save
	// back into the owning feature's config (the message, embeds and card stay
	// managed on that feature's tab). Today Welcome (welcome/goodbye tabs, plus a
	// DM router) and Leveling (a single channel surface, no DM) share this
	// "editable spine" shape; Auto-roles and Reaction-roles menus add a variant
	// whose spine is a read-only apply/grant step (no message, so no click router)
	// and whose only editable part is the post-spine tail. featureEditable turns
	// the shared canvas editing on; the save routes to the matching endpoint by
	// feature_tab.
	const featureEditable = $derived(
		!!auto?.builtin &&
			(auto?.feature_tab === 'welcome' ||
				auto?.feature_tab === 'leveling' ||
				auto?.feature_tab === 'auto-roles' ||
				auto?.feature_tab === 'reaction-roles')
	);
	// Welcome distinguishes its two built-in ids (join vs leave) as config tabs;
	// leveling has a single surface so this is only meaningful for welcome.
	const welcomeKind = $derived(autoId.includes('leave') ? 'goodbye' : 'welcome');
	const featureDirty = $derived(loaded && !!auto && featureEditable ? JSON.stringify(auto) !== baseline : false);
	let featureSaving = $state<'idle' | 'saving' | 'saved' | 'error'>('idle');
	let featureErr = $state('');
	let featureDockTimer: ReturnType<typeof setTimeout> | null = null;

	const triggerMeta = $derived(auto ? TRIGGER_BY_KEY.get(auto.trigger_type) : undefined);

	const selectedStep = $derived.by(() => {
		if (!auto) return null;
		// On the welcome flow the spine (builtin-*) is generated and read-only;
		// only its component dots are interactive. Never open the editor drawer for
		// a spine node, or its in-drawer field edits would be accepted then silently
		// dropped on save (the spine is regenerated from config, not persisted here).
		if (featureEditable && selectedId.startsWith('builtin-')) return null;
		return (
			findStep(auto.definition.steps ?? [], selectedId) ??
			(auto.definition.scratch ?? []).reduce<Step | null>((acc, ch) => acc ?? findStep(ch, selectedId), null)
		);
	});

	const errorPaths = $derived(buildErrorPaths(validation?.issues ?? []));
	const issueCount = $derived(validation?.issues?.length ?? 0);
	const errorCount = $derived(validation?.issues?.filter((i) => i.severity === 'error').length ?? 0);

	const dockState = $derived.by<DockState>(() => {
		if (!auto || readonly) return 'hidden';
		if (phase !== 'idle') return phase;
		if (dirty) return 'dirty';
		if (auto.status === 'draft') return 'resting';
		return 'hidden';
	});
	const inFlight = $derived(phase === 'saving' || phase === 'publishing');

	const baseEnabled = $derived.by(() => {
		if (!baseline) return auto?.enabled ?? true;
		try {
			return !!JSON.parse(baseline).enabled;
		} catch {
			return auto?.enabled ?? true;
		}
	});

	const MESSAGE_KINDS = ['send_message', 'send_dm', 'embed_send'];
	const drawerOpen = $derived(!!selectedStep);
	const drawerWide = $derived(!!selectedStep && MESSAGE_KINDS.includes(selectedStep.kind));

	// Editor-wide expression scope. Automations have no slash options; instead
	// the trigger contributes `.Event.*` fields as extraVars.
	const exprScope: ExprScope = $state({ options: [], variables: [], extraVars: [] });
	setContext(EXPR_SCOPE_CTX, exprScope);
	// Tell shared step editors they're in an automation (1-min waits, replies
	// are click responses, etc.).
	setContext(AUTOMATION_CTX, true);

	// ── Event-native palette ──
	// At the root there's no interaction, so reply/defer/modal are hidden; a
	// Wait-for is first-class. Reply/modal reappear only when adding onto a
	// click path or a component/modal wait's continuation (there's an
	// interaction there to respond to).
	const EVENT_EXCLUDE = new Set([
		'reply',
		'edit_reply',
		'defer_reply',
		'modal_open',
		'embed_send',
		'run_command',
		'image_attach'
	]);
	const baseKinds: StepKindMeta[] = STEP_KINDS.filter(
		(k) => !EVENT_EXCLUDE.has(k.kind) && (!k.hidden || k.kind === 'wait_for')
	).map((k) =>
		k.kind === 'wait_for'
			? {
					...k,
					hidden: false,
					label: 'Wait for…',
					short: 'Pause until a click, message or reaction (up to 1 min), then continue.'
				}
			: k
	);
	const interactionExtra: StepKindMeta[] = [
		{ ...(STEP_KIND_BY_KIND.get('reply') as StepKindMeta), short: 'Reply to the button click or modal.' },
		{ ...(STEP_KIND_BY_KIND.get('modal_open') as StepKindMeta), short: 'Open a modal in response to the click.' }
	];

	function interactionContext(ctx: { root: boolean; sourceId: string | null; handle: string | null }): boolean {
		if (ctx.root) return false;
		const h = ctx.handle ?? '';
		if (h.startsWith('component-')) return true; // a button-click path
		if ((h === 'out' || h === 'after' || h === '') && ctx.sourceId && auto) {
			const src = findStep(auto.definition.steps ?? [], ctx.sourceId);
			if (src?.kind === 'wait_for') {
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
				const t = ((src.spec as any)?.trigger ?? 'component') as string;
				return t === 'component' || t === 'modal';
			}
		}
		return false;
	}
	function paletteFor(ctx: { root: boolean; sourceId: string | null; handle: string | null }): StepKindMeta[] {
		return interactionContext(ctx) ? [...interactionExtra, ...baseKinds] : baseKinds;
	}
	$effect(() => {
		exprScope.options = [];
		exprScope.variables = auto?.definition.variables ?? [];
		exprScope.steps = auto?.definition.steps ?? [];
		exprScope.extraVars = auto ? triggerEventVars(auto.trigger_type) : [];
	});

	// ── validation path → canvas node helpers (same scheme as commands) ──
	function normalisePath(path: string): string[] {
		return path
			.replace(/\[(\d+)\]/g, '.$1')
			.replace(/(^|\.)(?:spec\.)?branches\./g, '$1spec.branches.')
			.split('.');
	}
	function buildErrorPaths(issues: ValidationIssue[]): Set<string> {
		const set = new Set<string>();
		for (const iss of issues) {
			const parts = normalisePath(iss.path);
			let acc = '';
			for (const part of parts) {
				acc = acc ? `${acc}.${part}` : part;
				if (/^\d+$/.test(part)) set.add(acc);
			}
		}
		return set;
	}
	function stepIdAtPath(path: string): string {
		if (!auto) return '';
		const segs = normalisePath(path);
		let arr: Step[] | undefined;
		let cur: Step | undefined;
		let lastId = '';
		let i = 0;
		if (segs[0] === 'steps') {
			arr = auto.definition.steps ?? [];
			i = 1;
		} else if (segs[0] === 'scratch' && /^\d+$/.test(segs[1] ?? '')) {
			arr = (auto.definition.scratch ?? [])[Number(segs[1])];
			i = 2;
		} else return '';
		while (i < segs.length) {
			const s = segs[i];
			if (arr && /^\d+$/.test(s)) {
				cur = arr[Number(s)];
				arr = undefined;
				if (cur?.id) lastId = cur.id;
				i++;
				continue;
			}
			if (!cur) break;
			if (s === 'then' || s === 'else' || s === 'default' || s === 'on_error') {
				arr = cur[s] ?? [];
				cur = undefined;
				i++;
				continue;
			}
			if (s === 'cases' && /^\d+$/.test(segs[i + 1] ?? '') && segs[i + 2] === 'do') {
				arr = cur.cases?.[Number(segs[i + 1])]?.do ?? [];
				cur = undefined;
				i += 3;
				continue;
			}
			if (s === 'on_error_cases' && /^\d+$/.test(segs[i + 1] ?? '') && segs[i + 2] === 'do') {
				arr = cur.on_error_cases?.[Number(segs[i + 1])]?.do ?? [];
				cur = undefined;
				i += 3;
				continue;
			}
			if (s === 'spec' && segs[i + 1] === 'branches' && /^\d+$/.test(segs[i + 2] ?? '')) {
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
				arr = ((cur.spec as any)?.branches ?? [])[Number(segs[i + 2])];
				cur = undefined;
				i += 3;
				continue;
			}
			break;
		}
		return lastId;
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

	// ── load ──
	$effect(() => {
		void autoId;
		const gen = ++loadGen;
		loaded = false;
		loadError = '';
		auto = null;
		baseline = '';
		selectedId = '';
		validation = null;
		runs = [];
		clearDockTimer();
		phase = 'idle';
		dockError = '';
		(async () => {
			try {
				const fresh = await reload();
				if (gen === loadGen && fresh && !fresh.builtin) await loadRuns();
			} catch (e) {
				if (gen !== loadGen) return;
				loadError = e instanceof Error ? e.message : String(e);
			}
			if (gen === loadGen) loaded = true;
		})();
	});

	async function fetchAuto(): Promise<EditAutomation> {
		const a = await api.automation(store.id, autoId);
		return {
			id: a.id,
			name: a.name,
			description: a.description,
			enabled: a.enabled,
			status: a.status ?? 'draft',
			version: a.version ?? 1,
			trigger_type: a.trigger_type,
			trigger_config: a.trigger_config ?? {},
			definition: normaliseDefinition(a.definition ?? {}),
			builtin: !!a.builtin,
			feature_name: a.feature_name,
			feature_tab: a.feature_tab
		};
	}
	async function reload(): Promise<EditAutomation | null> {
		const gen = loadGen;
		const fresh = await fetchAuto();
		if (gen !== loadGen) return null;
		auto = fresh;
		baseline = JSON.stringify(fresh);
		if (!fresh.builtin) void validate();
		return fresh;
	}
	async function loadRuns() {
		const r = await api.automationRuns(store.id, autoId, 25);
		runs = r.runs ?? [];
	}
	function normaliseDefinition(d: Partial<Definition>): Definition {
		return {
			variables: d.variables ?? [],
			steps: d.steps ?? [],
			scratch: (d.scratch ?? []).map((ch) => ch),
			ui_hints: d.ui_hints
		};
	}

	async function validate() {
		if (!auto || auto.builtin) {
			validating = false;
			return;
		}
		const gen = ++validateGen;
		const forGen = loadGen;
		try {
			const r = await api.validateAutomation(store.id, {
				name: auto.name,
				trigger_type: auto.trigger_type,
				trigger_config: auto.trigger_config,
				definition: auto.definition
			});
			if (gen !== validateGen || forGen !== loadGen || !auto) return;
			validation = r.validation;
		} catch {
			/* ignore */
		} finally {
			if (gen === validateGen) validating = false;
		}
	}
	let validateTimer: ReturnType<typeof setTimeout>;
	$effect(() => {
		if (!auto || !loaded || auto.builtin) return;
		void JSON.stringify({ d: auto.definition, t: auto.trigger_config, ty: auto.trigger_type, n: auto.name });
		clearTimeout(validateTimer);
		validating = true;
		validateTimer = setTimeout(validate, 400);
	});

	function clearDockTimer() {
		if (dockTimer) {
			clearTimeout(dockTimer);
			dockTimer = null;
		}
	}

	async function save(thenPublish = false) {
		if (!auto || inFlight || readonly) return;
		clearDockTimer();
		phase = thenPublish ? 'publishing' : 'saving';
		dockError = '';
		const sent = JSON.stringify(auto);
		const gen = loadGen;
		try {
			const r = await api.upsertAutomation(store.id, {
				id: auto.id,
				name: auto.name,
				description: auto.description,
				enabled: auto.enabled,
				status: thenPublish ? 'published' : auto.status,
				trigger_type: auto.trigger_type,
				trigger_config: auto.trigger_config,
				definition: auto.definition
			});
			if (gen !== loadGen) return;
			validation = r.validation;
			const fresh = await fetchAuto();
			if (gen !== loadGen) return;
			baseline = JSON.stringify(fresh);
			if (auto && JSON.stringify(auto) === sent) {
				auto = fresh;
				void validate();
			} else if (auto) {
				auto.version = fresh.version;
				auto.status = fresh.status;
			}
			phase = thenPublish ? 'published' : 'saved';
			dockTimer = setTimeout(() => (phase = 'idle'), thenPublish ? 1600 : 1400);
		} catch (e) {
			if (gen !== loadGen) return;
			dockErrorAction = thenPublish ? 'publish' : 'save';
			dockError = e instanceof Error ? e.message : 'Request failed';
			phase = 'error';
			dockTimer = setTimeout(() => {
				phase = 'idle';
				dockError = '';
			}, 6000);
		}
	}

	$effect(() => {
		if (dirty && (phase === 'saved' || phase === 'published')) {
			clearDockTimer();
			phase = 'idle';
		}
	});

	// The welcome flow is read-only except for its button click actions. Spine
	// (builtin-*) nodes can't be opened in the editor drawer (see selectedStep),
	// dragged from (except their component dots), or deleted, so a spine edit that
	// wouldn't persist can never be accepted-then-silently-discarded.
	const isSpineNode = (id: string | null) => !!id && id.startsWith('builtin-');
	// tailAnchorId is the spine node the editable tail hangs off. Welcome/leveling
	// anchor on the channel message ('builtin-send'); auto-roles and reaction-role
	// menus have no message, so their tail hangs off the last leading spine node
	// instead (the grant step, or the menu's builtin-apply/builtin-disabled).
	const tailAnchorId = $derived.by(() => {
		if (!auto) return '';
		const steps = auto.definition.steps ?? [];
		if (auto.feature_tab === 'auto-roles' || auto.feature_tab === 'reaction-roles') {
			let last = '';
			for (const s of steps) {
				if (!isSpineNode(s.id)) break;
				last = s.id;
			}
			return last;
		}
		return steps.some((s) => s.id === 'builtin-send') ? 'builtin-send' : '';
	});
	function welcomeAddFromHandle(sourceNodeId: string, handle: string | null, kind: string) {
		// Off a spine node two handles are live: a button dot ('component-…'),
		// which wires that button's click action, and the tail anchor's main out
		// handle, which anchors the post-spine tail ("connect a new action after
		// sending the message" / after granting roles). Block the rest (the spine's
		// error / DM handles can't persist edits). Tail steps (their own non-builtin
		// ids) stay fully chainable.
		if (isSpineNode(sourceNodeId)) {
			const h = handle ?? '';
			const isButtonDot = h.startsWith('component-');
			const isTailAnchor =
				!!tailAnchorId && sourceNodeId === tailAnchorId && (h === 'out' || h === 'after' || h === '');
			if (!isButtonDot && !isTailAnchor) return;
		}
		addFromHandle(sourceNodeId, handle, kind);
	}
	function welcomeDeleteStep(id: string) {
		if (isSpineNode(id)) return; // the generated message/card/DM steps aren't deletable here
		deleteStep(id);
	}
	// guardSpine wraps a node mutation so it no-ops on the generated spine nodes
	// (builtin-*): their edits regenerate from config and wouldn't persist. Tail
	// and click-action steps (their own non-builtin ids) pass straight through, so
	// branching, error handlers and chain edits work on them exactly like a
	// regular automation.
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	function guardSpine(fn: (id: string, ...rest: any[]) => void) {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		return (id: string, ...rest: any[]) => {
			if (!isSpineNode(id)) fn(id, ...rest);
		};
	}

	// extractWelcomeActions reads the click-routers (the wait_for + switch pairs
	// the canvas builds from button dots) back into the per-suffix action programs
	// the Welcome config stores. There can be two: one right after the DM
	// (builtin-dm) and one right after the channel message (builtin-send). Anchor
	// on those spine nodes precisely — a tail step could itself be a wait_for +
	// switch and must not be mistaken for a button router. Empty cases (all steps
	// deleted) drop out so the button cleanly reverts to "no action".
	type WelcomeAction = { suffix: string; steps: Step[] };
	function routerActions(sw?: Step): WelcomeAction[] {
		return (sw?.cases ?? [])
			.map((c) => ({ suffix: c.when?.src ?? '', steps: c.do ?? [] }))
			.filter((a) => a.suffix && a.steps.length > 0);
	}
	function extractWelcomeActions(def: Definition): { channel: WelcomeAction[]; dm: WelcomeAction[] } {
		const steps = def.steps ?? [];
		const out: { channel: WelcomeAction[]; dm: WelcomeAction[] } = { channel: [], dm: [] };
		const dmIdx = steps.findIndex((s) => s.id === 'builtin-dm');
		if (dmIdx >= 0 && isClickWait(steps[dmIdx + 1]) && isClickSwitch(steps[dmIdx + 2], steps[dmIdx + 1])) {
			out.dm = routerActions(steps[dmIdx + 2]);
		}
		const sendIdx = steps.findIndex((s) => s.id === 'builtin-send');
		if (sendIdx >= 0 && isClickWait(steps[sendIdx + 1]) && isClickSwitch(steps[sendIdx + 2], steps[sendIdx + 1])) {
			out.channel = routerActions(steps[sendIdx + 2]);
		}
		return out;
	}

	// extractWelcomeTail reads the post-message flow back out of the generated
	// definition: everything after the channel message (and its fused click
	// router) is the editable tail the admin wired ("connect a new action after
	// sending the message").
	function extractWelcomeTail(def: Definition): Step[] {
		const steps = def.steps ?? [];
		const sendIdx = steps.findIndex((s) => s.id === 'builtin-send');
		if (sendIdx < 0) return [];
		return steps.slice(insertionIndex(steps, sendIdx));
	}

	// extractSpineTail reads the follow-up flow back out of a generated definition
	// whose spine is just read-only steps at the head (all `builtin-*`): auto-roles
	// (the grant step) and reaction-role menus (builtin-apply / builtin-disabled).
	// Neither has a message or buttons, so the editable tail is everything after
	// the leading spine (the flow the admin wired off its out handle). We skip the
	// leading spine nodes rather than anchoring on a fixed id, so a tail step of
	// its own never gets mistaken for the spine.
	function extractSpineTail(def: Definition): Step[] {
		const steps = def.steps ?? [];
		let i = 0;
		while (i < steps.length && isSpineNode(steps[i].id)) i++;
		return steps.slice(i);
	}

	// saveFeatureActions writes the canvas-authored click actions + tail back into
	// the owning feature's config, routing to the right endpoint by feature_tab:
	// welcome takes a kind (welcome/goodbye) and a DM router; leveling is a single
	// channel surface with no DM tab; auto-roles and reaction-role menus have no
	// message (so no click actions), only the post-spine tail. Reaction-role
	// builtins are per-menu ("reactionroles.menu.<id>"), so the menu id parses out
	// of the automation id.
	async function saveFeatureActions() {
		if (!auto || featureSaving === 'saving' || !featureDirty) return;
		if (featureDockTimer) clearTimeout(featureDockTimer);
		featureSaving = 'saving';
		featureErr = '';
		const gen = loadGen;
		try {
			if (auto.id.startsWith('reactionroles.menu.')) {
				const menuId = Number(auto.id.split('.')[2]);
				await api.saveMenuTail(store.id, menuId, extractSpineTail(auto.definition));
			} else if (auto.feature_tab === 'auto-roles') {
				await api.saveAutoroleActions(store.id, extractSpineTail(auto.definition));
			} else if (auto.feature_tab === 'leveling') {
				const acts = extractWelcomeActions(auto.definition);
				await api.saveLevelingActions(store.id, acts.channel, extractWelcomeTail(auto.definition));
			} else {
				const acts = extractWelcomeActions(auto.definition);
				await api.saveWelcomeActions(
					store.id,
					welcomeKind,
					acts.channel,
					acts.dm,
					extractWelcomeTail(auto.definition)
				);
			}
			if (gen !== loadGen) return;
			const fresh = await fetchAuto();
			if (gen !== loadGen) return;
			auto = fresh;
			baseline = JSON.stringify(fresh);
			featureSaving = 'saved';
			featureDockTimer = setTimeout(() => (featureSaving = 'idle'), 1500);
		} catch (e) {
			if (gen !== loadGen) return;
			featureErr = e instanceof Error ? e.message : 'Could not save';
			featureSaving = 'error';
		}
	}
	function resetFeatureActions() {
		if (baseline) auto = JSON.parse(baseline);
		featureSaving = 'idle';
		featureErr = '';
	}

	function onShortcut(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') {
			e.preventDefault();
			if (featureEditable) {
				if (featureDirty) void saveFeatureActions();
				return;
			}
			if (inFlight || readonly) return;
			if (dirty) void save(false);
			else {
				pillVisible = true;
				if (pillTimer) clearTimeout(pillTimer);
				pillTimer = setTimeout(() => (pillVisible = false), 1200);
			}
		}
	}

	function reset() {
		if (baseline) auto = JSON.parse(baseline);
	}

	function jumpToIssue(iss: ValidationIssue) {
		const stepId = stepIdAtPath(iss.path);
		if (stepId) selectedId = stepId;
		else if (iss.path.startsWith('trigger')) triggerOpen = true;
		else settingsOpen = true;
	}

	// ── step mutations (identical canvas semantics to the command editor) ──
	// tuneForEvent adjusts a freshly-created step's defaults for event context:
	// waits cap at 1 minute, so a new Wait-for defaults to 30s, not the
	// command-side 10m.
	function tuneForEvent(ns: Step) {
		if (ns.kind === 'wait_for') {
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			ns.spec = { ...((ns.spec ?? {}) as any), timeout: '30s' };
		}
	}
	function addAtRoot(kind: string) {
		if (!auto) return;
		const ns = newStep(kind);
		tuneForEvent(ns);
		auto.definition.steps = [...(auto.definition.steps ?? []), ns];
		selectedId = ns.id;
	}
	function absorbFollowing(ns: Step, following: Step[]): boolean {
		if (following.length === 0) return false;
		if (ns.kind === 'if') ns.then = following;
		else if (ns.kind === 'switch') ns.default = following;
		else return false;
		return true;
	}
	function addFromHandle(sourceNodeId: string, handle: string | null, kind: string) {
		if (!auto) return;
		const ns = newStep(kind);
		tuneForEvent(ns);
		if (sourceNodeId === ENTRY_ID) {
			const rest = (auto.definition.steps ?? []).slice();
			auto.definition.steps = absorbFollowing(ns, rest) ? [ns] : [ns, ...rest];
			selectedId = ns.id;
			return;
		}
		const routerOwner = errorRouterOwner(sourceNodeId);
		if (routerOwner) {
			const owner = locateStep(auto.definition.steps ?? [], routerOwner);
			if (!owner) return;
			const h = handle ?? 'default';
			if (h.startsWith('arm-')) {
				const ei = Number(h.slice(4));
				const cases = owner.step.on_error_cases ?? [];
				if (!cases[ei]) return;
				cases[ei].do = [...(cases[ei].do ?? []), ns];
				owner.step.on_error_cases = [...cases];
			} else owner.step.on_error = [...(owner.step.on_error ?? []), ns];
			auto.definition.steps = [...(auto.definition.steps ?? [])];
			selectedId = ns.id;
			return;
		}
		const located = locateStep(auto.definition.steps ?? [], sourceNodeId);
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
		} else if (h === 'on_timeout') {
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const spec = (src.spec ?? {}) as any;
			spec.on_timeout = [...(spec.on_timeout ?? []), ns];
			src.spec = spec;
		} else if (h.startsWith('component-')) {
			addClickAction(branch, index, h.slice('component-'.length), ns);
		} else branch.splice(index + 1, 0, ns);
		auto.definition.steps = [...(auto.definition.steps ?? [])];
		selectedId = ns.id;
	}
	function locateAnywhere(id: string): { branch: Step[]; index: number; step: Step } | null {
		if (!auto) return null;
		const hit = locateStep(auto.definition.steps ?? [], id);
		if (hit) return hit;
		for (const ch of auto.definition.scratch ?? []) {
			const f = locateStep(ch, id);
			if (f) return f;
		}
		return null;
	}
	function locateStep(steps: Step[], id: string): { branch: Step[]; index: number; step: Step } | null {
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
		if (!auto) return;
		const located = locateAnywhere(id);
		if (!located) return;
		located.branch.splice(located.index, 1);
		auto.definition.scratch = (auto.definition.scratch ?? []).filter((ch) => ch.length > 0);
		auto.definition.steps = [...(auto.definition.steps ?? [])];
		if (selectedId === id) selectedId = '';
	}
	function detachToScratch(id: string) {
		if (!auto) return;
		const located = locateAnywhere(id);
		if (!located) return;
		const chain = located.branch.splice(located.index);
		if (chain.length === 0) return;
		auto.definition.scratch = [...(auto.definition.scratch ?? []), chain];
		auto.definition.steps = [...(auto.definition.steps ?? [])];
	}
	function attachScratch(sourceNodeId: string, handle: string | null, headId: string) {
		if (!auto) return;
		const all = auto.definition.scratch ?? [];
		const idx = all.findIndex((ch) => ch[0]?.id === headId);
		if (idx < 0) return;
		const chain = all[idx];
		auto.definition.scratch = all.filter((_, i) => i !== idx);
		insertChain(sourceNodeId, handle, chain);
		auto.definition.steps = [...(auto.definition.steps ?? [])];
	}
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
	function insertionIndex(branch: Step[], index: number): number {
		const next = branch[index + 1];
		const nextNext = branch[index + 2];
		if (isClickWait(next) && isClickSwitch(nextNext, next)) return index + 3;
		return index + 1;
	}
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
		wait.spec = { trigger: 'component', into: 'click', timeout: '30s' };
		const sw = newStep('switch');
		sw.spec = { on: { lang: 'tmpl', src: '{{ .Vars.click.id }}' } };
		sw.cases = [{ when: { lang: 'tmpl', src: sfx }, do: [ns] }];
		sw.default = [];
		branch.splice(index + 1, 0, wait, sw);
	}
	function insertChain(sourceNodeId: string, handle: string | null, chain: Step[]) {
		if (!auto || chain.length === 0) return;
		if (sourceNodeId === ENTRY_ID) {
			auto.definition.steps = [...chain, ...(auto.definition.steps ?? [])];
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
			} else owner.step.on_error = [...(owner.step.on_error ?? []), ...chain];
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
		} else if (h === 'on_timeout') {
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const spec = (src.spec ?? {}) as any;
			spec.on_timeout = [...(spec.on_timeout ?? []), ...chain];
			src.spec = spec;
		} else branch.splice(index + 1, 0, ...chain);
	}
	function addErrorRouter(id: string) {
		if (!auto) return;
		const located = locateStep(auto.definition.steps ?? [], id);
		if (!located) return;
		if (located.step.on_error === undefined && !located.step.on_error_cases?.length) located.step.on_error = [];
		auto.definition.steps = [...(auto.definition.steps ?? [])];
	}
	function removeErrorRouter(id: string) {
		if (!auto) return;
		const located = locateStep(auto.definition.steps ?? [], id);
		if (!located) return;
		located.step.on_error = undefined;
		located.step.on_error_cases = undefined;
		auto.definition.steps = [...(auto.definition.steps ?? [])];
	}
	function absorbAfterInto(id: string, which: 'then' | 'else' | 'default') {
		if (!auto) return;
		const located = locateStep(auto.definition.steps ?? [], id);
		if (!located) return;
		const following = located.branch.splice(located.index + 1);
		if (following.length === 0) return;
		located.step[which] = [...(located.step[which] ?? []), ...following];
		auto.definition.steps = [...(auto.definition.steps ?? [])];
	}
	function truncateChain(id: string) {
		if (!auto) return;
		const located = locateStep(auto.definition.steps ?? [], id);
		if (!located) return;
		located.branch.splice(located.index);
		auto.definition.steps = [...(auto.definition.steps ?? [])];
		if (selectedId && !findStep(auto.definition.steps ?? [], selectedId)) selectedId = '';
	}
	function addCase(id: string) {
		if (!auto) return;
		const located = locateStep(auto.definition.steps ?? [], id);
		if (!located || located.step.kind !== 'switch') return;
		const ns = newStep('send_message');
		located.step.cases = [...(located.step.cases ?? []), { when: { lang: 'tmpl', src: '' }, do: [ns] }];
		auto.definition.steps = [...(auto.definition.steps ?? [])];
		selectedId = ns.id;
	}
	function addParallelBranchSlot(id: string) {
		if (!auto) return;
		const located = locateStep(auto.definition.steps ?? [], id);
		if (!located || located.step.kind !== 'parallel') return;
		const ns = newStep('send_message');
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const spec = (located.step.spec ?? {}) as any;
		spec.branches = [...(spec.branches ?? []), [ns]];
		located.step.spec = spec;
		auto.definition.steps = [...(auto.definition.steps ?? [])];
		selectedId = ns.id;
	}

	function addVariable() {
		if (!auto) return;
		auto.definition.variables = [
			...(auto.definition.variables ?? []),
			{ name: `var${(auto.definition.variables?.length ?? 0) + 1}`, type: 'string', scope: 'run' }
		];
	}
	function removeVariable(i: number) {
		if (!auto) return;
		auto.definition.variables = (auto.definition.variables ?? []).filter((_, idx) => idx !== i);
	}

	// ── trigger-config helpers (comma-separated id lists) ──
	function listStr(arr?: string[]): string {
		return (arr ?? []).join(', ');
	}
	function parseList(s: string): string[] {
		return s
			.split(/[\s,]+/)
			.map((x) => x.trim().replace(/[<>@&#!]/g, ''))
			.filter(Boolean);
	}
	function tcfg(): TriggerConfig {
		if (!auto) return {};
		return auto.trigger_config;
	}
	function setCfg<K extends keyof TriggerConfig>(key: K, val: TriggerConfig[K]) {
		if (!auto) return;
		auto.trigger_config = { ...auto.trigger_config, [key]: val };
	}
	function supports(filter: string): boolean {
		return (triggerMeta?.filters ?? []).some((f) => f === filter);
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
			if (e.key === 'Escape' && !e.defaultPrevented) selectedId = '';
		};
		window.addEventListener('keydown', onKey);
		return () => window.removeEventListener('keydown', onKey);
	});

	function relTime(iso: string): string {
		const diff = (Date.now() - new Date(iso).getTime()) / 1000;
		if (diff < 60) return `${Math.round(diff)}s`;
		if (diff < 3600) return `${Math.round(diff / 60)}m`;
		if (diff < 86400) return `${Math.round(diff / 3600)}h`;
		return `${Math.round(diff / 86400)}d`;
	}
</script>

<svelte:head>
	<title>{auto ? auto.name : 'Automation'} · {store.name} · Dia</title>
</svelte:head>

<svelte:window onkeydown={onShortcut} />

{#if loadError}
	<div class="flex h-full flex-col bg-bg">
		<div class="flex h-12 shrink-0 items-center gap-3 border-b border-line bg-bg px-3">
			<button
				type="button"
				class="grid size-8 place-items-center rounded text-muted hover:bg-surface hover:text-ink"
				onclick={() => goto(`/servers/${store.id}/automations`)}
				title="Back"
			>
				<ChevronLeft size={14} />
			</button>
			<span class="font-mono text-[10px] uppercase tracking-[0.14em] text-faint">Automations</span>
			<div class="h-3.5 w-px bg-line"></div>
			<span class="font-mono text-[12px] text-danger">Could not load automation</span>
		</div>
		<div class="flex flex-1 items-center justify-center px-5">
			<div class="text-center">
				<CircleAlert size={18} class="mx-auto mb-2 text-danger" />
				<p class="text-[12.5px] text-ink">{loadError}</p>
				<button
					type="button"
					class="mt-4 inline-flex h-7 items-center rounded-md bg-ink px-2.5 text-[12px] font-medium text-bg"
					onclick={() => goto(`/servers/${store.id}/automations`)}
				>
					Back
				</button>
			</div>
		</div>
	</div>
{:else if !loaded || !auto}
	<div class="flex h-full flex-col bg-bg">
		<div class="flex h-12 shrink-0 items-center gap-3 border-b border-line bg-bg px-3">
			<div class="skeleton size-7 rounded"></div>
			<span class="font-mono text-[10px] uppercase tracking-[0.14em] text-faint">Automations</span>
		</div>
		<div class="flex flex-1 items-center justify-center">
			<span class="dots-loader" aria-hidden="true"><span></span><span></span><span></span></span>
		</div>
	</div>
{:else}
	<div class="flex h-full flex-col bg-bg text-ink fade-in">
		<header class="flex h-12 shrink-0 items-center gap-2 border-b border-line bg-bg px-3">
			<button
				type="button"
				class="grid size-8 place-items-center rounded text-muted transition-colors hover:bg-surface hover:text-ink"
				onclick={() => goto(`/servers/${store.id}/automations`)}
				title="Back to automations"
			>
				<ChevronLeft size={14} />
			</button>

			{#if readonly}
				<span class="grid size-7 place-items-center rounded-[7px] border border-line text-faint" title="Built-in, managed by {auto.feature_name}">
					<Lock size={13} />
				</span>
			{:else}
				<button
					type="button"
					role="switch"
					aria-checked={auto.enabled}
					aria-label="Automation enabled"
					class="grid size-7 shrink-0 place-items-center rounded-[7px] border transition-colors duration-200 {auto.enabled
						? 'border-success/40 bg-success/[0.08] text-success hover:border-success/70'
						: 'border-line text-faint hover:border-line-strong hover:text-muted'} disabled:opacity-60"
					disabled={inFlight}
					onclick={() => auto && (auto.enabled = !auto.enabled)}
					title={auto.enabled ? 'On. Click to turn off; applies on save.' : 'Off. Click to turn on; applies on save.'}
				>
					{#key auto.enabled}
						<span class="power-flip grid place-items-center"><Power size={13} /></span>
					{/key}
				</button>
			{/if}

			{#if !readonly && auto.enabled !== baseEnabled}
				<span
					in:fly={{ x: -4, duration: dur(160), easing: cubicOut }}
					out:fade={{ duration: dur(120) }}
					class="inline-flex shrink-0 items-center gap-1 font-mono text-[9px] font-medium uppercase tracking-[0.14em] text-faint"
				>
					<span class="size-1 animate-pulse rounded-full bg-ink/70"></span> on save
				</span>
			{/if}

			{#if readonly}
				<span class="truncate text-[13px] font-medium text-ink">{auto.name}</span>
			{:else}
				<input
					class="min-w-0 rounded border border-transparent bg-transparent px-1 py-0.5 text-[13px] font-medium focus:border-line focus:outline-none {auto.enabled ? 'text-ink' : 'text-muted'}"
					style="width: {Math.max(8, (auto.name?.length ?? 0) + 2)}ch"
					bind:value={auto.name}
				/>
			{/if}

			<!-- Trigger capsule -->
			<button
				type="button"
				class="inline-flex h-[22px] shrink-0 items-center gap-1 rounded border border-line bg-surface/40 px-1.5 font-mono text-[10px] uppercase tracking-[0.1em] text-muted transition-colors hover:border-line-strong hover:text-ink"
				onclick={() => (triggerOpen = true)}
				title="Configure the trigger"
			>
				<Zap size={10} />
				{triggerMeta?.label ?? auto.trigger_type}
			</button>

			{#if !readonly && auto.status === 'draft'}
				<span class="shrink-0 font-mono text-[10px] uppercase tracking-[0.12em] text-faint">draft</span>
			{/if}

			{#if !readonly && validation && issueCount > 0}
				{@const issueLabel = errorCount > 0 ? `${errorCount} error${errorCount === 1 ? '' : 's'}` : `${issueCount} warning${issueCount === 1 ? '' : 's'}`}
				<Popover.Root>
					<Popover.Trigger
						class="inline-flex h-[22px] shrink-0 items-center gap-1 rounded border px-1.5 font-mono text-[10px] transition-colors {errorCount > 0
							? 'border-danger/30 bg-danger/5 text-danger hover:border-danger/50'
							: 'border-line-strong bg-surface/40 text-muted hover:text-ink'}"
						title={issueLabel}
					>
						<CircleAlert size={10} />
						{errorCount > 0 ? errorCount : issueCount}
					</Popover.Trigger>
					<Popover.Content class="w-[340px] p-1.5" side="bottom" align="start" sideOffset={8}>
						<PreflightIssues issues={validation.issues} onJump={jumpToIssue} />
					</Popover.Content>
				</Popover.Root>
			{/if}

			<div class="ml-auto flex items-center gap-1">
				{#if readonly}
					<a
						href={`/servers/${store.id}/${auto.feature_tab}`}
						class="inline-flex h-7 items-center gap-1.5 rounded-md bg-ink px-2.5 text-[12px] font-medium text-bg transition-opacity hover:opacity-90"
					>
						Configure in {auto.feature_name}
						<ArrowRight size={12} />
					</a>
				{:else}
					<button
						type="button"
						class="inline-flex h-7 items-center gap-1.5 rounded-md px-2 text-[12px] font-medium text-muted transition-colors hover:bg-surface hover:text-ink"
						onclick={() => (triggerOpen = true)}
						title="Trigger & filters"
					>
						<Zap size={12} />
						<span class="hidden sm:inline">Trigger</span>
					</button>
					<button
						type="button"
						class="grid size-7 place-items-center rounded text-faint transition-colors hover:bg-surface hover:text-ink"
						onclick={() => (settingsOpen = true)}
						title="Settings"
					>
						<Settings size={13} />
					</button>
				{/if}
			</div>
		</header>

		<div class="relative min-h-0 flex-1 overflow-hidden bg-bg">
			{#if readonly && !featureEditable}
				<div
					class="pointer-events-none absolute left-1/2 top-3 z-30 -translate-x-1/2 whitespace-nowrap rounded-full border border-line bg-surface/90 px-3 py-1 font-mono text-[10px] uppercase tracking-[0.12em] text-muted backdrop-blur"
				>
					<Lock size={9} class="mr-1 inline" /> Built-in · read-only · managed by {auto.feature_name}
				</div>
				<FlowCanvas
					steps={auto.definition.steps as Step[]}
					scratch={auto.definition.scratch ?? []}
					commandName={triggerMeta?.label ?? auto.trigger_type}
					commandId={auto.id}
					bind:selectedId
					showLegend={false}
				/>
			{:else if featureEditable}
				<div
					class="pointer-events-none absolute left-1/2 top-3 z-30 -translate-x-1/2 max-w-[92%] truncate rounded-full border border-line bg-surface/90 px-3 py-1 text-center font-mono text-[10px] uppercase tracking-[0.12em] text-muted backdrop-blur"
				>
					<MousePointerClick size={9} class="mr-1 inline" />
					{#if auto.feature_tab === 'auto-roles'}
						Drag off the grant step to add a follow-up action. The roles granted on join are managed in {auto.feature_name}.
					{:else if auto.feature_tab === 'reaction-roles'}
						The grey steps mirror this menu's role assignment and are managed on the Reaction Roles tab. Steps you connect after them run when a member picks their roles.
					{:else}
						Drag off the message to add a follow-up action, or off a button's dot to set what it does. Message, embed &amp; card are managed in {auto.feature_name}.
					{/if}
				</div>
				<FlowCanvas
					steps={auto.definition.steps as Step[]}
					scratch={auto.definition.scratch ?? []}
					commandName={triggerMeta?.label ?? auto.trigger_type}
					commandId={auto.id}
					bind:selectedId
					{errorPaths}
					palette={paletteFor}
					showLegend={false}
					onAddFromHandle={welcomeAddFromHandle}
					onDeleteStep={welcomeDeleteStep}
					onDetach={guardSpine(detachToScratch)}
					onAttachScratch={attachScratch}
					onAddErrorRouter={guardSpine(addErrorRouter)}
					onRemoveErrorRouter={removeErrorRouter}
					onTruncateChain={guardSpine(truncateChain)}
					onAbsorbAfter={guardSpine(absorbAfterInto)}
					onAddCase={guardSpine(addCase)}
					onAddParallelBranch={guardSpine(addParallelBranchSlot)}
				/>
				{#if featureDirty || featureSaving !== 'idle'}
					<div
						class="pointer-events-none absolute inset-x-4 bottom-4 z-40 flex justify-center"
						transition:fly={{ y: 14, duration: dur(180), easing: cubicOut }}
					>
						<div
							class="pointer-events-auto flex h-11 items-center gap-2.5 rounded-[14px] border bg-surface/95 px-3.5 shadow-[0_16px_40px_-12px_rgba(0,0,0,0.7)] backdrop-blur-md {featureSaving ===
							'error'
								? 'border-danger/40'
								: 'border-line'}"
						>
							{#if featureSaving === 'saving'}
								<Loader2 size={15} class="animate-spin text-muted" />
								<span class="text-[12.5px] text-muted">Saving…</span>
							{:else if featureSaving === 'saved'}
								<span class="grid size-4 place-items-center rounded-full bg-success/15 text-success"
									><Check size={11} /></span
								>
								<span class="text-[12.5px] text-ink">Saved</span>
							{:else if featureSaving === 'error'}
								<CircleAlert size={15} class="text-danger" />
								<span class="max-w-[16rem] truncate text-[12.5px] text-ink" title={featureErr}
									>{featureErr || "Couldn't save"}</span
								>
								<button
									type="button"
									onclick={() => saveFeatureActions()}
									class="ml-1 inline-flex h-7 items-center rounded-md bg-ink px-2.5 text-[12px] font-medium text-bg transition-opacity hover:opacity-90"
									>Retry</button
								>
							{:else}
								<span class="size-1.5 animate-pulse rounded-full bg-accent"></span>
								<span class="text-[12.5px] text-muted">Unsaved button actions</span>
								<div class="ml-1 flex items-center gap-1.5">
									<button
										type="button"
										onclick={resetFeatureActions}
										class="inline-flex h-7 items-center rounded-md border border-line-strong px-2.5 text-[12px] font-medium text-muted transition-colors hover:text-ink"
										>Discard</button
									>
									<button
										type="button"
										onclick={() => saveFeatureActions()}
										class="inline-flex h-7 items-center gap-1.5 rounded-md bg-ink px-3 text-[12px] font-medium text-bg transition-opacity hover:opacity-90"
										>Save <kbd class="hidden font-mono text-[10px] text-bg/60 sm:inline">⌘S</kbd></button
									>
								</div>
							{/if}
						</div>
					</div>
				{/if}
			{:else}
				<div
					class="absolute inset-0 transition-[filter] duration-300 motion-reduce:transition-none {auto.enabled ? '' : 'brightness-[0.85] saturate-[0.85]'}"
				>
					<FlowCanvas
						steps={auto.definition.steps as Step[]}
						scratch={auto.definition.scratch ?? []}
						commandName={triggerMeta?.label ?? auto.trigger_type}
						commandId={auto.id}
						bind:selectedId
						{errorPaths}
						palette={paletteFor}
						showLegend={dockState === 'hidden' && !pillVisible}
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
				</div>

				{#if !auto.enabled}
					<div
						in:fly={{ y: -8, duration: dur(200), easing: cubicOut }}
						out:fade={{ duration: dur(150) }}
						class="pointer-events-none absolute left-1/2 top-3 z-30 -translate-x-1/2 whitespace-nowrap rounded-full border border-line bg-surface/90 px-3 py-1 font-mono text-[10px] uppercase tracking-[0.12em] text-muted backdrop-blur"
					>
						Off · won't run{auto.enabled !== baseEnabled ? ' · applies on save' : ''}
					</div>
				{/if}

				<ReleaseDock
					dock={dockState}
					status={auto.status}
					enabled={auto.enabled}
					version={auto.version}
					{validating}
					hasValidation={validation !== null}
					{errorCount}
					issues={validation?.issues ?? []}
					error={dockError}
					errorAction={dockErrorAction}
					{pillVisible}
					{drawerOpen}
					{drawerWide}
					onSave={() => save(false)}
					onPublish={() => save(true)}
					onDiscard={reset}
					onRetry={() => save(dockErrorAction === 'publish')}
					onDismissError={() => {
						clearDockTimer();
						phase = 'idle';
						dockError = '';
					}}
					onJumpToIssue={jumpToIssue}
				/>
			{/if}

			{#if selectedStep}
				<StepDrawer step={selectedStep as Step} onClose={() => (selectedId = '')} onDelete={(id) => deleteStep(id)} />
			{/if}
		</div>
	</div>

	<!-- ── Trigger dialog ── -->
	<Dialog.Root bind:open={triggerOpen}>
		<Dialog.Content class="max-w-2xl">
			<Dialog.Header>
				<Dialog.Title>Trigger & filters</Dialog.Title>
				<Dialog.Description>{triggerMeta?.description ?? 'When this automation runs.'}</Dialog.Description>
			</Dialog.Header>

			<div class="grid max-h-[65vh] gap-5 overflow-y-auto pr-1">
				<!-- Trigger kind -->
				<section>
					<div class="mb-1.5 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">When</div>
					{#if readonly}
						<p class="font-mono text-[11.5px] text-muted">{triggerMeta?.label}</p>
					{:else}
						<FieldSelect
							value={auto.trigger_type}
							onChange={(v) => auto && (auto.trigger_type = v)}
							options={TRIGGERS.map((t) => ({ value: t.key, label: t.label }))}
						/>
						<p class="mt-1 font-mono text-[10px] text-faint">
							<span class="text-muted">.User</span> is {triggerMeta?.actor}.
						</p>
					{/if}
				</section>

				{#if !readonly}
					<!-- Filters -->
					{#if supports('channels')}
						<section class="space-y-2">
							<div>
								<div class="mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Only in channels</div>
								<ChannelPicker
									multiple
									value={tcfg().channels ?? []}
									onChange={(v) => setCfg('channels', v as string[])}
									placeholder="Any channel"
								/>
							</div>
							<div>
								<div class="mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Ignore channels</div>
								<ChannelPicker
									multiple
									value={tcfg().ignore_channels ?? []}
									onChange={(v) => setCfg('ignore_channels', v as string[])}
									placeholder="None ignored"
								/>
							</div>
						</section>
					{/if}

					{#if supports('roles')}
						<section>
							<div class="mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Member roles</div>
							<RolePicker
								multiple
								value={tcfg().roles ?? []}
								onChange={(v) => setCfg('roles', v as string[])}
								placeholder="Any member"
							/>
						</section>
					{/if}

					{#if supports('role')}
						<section>
							<div class="mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Watched role</div>
							<RolePicker
								value={tcfg().role ?? ''}
								onChange={(v) => setCfg('role', v as string)}
								placeholder="Any role change"
							/>
							<p class="mt-1 font-mono text-[10px] text-faint">Tip: use the Server Booster role to catch boosts.</p>
						</section>
					{/if}

					{#if supports('keywords')}
						<section class="grid grid-cols-[1fr_8rem] gap-2">
							<div>
								<div class="mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Keywords</div>
								<input
									class="h-7 w-full rounded-md border border-line bg-bg px-2 text-[11.5px] focus:border-line-strong focus:outline-none"
									placeholder="comma-separated (blank = any message)"
									value={listStr(tcfg().keywords)}
									oninput={(e) => setCfg('keywords', parseList((e.currentTarget as HTMLInputElement).value))}
								/>
							</div>
							<div>
								<div class="mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Match</div>
								<FieldSelect
									value={tcfg().match_mode ?? 'contains'}
									onChange={(v) => setCfg('match_mode', v as TriggerConfig['match_mode'])}
									options={[
										{ value: 'contains', label: 'contains' },
										{ value: 'word', label: 'whole word' },
										{ value: 'equals', label: 'equals' }
									]}
								/>
							</div>
						</section>
					{/if}

					{#if supports('emojis')}
						<section>
							<div class="mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Emojis</div>
							<input
								class="h-7 w-full rounded-md border border-line bg-bg px-2 text-[11.5px] focus:border-line-strong focus:outline-none"
								placeholder="👍, ✅, emoji name or id (blank = any)"
								value={listStr(tcfg().emojis)}
								oninput={(e) => setCfg('emojis', parseList((e.currentTarget as HTMLInputElement).value))}
							/>
						</section>
					{/if}

					{#if supports('ignore_bots')}
						<section class="flex items-center justify-between">
							<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Ignore bots</div>
							<button
								type="button"
								role="switch"
								aria-label="Ignore bots"
								aria-checked={!!tcfg().ignore_bots}
								class="h-5 w-9 rounded-full border transition-colors {tcfg().ignore_bots ? 'border-success/40 bg-success/30' : 'border-line bg-surface'}"
								onclick={() => setCfg('ignore_bots', !tcfg().ignore_bots)}
							>
								<span class="block size-3.5 rounded-full bg-ink transition-transform {tcfg().ignore_bots ? 'translate-x-4' : 'translate-x-0.5'}"></span>
							</button>
						</section>
					{/if}

					{#if supports('cooldown')}
						<section class="grid grid-cols-2 gap-3">
							<div>
								<div class="mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Cooldown</div>
								<FieldSelect
									value={tcfg().cooldown?.scope ?? 'none'}
									onChange={(v) => {
										if (v === 'none') setCfg('cooldown', undefined);
										else setCfg('cooldown', { scope: v as 'user' | 'channel' | 'guild', seconds: tcfg().cooldown?.seconds ?? 30 });
									}}
									options={[
										{ value: 'none', label: 'None' },
										{ value: 'user', label: 'Per user' },
										{ value: 'channel', label: 'Per channel' },
										{ value: 'guild', label: 'Per guild' }
									]}
								/>
							</div>
							<div>
								<div class="mb-1 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Seconds</div>
								<NumberField
									min={0}
									suffix="s"
									value={tcfg().cooldown?.seconds ?? 0}
									disabled={!tcfg().cooldown}
									onChange={(n) => {
										const cd = tcfg().cooldown;
										if (cd) setCfg('cooldown', { ...cd, seconds: n ?? 0 });
									}}
								/>
							</div>
						</section>
					{/if}
				{/if}
			</div>
		</Dialog.Content>
	</Dialog.Root>

	<!-- ── Settings dialog ── -->
	<Dialog.Root bind:open={settingsOpen}>
		<Dialog.Content class="max-w-3xl">
			<Dialog.Header>
				<Dialog.Title>Automation settings</Dialog.Title>
				<Dialog.Description>Description, variables, and recent runs.</Dialog.Description>
			</Dialog.Header>

			<div class="grid max-h-[65vh] gap-5 overflow-y-auto pr-1">
				<section>
					<div class="mb-1.5 font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Description</div>
					<input
						class="h-7 w-full rounded-md border border-line bg-bg px-2 text-[12px] text-ink placeholder:text-faint focus:border-line-strong focus:outline-none"
						bind:value={auto.description}
						maxlength="200"
						placeholder="What does this automation do?"
					/>
				</section>

				<section>
					<div class="mb-1.5 flex items-center justify-between">
						<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Variables</div>
						<button
							type="button"
							onclick={addVariable}
							class="inline-flex h-6 items-center gap-1 rounded border border-line bg-bg px-1.5 text-[11px] font-medium text-muted hover:border-line-strong hover:text-ink"
						>
							<Plus size={11} /> Add
						</button>
					</div>
					{#if (auto.definition.variables?.length ?? 0) === 0}
						<p class="font-mono text-[10.5px] text-faint">No variables.</p>
					{:else}
						<div class="space-y-1">
							{#each auto.definition.variables ?? [] as v, i (i)}
								<div class="grid items-center gap-1.5 sm:grid-cols-[1fr_6rem_1fr_1.5rem]">
									<input class="h-7 rounded-md border border-line bg-bg px-2 text-[11.5px] focus:border-line-strong focus:outline-none" placeholder="name" bind:value={v.name} />
									<FieldSelect bind:value={v.type} options={['string', 'int', 'float', 'bool', 'list', 'object'].map((t) => ({ value: t, label: t }))} />
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
									<button type="button" class="grid size-7 place-items-center rounded text-faint hover:bg-surface hover:text-danger" onclick={() => removeVariable(i)} aria-label="Remove">
										<Trash2 size={11} />
									</button>
								</div>
							{/each}
						</div>
					{/if}
				</section>

				<section>
					<div class="mb-1.5 flex items-center justify-between">
						<div class="font-mono text-[10px] font-medium uppercase tracking-[0.14em] text-faint">Recent runs</div>
						<button type="button" onclick={loadRuns} class="inline-flex h-6 items-center gap-1 rounded border border-line bg-bg px-1.5 text-[11px] font-medium text-muted hover:border-line-strong hover:text-ink">
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
	@keyframes power-flip-in {
		from {
			transform: rotate(-90deg) scale(0.85);
			opacity: 0.6;
		}
		to {
			transform: none;
			opacity: 1;
		}
	}
	.power-flip {
		animation: power-flip-in 180ms cubic-bezier(0.22, 1, 0.36, 1);
	}
	@media (prefers-reduced-motion: reduce) {
		.fade-in,
		.power-flip {
			animation: none;
		}
	}
</style>
