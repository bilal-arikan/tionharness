// useWorkspaces owns the workspace list, the active workspace selection (mirrored
// into the api client + localStorage) and the cross-workspace "unread activity"
// badge set. Switch/create/delete are exposed as stable callbacks.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, setActiveWorkspace, getActiveWorkspace } from '../api'
import type { Workspace } from '../types'
import type { NewWorkspaceData } from '../components/workspace/WorkspaceCreateModal'

// The unread-activity badge set is SHARED across every browser window of the same
// origin via localStorage + the 'storage' event. SwarmGo is single-user, so a
// workspace that gained activity should show a dot in every open window, and
// reading it in any window (becoming its active workspace, or live-viewing new
// activity in it) clears the dot everywhere — "seen anywhere = seen".
const UNREAD_KEY = 'swarmgo.unreadWs'

function readSharedUnread(): Set<string> {
  try {
    const raw = localStorage.getItem(UNREAD_KEY)
    const arr = raw ? JSON.parse(raw) : []
    return new Set(Array.isArray(arr) ? arr.filter((x) => typeof x === 'string') : [])
  } catch {
    return new Set()
  }
}

function writeSharedUnread(set: Set<string>) {
  try {
    localStorage.setItem(UNREAD_KEY, JSON.stringify([...set]))
  } catch {
    /* quota / serialization failure — non-fatal, the badge just won't persist */
  }
}

export function useWorkspaces(setError: (msg: string) => void) {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [activeWorkspaceId, setActiveWorkspaceId] = useState<string | null>(getActiveWorkspace())
  // Raw shared unread set (cross-window union, mirrored to localStorage). The
  // displayed set (`unreadWs` below) filters out this window's active workspace.
  const [unreadRaw, setUnreadRaw] = useState<Set<string>>(() => readSharedUnread())

  // Sync the badge set when another window mutates it (storage events fire in
  // every same-origin document except the one that wrote the change).
  useEffect(() => {
    const onStorage = (e: StorageEvent) => {
      if (e.key !== null && e.key !== UNREAD_KEY) return
      setUnreadRaw(readSharedUnread())
    }
    window.addEventListener('storage', onStorage)
    return () => window.removeEventListener('storage', onStorage)
  }, [])

  // Badge a workspace (ignored if it is THIS window's active one). Persisted so
  // other windows pick it up via their storage listener.
  const markWorkspaceUnread = useCallback((id: string) => {
    if (!id || id === getActiveWorkspace()) return
    setUnreadRaw((prev) => {
      if (prev.has(id)) return prev
      const next = new Set(prev)
      next.add(id)
      writeSharedUnread(next)
      return next
    })
  }, [])

  // Clear a workspace's badge everywhere (it has been seen in some window).
  const markWorkspaceRead = useCallback((id: string) => {
    setUnreadRaw((prev) => {
      if (!prev.has(id)) return prev
      const next = new Set(prev)
      next.delete(id)
      writeSharedUnread(next)
      return next
    })
  }, [])

  // Whatever workspace is active in THIS window is, by definition, being seen —
  // clear its badge from the shared set (covers initial load, switching, and a
  // window inheriting a badge for the workspace it already shows).
  useEffect(() => {
    if (activeWorkspaceId) markWorkspaceRead(activeWorkspaceId)
  }, [activeWorkspaceId, markWorkspaceRead])

  // Displayed badges never include this window's active workspace.
  const unreadWs = useMemo(() => {
    if (!activeWorkspaceId || !unreadRaw.has(activeWorkspaceId)) return unreadRaw
    const next = new Set(unreadRaw)
    next.delete(activeWorkspaceId)
    return next
  }, [unreadRaw, activeWorkspaceId])

  // Initial load: pick the active workspace (saved or first).
  useEffect(() => {
    api
      .listWorkspaces()
      .then((list) => {
        setWorkspaces(list)
        const saved = getActiveWorkspace()
        const valid = list.find((w) => w.id === saved)
        const chosen = valid?.id ?? list[0]?.id ?? null
        if (chosen) {
          setActiveWorkspace(chosen)
          setActiveWorkspaceId(chosen)
        }
      })
      .catch((e) => setError((e as Error).message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const switchWorkspace = useCallback((id: string) => {
    setActiveWorkspace(id)
    setActiveWorkspaceId(id)
    // The active-workspace effect clears this workspace's badge (here and, via
    // localStorage, in every other window).
  }, [])

  const createWorkspace = useCallback(async (data: NewWorkspaceData) => {
    try {
      const wsNew = await api.createWorkspace(data)
      // Re-fetch the list so the icon/color (stored in ws-settings, absent from
      // the create response) are reflected immediately; fall back to appending.
      try {
        setWorkspaces(await api.listWorkspaces())
      } catch {
        setWorkspaces((prev) => [...prev, wsNew])
      }
      setActiveWorkspace(wsNew.id)
      setActiveWorkspaceId(wsNew.id)
    } catch (e) {
      setError((e as Error).message)
    }
  }, [setError])

  // Delete the active workspace, then switch to another (backend forbids
  // deleting the last one).
  const deleteActiveWorkspace = useCallback(async () => {
    if (!activeWorkspaceId) return
    const target = workspaces.find((w) => w.id === activeWorkspaceId)
    if (!confirm(`"${target?.name ?? 'Bu workspace'}" ve tüm verisi kalıcı olarak silinsin mi?`)) return
    try {
      await api.deleteWorkspace(activeWorkspaceId)
      const remaining = workspaces.filter((w) => w.id !== activeWorkspaceId)
      setWorkspaces(remaining)
      const next = remaining[0]?.id ?? null
      if (next) {
        setActiveWorkspace(next)
        setActiveWorkspaceId(next)
      }
    } catch (e) {
      setError((e as Error).message)
    }
  }, [activeWorkspaceId, workspaces, setError])

  // Delete any workspace by id (used by the switcher's per-row trash button).
  // The backend forbids deleting the last one; if the active workspace is
  // removed, switch to whatever remains.
  const deleteWorkspace = useCallback(
    async (id: string) => {
      const target = workspaces.find((w) => w.id === id)
      if (workspaces.length <= 1) {
        setError('Son workspace silinemez.')
        return
      }
      if (!confirm(`"${target?.name ?? 'Bu workspace'}" ve tüm verisi kalıcı olarak silinsin mi?`)) return
      try {
        await api.deleteWorkspace(id)
        const remaining = workspaces.filter((w) => w.id !== id)
        setWorkspaces(remaining)
        if (id === activeWorkspaceId) {
          const next = remaining[0]?.id ?? null
          if (next) {
            setActiveWorkspace(next)
            setActiveWorkspaceId(next)
          }
        }
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [workspaces, activeWorkspaceId, setError],
  )

  // Refresh the workspace list (e.g. after a rename in settings).
  const refreshWorkspaces = useCallback(() => {
    api.listWorkspaces().then(setWorkspaces).catch(() => {})
  }, [])

  return {
    workspaces,
    activeWorkspaceId,
    unreadWs,
    markWorkspaceUnread,
    markWorkspaceRead,
    switchWorkspace,
    createWorkspace,
    deleteActiveWorkspace,
    deleteWorkspace,
    refreshWorkspaces,
  }
}
