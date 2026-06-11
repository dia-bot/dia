// Tree ↔ graph adapter for the canvas-based custom-command editor.
//
// Our backend model is a NESTED TREE of Step objects (Step.then/else/cases/
// default/on_error). The canvas wants a flat list of nodes + edges. This file
// flattens the tree into node + edge arrays, derives stable IDs, labels each
// branch edge ("then" / "else" / "case 1" / "default" / "body" / "branch 1" /
// "on error"), and runs dagre to compute (x, y) positions.
//
// The tree stays the source of truth: edits go through tree mutations (in
// StepInspector / palette) and the canvas re-derives nodes+edges. We don't
// translate edge changes back to tree mutations in v1 — drag-to-connect
// instead creates *new* steps via the +-picker that opens on drop.

import dagre from '@dagrejs/dagre';
import type { Step } from '$lib/commands/types';
import { STEP_CATEGORIES, STEP_KIND_BY_KIND } from '$lib/commands/types';

// XY type aliases (independent of svelte-flow at file level — adapter is
// usable in tests without pulling the canvas in).
export type FlowNode = {
	id: string;
	type: string;
	position: { x: number; y: number };
	data: NodeData;
	draggable?: boolean;
	selectable?: boolean;
};

export type FlowEdge = {
	id: string;
	source: string;
	target: string;
	sourceHandle?: string;
	targetHandle?: string;
	type: string;
	label?: string;
	data?: EdgeData;
	markerEnd?: string;
	animated?: boolean;
};

export type NodeData = {
	step?: Step; // omitted for synthetic entry / end nodes
	kind: string; // step kind ('reply', 'if', etc.) OR 'entry' | 'end'
	stepPath: string; // dot-path inside the Definition (e.g. "steps.0.then.1")
	parentStepId?: string;
	branchLabel?: string;
	// Display-only metadata cached for the node component.
	commandName?: string; // entry node only
	isStart?: boolean; // true for the first step at the top of the root branch
	category?: string; // step category label ("Reply", "Discord", "Flow", etc.)
	endsHere?: boolean; // step.kind is 'exit' or 'fail' (terminates this branch)
	ownerId?: string; // error-router nodes: the step the handlers belong to
};

export type Slot =
	| { kind: 'after' }
	| { kind: 'then' }
	| { kind: 'else' }
	| { kind: 'body' }
	| { kind: 'default' }
	| { kind: 'case'; index: number }
	| { kind: 'branch'; index: number }
	| { kind: 'on_error' }
	| { kind: 'on_error_case'; index: number };

/**
 * slotsForStep returns the legal insertion slots on a target step. Used by the
 * canvas's drag-onto-node menu to enumerate where a dragged step can be
 * dropped.
 */
export function slotsForStep(step: Step): Slot[] {
	const slots: Slot[] = [{ kind: 'after' }];
	if (step.kind === 'if') {
		slots.push({ kind: 'then' }, { kind: 'else' });
	} else if (step.kind === 'switch') {
		(step.cases ?? []).forEach((_, i) => slots.push({ kind: 'case', index: i }));
		slots.push({ kind: 'default' });
	} else if (step.kind === 'loop') {
		slots.push({ kind: 'body' });
	} else if (step.kind === 'parallel') {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const branches = ((step.spec ?? {}) as any).branches ?? [];
		branches.forEach((_: unknown, i: number) => slots.push({ kind: 'branch', index: i }));
	}
	slots.push({ kind: 'on_error' });
	(step.on_error_cases ?? []).forEach((_, i) =>
		slots.push({ kind: 'on_error_case', index: i })
	);
	return slots;
}

export type EdgeData = {
	branch?: BranchKind; // when the edge represents a branch into a sub-flow
	// The step the branch belongs to + which arm — lets the canvas open a
	// connection editor when the line is clicked (edit a case's match value,
	// an error arm's patterns, the if condition…).
	sourceId?: string;
	caseIndex?: number;
	// True for the short step → error-router rail (removing it removes ALL
	// error handling on the step, not one arm).
	rail?: boolean;
	// Click paths hide their wait_for plumbing: the wait step (and, for
	// routed multi-button paths, the switch) ride on the edge so the line
	// panel can edit them (who can click, timeout, remove the path).
	clickWait?: Step;
	clickSwitch?: Step;
	sourceStepPath?: string;
	targetStepPath?: string;
};

export type BranchKind =
	| 'then'
	| 'else'
	| 'case'
	| 'default'
	| 'body'
	| 'parallel'
	| 'on_error'
	| 'click'
	| 'after';

