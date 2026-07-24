// useActivity polls the backend for per-view "work in progress" flags and
// exposes them as a Set<View> the nav rail badges with a live indicator. The
// chat view also reflects THIS window's in-flight stream instantly, so the dot
// appears without waiting for the next poll.
//
// Cross-window live updates no longer need their own SSE subscription: App.tsx's
// central SSE handler bumps the 'activity' refresh signal whenever a run
// lifecycle event lands in the active workspace, and the hook re-polls on
// every bump. Saves a per-panel SSE listener (the singleton EventSource stays
// — only this hook's filter callback goes away).
import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '@/api'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import type { View } from './NavRail'

const POLL_MS = 3000

export function useActivity(activeWorkspaceId: string | null, chatStreaming: boolean): Set<View> {
  const [busy, setBusy] = useState<Set<View>>(() => new Set())
  // App.tsx bumps this signal on every run-lifecycle event in the active
  // workspace; the polling effect re-runs and re-pulls /api/activity.
  const activityTick = useRefreshTrigger('activity')

  // Stable poll fn so the deps stay valid across re-renders.
  const poll = useCallback(async () => {
    try {
      const a = await api.getActivity()
      const s = new Set<View>()
      if (a.chat) s.add('chat')
      if (a.task) s.add('board')
      if (a.flow) s.add('flows')
      if (a.schedule) s.add('schedules')
      // Every run funnels into a session, and the chat screen is now the unified
      // transcript view for all of them — so ANY in-flight run (including a
      // background session a flow/schedule/agent spawned, which no other view
      // owns) lights the chat dot.
      if (a.executions) s.add('chat')
      if (a.insights) s.add('insights')
      setBusy(s)
    } catch {
      /* transient fetch failure — keep the last known flags */
    }
  }, [])

  useEffect(() => {
    if (!activeWorkspaceId) {
      // Workspace switched away: drop the stale busy flags immediately. This
      // is a legitimate "reset on dep change" rather than a setState-in-effect
      // smell — the rule's strict variant flags it, hence the disable.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setBusy(new Set())
      return
    }
    let cancelled = false
    const guardedPoll = async () => {
      if (cancelled) return
      await poll()
    }
    guardedPoll()
    const id = setInterval(guardedPoll, POLL_MS)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [activeWorkspaceId, poll, activityTick])

  // Merge the instant local chat-stream signal so the chat dot has no poll lag.
  return useMemo(() => {
    if (!chatStreaming || busy.has('chat')) return busy
    const s = new Set(busy)
    s.add('chat')
    return s
  }, [busy, chatStreaming])
}
