// useWorkspaces owns the workspace list, the active workspace selection (mirrored
// into the api client + localStorage) and the cross-workspace "unread activity"
// badge set. Switch/create/delete are exposed as stable callbacks.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { api, setActiveWorkspace, getActiveWorkspace, clearActiveWorkspace } from '@/api'
import { toast } from '@/shared/components'
import type { Workspace } from '@/types'
import type { NewWorkspaceData } from '@/features/workspace/WorkspaceCreateModal'

// The unread-activity badge set is SHARED across every browser window of the same
// origin via localStorage + the 'storage' event. TionHarness is single-user, so a
// workspace that gained activity should show a dot in every open window, and
// reading it in any window (becoming its active workspace, or live-viewing new
// activity in it) clears the dot everywhere — "seen anywhere = seen".
const UNREAD_KEY = 'tionharness.unreadWs'

// The favorite workspace opens on a fresh launch (cold start with no deep-linked
// workspace in the URL). Device-local, like the active-workspace pointer.
const FAVORITE_KEY = 'tionharness.favoriteWs'

function readFavorite(): string | null {
  try {
    return localStorage.getItem(FAVORITE_KEY) || null
  } catch {
    return null
  }
}

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
  // True until the initial workspace list resolves. Drives the first-run splash
  // and the "zero workspaces → onboarding" decision in App (we must not show the
  // onboarding screen until we actually know the list is empty).
  const [loading, setLoading] = useState(true)
  const [activeWorkspaceId, setActiveWorkspaceId] = useState<string | null>(getActiveWorkspace())
  // Raw shared unread set (cross-window union, mirrored to localStorage). The
  // displayed set (`unreadWs` below) filters out this window's active workspace.
  const [unreadRaw, setUnreadRaw] = useState<Set<string>>(() => readSharedUnread())
  const [favoriteWorkspaceId, setFavoriteWorkspaceId] = useState<string | null>(() =>
    readFavorite(),
  )

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
        // Cold start (no in-app pointer yet) → favorite wins; otherwise keep the
        // last-active. A URL-deep-linked workspace is applied afterwards by the
        // route machinery, so explicit links still override the favorite.
        const fav = readFavorite()
        const favValid = fav && list.some((w) => w.id === fav) ? fav : null
        // Last-resort fallback (no saved pointer, no favorite): take the first
        // OPENABLE workspace. The list is in creation order, so the oldest entry
        // leads it — and if that one is degraded, picking it lands a fresh browser
        // on a workspace whose every request fails. A saved or starred pointer is
        // an explicit user choice and is NOT overridden, degraded or not. With
        // nothing healthy at all we keep the old behaviour (first entry) so the
        // switcher still has a selection to show.
        const firstHealthy = list.find((w) => !w.degraded)?.id
        const chosen =
          (!saved && favValid) || valid?.id || favValid || firstHealthy || list[0]?.id || null
        if (chosen) {
          setActiveWorkspace(chosen)
          setActiveWorkspaceId(chosen)
        }
      })
      .catch((e) => setError((e as Error).message))
      .finally(() => setLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const switchWorkspace = useCallback((id: string) => {
    setActiveWorkspace(id)
    setActiveWorkspaceId(id)
    // The active-workspace effect clears this workspace's badge (here and, via
    // localStorage, in every other window).
  }, [])

  // Returns the created workspace on success (so callers can run post-create
  // gates, e.g. the claude-cli auth check), or undefined when creation failed.
  const createWorkspace = useCallback(
    async (data: NewWorkspaceData) => {
      try {
        const wsNew = await api.createWorkspace(data)
        // The optional `git init` is advisory: the workspace was created regardless,
        // so a failure is surfaced as an error banner instead of failing the flow.
        if (wsNew.gitInitError) setError('Git deposu başlatılamadı: ' + wsNew.gitInitError)
        // Re-fetch the list so the icon/color (stored in ws-settings, absent from
        // the create response) are reflected immediately; fall back to appending.
        try {
          setWorkspaces(await api.listWorkspaces())
        } catch {
          setWorkspaces((prev) => [...prev, wsNew])
        }
        setActiveWorkspace(wsNew.id)
        setActiveWorkspaceId(wsNew.id)
        toast.success('Workspace oluşturuldu')
        return wsNew
      } catch (e) {
        setError((e as Error).message)
        return undefined
      }
    },
    [setError],
  )

  // Adopt an existing on-disk workspace folder (first-run "select workspace"):
  // attach it on the backend, then make it active. Unlike createWorkspace this
  // RE-THROWS on failure so the caller (onboarding screen) can render the
  // validation error inline (invalid folder / already attached).
  const attachWorkspace = useCallback(async (path: string) => {
    const wsNew = await api.attachWorkspace(path)
    // Re-fetch so the icon/color from the adopted ws-settings.json show at once;
    // fall back to appending the create response on a list-fetch hiccup.
    try {
      setWorkspaces(await api.listWorkspaces())
    } catch {
      setWorkspaces((prev) => [...prev, wsNew])
    }
    setActiveWorkspace(wsNew.id)
    setActiveWorkspaceId(wsNew.id)
    toast.success('Workspace eklendi')
    return wsNew
  }, [])

  // Delete the active workspace, then switch to another. Deleting the LAST one is
  // allowed: with nothing remaining we clear the active pointer so App falls back
  // to the onboarding screen (no default workspace is re-seeded).
  const deleteActiveWorkspace = useCallback(async () => {
    if (!activeWorkspaceId) return
    const target = workspaces.find((w) => w.id === activeWorkspaceId)
    const last = workspaces.length <= 1
    const msg = last
      ? `"${target?.name ?? 'Bu workspace'}" son workspace — silinince ilk kurulum ekranına dönersin. Tüm verisiyle silinsin mi?`
      : `"${target?.name ?? 'Bu workspace'}" ve tüm verisi kalıcı olarak silinsin mi?`
    if (!confirm(msg)) return
    try {
      await api.deleteWorkspace(activeWorkspaceId)
      toast.success('Workspace silindi')
      const remaining = workspaces.filter((w) => w.id !== activeWorkspaceId)
      setWorkspaces(remaining)
      const next = remaining[0]?.id ?? null
      if (next) {
        setActiveWorkspace(next)
        setActiveWorkspaceId(next)
      } else {
        clearActiveWorkspace()
        setActiveWorkspaceId(null)
      }
    } catch (e) {
      setError((e as Error).message)
    }
  }, [activeWorkspaceId, workspaces, setError])

  // Delete any workspace by id (used by the switcher's per-row trash button).
  // Deleting the last one is allowed: when the removed workspace was active and
  // nothing remains, clear the active pointer so App shows the onboarding screen.
  const deleteWorkspace = useCallback(
    async (id: string) => {
      const target = workspaces.find((w) => w.id === id)
      const last = workspaces.length <= 1
      const msg = last
        ? `"${target?.name ?? 'Bu workspace'}" son workspace — silinince ilk kurulum ekranına dönersin. Tüm verisiyle silinsin mi?`
        : `"${target?.name ?? 'Bu workspace'}" ve tüm verisi kalıcı olarak silinsin mi?`
      if (!confirm(msg)) return
      try {
        await api.deleteWorkspace(id)
        toast.success('Workspace silindi')
        const remaining = workspaces.filter((w) => w.id !== id)
        setWorkspaces(remaining)
        if (id === activeWorkspaceId) {
          const next = remaining[0]?.id ?? null
          if (next) {
            setActiveWorkspace(next)
            setActiveWorkspaceId(next)
          } else {
            clearActiveWorkspace()
            setActiveWorkspaceId(null)
          }
        }
      } catch (e) {
        setError((e as Error).message)
      }
    },
    [workspaces, activeWorkspaceId, setError],
  )

  // Toggle a workspace as the startup favorite (clicking the current favorite
  // clears it). Persisted device-local; consumed by the next cold start.
  const setFavoriteWorkspace = useCallback((id: string) => {
    setFavoriteWorkspaceId((prev) => {
      const next = prev === id ? null : id
      try {
        if (next) localStorage.setItem(FAVORITE_KEY, next)
        else localStorage.removeItem(FAVORITE_KEY)
      } catch {
        /* storage unavailable — non-fatal, favorite just won't persist */
      }
      return next
    })
  }, [])

  // Refresh the workspace list (e.g. after a rename in settings).
  const refreshWorkspaces = useCallback(() => {
    api
      .listWorkspaces()
      .then(setWorkspaces)
      .catch(() => {})
  }, [])

  return {
    workspaces,
    loading,
    activeWorkspaceId,
    unreadWs,
    favoriteWorkspaceId,
    setFavoriteWorkspace,
    markWorkspaceUnread,
    markWorkspaceRead,
    switchWorkspace,
    createWorkspace,
    attachWorkspace,
    deleteActiveWorkspace,
    deleteWorkspace,
    refreshWorkspaces,
  }
}