// What the connection editor needs to know about a clicked line.
export type EdgeInfo = {
	branch?: BranchKind;
	caseIndex?: number;
	rail?: boolean;
	// For click paths: the hidden wait_for (and routing switch) the line
	// represents.
	clickWait?: Step;
	clickSwitch?: Step;
	sourceStep: Step | null;
	targetId: string;
};

// Public ID of the synthetic entry node (the /command pill).
export const ENTRY_ID = '__entry__';

// Node sizes (matches the visual sizing in the node components; used for
// dagre's box reservation so edges land in the right place).
export const NODE_W = 240;
export const NODE_H = 64;
export const BRANCH_W = 230;
export const BRANCH_H = 60;
export const PILL_W = 150;
export const PILL_H = 36;

function isBranching(kind: string): boolean {
	return kind === 'if' || kind === 'switch' || kind === 'loop' || kind === 'parallel';
}

// componentClickSuffix: when `next` is a wait_for parked on a component click
// whose custom_id suffix belongs to one of `prev`'s buttons/selects, the edge
// between them leaves FROM that component's own dot — the "on click" line.
function componentClickSuffix(prev: Step | null, next: Step): string | null {
	if (!prev || next.kind !== 'wait_for') return null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const ns = (next.spec ?? {}) as any;
	if ((ns.trigger ?? 'component') !== 'component' || !ns.custom_id_suffix) return null;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const rows = ((prev.spec ?? {}) as any).components ?? [];
	for (const row of rows) {
		for (const c of row.components ?? []) {
			// Manually-configured ids are for hand-built routing / future
			// automations: no visual click path claims them.
			if (c.custom_id_manual) continue;
			if (c.custom_id_suffix === ns.custom_id_suffix) return ns.custom_id_suffix as string;
		}
	}
	return null;
}

// Hidden click-router cluster: ONE wait_for (no suffix = any button) plus a
// switch on the clicked id, placed right after a message with buttons. The
// canvas fuses the pair into per-button "on click" lines.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
function isHiddenClickWait(s: Step): boolean {
	if (s.kind !== 'wait_for') return false;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const sp = (s.spec ?? {}) as any;
	return !sp.custom_id_suffix && (sp.trigger ?? 'component') === 'component' && !!sp.into;
}
function isClickRouterSwitch(sw: Step | undefined, wait: Step): boolean {
	if (!sw || sw.kind !== 'switch') return false;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const into = ((wait.spec ?? {}) as any).into;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	return ((sw.spec ?? {}) as any).on?.src === `{{ .Vars.${into}.id }}`;
}
function buttonSuffixes(step: Step | null): Set<string> {
	const out = new Set<string>();
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const rows = (((step?.spec ?? {}) as any).components ?? []) as { components: any[] }[];
	for (const row of rows) {
		for (const c of row.components ?? []) {
			if (!c.custom_id_manual && c.custom_id_suffix) out.add(c.custom_id_suffix as string);
		}
	}
	return out;
}
function noHandlerExtras(s: Step): boolean {
	return (
		s.on_error === undefined &&
		(s.on_error_cases?.length ?? 0) === 0 &&
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		((((s.spec ?? {}) as any).on_timeout ?? []).length ?? 0) === 0
	);
}

// errorRouterId derives the synthetic on-error router node's id for a step.
// The router renders as a small switch-style card: one outgoing arm per typed
// error case plus an "else" arm for the default handler.
export function errorRouterId(stepId: string): string {
	return `err:${stepId}`;
}
export function errorRouterOwner(nodeId: string): string | null {
	return nodeId.startsWith('err:') ? nodeId.slice(4) : null;
}

function joinPath(...parts: (string | number)[]): string {
	return parts.filter((p) => p !== '' && p !== undefined && p !== null).join('.');
}

