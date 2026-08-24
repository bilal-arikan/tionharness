import { useEffect, useMemo, useRef, useState } from 'react'
import { Trash2, Archive, ArchiveRestore } from 'lucide-react'
import { api } from '@/api'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import type { Agent, Task, TaskPatch, Flow, BoardColumnDef, BoardViewDef } from '@/types'
import { artifactKindForUpload } from '@/features/artifacts/artifactMeta'
import { ViewButton } from '@/features/view/ViewButton'
import { TaskFormModal } from './TaskFormModal'
import { BoardColumnEditor } from './BoardColumnEditor'
import {
  Button,
  SelectionBar,
  SelectionBarButton,
  PaneHeader,
  LoadingState,
} from '@/shared/components'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { useStableCallback } from '@/shared/lib/useStableCallback'
import { TaskCard, type TaskCardMeta } from './TaskCard'
import { BoardFilterBar } from './views/BoardFilterBar'
import { useBoardView } from './views/useBoardView'
import { filterTasks, parseDeps, sortTasks, topoLevels } from './views/filterTasks'
import { DROP_REFUSED_REASON, columnKeysOf, deriveColumns, dropPatch } from './views/deriveColumns'
import { todayISO } from './views/filterTasks'

// Fallback columns used until workspace settings are loaded.
const DEFAULT_COLUMNS: BoardColumnDef[] = [
  { key: 'pbi', label: 'PBI', color: '' },
  { key: 'todo', label: 'Yapılacak', color: '' },
  { key: 'in_progress', label: 'Devam Eden', color: '' },
  { key: 'review', label: 'İnceleme', color: '' },
  { key: 'done', label: 'Bitti', color: '' },
  { key: 'failed', label: 'Başarısız', color: '' },
]

// Current unix time in seconds, matching the backend's task timestamps — used
// for optimistic createdAt/updatedAt so cards sort consistently before reload.
const nowSec = () => Math.floor(Date.now() / 1000)

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
}

