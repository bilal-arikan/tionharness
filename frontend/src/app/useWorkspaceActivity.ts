// useWorkspaceActivity polls GET /api/workspaces/activity and exposes the set of
// workspace ids that currently have a run in flight — so the workspace switcher
// can pulse a "çalışıyor" dot on EVERY busy workspace, not just the active one.
//
// The active workspace's per-view busy flags come from useActivity (/api/activity);
// this hook is the cross-workspace complement, covering non-active workspaces the
// per-view poll never sees. Completed-run signalling stays on the SSE unread badge
// (markWorkspaceUnread) — this hook only reports the live/running dimension.
//
// The poll is backed up by two refresh signals, so a flip lands without waiting
// for the next tick:
//   - 'executions'        — bumped on every run-lifecycle event in the ACTIVE
//                           workspace (covers this workspace's own start/stop).
//   - 'workspace-activity'— bumped on run-lifecycle events in ANY workspace,
//                           including non-active ones, so a run starting/finishing
//                           in a background workspace lights its switcher pulse
//                           instantly instead of on the next poll.
import { useEffect, useMemo } from 'react'
import { api } from '@/api'
import { useAsync } from '@/shared/hooks/useAsync'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { SIGNAL_EXECUTIONS, SIGNAL_WORKSPACE_ACTIVITY } from './eventToRefreshSignals'

// Backstop only: the two SSE signals below are what make the pulse feel live, so
// the interval just covers a missed/reconnected event. It was 4s, which — times
// every open window, times every workspace the endpoint reports on — was the
// single largest source of idle backend work.
const POLL_MS = 20000

export function useWorkspaceActivity(activeWorkspaceId: string | null): Set<string> {
  const { data, refresh } = useAsync(
    () =>
      activeWorkspaceId
        ? api.listWorkspacesActivity()
        : Promise.resolve([] as { id: string; running: boolean }[]),
    [activeWorkspaceId],
    { pollMs: POLL_MS },
  )

  // Instant refresh on run-lifecycle events: active-workspace ones (executions)
  // plus any workspace's start/stop (workspace-activity, covers non-active ones).
  const tickExec = useRefreshTrigger(SIGNAL_EXECUTIONS)
  const tickWs = useRefreshTrigger(SIGNAL_WORKSPACE_ACTIVITY)
  useEffect(() => {
    refresh()
  }, [tickExec, tickWs, refresh])

  return useMemo(() => {
    const s = new Set<string>()
    for (const w of data ?? []) {
      if (w.running) s.add(w.id)
    }
    return s
  }, [data])
}