// treeToGraph walks the Definition.steps tree and returns the nodes + edges
// that represent it. The graph is rooted at a synthetic ENTRY node (the
// `/commandname` pill). Chains simply stop at their last step — no synthetic
// end caps; an empty branch is just an unconnected handle to drag from.
export function treeToGraph(
	steps: Step[],
	commandName: string,
	scratch: Step[][] = []
): { nodes: FlowNode[]; edges: FlowEdge[] } {
	const nodes: FlowNode[] = [];
	const edges: FlowEdge[] = [];

	nodes.push({
		id: ENTRY_ID,
		type: 'entry',
		position: { x: 0, y: 0 },
		data: { kind: 'entry', stepPath: '', commandName },
		draggable: false
	});

	// walkBranch lays out one sequential chain. parentId/parentHandle is the
	// upstream attachment point (entry, a step's branch handle, or the previous
	// sibling). branchLabel labels the FIRST edge into this branch.
	function walkBranch(
		branchSteps: Step[],
		path: string,
		upstream: {
			id: string;
			handle?: string;
			label?: string;
			branch?: BranchKind;
			caseIndex?: number;
			// The step the branch belongs to (differs from `id` when the line
			// leaves a synthetic node, e.g. an error router).
			ownerId?: string;
			// Click arms: the hidden listener + routing switch they belong to.
			clickWait?: Step;
			clickSwitch?: Step;
		}
	): { tailId: string | null } {
		let prevId = upstream.id;
		let prevHandle = upstream.handle;
		let prevStep: Step | null = null;
		let pendingLabel: string | undefined = upstream.label;
		let pendingBranch: BranchKind | undefined = upstream.branch;
		let pendingCaseIndex: number | undefined = upstream.caseIndex;
		let pendingClickPair: { wait: Step; sw?: Step } | null =
			upstream.branch === 'click' && upstream.clickWait
				? { wait: upstream.clickWait, sw: upstream.clickSwitch }
				: null;

		if (branchSteps.length === 0) {
			// Empty branch: nothing to draw — the branch handle on the node is
			// the affordance (drag from it, or drop-pick, to start the path).
			return { tailId: null };
		}

		let pendingClick: { sfx: string; wait: Step } | null = null;
		for (let i = 0; i < branchSteps.length; i++) {
			const step = branchSteps[i];
			const stepPath = joinPath(path, i);

			// Click-router cluster: ONE listener (any button) + a switch on the
			// clicked id, sitting right after a message with buttons. Hide the
			// pair entirely: each case renders as its own "on click" line from
			// that button's dot, and the message's bottom dot still carries
			// "what happens after".
			if (!pendingBranch && !pendingClick && isHiddenClickWait(step) && i + 1 < branchSteps.length) {
				const sw = branchSteps[i + 1];
				const suffixes = buttonSuffixes(prevStep);
				if (
					isClickRouterSwitch(sw, step) &&
					suffixes.size > 0 &&
					noHandlerExtras(step) &&
					noHandlerExtras(sw) &&
					(sw.default ?? []).length === 0 &&
					(sw.cases ?? []).length > 0 &&
					(sw.cases ?? []).every((c) => suffixes.has(c.when?.src ?? ''))
				) {
					const swPath = joinPath(path, i + 1);
					(sw.cases ?? []).forEach((c, ci) => {
						walkBranch(c.do, joinPath(swPath, 'cases', ci, 'do'), {
							id: prevId,
							handle: `component-${c.when?.src ?? ''}`,
							label: 'on click',
							branch: 'click',
							caseIndex: ci,
							clickWait: step,
							clickSwitch: sw
						});
					});
					i++; // the switch is consumed too
					continue; // prev stays the message: its out dot = "after"
				}
			}

			// A plain click-wait followed by its action is pure plumbing: hide
			// the wait node and let the action connect straight from the
			// button's dot. The wait rides on the edge (click the line to
			// edit who can click / the timeout). Waits with error handling or
			// timeout branches stay visible — they're doing more than routing.
			if (!pendingBranch && !pendingClick) {
				const sfx = componentClickSuffix(prevStep, step);
				const hasExtras =
					step.on_error !== undefined ||
					(step.on_error_cases?.length ?? 0) > 0 ||
					// eslint-disable-next-line @typescript-eslint/no-explicit-any
					((((step.spec ?? {}) as any).on_timeout ?? []).length ?? 0) > 0;
				if (sfx && i + 1 < branchSteps.length && !hasExtras) {
					pendingClick = { sfx, wait: step };
					continue;
				}
			}
			const nodeType = nodeTypeFor(step.kind);
			const meta = STEP_KIND_BY_KIND.get(step.kind);
			const category =
				STEP_CATEGORIES.find((c) => c.id === meta?.category)?.label ?? '';

			nodes.push({
				id: step.id,
				type: nodeType,
				position: { x: 0, y: 0 },
				data: {
					step,
					kind: step.kind,
					stepPath,
					isStart: path === 'steps' && i === 0,
					category,
					endsHere: step.kind === 'exit' || step.kind === 'fail'
				}
			});

			// A wait_for parked on the previous step's button leaves from THAT
			// button's own dot — drawn as an "on click" line.
			let edgeHandle = prevHandle;
			let edgeLabel = pendingLabel;
			let edgeType =
				pendingBranch === 'on_error' ? 'error' : pendingLabel ? 'branch' : 'plain';
			let clickWait: Step | undefined;
			let clickSwitch: Step | undefined;
			if (pendingClickPair) {
				clickWait = pendingClickPair.wait;
				clickSwitch = pendingClickPair.sw;
				pendingClickPair = null;
			}
			if (pendingClick) {
				edgeHandle = `component-${pendingClick.sfx}`;
				edgeLabel = 'on click';
				edgeType = 'branch';
				clickWait = pendingClick.wait;
				pendingClick = null;
			} else if (!pendingBranch) {
				const sfx = componentClickSuffix(prevStep, step);
				if (sfx) {
					edgeHandle = `component-${sfx}`;
					edgeLabel = 'on click';
					edgeType = 'branch';
				}
			}

			if (prevId !== '') {
				edges.push({
					id: `${prevId}__${edgeHandle ?? 'out'}__${step.id}`,
					source: prevId,
					sourceHandle: edgeHandle,
					target: step.id,
					// Click paths run left-to-right: enter the action card from
					// its left edge so the line and label never hide behind it.
					targetHandle: clickWait ? 'in-left' : undefined,
					type: edgeType,
					label: edgeLabel,
					data: {
						branch: clickWait ? 'click' : pendingBranch,
						clickWait,
						clickSwitch,
						sourceId:
							pendingBranch && prevId !== ENTRY_ID
								? (upstream.ownerId ?? upstream.id)
								: undefined,
						caseIndex: pendingCaseIndex,
						sourceStepPath: prevId === ENTRY_ID ? '' : undefined,
						targetStepPath: stepPath
					}
				});
			}

			// Recurse into branches. Each branch produces an independent chain
			// that DOES NOT rejoin the parent visually — the parent's "after"
			// edge to the next sibling carries control flow.
			if (step.kind === 'if') {
				walkBranch(step.then ?? [], joinPath(stepPath, 'then'), {
					id: step.id,
					handle: 'then',
					label: 'then',
					branch: 'then'
				});
				walkBranch(step.else ?? [], joinPath(stepPath, 'else'), {
					id: step.id,
					handle: 'else',
					label: 'else',
					branch: 'else'
				});
			} else if (step.kind === 'switch') {
				(step.cases ?? []).forEach((c, ci) => {
					// The match value rides on the line — click it to edit.
					const src = c.when?.src ?? '';
					const lbl = src
						? `= ${src.length > 18 ? src.slice(0, 17) + '…' : src}`
						: `case ${ci + 1}`;
					walkBranch(c.do, joinPath(stepPath, 'cases', ci, 'do'), {
						id: step.id,
						handle: `case-${ci}`,
						label: lbl,
						branch: 'case',
						caseIndex: ci
					});
				});
				walkBranch(step.default ?? [], joinPath(stepPath, 'default'), {
					id: step.id,
					handle: 'default',
					label: 'default',
					branch: 'default'
				});
			} else if (step.kind === 'loop') {
				walkBranch(step.then ?? [], joinPath(stepPath, 'then'), {
					id: step.id,
					handle: 'body',
					label: 'body',
					branch: 'body'
				});
			} else if (step.kind === 'parallel') {
				// eslint-disable-next-line @typescript-eslint/no-explicit-any
				const branches = ((step.spec ?? {}) as any).branches ?? [];
				branches.forEach((b: Step[], bi: number) => {
					walkBranch(b, joinPath(stepPath, 'spec', 'branches', bi), {
						id: step.id,
						handle: `branch-${bi}`,
						label: `branch ${bi + 1}`,
						branch: 'parallel',
						caseIndex: bi
					});
				});
			}

			// Error handling renders as a switch-style ROUTER node hanging off
			// the step's red dot: one outgoing arm per typed error case (its
			// `when` patterns ride on the line) plus an "else" arm for the
			// default handler. Click the router to describe the cases.
			const errorCases = step.on_error_cases ?? [];
			if (errorCases.length > 0 || step.on_error !== undefined) {
				const rid = errorRouterId(step.id);
				nodes.push({
					id: rid,
					type: 'error_router',
					position: { x: 0, y: 0 },
					data: { step, kind: 'error_router', stepPath, ownerId: step.id }
				});
				edges.push({
					id: `${step.id}__on_error__${rid}`,
					source: step.id,
					sourceHandle: 'on_error',
					target: rid,
					type: 'error',
					label: 'on error',
					data: { branch: 'on_error', sourceId: step.id, rail: true }
				});
				errorCases.forEach((ec, ei) => {
					walkBranch(ec.do ?? [], joinPath(stepPath, 'on_error_cases', ei, 'do'), {
						id: rid,
						handle: `arm-${ei}`,
						label: (ec.when ?? []).join(' | ') || '*',
						branch: 'on_error',
						caseIndex: ei,
						ownerId: step.id
					});
				});
				if (step.on_error !== undefined) {
					walkBranch(step.on_error, joinPath(stepPath, 'on_error'), {
						id: rid,
						handle: 'default',
						label: errorCases.length ? 'else' : undefined,
						branch: 'on_error',
						ownerId: step.id
					});
				}
			}

			// Next sibling continuation — plain edge, no label.
			prevId = step.id;
			prevHandle = 'out';
			prevStep = step;
			pendingLabel = undefined;
			pendingBranch = undefined;
			pendingCaseIndex = undefined;
		}

		// No end cap — the chain just stops at its last step.
		return { tailId: prevId };
	}

	walkBranch(steps, 'steps', { id: ENTRY_ID, handle: 'out' });

	// Disconnected islands: each scratch chain renders with NO incoming edge.
	// The head's free top dot is the reattach target (drag any dot onto it).
	scratch.forEach((chain, si) => {
		walkBranch(chain, `scratch.${si}`, { id: '' });
	});

	// Branching nodes only show their "after" handle when something actually
	// chains off it (legacy flows) — fresh ifs/switches expose just their arms.
	const afterSources = new Set(
		edges.filter((e) => e.sourceHandle === 'out').map((e) => e.source)
	);
	for (const n of nodes) {
		if ((n.type === 'if' || n.type === 'switch') && afterSources.has(n.id)) {
			(n.data as Record<string, unknown>).hasAfter = true;
		}
	}

	return { nodes, edges };
}