export function TaskBoard({ agents, onError }: Props) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [flows, setFlows] = useState<Flow[]>([])
  const [columns, setColumns] = useState<BoardColumnDef[]>(DEFAULT_COLUMNS)
  // User-created saved views, pulled from workspace settings alongside columns.
  const [savedViews, setSavedViews] = useState<BoardViewDef[]>([])
  const [dragId, setDragId] = useState<string | null>(null)
  // Card id currently under an OS file-drag (for the "drop to attach" highlight).
  const [fileDropId, setFileDropId] = useState<string | null>(null)
  // Create/edit popup state: null = closed.
  const [modal, setModal] = useState<{ mode: 'create' | 'edit'; taskId: string | null } | null>(
    null,
  )
  // Left-side column editor panel.
  const [editorOpen, setEditorOpen] = useState(false)
  // Transient hint under the filter bar: a card that a drag pushed out of the
  // current filter would otherwise just vanish silently.
  const [hint, setHint] = useState<{ text: string; undo?: () => void } | null>(null)
  const hintTimer = useRef<number | null>(null)
  // True until the first task list lands, so the board shows a loading state
  // instead of empty columns. Later reloads (SSE ticks) keep the board on screen.
  const [loading, setLoading] = useState(true)
  // Archived view: shows only archived cards (with a restore action) instead of
  // the active board. The backend excludes archived from the default list, so the
  // archived view asks for the full list (?archived=1) and keeps just the archived.
  const [showArchived, setShowArchived] = useState(false)

  const reload = () =>
    api
      .listTasks(showArchived)
      .then((list) => setTasks(showArchived ? list.filter((t) => t.archived) : list))
      .catch((e) => onError(e.message))
      .finally(() => setLoading(false))

  // Columns and saved views live in the same settings document, so one GET
  // serves both.
  const loadColumns = () =>
    api
      .getWorkspaceSettings()
      .then((s) => {
        if (s.boardColumns && s.boardColumns.length > 0) {
          setColumns(s.boardColumns)
        }
        setSavedViews(s.boardViews ?? [])
      })
      .catch(() => {
        // non-fatal: keep defaults
      })

  useEffect(() => {
    reload()
    loadColumns()
    api
      .listFlows()
      .then(setFlows)
      .catch((e) => onError(e.message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Cross-window live sync: App.tsx's central SSE handler bumps the 'board'
  // refresh signal on every task CRUD / board column change in the active
  // workspace (200ms debounced). We re-pull BOTH the task list and the
  // columns since a "board" event could be either, and a single GET per
  // panel keeps the wire cheap.
  const boardTick = useRefreshTrigger('board')
  useEffect(() => {
    reload()
    loadColumns()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [boardTick])

  // Reload when switching between the active board and the archived view.
  useEffect(() => {
    setLoading(true)
    reload()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showArchived])

  const saveColumns = async (cols: BoardColumnDef[]) => {
    const updated = await api.updateWorkspaceSettings({ boardColumns: cols })
    setColumns(updated.boardColumns ?? cols)
    setEditorOpen(false)
    // Tell the Network screen so its live-mode column anchors can refresh
    // immediately (without waiting for an autonomous task event).
    window.dispatchEvent(new CustomEvent('tionharness:board-columns-changed'))
  }

  // showHint displays a transient message under the filter bar (auto-clearing),
  // optionally with an undo action.
  const showHint = (text: string, undo?: () => void) => {
    if (hintTimer.current) window.clearTimeout(hintTimer.current)
    setHint({ text, undo })
    hintTimer.current = window.setTimeout(() => setHint(null), 6000)
  }
  useEffect(
    () => () => {
      if (hintTimer.current) window.clearTimeout(hintTimer.current)
    },
    [],
  )

  // applyPatch optimistically applies a task edit and persists it, rolling the
  // board back to server truth if the write fails.
  const applyPatch = async (task: Task, patch: TaskPatch) => {
    setTasks((prev) =>
      prev.map((t) => (t.id === task.id ? { ...t, ...patch, updatedAt: nowSec() } : t)),
    )
    try {
      await api.updateTask(task.id, patch)
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  // Attach OS-dropped files to a card: upload each into the workspace, save it as
  // an artifact, then append the new artifact ids to the task and persist.
  const attachFilesToTask = async (task: Task, files: File[]) => {
    if (files.length === 0 || task.id.startsWith('temp-')) return
    const newIds: string[] = []
    for (const file of files) {
      try {
        const att = await api.uploadFile(task.id, file)
        const a = await api.createArtifact({
          title: att.name,
          kind: artifactKindForUpload(att),
          content: att.textContent ?? '',
          sourcePath: att.relPath,
          origin: 'manual',
        })
        newIds.push(a.id)
      } catch (e) {
        onError(`"${file.name}" eklenemedi: ${(e as Error).message}`)
      }
    }
    if (newIds.length === 0) return
    const artifactIds = [...(task.artifactIds ?? []), ...newIds]
    setTasks((prev) => prev.map((t) => (t.id === task.id ? { ...t, artifactIds } : t)))
    try {
      await api.updateTask(task.id, { artifactIds })
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  // Upsert: a created task is prepended, an edited task replaced in place.
  const onSaved = (saved: Task) => {
    setTasks((prev) =>
      prev.some((t) => t.id === saved.id)
        ? prev.map((t) => (t.id === saved.id ? saved : t))
        : [saved, ...prev],
    )
  }

  const onDeleted = (id: string) => {
    setTasks((prev) => prev.filter((t) => t.id !== id))
  }

  // Reconcile an optimistic create: drop the temp card and upsert the server row
  // (upsert, not blind prepend, so a concurrent SSE-driven reload that already
  // pulled the real task can't produce a duplicate). real = null removes the
  // temp card when the create failed.
  const onReplaceTemp = (tempId: string, real: Task | null) => {
    setTasks((prev) => {
      const rest = prev.filter((t) => t.id !== tempId)
      if (!real) return rest
      return rest.some((t) => t.id === real.id)
        ? rest.map((t) => (t.id === real.id ? real : t))
        : [real, ...rest]
    })
  }

  const modalTask = modal?.taskId ? (tasks.find((t) => t.id === modal.taskId) ?? null) : null

  // ---- View layer: filter → derive columns → sort ------------------------

  const view = useBoardView(savedViews, setSavedViews, onError)
  const { filter, groupBy, sort } = view.live

  // The filter runs against the FULL list because dependency state is relational
  // (a card's blocked-ness depends on cards the filter may have hidden).
  const visible = useMemo(() => filterTasks(tasks, filter), [tasks, filter])

  // Columns are derived from the grouping axis; 'status' returns the workspace's
  // configured column set verbatim. Derived from the filtered list so an axis
  // does not sprout columns for cards the filter excluded.
  const derivedColumns = useMemo(
    () => deriveColumns(groupBy, visible, agents, columns),
    [groupBy, visible, agents, columns],
  )

  // Topo levels only matter for the dependency sort, and are computed over the
  // full list so a hidden blocker still pushes its dependents down.
  const levels = useMemo(() => (sort === 'deps' ? topoLevels(tasks) : null), [sort, tasks])

  const today = todayISO()

  // The rendered cards, per column key, in their final order.
  const cardsByColumn = useMemo(() => {
    const m = new Map<string, Task[]>()
    for (const col of derivedColumns) m.set(col.key, [])
    for (const t of visible) {
      for (const key of columnKeysOf(t, groupBy, today)) {
        m.get(key)?.push(t)
      }
    }
    for (const [key, arr] of m) m.set(key, sortTasks(arr, sort, levels))
    return m
  }, [derivedColumns, visible, groupBy, sort, levels, today])

  // Everything a card shows that is DERIVED rather than on the task itself, keyed
  // by task id and computed once per data change.
  //
  // It used to be inline in the render loop: each card ran agents.find +
  // flows.find, then a tasks.find PER DEPENDENCY. That is O(cards × deps) linear
  // scans, redone on every board render — and the board re-renders on drag-over,
  // file-drop hover and selection ticks, none of which can change any of it. On a
  // large board with dependencies that is what makes dragging feel heavy.
  const cardMeta = useMemo(() => {
    const taskById = new Map(tasks.map((t) => [t.id, t]))
    const agentById = new Map(agents.map((a) => [a.id, a]))
    const flowById = new Map(flows.map((f) => [f.id, f]))
    // Dependency chips colour by the STATUS column of the blocker, whatever the
    // current grouping axis is — so this reads `columns`, not derivedColumns.
    const colorByState = new Map(columns.map((c) => [c.key, c.color ?? null]))

    const m = new Map<string, TaskCardMeta>()
    for (const t of tasks) {
      const depIds = parseDeps(t.dependencies)
      const unmetDeps = depIds.filter((id) => {
        const dep = taskById.get(id)
        return dep && dep.boardState !== 'done'
      })
      const firstUnmet = unmetDeps.length > 0 ? taskById.get(unmetDeps[0]) : undefined
      m.set(t.id, {
        owner: agentById.get(t.ownerAgentId),
        flow: t.flowId ? flowById.get(t.flowId) : undefined,
        depIds,
        unmetDeps,
        unmetColColor: firstUnmet ? (colorByState.get(firstUnmet.boardState) ?? null) : null,
      })
    }
    return m
  }, [tasks, agents, flows, columns])

  // Multi-select (Ctrl/Cmd+Click, Shift-range) for bulk move/assign/delete.
  // The ordered id list mirrors the on-screen render order (column by column,
  // each column in its current sort) so Shift+Click ranges are predictable —
  // and, critically, it is built from the VISIBLE cards only, so a Shift range
  // can never sweep up a card the filter is hiding.
  const sel = useMultiSelect()
  const orderedIds = useMemo(
    () => derivedColumns.flatMap((col) => (cardsByColumn.get(col.key) ?? []).map((t) => t.id)),
    [derivedColumns, cardsByColumn],
  )

  // Card handlers, given stable identities so TaskCard's memo actually holds:
  // a memoized card re-renders when ANY prop changes identity, and handlers
  // declared inline in the render loop are fresh functions every time.
  //
  // The file-drop pair uses functional setState rather than reading fileDropId,
  // which is what keeps these independent of the very state they update — a
  // dependency on it would rebuild the callbacks on every hover tick and defeat
  // the memo for the whole board.
  const onCardDragStart = useStableCallback((taskId: string) => setDragId(taskId))!
  // Drag can end without a drop (Esc, dropped outside any column) — always
  // clear dragId so a stale id can't cause the next unrelated drop to move
  // the wrong card.
  const onCardDragEnd = useStableCallback(() => setDragId(null))!
  const onCardOpenOrSelect = useStableCallback((e: React.MouseEvent, taskId: string) => {
    if (sel.handleClick(e, taskId, orderedIds)) return
    setModal({ mode: 'edit', taskId })
  })!
  const onCardFileDragEnter = useStableCallback((taskId: string) =>
    setFileDropId((cur) => (cur === taskId ? cur : taskId)),
  )!
  const onCardFileDragLeave = useStableCallback((taskId: string) =>
    setFileDropId((cur) => (cur === taskId ? null : cur)),
  )!
  const onCardFileDrop = useStableCallback((task: Task, files: File[]) => {
    setFileDropId(null)
    void attachFilesToTask(task, files)
  })!
  // Stable so TaskCard's memo holds. Only wired into cards in the archived view.
  const onCardUnarchive = useStableCallback((task: Task) => void setArchived(task, false))!

  // Dropping a card onto a column writes whatever field the current axis names.
  // A refused drop (the 'due' axis cannot invent a date) says so instead of
  // silently doing nothing.
  //
  // announce is skipped for mouse drag (the card lands in its new column,
  // visibly) and passed for the keyboard shortcut, whose only feedback is
  // otherwise a silent DOM reorder — a screen reader needs the aria-live hint
  // to know the move happened at all.
  const handleDrop = (task: Task, columnKey: string, announce?: string) => {
    const patch = dropPatch(groupBy, columnKey, task)
    if (!patch) {
      const reason = DROP_REFUSED_REASON[groupBy]
      if (reason) showHint(reason)
      return
    }
    // Snapshot only the fields the patch touches, so undo restores exactly what
    // the drop changed.
    const before = Object.fromEntries(
      Object.keys(patch).map((k) => [k, task[k as keyof Task]]),
    ) as TaskPatch
    void applyPatch(task, patch)
    // If the moved card no longer matches the filter it disappears on the spot.
    // Say so — a silently vanishing card reads as data loss.
    const after = { ...task, ...patch }
    const stillVisible = filterTasks(
      [...tasks.filter((t) => t.id !== task.id), after],
      filter,
    ).some((t) => t.id === task.id)
    if (!stillVisible) {
      showHint(`"${task.title}" filtre dışında kaldı`, () => void applyPatch(task, before))
    } else if (announce) {
      showHint(announce)
    }
  }

  // Keyboard equivalent of dragging a card onto an adjacent column, used by
  // TaskCard's ArrowLeft/ArrowRight handler. Reuses handleDrop so the write and
  // the "moved out of filter" safety net stay in one place.
  const onCardMoveColumn = useStableCallback((taskId: string, direction: -1 | 1) => {
    const task = tasks.find((x) => x.id === taskId)
    if (!task) return
    const keys = derivedColumns.map((c) => c.key)
    const currentKey = columnKeysOf(task, groupBy, today)[0]
    const idx = keys.indexOf(currentKey)
    if (idx === -1) return
    const nextIdx = idx + direction
    if (nextIdx < 0 || nextIdx >= keys.length) return
    const nextCol = derivedColumns[nextIdx]
    handleDrop(task, nextCol.key, `"${task.title}", ${nextCol.label} sütununa taşındı`)
  })!

  const bulkMove = async (boardState: string) => {
    if (!boardState) return
    const ids = [...sel.selected]
    setTasks((prev) =>
      prev.map((t) => (sel.selected.has(t.id) ? { ...t, boardState, updatedAt: nowSec() } : t)),
    )
    sel.clear()
    try {
      await Promise.all(ids.map((id) => api.updateTask(id, { boardState })))
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }
  const bulkAssign = async (ownerAgentId: string) => {
    const ids = [...sel.selected]
    setTasks((prev) =>
      prev.map((t) => (sel.selected.has(t.id) ? { ...t, ownerAgentId, updatedAt: nowSec() } : t)),
    )
    sel.clear()
    try {
      await Promise.all(ids.map((id) => api.updateTask(id, { ownerAgentId })))
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }
  const bulkDelete = async () => {
    const ids = [...sel.selected]
    if (ids.length === 0) return
    if (!confirm(`${ids.length} görev silinsin mi?`)) return
    setTasks((prev) => prev.filter((t) => !sel.selected.has(t.id)))
    sel.clear()
    try {
      await Promise.all(ids.map((id) => api.deleteTask(id)))
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  // Archive or restore a single card. Either way it leaves the CURRENT view (an
  // archived card drops off the active board; a restored card drops off the
  // archived view), so we optimistically remove it and roll back on failure.
  const setArchived = async (task: Task, archived: boolean) => {
    setTasks((prev) => prev.filter((t) => t.id !== task.id))
    try {
      await api.archiveTask(task.id, archived)
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  const bulkArchive = async (archived: boolean) => {
    const ids = [...sel.selected]
    if (ids.length === 0) return
    setTasks((prev) => prev.filter((t) => !sel.selected.has(t.id)))
    sel.clear()
    try {
      await Promise.all(ids.map((id) => api.archiveTask(id, archived)))
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  // Task count per column key — used by the editor to guard against deleting
  // non-empty columns.
  const taskCountByColumn: Record<string, number> = {}
  for (const t of tasks) {
    taskCountByColumn[t.boardState] = (taskCountByColumn[t.boardState] ?? 0) + 1
  }

  return (
    <div className="flex min-h-0 flex-1">
      {/* Left: column editor panel */}
      {editorOpen && (
        <BoardColumnEditor
          columns={columns}
          taskCountByColumn={taskCountByColumn}
          onSave={saveColumns}
          onClose={() => setEditorOpen(false)}
        />
      )}

      <div className="flex h-full flex-1 flex-col overflow-hidden">
        {/* Top bar: title + board actions. Column editing only makes sense on the
            status axis — the other axes derive their columns from the data. */}
        <PaneHeader
          title={showArchived ? 'Görevler — Arşiv' : 'Görevler'}
          right={
            <>
              {/* The board's projection — the same bytes an agent gets from
                  get_view{kind:'board'}: column histogram + the signals. */}
              <ViewButton target={{ kind: 'board', id: 'board' }} />
              <button
                data-testid="task-board-archived-toggle"
                onClick={() => {
                  sel.clear()
                  setShowArchived((v) => !v)
                }}
                title={showArchived ? 'Aktif panoya dön' : 'Arşivlenenleri göster'}
                className={`flex flex-shrink-0 items-center gap-1 rounded border px-2 py-1 text-xs transition ${
                  showArchived
                    ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                    : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]'
                }`}
              >
                <Archive size={13} /> {showArchived ? 'Panoya dön' : 'Arşiv'}
              </button>
              {!showArchived && groupBy === 'status' && (
                <button
                  data-testid="task-board-columns-editor"
                  onClick={() => setEditorOpen((v) => !v)}
                  title="Sütunları düzenle"
                  className={`flex-shrink-0 rounded border px-2 py-1 text-xs transition ${
                    editorOpen
                      ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                      : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]'
                  }`}
                >
                  ⊞ Sütunlar
                </button>
              )}
              {!showArchived && (
                <div data-testid="task-create-submit">
                  <Button onClick={() => setModal({ mode: 'create', taskId: null })}>
                    + Görev
                  </Button>
                </div>
              )}
            </>
          }
        />

        <BoardFilterBar
          view={view}
          tasks={tasks}
          visibleCount={visible.length}
          agents={agents}
          boardColumns={columns}
        />

        {hint && (
          <div
            role="status"
            aria-live="polite"
            className="flex items-center gap-2 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-1.5 text-xs text-[var(--color-text-dim)]"
          >
            <span className="min-w-0 flex-1 truncate">{hint.text}</span>
            {hint.undo && (
              <button
                onClick={() => {
                  hint.undo?.()
                  setHint(null)
                }}
                className="flex-shrink-0 rounded border border-[var(--color-accent)] px-1.5 py-0.5 text-[var(--color-accent)]"
              >
                Geri al
              </button>
            )}
            <button
              onClick={() => setHint(null)}
              className="flex-shrink-0 px-1 text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
            >
              ✕
            </button>
          </div>
        )}

        {showArchived && !loading && (
          <div className="flex items-center gap-2 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-1.5 text-xs text-[var(--color-text-dim)]">
            <Archive size={13} className="flex-shrink-0" />
            <span className="min-w-0 flex-1">
              {tasks.length === 0
                ? 'Arşivlenmiş görev yok.'
                : `${tasks.length} arşivlenmiş görev — bir kartı geri almak için “Geri al”e bas.`}
            </span>
          </div>
        )}

        {/* Board. Hidden (not unmounted) during the first load so column widths and
            scroll position are already settled when the cards appear. */}
        {loading && <LoadingState label="Görevler yükleniyor…" className="flex-1" />}
        <div className={`flex flex-1 gap-3 overflow-x-auto p-4 ${loading ? 'hidden' : ''}`}>
          {derivedColumns.length === 0 && (
            <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
              Bu filtreyle eşleşen görev yok.
            </div>
          )}
          {derivedColumns.map((col, colIdx) => {
            const colTasks = cardsByColumn.get(col.key) ?? []

            return (
              <div
                key={col.key}
                onDragOver={(e) => e.preventDefault()}
                onDrop={(e) => {
                  // An OS file dropped on empty column space (not onto a card)
                  // has no task to attach to — say so instead of silently
                  // discarding it.
                  if (Array.from(e.dataTransfer.types).includes('Files')) {
                    showHint('Dosya eklemek için bir görev kartının üzerine bırakın')
                    setDragId(null)
                    return
                  }
                  const t = tasks.find((x) => x.id === dragId)
                  if (t) handleDrop(t, col.key)
                  setDragId(null)
                }}
                className="flex w-64 flex-shrink-0 flex-col rounded-lg bg-[var(--color-surface)]"
              >
                <div
                  role="heading"
                  aria-level={3}
                  aria-label={`${col.label} sütunu, ${colTasks.length} görev`}
                  className="flex items-center justify-between rounded-t-lg px-3 py-2 text-xs font-medium uppercase tracking-wide"
                  style={
                    col.color
                      ? {
                          backgroundColor: col.color + '22',
                          color: col.color,
                          borderBottom: `2px solid ${col.color}44`,
                        }
                      : undefined
                  }
                >
                  <span className={col.color ? '' : 'text-[var(--color-text-dim)]'}>
                    {col.label}
                  </span>
                  <span
                    className="rounded px-1.5"
                    style={
                      col.color
                        ? { backgroundColor: col.color + '33' }
                        : { backgroundColor: 'var(--color-surface-2)' }
                    }
                  >
                    {colTasks.length}
                  </span>
                </div>
                <div className="flex-1 space-y-2 overflow-y-auto px-2 pb-2 pt-2">
                  {colTasks.map((t) => {
                    // Cards are drawn from `visible`, a subset of `tasks`, so every
                    // card id has an entry — a miss is a real bug, not a case to
                    // paper over with defaults.
                    const meta = cardMeta.get(t.id)!
                    return (
                      <TaskCard
                        key={t.id}
                        task={t}
                        meta={meta}
                        selected={sel.isSelected(t.id)}
                        fileDropActive={fileDropId === t.id}
                        today={today}
                        columnIndex={colIdx}
                        columnCount={derivedColumns.length}
                        columnLabel={col.label}
                        onDragStart={onCardDragStart}
                        onDragEnd={onCardDragEnd}
                        onOpenOrSelect={onCardOpenOrSelect}
                        onFileDragEnter={onCardFileDragEnter}
                        onFileDragLeave={onCardFileDragLeave}
                        onFileDrop={onCardFileDrop}
                        onMoveColumn={onCardMoveColumn}
                        onUnarchive={showArchived ? onCardUnarchive : undefined}
                      />
                    )
                  })}
                </div>
              </div>
            )
          })}
        </div>

        <SelectionBar
          count={sel.count}
          onClear={sel.clear}
          onSelectAll={orderedIds.length ? () => sel.selectAll(orderedIds) : undefined}
        >
          <select
            value=""
            onChange={(e) => bulkMove(e.target.value)}
            title="Seçili görevleri sütuna taşı"
            className="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
          >
            <option value="">↦ Sütuna taşı…</option>
            {columns.map((c) => (
              <option key={c.key} value={c.key}>
                {c.label}
              </option>
            ))}
          </select>
          <select
            value=""
            onChange={(e) => bulkAssign(e.target.value)}
            title="Seçili görevlere ajan ata"
            className="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
          >
            <option value="">⊕ Ajan ata…</option>
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
          {showArchived ? (
            <SelectionBarButton
              icon={<ArchiveRestore size={13} />}
              onClick={() => bulkArchive(false)}
            >
              Geri al
            </SelectionBarButton>
          ) : (
            <SelectionBarButton icon={<Archive size={13} />} onClick={() => bulkArchive(true)}>
              Arşivle
            </SelectionBarButton>
          )}
          <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulkDelete} danger>
            Sil
          </SelectionBarButton>
        </SelectionBar>
      </div>

      {modal && (
        <TaskFormModal
          mode={modal.mode}
          task={modalTask ?? undefined}
          agents={agents}
          flows={flows}
          columns={columns}
          tasks={tasks}
          defaultBoardState={columns[0]?.key}
          onClose={() => setModal(null)}
          onSaved={onSaved}
          onReplaceTemp={onReplaceTemp}
          onDeleted={onDeleted}
          onArchived={onDeleted}
          onError={onError}
        />
      )}
    </div>
  )
}
