// Board view state: which view is selected, the live (possibly edited) filter,
// and persistence of user-created views into workspace settings.
//
// The SELECTION lives in localStorage, not in workspace settings: TionSwarm
// supports multiple windows on one workspace (see _Docs/30-COKLU-PENCERE.md),
// and two windows sitting on different views must not overwrite each other. The
// view DEFINITIONS are shared and do live in workspace settings.
//
// Nothing here mirrors props into state: `saved` is owned by the board (which
// already reloads settings on the SSE 'board' signal) and everything else is
// derived, so a view saved in another window shows up without a sync effect.

import { useCallback, useEffect, useMemo, useState } from 'react'
import { api } from '@/api'
import type { BoardFilter, BoardGroupBy, BoardSort, BoardViewDef } from '@/types'
import {
  BUILTIN_VIEWS,
  isBuiltinId,
  resolveView,
  sameView,
  slugifyViewLabel,
  uniqueViewID,
  type ResolvedView,
} from './boardViewTypes'

const SELECTION_KEY = 'tionswarm.board.viewId'

function loadSelection(): string {
  return localStorage.getItem(SELECTION_KEY) ?? BUILTIN_VIEWS[0].id
}

export interface BoardViewState {
  /** Built-ins followed by the workspace's saved views — the full menu. */
  allViews: BoardViewDef[]
  /** The view actually in effect (falls back to "Tümü" if the stored id is gone). */
  selectedId: string
  /** The view the board is rendering, including unsaved edits. */
  live: ResolvedView
  /** True when `live` has drifted from the selected view's stored definition. */
  dirty: boolean
  selectView: (id: string) => void
  setFilter: (f: BoardFilter) => void
  setGroupBy: (g: BoardGroupBy) => void
  setSort: (s: BoardSort) => void
  /** Discard unsaved edits, back to the selected view's stored definition. */
  revert: () => void
  /** Clear every facet without changing groupBy/sort. */
  clearFilter: () => void
  /** Persist `live` as a new view and select it. Returns the new id. */
  saveAsNew: (label: string) => Promise<string>
  /** Overwrite the selected saved view with `live`. */
  saveOverwrite: () => Promise<void>
  renameView: (id: string, label: string) => Promise<void>
  deleteView: (id: string) => Promise<void>
}

// useBoardView owns view selection and editing. `saved` and `onSavedChange` hand
// ownership of the persisted list to the caller, so there is exactly one copy of
// it in the tree.
export function useBoardView(
  saved: BoardViewDef[],
  onSavedChange: (views: BoardViewDef[]) => void,
  onError: (msg: string) => void,
): BoardViewState {
  const [storedId, setStoredId] = useState<string>(loadSelection)
  // Unsaved edits. null = "no edits", so the selected view's definition shows
  // through — including one that only arrives once settings load.
  const [edits, setEdits] = useState<ResolvedView | null>(null)

  const allViews = useMemo(() => [...BUILTIN_VIEWS, ...saved], [saved])

  // A view deleted in another window leaves this one pointing at nothing; fall
  // back to "Tümü" rather than rendering an empty board under a stale name.
  const selectedDef = useMemo(
    () => allViews.find((v) => v.id === storedId) ?? BUILTIN_VIEWS[0],
    [allViews, storedId],
  )
  const selectedId = selectedDef.id

  const stored = useMemo(() => resolveView(selectedDef), [selectedDef])
  const live = edits ?? stored
  const dirty = edits !== null && !sameView(edits, stored)

  useEffect(() => {
    localStorage.setItem(SELECTION_KEY, selectedId)
  }, [selectedId])

  const selectView = useCallback((id: string) => {
    setStoredId(id)
    setEdits(null)
  }, [])

  const revert = useCallback(() => setEdits(null), [])

  // Edit helpers seed from the CURRENT live view, so the first edit after
  // selecting a view keeps that view's other axes instead of resetting them.
  const edit = useCallback(
    (part: Partial<ResolvedView>) => setEdits((prev) => ({ ...(prev ?? stored), ...part })),
    [stored],
  )

  const clearFilter = useCallback(() => edit({ filter: {} }), [edit])
  const setFilter = useCallback((filter: BoardFilter) => edit({ filter }), [edit])
  const setGroupBy = useCallback((groupBy: BoardGroupBy) => edit({ groupBy }), [edit])
  const setSort = useCallback((sort: BoardSort) => edit({ sort }), [edit])

  // persist writes the whole view list (the backend patch replaces it wholesale)
  // and adopts the server's echo, so a rejected save cannot leave the UI showing
  // a view that was never stored.
  const persist = useCallback(
    async (next: BoardViewDef[]) => {
      const updated = await api.updateWorkspaceSettings({ boardViews: next })
      onSavedChange(updated.boardViews ?? [])
    },
    [onSavedChange],
  )

  const saveAsNew = useCallback(
    async (label: string) => {
      const trimmed = label.trim()
      if (!trimmed) throw new Error('Görünüm adı boş olamaz')
      const id = uniqueViewID(slugifyViewLabel(trimmed), new Set(allViews.map((v) => v.id)))
      await persist([
        ...saved,
        { id, label: trimmed, filter: live.filter, groupBy: live.groupBy, sort: live.sort },
      ])
      setStoredId(id)
      setEdits(null)
      return id
    },
    [allViews, live, persist, saved],
  )

  const saveOverwrite = useCallback(async () => {
    if (isBuiltinId(selectedId)) {
      throw new Error('Hazır görünümlerin üzerine yazılamaz — "Yeni olarak kaydet" kullanın')
    }
    await persist(
      saved.map((v) =>
        v.id === selectedId
          ? { ...v, filter: live.filter, groupBy: live.groupBy, sort: live.sort }
          : v,
      ),
    )
    setEdits(null)
  }, [live, persist, saved, selectedId])

  const renameView = useCallback(
    async (id: string, label: string) => {
      const trimmed = label.trim()
      if (!trimmed) throw new Error('Görünüm adı boş olamaz')
      await persist(saved.map((v) => (v.id === id ? { ...v, label: trimmed } : v)))
    },
    [persist, saved],
  )

  const deleteView = useCallback(
    async (id: string) => {
      await persist(saved.filter((v) => v.id !== id))
      if (id === selectedId) selectView(BUILTIN_VIEWS[0].id)
    },
    [persist, saved, selectView, selectedId],
  )

  // Surface a failed save instead of letting it reject into an unhandled promise
  // from an onClick handler.
  const guard =
    <A extends unknown[], R>(fn: (...a: A) => Promise<R>) =>
    async (...a: A): Promise<R> => {
      try {
        return await fn(...a)
      } catch (e) {
        onError((e as Error).message)
        throw e
      }
    }

  return {
    allViews,
    selectedId,
    live,
    dirty,
    selectView,
    setFilter,
    setGroupBy,
    setSort,
    revert,
    clearFilter,
    saveAsNew: guard(saveAsNew),
    saveOverwrite: guard(saveOverwrite),
    renameView: guard(renameView),
    deleteView: guard(deleteView),
  }
}