function nodeTypeFor(kind: string): string {
	if (kind === 'if') return 'if';
	if (kind === 'switch') return 'switch';
	if (kind === 'loop') return 'loop';
	if (kind === 'parallel') return 'parallel';
	return 'step';
}

// applyLayout uses dagre to compute (x, y) for every node and returns a new
// array. The main top-to-bottom chain is weighted heavily so it stays a
// straight spine; branch arms fan out beside it and error rails barely tug.
export function applyLayout(
	nodes: FlowNode[],
	edges: FlowEdge[],
	opts: { rankdir?: 'TB' | 'LR' } = {}
): FlowNode[] {
	const g = new dagre.graphlib.Graph();
	g.setDefaultEdgeLabel(() => ({}));
	g.setGraph({
		rankdir: opts.rankdir ?? 'TB',
		nodesep: 120,
		ranksep: 90,
		marginx: 40,
		marginy: 40,
		edgesep: 90
	});
	for (const n of nodes) {
		const dims = sizeForNode(n);
		g.setNode(n.id, { width: dims.w, height: dims.h });
	}
	for (const e of edges) {
		const weight = e.type === 'error' ? 1 : e.type === 'branch' ? 2 : 8;
		g.setEdge(e.source, e.target, { weight });
	}
	dagre.layout(g);

	return nodes.map((n) => {
		const p = g.node(n.id);
		if (!p) return n;
		return { ...n, position: { x: p.x - p.width / 2, y: p.y - p.height / 2 } };
	});
}

