import { useEffect, useMemo } from 'react'
import { api } from '@/api'
import type { ExecutionRuntimeRow } from '@/types'
import { useAsync } from '@/shared/hooks/useAsync'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { SIGNAL_EXECUTIONS } from './eventToRefreshSignals'

const POLL_MS = 20000
// Run-lifecycle events arrive in bursts (a coordinator fanning out workers
// fires one per spawn); the signal-driven refetch waits for the burst to
// settle instead of issuing one request per event.
const SIGNAL_SETTLE_MS = 400

export interface ExecutionRuntime {
  running: boolean
  lastStatus: string
  coordinatorSessionId?: string
  rootCoordinatorSessionId?: string
}

export interface ExecutionRuntimeState {
  // sessionId → live/last-run facts. Sessions absent from the map are idle.
  runtimeById: Map<string, ExecutionRuntime>
}

// Polls the compact runtime feed (GET /api/executions/runtime) rather than the
// full executions list: the sidebar only needs running/lastStatus/lineage per
// session, and the full feed carried every session in the workspace with its
// title and agent name on every poll and every run-lifecycle event.
export function useExecutionRuntime(activeWorkspaceId: string | null): ExecutionRuntimeState {
  const { data, refresh } = useAsync(
    () =>
      activeWorkspaceId ? api.listExecutionRuntime() : Promise.resolve([] as ExecutionRuntimeRow[]),
    [activeWorkspaceId],
    { pollMs: POLL_MS },
  )
  const rows = useMemo(() => data ?? [], [data])

  // Cross-window live sync: the central SSE dispatcher bumps this signal on every
  // run-lifecycle event in the active workspace. Coalesced with a short settle
  // window; the initial load is the useAsync above, not this effect.
  const tick = useRefreshTrigger(SIGNAL_EXECUTIONS)
  useEffect(() => {
    if (tick === 0) return
    const t = window.setTimeout(refresh, SIGNAL_SETTLE_MS)
    return () => window.clearTimeout(t)
  }, [tick, refresh])

  const runtimeById = useMemo(() => {
    const m = new Map<string, ExecutionRuntime>()
    for (const e of rows) {
      m.set(e.sessionId, {
        running: e.running,
        lastStatus: e.lastStatus ?? '',
        coordinatorSessionId: e.coordinatorSessionId,
        rootCoordinatorSessionId: e.rootCoordinatorSessionId,
      })
    }
    return m
  }, [rows])

  return { runtimeById }
}
