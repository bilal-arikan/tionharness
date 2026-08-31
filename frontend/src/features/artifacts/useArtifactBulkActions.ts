import { useCallback, useState } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { api } from '@/api'
import type { Artifact } from '@/types'
import type { MultiSelect } from '@/shared/hooks/useMultiSelect'
import { useGroupDnD, type GroupDnD } from '@/shared/hooks/useGroupDnD'
import { UNGROUPED, artifactGroupKey, artifactId } from './artifactGrouping'

export interface ArtifactBulkActions {
  // Draft group name + busy flag for the bulk "set group" action on the selection.
  bulkGroup: string
  setBulkGroup: Dispatch<SetStateAction<string>>
  bulkGroupBusy: boolean
  // Busy flag for the bulk archive / un-archive action on the selection.
  bulkArchiveBusy: boolean
  bulkDelete: () => Promise<void>
  bulkSetGroup: (group: string) => void
  bulkSetArchived: (archived: boolean) => void
  dnd: GroupDnD<Artifact>
}

export interface UseArtifactBulkActionsOptions {
  list: Artifact[]
  sel: MultiSelect
  activeId: string | null
  setActiveId: Dispatch<SetStateAction<string | null>>
  setList: Dispatch<SetStateAction<Artifact[]>>
  setActive: Dispatch<SetStateAction<Artifact | null>>
  reload: () => void
  onError: (msg: string) => void
}

// useArtifactBulkActions owns every action that operates on MORE than one
// artifact: the multi-select bulk bar (delete, set group, archive) and the
// drag-and-drop group move, which shares the same group endpoint.
export function useArtifactBulkActions({
  list,
  sel,
  activeId,
  setActiveId,
  setList,
  setActive,
  reload,
  onError,
}: UseArtifactBulkActionsOptions): ArtifactBulkActions {
  const [bulkGroup, setBulkGroup] = useState('')
  const [bulkGroupBusy, setBulkGroupBusy] = useState(false)
  const [bulkArchiveBusy, setBulkArchiveBusy] = useState(false)

  const bulkDelete = useCallback(async () => {
    const ids = [...sel.selected]
    if (ids.length === 0) return
    if (!confirm(`${ids.length} artifact kalıcı olarak silinsin mi?`)) return
    setList((prev) => prev.filter((a) => !sel.selected.has(a.id)))
    setActiveId((cur) => (cur && sel.selected.has(cur) ? null : cur))
    sel.clear()
    try {
      await Promise.all(ids.map((id) => api.deleteArtifact(id)))
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }, [sel, onError, reload, setList, setActiveId])

  // Bulk-set the `group` of every selected artifact at once, so a batch lands
  // under one collapsible header without opening each artifact. An empty group
  // ungroups them. Keeps the selection so the user can chain another action; the
  // list re-buckets after the reload.
  const bulkSetGroup = useCallback(
    (group: string) => {
      const ids = [...sel.selected]
      if (ids.length === 0) return
      setBulkGroupBusy(true)
      Promise.all(ids.map((id) => api.setArtifactGroup(id, group)))
        .then(() => {
          setBulkGroup('')
          reload()
          if (activeId && sel.selected.has(activeId)) {
            api
              .getArtifact(activeId)
              .then(setActive)
              .catch(() => {})
          }
        })
        .catch((e) => onError((e as Error).message))
        .finally(() => setBulkGroupBusy(false))
    },
    [sel.selected, reload, activeId, onError, setActive],
  )

  // Bulk archive / un-archive every selected artifact at once (a soft, reversible
  // hide). In the default view this archives the selection; in the archived view
  // it restores it. Clears the selection since the affected cards leave the
  // current view after the reload.
  const bulkSetArchived = useCallback(
    (archived: boolean) => {
      const ids = [...sel.selected]
      if (ids.length === 0) return
      setBulkArchiveBusy(true)
      sel.clear()
      Promise.all(ids.map((id) => api.setArtifactArchived(id, archived)))
        .then(() => {
          // The affected artifacts drop out of the current view; clear the
          // detail selection if it was one of them so the viewer doesn't dangle.
          setActiveId((cur) => (cur && ids.includes(cur) ? null : cur))
        })
        .catch((e) => onError((e as Error).message))
        .finally(() => {
          setBulkArchiveBusy(false)
          reload()
        })
    },
    [sel, reload, onError, setActiveId],
  )

  // Drag-and-drop group move: dropping a card on a group header rewrites its
  // `group` through the same endpoint the bulk action uses. Dragging a card that
  // belongs to the current selection moves the whole selection.
  const moveToGroup = useCallback(
    (ids: string[], group: string) => {
      Promise.all(ids.map((id) => api.setArtifactGroup(id, group)))
        .then(() => {
          if (activeId && ids.includes(activeId)) return api.getArtifact(activeId).then(setActive)
        })
        .catch((e) => onError((e as Error).message))
        // Refresh either way: on success to re-bucket the list, on failure so the
        // cards snap back to the persisted truth instead of a half-applied move.
        .finally(() => reload())
    },
    [activeId, reload, onError, setActive],
  )
  const dnd = useGroupDnD<Artifact>({
    items: list,
    idOf: artifactId,
    groupOf: artifactGroupKey,
    ungroupedLabel: UNGROUPED,
    selectedIds: sel.selected,
    onMove: moveToGroup,
  })

  return {
    bulkGroup,
    setBulkGroup,
    bulkGroupBusy,
    bulkArchiveBusy,
    bulkDelete,
    bulkSetGroup,
    bulkSetArchived,
    dnd,
  }
}