function sizeForNode(n: FlowNode): { w: number; h: number } {
	if (n.type === 'entry') return { w: PILL_W, h: PILL_H };
	if (n.type === 'error_router') {
		const arms = (n.data.step?.on_error_cases?.length ?? 0) + 1;
		return { w: 200, h: 40 + arms * 28 };
	}
	if (isBranching(n.data.kind)) return { w: BRANCH_W, h: BRANCH_H };
	// Message cards grow with their button rows (one line each) and the
	// on-error footer; undercounting makes dagre overlap the next card.
	let h = NODE_H + 26; // body + footer
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const rows = (((n.data.step?.spec ?? {}) as any).components ?? []) as {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		components: any[];
	}[];
	const count = rows.reduce((acc, r) => acc + (r.components?.length ?? 0), 0);
	if (count > 0) h += 22 + count * 28;
	return { w: NODE_W, h };
}

// findStepByPath returns the step at the given dot-path inside a Definition's
// steps array, or null. Used to translate canvas drop-target paths to tree
// mutations.
export function findStepByPath(steps: Step[], path: string): Step | null {
	if (!path.startsWith('steps')) return null;
	const parts = path.split('.').slice(1); // drop "steps"
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	let cursor: any = steps;
	for (let i = 0; i < parts.length; i++) {
		const p = parts[i];
		if (cursor == null) return null;
		const asNum = Number(p);
		if (!Number.isNaN(asNum)) {
			cursor = cursor[asNum];
		} else {
			cursor = cursor[p];
		}
	}
	return (cursor as Step) ?? null;
}
