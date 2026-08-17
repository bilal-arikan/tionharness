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
//
// The interval is a BACKSTOP for a missed/reconnected SSE event, not the primary
// path — hence coarse. It runs through useAsync so it also inherits the
// visibility gate (a hidden window polls nothing).
import { useEffect, useMemo } from 'react'
import { api } from '@/api'
import { useAsync } from '@/shared/hooks/useAsync'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { SIGNAL_ACTIVITY } from './eventToRefreshSignals'
import type { View } from './NavRail'

const POLL_MS = 15000

export function useActivity(activeWorkspaceId: string | null, chatStreaming: boolean): Set<View> {
  const { data, refresh } = useAsync(
    () => (activeWorkspaceId ? api.getActivity() : Promise.resolve(null)),
    [activeWorkspaceId],
    { pollMs: POLL_MS },
  )

  // App.tsx bumps this signal on every run-lifecycle event in the active
  // workspace — the path that actually makes the dots feel live.
  const activityTick = useRefreshTrigger(SIGNAL_ACTIVITY)
  useEffect(() => {
    refresh()
  }, [activityTick, refresh])

  return useMemo(() => {
    const s = new Set<View>()
    // No workspace (or nothing fetched yet) means no busy flags, so switching
    // away drops the previous workspace's dots rather than stranding them.
    if (!activeWorkspaceId || !data) return chatStreaming ? new Set<View>(['chat']) : s

    if (data.chat) s.add('chat')
    if (data.flow) s.add('flows')
    if (data.schedule) s.add('schedules')
    // Every run funnels into a session, and the chat screen is now the unified
    // transcript view for all of them — so ANY in-flight run (including a
    // background session a flow/schedule/agent spawned, which no other view
    // owns) lights the chat dot.
    if (data.executions) s.add('chat')
    if (data.insights) s.add('insights')
    // Merge this window's own stream so the chat dot has no poll lag at all.
    if (chatStreaming) s.add('chat')
    return s
  }, [data, activeWorkspaceId, chatStreaming])
}
