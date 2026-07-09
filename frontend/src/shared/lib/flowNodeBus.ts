// A payload-carrying pub/sub for live flow-node lifecycle frames (flow_node),
// scoped per flow-run id. It is the flow-run counterpart of stepBus.ts: the
// single SSE feed (useAppEvents) publishes each `flownode` frame here, and the
// run viewer (RunView) subscribes for the run it is showing so per-node
// start/done/error + output render the moment they happen — ahead of the
// periodic run-state poll, and for autonomous/scheduled runs too (which have no
// per-request SSE of their own).

import type { FlowNodeEvent } from '@/types'

type NodeListener = (ev: FlowNodeEvent) => void

// Per-run subscriber sets, keyed by flow run id so a viewer only hears frames
// for the run it currently shows.
const listeners = new Map<string, Set<NodeListener>>()

// publishFlowNode delivers one node lifecycle frame to every subscriber watching
// `runId`. Called by useAppEvents for every `flownode` SSE frame.
export function publishFlowNode(runId: string, ev: FlowNodeEvent): void {
  listeners.get(runId)?.forEach((l) => l(ev))
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
