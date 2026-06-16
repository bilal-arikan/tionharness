// useWorkspaces owns the workspace list, the active workspace selection (mirrored
// into the api client + localStorage) and the cross-workspace "unread activity"
// badge set. Switch/create/delete are exposed as stable callbacks.
import { useCallback, useEffect, useState } from 'react'
import { api, setActiveWorkspace, getActiveWorkspace } from '../api'
import type { Workspace } from '../types'
import type { NewWorkspaceData } from '../components/workspace/WorkspaceCreateModal'

export function useWorkspaces(setError: (msg: string) => void) {
  const [workspaces, setWorkspaces] = useState<Workspace[]>([])
  const [activeWorkspaceId, setActiveWorkspaceId] = useState<string | null>(getActiveWorkspace())
  // Workspaces (other than the active one) with pending activity, shown as a
  // badge in the switcher. Populated from the autonomous-event feed.
  const [unreadWs, setUnreadWs] = useState<Set<string>>(() => new Set())

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
    // Switching to a workspace clears its pending-activity badge.
    setUnreadWs((prev) => {
      if (!prev.has(id)) return prev
      const next = new Set(prev)
      next.delete(id)
      return next
    })
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

  // Refresh the workspace list (e.g. after a rename in settings).
  const refreshWorkspaces = useCallback(() => {
    api.listWorkspaces().then(setWorkspaces).catch(() => {})
  }, [])

  return {
    workspaces,
    activeWorkspaceId,
    unreadWs,
    setUnreadWs,
    switchWorkspace,
    createWorkspace,
    deleteActiveWorkspace,
    refreshWorkspaces,
  }
}
