<script lang="ts">
	import {
		SvelteFlow,
		Background,
		Controls,
		BackgroundVariant,
		Panel,
		MarkerType,
		useSvelteFlow,
		type Edge as XYEdge,
		type Node as XYNode,
		type NodeTypes,
		type EdgeTypes,
		type OnConnectEnd
	} from '@xyflow/svelte';
	import type { Step } from '$lib/commands/types';
	import {
		applyLayout,
		treeToGraph,
		ENTRY_ID,
		errorRouterId,
		errorRouterOwner,
		type EdgeData,
		type EdgeInfo,
		type FlowEdge,
		type FlowNode,
		type NodeData
	} from './adapter';
	import KindPicker from '../KindPicker.svelte';
	import EdgePanel from './EdgePanel.svelte';
	import ErrorRouterPanel from './ErrorRouterPanel.svelte';
	import LayoutGrid from 'lucide-svelte/icons/layout-grid';
	import Maximize from 'lucide-svelte/icons/maximize';
	import Plus from 'lucide-svelte/icons/plus';
	import Workflow from 'lucide-svelte/icons/workflow';

	import StepNode from './StepNode.svelte';
	import IfNode from './IfNode.svelte';
	import SwitchNode from './SwitchNode.svelte';
	import LoopNode from './LoopNode.svelte';
	import ParallelNode from './ParallelNode.svelte';
	import EntryNode from './EntryNode.svelte';
	import ErrorRouterNode from './ErrorRouterNode.svelte';
	import PlainEdge from './PlainEdge.svelte';
	import BranchEdge from './BranchEdge.svelte';
	import ErrorEdge from './ErrorEdge.svelte';
	import { onMount, untrack } from 'svelte';
	import { dragState } from './path-tween.svelte';
	import { fly, fade } from 'svelte/transition';
	import { cubicOut } from 'svelte/easing';
	import { dur } from '$lib/motion';

	type FinalConnectionState = Parameters<OnConnectEnd>[1];

	let {
		steps,
		scratch = [],
		commandName,
		commandId = 0,
		selectedId = $bindable<string>(),
		errorPaths = new Set<string>(),
		onAddAtRoot,
		onAddFromHandle,
		onDeleteStep,
		onAddErrorRouter,
		onRemoveErrorRouter,
		onTruncateChain,
		onAbsorbAfter,
		onDetach,
		onAttachScratch,
		onAddCase,
		onAddParallelBranch,
		showLegend = true
	}: {
		steps: Step[];
		scratch?: Step[][];
		commandName: string;
		commandId?: number;
		selectedId: string;
		errorPaths?: Set<string>;
		showLegend?: boolean;
		onAddAtRoot?: (kind: string, position?: { x: number; y: number }) => void;
		onAddFromHandle?: (
			sourceNodeId: string,
			sourceHandle: string | null,
			kind: string,
			position: { x: number; y: number }
		) => void;
		onDeleteStep?: (id: string) => void;
		onAddErrorRouter?: (id: string) => void;
		onRemoveErrorRouter?: (id: string) => void;
		onTruncateChain?: (id: string) => void;
		onAbsorbAfter?: (id: string, which: 'then' | 'else' | 'default') => void;
		onDetach?: (id: string) => void;
		onAttachScratch?: (sourceId: string, handle: string | null, headId: string) => void;
		onAddCase?: (id: string) => void;
		onAddParallelBranch?: (id: string) => void;
	} = $props();

	const nodeTypes: NodeTypes = {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		step: StepNode as any,
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		if: IfNode as any,
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		switch: SwitchNode as any,
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		loop: LoopNode as any,
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		parallel: ParallelNode as any,
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		entry: EntryNode as any,
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		error_router: ErrorRouterNode as any
	};
	const edgeTypes: EdgeTypes = {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		plain: PlainEdge as any,
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		branch: BranchEdge as any,
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		error: ErrorEdge as any
	};

	let nodes = $state.raw<XYNode[]>([]);
	let edges = $state.raw<XYEdge[]>([]);

	// Position memory — the single source of where nodes sit. Seeded by dagre
	// on first paint (and Tidy), captured from the live canvas before every
	// rebuild, and extended incrementally: a NEW node lands right next to the
	// node it connects from; nothing already placed ever moves on its own.
	const positions = new Map<string, { x: number; y: number }>();

	function offsetFor(src: XYNode, handle: string | null | undefined): { x: number; y: number } {
		const { x, y } = src.position;
		const h = handle ?? 'out';
		const DY = 170;
		if (h === 'then') return { x: x - 190, y: y + DY };
		if (h === 'else') return { x: x + 190, y: y + DY };
		if (h === 'body') return { x: x - 170, y: y + DY };
		if (h === 'default' && (src.data as NodeData | undefined)?.kind !== 'error_router')
			return { x: x + 210, y: y + DY };
		if (h === 'on_error') return { x: x - 280, y: y + 110 }; // the router card
		if (h.startsWith('case-')) {
			const i = Number(h.slice(5)) || 0;
			return { x: x - 190 + i * 180, y: y + DY };
		}
		if (h.startsWith('branch-')) {
			const i = Number(h.slice(7)) || 0;
			return { x: x - 190 + i * 180, y: y + DY };
		}
		if (h.startsWith('component-')) {
			// Click chains fan out to the right, staggered per button so two
			// buttons' paths never overlap.
			const sfx = h.slice('component-'.length);
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			const rows = ((((src.data as NodeData | undefined)?.step?.spec ?? {}) as any).components ??
				[]) as { components: { custom_id_suffix?: string }[] }[];
			const all: string[] = [];
			for (const r of rows) for (const c of r.components ?? []) if (c.custom_id_suffix) all.push(c.custom_id_suffix);
			const idx = Math.max(0, all.indexOf(sfx));
			return { x: x + 400, y: y + idx * 180 };
		}
		if (h.startsWith('arm-')) {
			// Error-router arms fan out to the right, staggered per arm.
			const i = Number(h.slice(4)) || 0;
			return { x: x + 260, y: y + i * 110 };
		}
		if (h === 'default' && (src.data as NodeData | undefined)?.kind === 'error_router') {
			// The router's else arm sits below its case arms.
			const arms = ((src.data as NodeData | undefined)?.step?.on_error_cases ?? []).length;
			return { x: x + 260, y: y + arms * 110 };
		}
		return { x, y: y + DY }; // out — straight down the spine
	}

	// One-shot placement hint: when a step is born from a drag-to-empty-space
	// drop, it should appear where the user dropped it, not at the standard
	// offset. Set just before the mutation, consumed by the next place() pass.
	let pendingPlacement: { x: number; y: number } | null = null;

	function place(raw: FlowNode[], rawEdges: FlowEdge[]): FlowNode[] {
		// First paint (or after Tidy): dagre lays out the SPINE; click chains
		// (reached through a button's dot) are excluded and placed relative to
		// their button afterwards, so they fan right instead of stacking under
		// the message card.
		if (!positions.has(ENTRY_ID)) {
			pendingPlacement = null;
			const clickRoots = rawEdges
				.filter((e) => (e.sourceHandle ?? '').startsWith('component-'))
				.map((e) => e.target);
			const inClick = new Set(clickRoots);
			let grew = true;
			while (grew) {
				grew = false;
				for (const e of rawEdges) {
					if (inClick.has(e.source) && !inClick.has(e.target)) {
						inClick.add(e.target);
						grew = true;
					}
				}
			}
			const spineNodes = raw.filter((n) => !inClick.has(n.id));
			const spineEdges = rawEdges.filter(
				(e) => !inClick.has(e.source) && !inClick.has(e.target)
			);
			const laid = applyLayout(spineNodes, spineEdges, { rankdir: 'TB' });
			positions.clear();
			for (const n of laid) positions.set(n.id, n.position);
			// Click-chain nodes fall through to the multi-pass below as
			// unknowns, resolved via offsetFor (right of their button).
		}
		const out = raw.map((n) => ({ ...n, position: positions.get(n.id) ?? n.position }));
		const byId = new Map(out.map((n) => [n.id, n]));
		const unknown = new Set(out.filter((n) => !positions.has(n.id)).map((n) => n.id));

		if (pendingPlacement && unknown.size > 0) {
			// The freshly created node lands centered on the drop point.
			const first = out.find((n) => unknown.has(n.id));
			if (first) {
				first.position = { x: pendingPlacement.x - 124, y: pendingPlacement.y - 30 };
				unknown.delete(first.id);
			}
		}
		pendingPlacement = null;
		// Multi-pass so a freshly added chain resolves link by link.
		let guard = 0;
		while (unknown.size > 0 && guard++ < 12) {
			let progressed = false;
			for (const e of rawEdges) {
				if (!unknown.has(e.target) || unknown.has(e.source)) continue;
				const src = byId.get(e.source);
				const node = byId.get(e.target);
				if (!src || !node) continue;
				node.position = offsetFor(src as XYNode, e.sourceHandle);
				unknown.delete(e.target);
				progressed = true;
			}
			if (!progressed) break;
		}
		if (unknown.size > 0) {
			// Disconnected leftovers: stack them under everything else.
			let yMax = Math.max(
				0,
				...out.filter((n) => !unknown.has(n.id)).map((n) => n.position.y)
			);
			for (const id of unknown) {
				const node = byId.get(id);
				if (node) node.position = { x: 0, y: (yMax += 180) };
			}
		}
		for (const n of out) positions.set(n.id, n.position);
		return out;
	}

	const arrowMuted = {
		type: MarkerType.ArrowClosed,
		width: 15,
		height: 15,
		color: 'hsl(var(--muted-foreground) / 0.7)'
	};
	const arrowBorder = {
		type: MarkerType.ArrowClosed,
		width: 15,
		height: 15,
		color: 'hsl(var(--muted-foreground) / 0.75)'
	};
	const arrowDanger = {
		type: MarkerType.ArrowClosed,
		width: 15,
		height: 15,
		color: 'hsl(var(--destructive))'
	};

	// Subtree highlight: with a step selected, everything not reachable from it
	// fades back so the edited path pops (selection stays while the drawer is
	// open, so the flow behind it reads as context).
	function reachableFrom(rootId: string, allEdges: FlowEdge[]): Set<string> {
		const adj = new Map<string, string[]>();
		for (const e of allEdges) {
			const list = adj.get(e.source) ?? [];
			list.push(e.target);
			adj.set(e.source, list);
		}
		const seen = new Set<string>([rootId]);
		const queue = [rootId];
		while (queue.length) {
			const id = queue.shift()!;
			for (const t of adj.get(id) ?? []) {
				if (!seen.has(t)) {
					seen.add(t);
					queue.push(t);
				}
			}
		}
		return seen;
	}

	// Tidy sets this so the next rebuild relayouts from scratch instead of
	// re-capturing the on-screen positions (which would undo the relayout).
	let forceLayout = false;

	function rebuild() {
		// Capture live positions first — drags land in `nodes` via bind:nodes.
		// untrack: rebuild runs inside an $effect that also WRITES `nodes`;
		// reading it tracked would loop the effect forever.
		if (forceLayout) {
			forceLayout = false;
			positions.clear();
		} else {
			untrack(() => {
				for (const n of nodes) positions.set(n.id, n.position);
			});
		}
		const { nodes: rawNodes, edges: rawEdges } = treeToGraph(steps, commandName, scratch);
		const ids = new Set(rawNodes.map((n) => n.id));
		for (const k of [...positions.keys()]) if (!ids.has(k)) positions.delete(k);
		const lit =
			selectedId && rawNodes.some((n) => n.id === selectedId)
				? reachableFrom(selectedId, rawEdges)
				: null;
		const decoratedNodes = rawNodes.map((n) => decorateNode(n, lit));
		const positioned = place(decoratedNodes, rawEdges);
		nodes = positioned as XYNode[];
		edges = rawEdges.map((e) => decorateEdge(e, lit)) as XYEdge[];
	}

	function decorateNode(n: FlowNode, lit: Set<string> | null): FlowNode {
		const data = { ...n.data };
		const stepPath = data.stepPath;
		if (stepPath && errorPaths.has(stepPath)) {
			(data as Record<string, unknown>).hasError = true;
		}
		if (lit && n.id !== ENTRY_ID && !lit.has(n.id)) {
			(data as Record<string, unknown>).dimmed = true;
		}
		return { ...n, data };
	}

	function decorateEdge(e: FlowEdge, lit: Set<string> | null): FlowEdge {
		const marker =
			e.type === 'error' ? arrowDanger : e.type === 'branch' ? arrowMuted : arrowBorder;
		const dim = lit && !lit.has(e.source) ? 'opacity: 0.12;' : '';
		return {
			...e,
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
			markerEnd: marker as any,
			...(dim ? { style: dim } : {})
		};
	}

	$effect(() => {
		void steps;
		void scratch;
		void commandName;
		void errorPaths;
		void selectedId;
		rebuild();
	});

	function onnodeclick({ node }: { node: XYNode }) {
		edgePanel = null;
		if (node.type === 'entry') {
			selectedId = '';
			return;
		}
		if (node.type === 'error_router') {
			// The router opens its own case editor, not the step drawer.
			selectedId = '';
			routerOwnerId = (node.data as NodeData).ownerId ?? null;
			return;
		}
		routerOwnerId = null;
		selectedId = node.id;
	}

	function onpaneclick() {
		if (performance.now() - pickerOpenedAt < 250) return;
		selectedId = '';
		addOpen = false;
		dropPicker = null;
		edgePanel = null;
		routerOwnerId = null;
	}

	// ── click the on-error router → describe its cases ──────────────────────
	let routerOwnerId = $state<string | null>(null);
	const routerStep = $derived.by(() => {
		if (!routerOwnerId) return null;
		const n = nodes.find((x) => x.id === errorRouterId(routerOwnerId!));
		return ((n?.data as NodeData | undefined)?.step ?? null) as Step | null;
	});

	// ── click a line → connection editor ─────────────────────────────────────
	let edgePanel = $state<EdgeInfo | null>(null);

	function onedgeclick({ edge }: { edge: XYEdge }) {
		const d = (edge.data ?? {}) as EdgeData;
		const ownerId =
			d.sourceId ??
			errorRouterOwner(edge.source) ??
			(edge.source !== ENTRY_ID ? edge.source : undefined);
		// Anything error-related — the rail or an arm — opens the ONE flat
		// on-error editor rather than a per-line panel.
		if (d.branch === 'on_error' && ownerId) {
			edgePanel = null;
			selectedId = '';
			routerOwnerId = ownerId;
			return;
		}
		const owner = ownerId
			? ((nodes.find((n) => n.id === ownerId)?.data as NodeData | undefined)?.step ?? null)
			: null;
		edgePanel = {
			branch: d.branch,
			caseIndex: d.caseIndex,
			rail: d.rail,
			clickWait: d.clickWait,
			clickSwitch: d.clickSwitch,
			sourceStep: owner,
			targetId: edge.target,
			targetKind: (nodes.find((n) => n.id === edge.target)?.data as NodeData | undefined)?.step
				?.kind
		};
		selectedId = '';
		routerOwnerId = null;
	}

	let dragging = $state(false);
	function onnodedragstart() {
		dragging = true;
		dragState.active = true;
	}
	// The lib caches layouted edge geometry by object identity; refreshing
	// edge identities every drag frame forces it to re-derive coordinates so
	// lines FOLLOW the dragged card instead of freezing until the next
	// rebuild (the tween snaps during drags, so tracking is 1:1).
	function onnodedrag() {
		edges = edges.map((e) => ({ ...e }));
	}
	function onnodedragstop({
		targetNode,
		nodes: dragged
	}: {
		targetNode: XYNode | null;
		nodes: XYNode[];
	}) {
		dragging = false;
		dragState.active = false;
		// Remember every node that moved (multi-select drags move several).
		for (const n of dragged ?? (targetNode ? [targetNode] : [])) {
			positions.set(n.id, n.position);
		}
	}

	const { fitView, screenToFlowPosition } = useSvelteFlow();

	// Reattaching islands: the ONLY valid drag-to-connect target is the first
	// step of a disconnected chain. Everything else opens the drop picker.
	const scratchHeads = $derived(new Set(scratch.map((ch) => ch[0]?.id).filter(Boolean)));

	function isValidConnection(conn: { target?: string | null }): boolean {
		return !!conn.target && scratchHeads.has(conn.target);
	}

	function onconnect(conn: { source: string; sourceHandle: string | null; target: string }) {
		if (!scratchHeads.has(conn.target)) return;
		onAttachScratch?.(conn.source, conn.sourceHandle ?? null, conn.target);
	}

	const FIT_OPTS = { duration: 280, padding: 0.25, maxZoom: 0.95, minZoom: 0.4 } as const;

	// Initial-fit retry. SvelteFlow's built-in initial fit fires on mount,
	// usually before our dagre pass has populated `nodes`, so the viewport
	// lands somewhere empty. We wait for the first paint where we have at
	// least entry + one step, then re-fit. setTimeout (vs raf) gives the
	// renderer enough time to measure node widths in the DOM so fitView's
	// bounding box is accurate.
	let firstFitDone = $state(false);
	$effect(() => {
		if (firstFitDone) return;
		if (nodes.length < 2) return;
		firstFitDone = true;
		setTimeout(() => fitView({ ...FIT_OPTS, duration: 0 }), 60);
	});
	// Reset on command navigation so a new command also auto-centers. Keyed on
	// the command's id, not its (editable) name — typing in the name input must
	// not re-fit the viewport.
	$effect(() => {
		void commandId;
		firstFitDone = false;
	});

	// ── Add step (top-left menu) ─────────────────────────────────────────────
	let addOpen = $state(false);

	function pickFromMenu(kind: string) {
		addOpen = false;
		onAddAtRoot?.(kind);
	}

	// ── Drag a dot into empty space → pick the step to create there ─────────
	let containerEl = $state<HTMLDivElement | null>(null);
	let dropPicker = $state<{
		sourceId: string;
		handleId: string | null;
		flowPos: { x: number; y: number };
		left: number;
		top: number;
	} | null>(null);

	function onconnectend(event: MouseEvent | TouchEvent, state: FinalConnectionState) {
		if (state.isValid) return;
		if (!state.fromNode || !state.fromHandle) return;
		// Source handles only — dragging out of a target ("in") handle means
		// the user wanted to rewire upstream, not to create a step.
		if (state.fromHandle.type === 'target') return;

		const pt =
			'changedTouches' in event
				? { x: event.changedTouches[0].clientX, y: event.changedTouches[0].clientY }
				: { x: event.clientX, y: event.clientY };
		const flowPos = screenToFlowPosition(pt);

		// Dragging a step's red dot doesn't pick a step — it spawns the on-error
		// ROUTER at the drop point (or opens the existing one) so the cases can
		// be described and each arm wired up from the router itself.
		if (
			state.fromHandle.id === 'on_error' &&
			(state.fromNode.data as NodeData | undefined)?.kind !== 'error_router'
		) {
			const ownerId = state.fromNode.id;
			const exists = nodes.some((n) => n.id === errorRouterId(ownerId));
			if (!exists) {
				pendingPlacement = flowPos;
				onAddErrorRouter?.(ownerId);
			}
			selectedId = '';
			routerOwnerId = ownerId;
			return;
		}
		const rect = containerEl?.getBoundingClientRect();
		const PICKER_W = 256; // w-64
		const PICKER_H = 350; // max-h-80 list + "Add a connected step" header + borders
		const left = rect
			? Math.min(Math.max(pt.x - rect.left, 8), Math.max(rect.width - PICKER_W - 8, 8))
			: 16;
		const top = rect
			? Math.min(Math.max(pt.y - rect.top, 8), Math.max(rect.height - PICKER_H - 8, 8))
			: 16;

		dropPicker = {
			sourceId: state.fromNode.id,
			handleId: state.fromHandle.id ?? null,
			flowPos,
			left,
			top
		};
		pickerOpenedAt = performance.now();
	}

	// The mouseup that ends a drag can synthesize a click that would land on
	// the dismiss layer and instantly close the picker we just opened.
	let pickerOpenedAt = 0;

	function pickFromDrop(kind: string) {
		const p = dropPicker;
		dropPicker = null;
		if (!p) return;
		// The new node materialises exactly where the drag was dropped.
		pendingPlacement = p.flowPos;
		onAddFromHandle?.(p.sourceId, p.handleId, kind, p.flowPos);
	}

	function onKeydown(e: KeyboardEvent) {
		// Consume Escape only when one of our layers is actually open, so the
		// page-level handler (which clears the selection / closes the drawer)
		// only fires on the NEXT press — one layer per keypress.
		if (e.key === 'Escape' && (addOpen || dropPicker || edgePanel || routerOwnerId)) {
			e.preventDefault();
			e.stopImmediatePropagation();
			addOpen = false;
			dropPicker = null;
			edgePanel = null;
			routerOwnerId = null;
		}
	}

	onMount(() => {
		const onDelete = (e: Event) => onDeleteStep?.((e as CustomEvent).detail.id);
		// "On error" on a step card: create the router (no steps yet) and open
		// its case editor so the user describes what each arm catches.
		const onErrorH = (e: Event) => {
			const id = (e as CustomEvent).detail.id as string;
			onAddErrorRouter?.(id);
			selectedId = '';
			routerOwnerId = id;
		};
		const onErrorRemove = (e: Event) => {
			const id = (e as CustomEvent).detail.id as string;
			onRemoveErrorRouter?.(id);
			if (routerOwnerId === id) routerOwnerId = null;
		};
		const onEdgeLabel = (e: Event) => {
			const id = (e as CustomEvent).detail.id as string;
			const edge = edges.find((x) => x.id === id);
			if (edge) onedgeclick({ edge });
		};
		const onCase = (e: Event) => onAddCase?.((e as CustomEvent).detail.id);
		const onBranch = (e: Event) => onAddParallelBranch?.((e as CustomEvent).detail.id);
		document.addEventListener('dia-canvas-delete', onDelete);
		document.addEventListener('dia-canvas-add-error-handler', onErrorH);
		document.addEventListener('dia-canvas-remove-error-router', onErrorRemove);
		document.addEventListener('dia-canvas-edge-label', onEdgeLabel);
		document.addEventListener('dia-canvas-add-case', onCase);
		document.addEventListener('dia-canvas-add-branch', onBranch);
		return () => {
			document.removeEventListener('dia-canvas-delete', onDelete);
			document.removeEventListener('dia-canvas-add-error-handler', onErrorH);
			document.removeEventListener('dia-canvas-remove-error-router', onErrorRemove);
				document.removeEventListener('dia-canvas-edge-label', onEdgeLabel);
		document.removeEventListener('dia-canvas-add-case', onCase);
			document.removeEventListener('dia-canvas-add-branch', onBranch);
		};
	});

	export async function tidyUp() {
		// Relayout from scratch, then glide every card from where it was to its
		// new spot; the edges re-derive each frame so the lines travel along.
		const from = new Map(nodes.map((n) => [n.id, { ...n.position }]));
		forceLayout = true;
		rebuild();
		const total = dur(360);
		if (total > 0 && from.size > 0) {
			const target = new Map(nodes.map((n) => [n.id, { ...n.position }]));
			const start = performance.now();
			const tick = (now: number) => {
				if (dragState.active) return; // a drag takes over — stop steering
				const k = cubicOut(Math.min(1, (now - start) / total));
				nodes = nodes.map((n) => {
					const f = from.get(n.id);
					const to = target.get(n.id);
					if (!f || !to) return n;
					return {
						...n,
						position: { x: f.x + (to.x - f.x) * k, y: f.y + (to.y - f.y) * k }
					};
				});
				edges = edges.map((e) => ({ ...e }));
				if (now - start < total) requestAnimationFrame(tick);
			};
			requestAnimationFrame(tick);
		}
		await Promise.resolve();
		fitView(FIT_OPTS);
	}
	export function fit() {
		fitView(FIT_OPTS);
	}
