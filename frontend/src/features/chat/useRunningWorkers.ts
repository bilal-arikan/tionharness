// useRunningWorkers keeps a live snapshot of a coordinator session's workers for
// the chat screen, so the transcript can say "this conversation is waiting on N
// workers" instead of looking finished (see _Docs/47).
//
// Polling policy — the workers endpoint is cheap but not free, so it is only hit
// while something can actually change: the coordinator turn is streaming (a
// spawn_worker call may land mid-turn) or at least one worker is still running.
// An idle coordinator with no workers costs one fetch per session open.
import { useCallback, useEffect, useState } from 'react'
import { api } from '@/api'
import type { WorkerInfo } from '@/types'

const POLL_MS = 3000

export function useRunningWorkers(
  sessionId: string | null,
  enabled: boolean,
  // True while the coordinator's own turn is streaming — a worker may be spawned
  // at any tool step, so poll through it.
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

  // Refetch on mount/session change and on every streaming edge (turn start =
  // workers may appear, turn end = the running set is authoritative again).
  useEffect(() => {
    load()
  }, [load, streaming])

  const anyRunning = workers.some((w) => w.running)
  useEffect(() => {
    if (!enabled || !sessionId) return
    if (!streaming && !anyRunning) return
    const t = setInterval(load, POLL_MS)
    return () => clearInterval(t)
  }, [enabled, sessionId, streaming, anyRunning, load])

  return workers
}
