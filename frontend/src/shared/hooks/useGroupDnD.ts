import type { DragEvent } from 'react'
import { useCallback, useMemo, useRef, useState } from 'react'

// Custom MIME type stamped on a group-item drag. Drop targets check for it so
// they only react to our own list cards — and, just as importantly, an OS file
// drag (which carries 'Files' instead) still reaches the Artifacts screen's
// upload drop zone untouched.
export const GROUP_ITEM_MIME = 'application/x-tionswarm-group-item'

export interface UseGroupDnDOptions<T> {
  // Every item that may take part in a drag — the UNFILTERED list, since a
  // multi-selection can hold ids the current view has filtered out.
  items: T[]
  idOf: (item: T) => string
  // Bucket label of an item: the same key useGroupedList buckets on, so an item
  // with no group resolves to `ungroupedLabel`.
  groupOf: (item: T) => string
  // Bucket label standing in for "no group". Dropping there sends group="".
  ungroupedLabel: string
  // The current multi-selection. Dragging a selected card moves the whole
  // selection; dragging any other card moves only that card.
  selectedIds: ReadonlySet<string>
  // Persist the move. Only ever called with ids that actually change group.
  onMove: (ids: string[], group: string) => void
}

export interface GroupItemProps {
  draggable: true
  onDragStart: (e: DragEvent) => void
  onDragEnd: () => void
  'data-dragging': boolean
}

export interface GroupDropProps {
  onDragOver: (e: DragEvent) => void
  onDragLeave: (e: DragEvent) => void
  onDrop: (e: DragEvent) => void
  'data-drop-active': boolean
}

export interface GroupDnD<T> {
  // Ids currently being dragged; empty while idle.
  dragIds: ReadonlySet<string>
  // Label of the group the pointer hovers over as a VALID drop target.
  overGroup: string | null
  isOver: (groupName: string) => boolean
  // True for the click a browser may emit on the source element right after a
  // drag, so a card's onClick can skip the selection it would otherwise perform.
  // Consuming the flag clears it.
  consumedByDrag: () => boolean
  itemProps: (item: T) => GroupItemProps
  groupProps: (groupName: string) => GroupDropProps
}

function typesInclude(dt: DataTransfer, type: string): boolean {
  return Array.from(dt.types).includes(type)
}

// useGroupDnD is the native HTML5 drag-and-drop layer shared by the grouped list
// screens (Skills, Artifacts): drag a card onto a group and the card's group is
// rewritten through `onMove`. No third-party DnD dependency — the same approach
// the kanban board already uses.
//
// Two behaviours are deliberate:
//   - A drop whose dragged cards ALL already sit in the target group never
//     preventDefaults its dragover, so the browser refuses the drop outright and
//     `onMove` (and its API call) is never reached.
//   - `onDrop` stops propagation, so a card drop cannot bubble into an ancestor
//     file-upload drop zone.
export function useGroupDnD<T>(opts: UseGroupDnDOptions<T>): GroupDnD<T> {
  const { items, idOf, groupOf, ungroupedLabel, selectedIds, onMove } = opts

  const [dragIds, setDragIds] = useState<ReadonlySet<string>>(() => new Set<string>())
  const [overGroup, setOverGroup] = useState<string | null>(null)
  // Synchronous mirror of `dragIds`: dragover/drop must decide before React has
  // flushed the state update from dragstart.
  const dragIdsRef = useRef<string[]>([])
  // Set for the duration of a drag and cleared one macrotask after it ends, which
  // is long enough to swallow the trailing click some browsers emit.
  const draggedRef = useRef(false)
  const resetTimer = useRef<number | undefined>(undefined)

  const byId = useMemo(() => new Map(items.map((item) => [idOf(item), item])), [items, idOf])

  // The dragged ids whose group differs from `groupName` — the ones a drop would
  // actually move. Empty means the drop is a no-op.
  const moversFor = useCallback(
    (groupName: string): string[] =>
      dragIdsRef.current.filter((id) => {
        const item = byId.get(id)
        // Ids the current view no longer knows about (filtered out and then
        // deleted, say) simply cannot be moved.
        return item !== undefined && groupOf(item) !== groupName
      }),
    [byId, groupOf],
  )

  const endDrag = useCallback(() => {
    dragIdsRef.current = []
    setDragIds(new Set<string>())
    setOverGroup(null)
    window.clearTimeout(resetTimer.current)
    resetTimer.current = window.setTimeout(() => {
      draggedRef.current = false
    }, 0)
  }, [])

  const consumedByDrag = useCallback((): boolean => {
    if (!draggedRef.current) return false
    draggedRef.current = false
    return true
  }, [])

  const itemProps = useCallback(
    (item: T): GroupItemProps => {
      const id = idOf(item)
      return {
        draggable: true,
        onDragStart: (e: DragEvent) => {
          const ids = selectedIds.has(id) ? [...selectedIds] : [id]
          window.clearTimeout(resetTimer.current)
          dragIdsRef.current = ids
          draggedRef.current = true
          setDragIds(new Set(ids))
          e.dataTransfer.setData(GROUP_ITEM_MIME, ids.join(','))
          e.dataTransfer.effectAllowed = 'move'
        },
        onDragEnd: endDrag,
        'data-dragging': dragIds.has(id),
      }
    },
    [idOf, selectedIds, dragIds, endDrag],
  )

  const groupProps = useCallback(
    (groupName: string): GroupDropProps => ({
      onDragOver: (e: DragEvent) => {
        if (!typesInclude(e.dataTransfer, GROUP_ITEM_MIME)) return
        if (moversFor(groupName).length === 0) {
          // Everything dragged already lives here: refuse the drop (no
          // preventDefault ⇒ no `drop` event ⇒ no API call).
          e.dataTransfer.dropEffect = 'none'
          return
        }
        e.preventDefault()
        e.dataTransfer.dropEffect = 'move'
        setOverGroup((cur) => (cur === groupName ? cur : groupName))
      },
      onDragLeave: (e: DragEvent) => {
        // Moving between children of the same group must not clear the highlight.
        const next = e.relatedTarget
        if (next instanceof Node && e.currentTarget.contains(next)) return
        setOverGroup((cur) => (cur === groupName ? null : cur))
      },
      onDrop: (e: DragEvent) => {
        if (!typesInclude(e.dataTransfer, GROUP_ITEM_MIME)) return
        e.preventDefault()
        // Never let a card drop reach an ancestor file-upload drop zone.
        e.stopPropagation()
        const movers = moversFor(groupName)
        endDrag()
        if (movers.length === 0) return
        onMove(movers, groupName === ungroupedLabel ? '' : groupName)
      },
      'data-drop-active': overGroup === groupName,
    }),
    [moversFor, endDrag, onMove, ungroupedLabel, overGroup],
  )

  const isOver = useCallback((groupName: string) => overGroup === groupName, [overGroup])

  return { dragIds, overGroup, isOver, consumedByDrag, itemProps, groupProps }
}
