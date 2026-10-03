// Pub/sub for live flow-node frames (SSE `flownode`), scoped per flow id: the
// single SSE feed publishes each frame here; the Flows screen subscribes for
// the flow it shows so node cards light up as a turn runs through them.
import type { FlowNodeFrame } from '@/types'

type Listener = (frame: FlowNodeFrame) => void

const byFlow = new Map<string, Set<Listener>>()

export function publishFlowNode(frame: FlowNodeFrame): void {
  byFlow.get(frame.flowId)?.forEach((cb) => cb(frame))
  byFlow.get('*')?.forEach((cb) => cb(frame))
}

// subscribeFlowNode registers for one flow's frames ('*' = every flow).
export function subscribeFlowNode(flowId: string, cb: Listener): () => void {
  let set = byFlow.get(flowId)
  if (!set) {
    set = new Set()
    byFlow.set(flowId, set)
  }
  set.add(cb)
  return () => {
    set!.delete(cb)
    if (set!.size === 0) byFlow.delete(flowId)
  }
}
