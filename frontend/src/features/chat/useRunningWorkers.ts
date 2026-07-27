// useRunningWorkers keeps a live snapshot of a coordinator session's workers for
// the chat screen, so the transcript can say "this conversation is waiting on N
// workers" instead of looking finished (see _Docs/47).
//
// Event-driven, not polled: the backend publishes a `worker` SSE event on every
// transition (phase='start' when a worker turn begins, plus the completed/failed/
// killed event), useAppEvents fans those onto workerBus keyed by coordinator id,
// and this hook refetches the roster on each one. So the banner appears the moment
// a worker spawns and clears the moment the last one reports, with zero traffic
// while nothing is happening.
import { useCallback, useEffect, useState } from 'react'
import { api } from '@/api'
import { subscribeWorkerChange } from '@/shared/lib/workerBus'
import type { WorkerInfo } from '@/types'

export function useRunningWorkers(
  sessionId: string | null,
  enabled: boolean,
  // True while the coordinator's own turn is streaming. Used only as a coarse
  // refetch edge (turn start/end), since a worker's own transitions arrive over
  // the bus.
  streaming: boolean,
): WorkerInfo[] {
  const [workers, setWorkers] = useState<WorkerInfo[]>([])

  const load = useCallback(() => {
    if (!enabled || !sessionId) return
    api
      .listWorkers(sessionId)
      .then((d) => setWorkers(d.workers ?? []))
      .catch(() => setWorkers([]))
  }, [enabled, sessionId])

  // Drop the previous session's roster immediately on switch — a stale banner
  // from another coordinator would be actively misleading.
  useEffect(() => {
    setWorkers([])
  }, [sessionId])

  // Refetch on mount/session change and on every streaming edge. The streaming
  // edge also covers the case where this window missed a worker event (e.g. it
  // was opened after the workers had already started).
  useEffect(() => {
    load()
  }, [load, streaming])

  // Live worker transitions for THIS coordinator.
  useEffect(() => {
    if (!enabled || !sessionId) return
    return subscribeWorkerChange(sessionId, load)
  }, [enabled, sessionId, load])

  return workers
}
