// useActivity polls the backend for per-view "work in progress" flags and
// exposes them as a Set<View> the nav rail badges with a live indicator. The
// chat view also reflects THIS window's in-flight stream instantly, so the dot
// appears without waiting for the next poll.
import { useEffect, useMemo, useState } from 'react'
import { api } from '../api'
import type { View } from '../components/NavRail'

const POLL_MS = 3000

export function useActivity(activeWorkspaceId: string | null, chatStreaming: boolean): Set<View> {
  const [busy, setBusy] = useState<Set<View>>(() => new Set())

  useEffect(() => {
    if (!activeWorkspaceId) {
      setBusy(new Set())
      return
    }
    let cancelled = false
    const poll = async () => {
      try {
        const a = await api.getActivity()
        if (cancelled) return
        const s = new Set<View>()
        if (a.chat) s.add('chat')
        if (a.task) s.add('board')
        if (a.flow) s.add('flows')
        if (a.schedule) s.add('schedules')
        // The unified "Aktivite" view lights for ANY in-flight run — including
        // background sessions a flow/schedule/agent spawns that no other view owns.
        if (a.executions) s.add('executions')
        setBusy(s)
      } catch {
        /* transient fetch failure — keep the last known flags */
      }
    }
    poll()
    const id = setInterval(poll, POLL_MS)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [activeWorkspaceId])

  // Merge the instant local chat-stream signal so the chat dot has no poll lag.
  return useMemo(() => {
    if (!chatStreaming || busy.has('chat')) return busy
    const s = new Set(busy)
    s.add('chat')
    return s
  }, [busy, chatStreaming])
}
