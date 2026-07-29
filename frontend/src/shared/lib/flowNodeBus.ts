// A payload-carrying pub/sub for live flow-node lifecycle frames (flow_node),
// scoped per flow-run id. It is the flow-run counterpart of stepBus.ts: the
// single SSE feed (useAppEvents) publishes each `flownode` frame here, and the
// run viewer (RunView) subscribes for the run it is showing so per-node
// start/done/error + output render the moment they happen — ahead of the
// periodic run-state poll, and for autonomous/scheduled runs too (which have no
// per-request SSE of their own).
//
// Two subscription scopes, fed by the same frame:
//   - subscribeFlowNode(runId)  — exactly one run (what RunView shows today).
//   - subscribeFlowTree(rootId) — a run AND every subflow/spawn descendant of it.
//
// The tree scope exists because children are born mid-run: a composed flow
// creates child runs as it advances, so a viewer cannot know their ids up front
// to subscribe per child. Keying on the (stable) tree root instead means frames
// from a child that did not exist at subscribe time still arrive.

import type { FlowNodeEvent } from '@/types'

type NodeListener = (ev: FlowNodeEvent) => void

// A tree subscriber also needs to know WHICH run in the tree a frame came from —
// otherwise a parent's viewer would paint a child's node ids onto its own graph.
// (The per-run scope needs no such tag: there the run id is the key.)
export interface FlowTreeFrame {
  runId: string
  // Where `runId` hangs in the tree: the run that launched it and the node in
  // that run's graph which did. This is what lets a parent's canvas roll a
  // child's progress up onto the subflow/spawn node that started it — resolving
  // it from the fetched run tree instead would show nothing for a child born
  // after that fetch. BOTH are needed: node ids are unique only within one graph,
  // so a node-only key can address two runs in the same tree. Absent when the
  // frame is the tree root's own.
  parentRunId?: string
  parentNodeId?: string
  ev: FlowNodeEvent
}

type TreeListener = (frame: FlowTreeFrame) => void

// Per-run subscriber sets, keyed by flow run id so a viewer only hears frames
// for the run it currently shows.
const listeners = new Map<string, Set<NodeListener>>()

// Per-tree subscriber sets, keyed by the ROOT run id of the tree.
const treeListeners = new Map<string, Set<TreeListener>>()

// publishFlowNode delivers one node lifecycle frame to every subscriber watching
// `runId`, plus every subscriber watching the tree rooted at `rootId`. Called by
// useAppEvents for every `flownode` SSE frame.
//
// rootId is optional so frames from a backend that predates tree tagging still
// reach their per-run subscriber; it then falls back to runId (a run with no
// recorded root is its own root). The lineage tags ride along to the tree scope
// only — the per-run scope is already keyed by the run itself.
export function publishFlowNode(
  runId: string,
  ev: FlowNodeEvent,
  rootId?: string,
  lineage?: { parentRunId?: string; parentNodeId?: string },
): void {
  listeners.get(runId)?.forEach((l) => l(ev))
  const root = rootId || runId
  const treeSet = treeListeners.get(root)
  if (!treeSet) return
  const frame: FlowTreeFrame = { runId, ...lineage, ev }
  treeSet.forEach((l) => l(frame))
}

// subscribeFlowNode registers `cb` for `runId`'s node frames. Returns an
// unsubscribe function; the per-run set is pruned when it empties.
export function subscribeFlowNode(runId: string, cb: NodeListener): () => void {
  let set = listeners.get(runId)
  if (!set) {
    set = new Set()
    listeners.set(runId, set)
  }
  set.add(cb)
  return () => {
    const s = listeners.get(runId)
    if (!s) return
    s.delete(cb)
    if (s.size === 0) listeners.delete(runId)
  }
}

// subscribeFlowTree registers `cb` for every frame in the tree rooted at
// `rootRunId` — the root run itself and all of its subflow/spawn descendants,
// including ones spawned after this call. Each frame is tagged with the run it
// came from. Returns an unsubscribe function; the per-tree set is pruned when it
// empties.
export function subscribeFlowTree(rootRunId: string, cb: TreeListener): () => void {
  let set = treeListeners.get(rootRunId)
  if (!set) {
    set = new Set()
    treeListeners.set(rootRunId, set)
  }
  set.add(cb)
  return () => {
    const s = treeListeners.get(rootRunId)
    if (!s) return
    s.delete(cb)
    if (s.size === 0) treeListeners.delete(rootRunId)
  }
}
