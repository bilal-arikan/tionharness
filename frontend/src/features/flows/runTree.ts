// Pure shaping of a composed run's tree for the run viewer: turning the flat
// FlowRun[] the /tree endpoint returns into indented rows, and folding live node
// frames into per-child progress the parent's canvas can show.
//
// Kept free of React and of the network so both are testable on their own — the
// ordering rules here mirror the backend's (db.ListFlowRunTree) and getting them
// wrong draws a wrong hierarchy, which is exactly the bug that already bit once.

import type { FlowRun, FlowNodeEvent, FlowState } from '@/types'

export interface RunTreeRow {
  run: FlowRun
  // 0 for the root, +1 per ancestor. Drives the row's indentation.
  depth: number
}

// buildRunTreeRows assigns each run its depth in the tree. The backend already
// returns members breadth-first (parent before children), so a single pass that
// reads each run's parent depth is enough — no sorting here, and deliberately so:
// re-deriving order on the client would be a second, divergable source of truth.
//
// A run whose parent is not in the list (a deleted parent row, or a member the
// backend appended as unreachable) is treated as depth 0 rather than dropped —
// showing it unindented beats hiding a run that exists.
export function buildRunTreeRows(runs: FlowRun[]): RunTreeRow[] {
  const depthOf = new Map<string, number>()
  return runs.map((run) => {
    const parentDepth = run.parentRunId ? depthOf.get(run.parentRunId) : undefined
    const depth = parentDepth === undefined ? 0 : parentDepth + 1
    depthOf.set(run.id, depth)
    return { run, depth }
  })
}

// ChildProgress is what a parent's subflow/spawn node shows while the run it
// launched is executing: which node of the CHILD is live and how it ended.
export interface ChildProgress {
  // The child node's own lifecycle phase, straight off its latest frame.
  phase: FlowNodeEvent['phase']
  // Title of the child node that frame came from — deliberately shown instead of
  // a "3/7" ratio: the denominator (the child flow's node count) is not a bound
  // on how many nodes actually run, because a loop re-enters nodes and a branch
  // skips them, so the ratio would be both wrong and able to exceed 1.
  title: string
  // 1-based execution index of that node within the child run. Doubles as the
  // "how far along" signal a ratio was meant to give, without implying a total.
  index: number
  // The child run the frame came from, so a click can descend into it.
  runId: string
}

// ChildProgressMap is keyed by PARENT run id, then by the node id within that
// run's graph. Two levels, not one: node ids are unique only inside a single flow
// graph, so a flat node-id key would let a grandchild's progress land on the
// root's identically-named node.
export type ChildProgressMap = Record<string, Record<string, ChildProgress>>

// applyChildFrame folds one live tree frame into the progress map. Returns the
// same map when the frame carries no parent linkage — the tree root's own frames
// (they belong to the canvas already being drawn), and children started by the
// run_flow tool outside any node (those exist in the tree but hang off no node,
// so there is nowhere to roll them up to; the tree panel still lists them).
//
// Later frames win: the map holds each child's CURRENT node, not its history.
export function applyChildFrame(
  prev: ChildProgressMap,
  frame: { runId: string; parentRunId?: string; parentNodeId?: string; ev: FlowNodeEvent },
): ChildProgressMap {
  if (!frame.parentRunId || !frame.parentNodeId) return prev
  return {
    ...prev,
    [frame.parentRunId]: {
      ...prev[frame.parentRunId],
      [frame.parentNodeId]: {
        phase: frame.ev.phase,
        title: frame.ev.title,
        index: frame.ev.index,
        runId: frame.runId,
      },
    },
  }
}

// RUN_PHASE maps a persisted run status onto the lifecycle phase a live frame
// would have carried. "running" becomes "start" because that is the phase the
// badge renders as in-flight.
const RUN_PHASE: Record<FlowRun['status'], FlowNodeEvent['phase']> = {
  running: 'start',
  waiting: 'waiting',
  success: 'done',
  failure: 'error',
}

// childProgressFromTree seeds the progress map from the PERSISTED tree, so a
// composed run that finished before the viewer opened still shows a rollup badge
// and can still be descended into.
//
// This is not an optimisation — without it the feature only works while watching
// a run live, and the run list is normally opened after the fact. Live frames are
// layered on top (see RunTreeView) and win, because they are newer.
//
// The child's last executed node comes from its own persisted trace; a child with
// no trace yet (just created) still yields an entry, because the entry's real job
// is to record WHICH run hangs off the node — that is what descending needs.
export function childProgressFromTree(runs: FlowRun[]): ChildProgressMap {
  const out: ChildProgressMap = {}
  for (const run of runs) {
    if (!run.parentRunId || !run.parentNodeId) continue
    let title = ''
    let index = 0
    try {
      const st = run.state ? (JSON.parse(run.state) as FlowState) : null
      const trace = st?.trace ?? []
      const lastEntry = trace[trace.length - 1]
      if (lastEntry) {
        title = lastEntry.title || lastEntry.nodeId
        index = trace.length
      }
    } catch {
      // A corrupt/absent state must not take the whole tree down: the run id and
      // status alone are still worth showing.
    }
    out[run.parentRunId] = {
      ...out[run.parentRunId],
      [run.parentNodeId]: { phase: RUN_PHASE[run.status] ?? 'start', title, index, runId: run.id },
    }
  }
  return out
}

// mergeChildProgress layers live frames over the tree-seeded map. Live wins per
// (parent run, node): it reflects the child's CURRENT node, while the seed is
// only as fresh as the last tree fetch.
export function mergeChildProgress(seed: ChildProgressMap, live: ChildProgressMap): ChildProgressMap {
  const out: ChildProgressMap = { ...seed }
  for (const [runID, nodes] of Object.entries(live)) {
    out[runID] = { ...out[runID], ...nodes }
  }
  return out
}

// runTreeBreadcrumb is the ancestor chain from the root down to `runId`, used as
// the "Ana akış › Kod incelemesi › Test" trail when the viewer has descended into
// a child. Returns [] when the run is not in the list.
//
// Guards against a parent cycle (a corrupt row pointing at its own descendant) by
// refusing to visit a run twice — an infinite trail would hang the render.
export function runTreeBreadcrumb(runs: FlowRun[], runId: string): FlowRun[] {
  const byID = new Map(runs.map((r) => [r.id, r]))
  const chain: FlowRun[] = []
  const seen = new Set<string>()
  let cur = byID.get(runId)
  while (cur && !seen.has(cur.id)) {
    seen.add(cur.id)
    chain.unshift(cur)
    cur = cur.parentRunId ? byID.get(cur.parentRunId) : undefined
  }
  return chain
}