</script>

<svelte:window onkeydown={onKeydown} />

<div bind:this={containerEl} class="canvas-root relative h-full w-full" class:rf-dragging={dragging}>
	<SvelteFlow
		bind:nodes
		bind:edges
		{nodeTypes}
		{edgeTypes}
		fitView
		fitViewOptions={FIT_OPTS}
		nodesDraggable
		nodesConnectable
		elementsSelectable
		connectionRadius={32}
		{onnodeclick}
		{onpaneclick}
		{onedgeclick}
		{onnodedragstart}
		{onnodedrag}
		{onnodedragstop}
		{onconnectend}
		{onconnect}
		{isValidConnection}
		proOptions={{ hideAttribution: true }}
		colorMode="dark"
		minZoom={0.35}
		maxZoom={1.5}
	>
		<Background variant={BackgroundVariant.Dots} gap={28} size={0.9} />
		<Controls position="bottom-right" showLock={false} />

		<Panel position="top-left">
			<div class="flex items-center gap-1.5">
				<!-- Add step — the primary way in. -->
				<button
					type="button"
					onclick={() => (addOpen = !addOpen)}
					class="inline-flex h-8 items-center gap-1.5 rounded-lg bg-foreground px-3 text-[12px] font-medium text-background shadow-sm transition-opacity hover:opacity-90"
				>
					<Plus class="size-3.5" />
					Add step
				</button>

				<div
					class="flex items-center overflow-hidden rounded-lg border border-border/60 bg-card/90 shadow-sm backdrop-blur-md"
				>
					<button
						type="button"
						onclick={() => fit()}
						class="inline-flex h-8 items-center gap-1.5 px-2.5 text-[11.5px] font-medium text-muted-foreground transition-colors hover:bg-foreground/[0.06] hover:text-foreground"
						title="Center the flow in view"
					>
						<Maximize class="size-3.5" />
						Center
					</button>
					<div class="h-3.5 w-px bg-border/60"></div>
					<button
						type="button"
						onclick={() => tidyUp()}
						class="inline-flex h-8 items-center gap-1.5 px-2.5 text-[11.5px] font-medium text-muted-foreground transition-colors hover:bg-foreground/[0.06] hover:text-foreground"
						title="Re-run auto-layout"
					>
						<LayoutGrid class="size-3.5" />
						Tidy
					</button>
				</div>
			</div>
		</Panel>

		<!-- Legend: how the canvas works. Yields to the release dock. -->
		{#if showLegend}
		<Panel position="bottom-center">
			<div
				in:fade={{ duration: dur(160) }}
				out:fade={{ duration: dur(120) }}
				class="pointer-events-none hidden items-center gap-2 rounded-full border border-border/60 bg-card/90 px-3 py-1 font-mono text-[10px] text-muted-foreground/80 backdrop-blur-md md:flex"
			>
				<span>drag a dot → add a connected step</span>
				<span class="text-border">·</span>
				<span>click a line → edit / disconnect it</span>
				<span class="text-border">·</span>
				<span class="inline-flex items-center gap-1">
					<span class="size-1.5 rounded-full bg-destructive/80"></span>
					red dot → on-error switch
				</span>
			</div>
		</Panel>
		{/if}
	</SvelteFlow>

	<!-- Connection editor — click a line to edit when that path runs. -->
	{#if edgePanel}
		<EdgePanel
			info={edgePanel}
			onClose={() => (edgePanel = null)}
			onDeleteStep={(id) => onDeleteStep?.(id)}
			onTruncateChain={(id) => onTruncateChain?.(id)}
			onAbsorbAfter={(id, which) => onAbsorbAfter?.(id, which)}
			onDetach={(id) => onDetach?.(id)}
		/>
	{/if}

	<!-- On-error router editor — click the router card to describe its cases. -->
	{#if routerStep}
		<ErrorRouterPanel step={routerStep} onClose={() => (routerOwnerId = null)} />
	{/if}

	<!-- Dismiss layer + picker for handle-drops on empty canvas -->
	{#if addOpen || dropPicker}
		<!-- Above the StepDrawer (z-20) so a click anywhere light-dismisses. -->
		<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
		<div
			class="absolute inset-0 z-30"
			onclick={() => {
				if (performance.now() - pickerOpenedAt < 200) return;
				addOpen = false;
				dropPicker = null;
			}}
		></div>
	{/if}
	<!-- Add-step dropdown — rendered outside the xyflow Panel (which pins its
	     own low z-index stacking context) so it stacks above the dismiss layer.
	     Anchored under the Panel's top-left button (15px margin + 32px button). -->
	{#if addOpen}
		<div
			in:fly={{ y: -6, duration: dur(180), easing: cubicOut }}
			out:fly={{ y: -4, duration: dur(120), easing: cubicOut }}
			class="absolute left-[15px] top-[53px] z-40 overflow-hidden rounded-lg border border-border bg-popover shadow-[0_16px_40px_-12px_rgba(0,0,0,0.7)]"
		>
			<KindPicker onPick={pickFromMenu} />
		</div>
	{/if}
	{#if dropPicker}
		<div
			in:fly={{ y: -6, duration: dur(180), easing: cubicOut }}
			out:fly={{ y: -4, duration: dur(120), easing: cubicOut }}
			class="absolute z-40 overflow-hidden rounded-lg border border-border bg-popover shadow-[0_16px_40px_-12px_rgba(0,0,0,0.7)]"
			style="left: {dropPicker.left}px; top: {dropPicker.top}px;"
		>
			<div
				class="border-b border-border/60 px-3 py-1.5 font-mono text-[9.5px] font-medium uppercase tracking-[0.14em] text-muted-foreground"
			>
				Add a connected step
			</div>
			<KindPicker onPick={pickFromDrop} />
		</div>
	{/if}

	<!-- Empty flow: a quiet onboarding block over the canvas. -->
	{#if steps.length === 0 && !addOpen && !dropPicker}
		<div class="pointer-events-none absolute inset-0 z-10 grid place-items-center">
			<div
				in:fly={{ y: 8, duration: dur(220), easing: cubicOut }}
				out:fade={{ duration: dur(120) }}
				class="pointer-events-auto flex max-w-xs flex-col items-center rounded-xl border border-border bg-card/95 px-6 py-6 text-center shadow-[0_16px_48px_-16px_rgba(0,0,0,0.7)] backdrop-blur-sm"
			>
				<div
					class="mb-3 grid size-10 place-items-center rounded-lg border border-border bg-foreground/[0.05] text-muted-foreground"
				>
					<Workflow class="size-4" />
				</div>
				<p class="text-[13px] font-medium text-foreground">Build the flow</p>
				<p class="mt-1 text-[11.5px] leading-relaxed text-muted-foreground">
					Add the first step, then drag from a step's dots to branch, chain, and
					handle errors. Click any step to edit it.
				</p>
				<button
					type="button"
					class="mt-4 inline-flex h-8 items-center gap-1.5 rounded-lg bg-foreground px-3 text-[12px] font-medium text-background transition-opacity hover:opacity-90"
					onclick={() => (addOpen = true)}
				>
					<Plus class="size-3.5" />
					Add your first step
				</button>
			</div>
		</div>
	{/if}
</div>

<style>
	:global(.svelte-flow__node.selected) {
		outline: none;
	}
	:global(.svelte-flow__node) {
		transition:
			transform 280ms var(--canvas-ease),
			opacity 200ms ease;
	}
	:global(.svelte-flow__handle) {
		border-radius: 9999px;
		transition:
			background 140ms,
			box-shadow 140ms ease-out,
			opacity 220ms ease;
	}
	/* Left entry dots: the HANDLE div must never move (xyflow measures it
	   once at mount; displacing it re-anchors every line to a phantom spot).
	   The visible dot is a ::after pseudo that travels from the old
	   top-centre position to the left edge when a click path connects.
	   --dia-dot-dx is set per node width. */
	:global(.svelte-flow__handle.dia-left-dot) {
		background: transparent !important;
		border: none !important;
	}
	:global(.svelte-flow__handle.dia-left-dot)::after {
		content: '';
		position: absolute;
		inset: -1px;
		border-radius: 9999px;
		background: hsl(var(--muted-foreground) / 0.7);
		border: 2px solid hsl(var(--card));
	}
	:global(.svelte-flow__handle.dia-dot-in)::after {
		animation: dia-dot-move 360ms cubic-bezier(0.22, 1, 0.36, 1) both;
	}
	@keyframes -global-dia-dot-move {
		from {
			opacity: 0.4;
			translate: var(--dia-dot-dx, 124px) -22px;
		}
		to {
			opacity: 1;
			translate: 0 0;
		}
	}
	/* A generous invisible hit area — the visible dot stays small, but the
	   grab target is ~24px so connecting lines doesn't need pixel-hunting. */
	:global(.svelte-flow__handle::after) {
		content: '';
		position: absolute;
		inset: -8px;
		border-radius: 9999px;
	}
	/* Hover feedback via a ring, never transform — xyflow positions handles
	   with translate(), and overriding transform makes the dot jump corners. */
	:global(.svelte-flow__handle:hover) {
		background: hsl(var(--foreground)) !important;
		box-shadow:
			0 0 0 3px hsl(var(--background)),
			0 0 0 5px hsl(var(--foreground) / 0.35);
	}
	:global(.dia-edge) {
		transition:
			stroke 200ms ease,
			stroke-width 160ms ease,
			opacity 200ms ease;
	}
	:global(.svelte-flow__edge:hover .dia-edge-plain),
	:global(.svelte-flow__edge.selected .dia-edge-plain) {
		stroke: hsl(var(--foreground)) !important;
		stroke-width: 2.1 !important;
	}
	:global(.svelte-flow__edge:hover .dia-edge-branch),
	:global(.svelte-flow__edge.selected .dia-edge-branch) {
		stroke: hsl(var(--foreground)) !important;
		stroke-width: 2 !important;
	}
	:global(.svelte-flow__edge:hover .dia-edge-label) {
		color: hsl(var(--foreground)) !important;
		border-color: hsl(var(--border)) !important;
	}
	:global(.svelte-flow__connectionline) {
		stroke: hsl(var(--foreground) / 0.55) !important;
		stroke-width: 1.25;
		stroke-dasharray: 4 2;
		animation: connect-dash 600ms linear infinite;
	}
	:global(.svelte-flow) {
		background: hsl(var(--background));
	}
	:global(.svelte-flow__controls) {
		background: hsl(var(--card));
		border: 1px solid hsl(var(--border));
		border-radius: 0.375rem;
		box-shadow: none;
		overflow: hidden;
	}
	:global(.svelte-flow__controls-button) {
		background: hsl(var(--card));
		border-bottom: 1px solid hsl(var(--border));
		color: hsl(var(--muted-foreground));
		fill: currentColor;
		transition:
			background 120ms,
			color 120ms;
	}
	:global(.svelte-flow__controls-button:hover) {
		background: hsl(var(--secondary));
		color: hsl(var(--foreground));
	}
	:global(.svelte-flow__background) {
		background-color: hsl(var(--background));
	}
	:global(.svelte-flow__attribution) {
		display: none !important;
	}
</style>
