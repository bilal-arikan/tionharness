// useUnreadViews tracks which nav Views have UNSEEN activity in the active
// workspace (the "unread" signal), driven by the SSE event feed. Each workspace
// keeps its own set, persisted to localStorage and synced across windows via the
// 'storage' event — matching the cross-window workspace-unread badge behaviour.
import { useCallback, useEffect, useState } from 'react'
import type { View } from '../components/NavRail'

const keyFor = (wsId: string) => `swarmgo.unreadViews.${wsId}`

function read(wsId: string | null): Set<View> {
  if (!wsId) return new Set()
  try {
    const raw = localStorage.getItem(keyFor(wsId))
    const arr = raw ? JSON.parse(raw) : []
    return new Set(Array.isArray(arr) ? (arr as View[]) : [])
  } catch {
    return new Set()
  }
}

function write(wsId: string, set: Set<View>) {
  try {
    localStorage.setItem(keyFor(wsId), JSON.stringify([...set]))
  } catch {
    /* quota / serialization failure — non-fatal, badges just won't persist */
  }
}

export function useUnreadViews(activeWorkspaceId: string | null) {
  const [unreadViews, setUnreadViews] = useState<Set<View>>(() => read(activeWorkspaceId))

  // Each workspace owns its set: reload when the active workspace changes.
  useEffect(() => {
    setUnreadViews(read(activeWorkspaceId))
  }, [activeWorkspaceId])

  // Cross-window sync: another window marking/clearing a view fires a storage
  // event everywhere except the writer; re-read the active workspace's key.
  useEffect(() => {
    if (!activeWorkspaceId) return
    const onStorage = (e: StorageEvent) => {
      if (e.key !== null && e.key !== keyFor(activeWorkspaceId)) return
      setUnreadViews(read(activeWorkspaceId))
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [activeWorkspaceId])

  const markViewUnread = useCallback(
    (view: View) => {
      if (!activeWorkspaceId) return
      setUnreadViews((prev) => {
        if (prev.has(view)) return prev
        const next = new Set(prev)
        next.add(view)
        write(activeWorkspaceId, next)
        return next
      })
    },
    [activeWorkspaceId],
  )

  const markViewRead = useCallback(
    (view: View) => {
      if (!activeWorkspaceId) return
      setUnreadViews((prev) => {
        if (!prev.has(view)) return prev
        const next = new Set(prev)
        next.delete(view)
        write(activeWorkspaceId, next)
        return next
      })
    },
    [activeWorkspaceId],
  )

  return { unreadViews, markViewUnread, markViewRead }
}
