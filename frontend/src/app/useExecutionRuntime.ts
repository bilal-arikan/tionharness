// useExecutionRuntime polls GET /api/executions and exposes the two facts the
// session list needs but GET /api/sessions does not carry: whether a session's
// turn is running right now, and how its last task/flow run finished.
//
// Both endpoints are built on the same DB.ListSessions call; /api/executions
// merely enriches each row. The sessions sidebar consumes this map to render the
// live pulse dot + the pass/fail status pill for task and flow transcripts.
//
// The poll is backed up by the shared 'executions' refresh signal, which the SSE
// dispatcher bumps on every chat / flow / schedule / spawn / worker / task /
// session event, so a status flip lands without waiting for the next tick.
import { useEffect, useMemo } from 'react'
import { api } from '@/api'
import type { Execution } from '@/types'
import { useAsync } from '@/shared/hooks/useAsync'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { SIGNAL_EXECUTIONS } from './eventToRefreshSignals'

// Backstop only — SIGNAL_EXECUTIONS below carries the live updates.
const POLL_MS = 20000

export interface ExecutionRuntime {
  running: boolean
  lastStatus: string
}

export interface ExecutionRuntimeState {
  // sessionId → live/last-run facts. Sessions absent from the map are idle.
  runtimeById: Map<string, ExecutionRuntime>
}

export function useExecutionRuntime(activeWorkspaceId: string | null): ExecutionRuntimeState {
  const { data, refresh } = useAsync(
    () => (activeWorkspaceId ? api.listExecutions() : Promise.resolve([] as Execution[])),
    [activeWorkspaceId],
    { pollMs: POLL_MS },
  )
  const executions = useMemo(() => data ?? [], [data])

  // Cross-window live sync: the central SSE dispatcher bumps this signal on every
  // run-lifecycle event in the active workspace.
  const tick = useRefreshTrigger(SIGNAL_EXECUTIONS)
  useEffect(() => {
    refresh()
  }, [tick, refresh])

  const runtimeById = useMemo(() => {
    const m = new Map<string, ExecutionRuntime>()
    for (const e of executions) {
      m.set(e.sessionId, { running: e.running, lastStatus: e.lastStatus ?? '' })
    }
    return m
  }, [executions])

  return { runtimeById }
}
