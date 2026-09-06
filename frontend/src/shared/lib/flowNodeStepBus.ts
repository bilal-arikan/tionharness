// A payload-carrying pub/sub for live per-node tool/thinking steps (flow_node_step),
// scoped per flow-run id. It is the per-flow-run step bus: the single SSE
// feed (useAppEvents) publishes each `flownodestep` frame here, and the run
// viewer's node inspector (RunNodeInspector) subscribes for the run it shows so a
// running agent node renders its steps the moment they happen — before the node
// finishes and its steps sidecar is written.

import type { TurnStep } from '@/types'

// One live step frame for a specific node within a run.
export interface FlowNodeStepFrame {
  nodeId: string
  step: TurnStep
}

type Listener = (frame: FlowNodeStepFrame) => void

// Per-run subscriber sets, keyed by flow run id so a viewer only hears frames
// for the run it currently shows.
const listeners = new Map<string, Set<Listener>>()

// publishFlowNodeStep delivers one live step frame to every subscriber watching
// `runId`. Called by useAppEvents for every `flownodestep` SSE frame.
export function publishFlowNodeStep(runId: string, frame: FlowNodeStepFrame): void {
  listeners.get(runId)?.forEach((l) => l(frame))
}

// subscribeFlowNodeStep registers `cb` for `runId`'s step frames. Returns an
// unsubscribe function; the per-run set is pruned when it empties.
export function subscribeFlowNodeStep(runId: string, cb: Listener): () => void {
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
