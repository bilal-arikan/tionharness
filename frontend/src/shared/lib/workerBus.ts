// A pub/sub for coordinator worker state changes, scoped per COORDINATOR session
// id. The single SSE feed (useAppEvents) publishes every `worker` frame here —
// both the phase="start" and the completion variants — and the chat's
// running-worker banner (useRunningWorkers) subscribes for the coordinator it
// shows, so the roster refetches the moment a worker starts or finishes instead of
// being discovered by a timer. Sibling of flowNodeStepBus.

type Listener = () => void

// Per-coordinator subscriber sets, so a banner only hears about its own workers.
const listeners = new Map<string, Set<Listener>>()

// publishWorkerChange notifies every subscriber watching `coordinatorSessionId`.
// Called by useAppEvents for each `worker` SSE event that carries a coordinatorId.
export function publishWorkerChange(coordinatorSessionId: string): void {
  listeners.get(coordinatorSessionId)?.forEach((l) => l())
}

// publishWorkerChangeAll notifies EVERY subscriber regardless of coordinator. Used
// when we know the roster may have changed but not for which coordinator — after an
// SSE reconnect, where the transitions that happened while the feed was down are
// lost and cannot be attributed.
export function publishWorkerChangeAll(): void {
  listeners.forEach((set) => set.forEach((l) => l()))
}

// subscribeWorkerChange registers `cb` for one coordinator's worker transitions.
// Returns an unsubscribe function; the per-coordinator set is pruned when empty.
export function subscribeWorkerChange(coordinatorSessionId: string, cb: Listener): () => void {
  let set = listeners.get(coordinatorSessionId)
  if (!set) {
    set = new Set()
    listeners.set(coordinatorSessionId, set)
  }
  set.add(cb)
  return () => {
    const s = listeners.get(coordinatorSessionId)
    if (!s) return
    s.delete(cb)
    if (s.size === 0) listeners.delete(coordinatorSessionId)
  }
}
