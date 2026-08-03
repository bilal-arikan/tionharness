import { useEffect, useMemo, useRef, useState } from 'react'
import { Paperclip, Trash2 } from 'lucide-react'
import { api } from '@/api'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import type { Agent, Task, TaskPatch, Flow, BoardColumnDef, BoardViewDef } from '@/types'
import { artifactKindForUpload } from '@/features/artifacts/artifactMeta'
import { AgentIdentity } from '@/shared/components/agents/AgentIdentity'
import { normalizeAvatar } from '@/shared/lib/avatar'
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

// Priority chip colors/labels, keyed by the stored priority slug.
const PRIORITY_META: Record<string, { label: string; color: string }> = {
  critical: { label: 'Kritik', color: '#ef4444' },
  high: { label: 'Yüksek', color: '#f59e0b' },
  medium: { label: 'Orta', color: '#3b82f6' },
  low: { label: 'Düşük', color: '#6b7280' },
}

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

  const reload = () =>
    api
      .listTasks()
      .then(setTasks)
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

  const saveColumns = async (cols: BoardColumnDef[]) => {
    const updated = await api.updateWorkspaceSettings({ boardColumns: cols })
    setColumns(updated.boardColumns ?? cols)
    setEditorOpen(false)
    // Tell the Network screen so its live-mode column anchors can refresh
    // immediately (without waiting for an autonomous task event).
    window.dispatchEvent(new CustomEvent('tionswarm:board-columns-changed'))
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

    const m = new Map<
      string,
      {
        owner: Agent | undefined
        flow: Flow | undefined
        depIds: string[]
        unmetDeps: string[]
        unmetColColor: string | null
      }
    >()
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

  // Dropping a card onto a column writes whatever field the current axis names.
  // A refused drop (the 'due' axis cannot invent a date) says so instead of
  // silently doing nothing.
  const handleDrop = (task: Task, columnKey: string) => {
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
    }
  }

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
          title="Görevler"
          right={
            <>
              {groupBy === 'status' && (
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
              <div data-testid="task-create-submit">
                <Button onClick={() => setModal({ mode: 'create', taskId: null })}>+ Görev</Button>
              </div>
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
          <div className="flex items-center gap-2 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-1.5 text-xs text-[var(--color-text-dim)]">
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

        {/* Board. Hidden (not unmounted) during the first load so column widths and
            scroll position are already settled when the cards appear. */}
        {loading && <LoadingState label="Görevler yükleniyor…" className="flex-1" />}
        <div className={`flex flex-1 gap-3 overflow-x-auto p-4 ${loading ? 'hidden' : ''}`}>
          {derivedColumns.length === 0 && (
            <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
              Bu filtreyle eşleşen görev yok.
            </div>
          )}
          {derivedColumns.map((col) => {
            const colTasks = cardsByColumn.get(col.key) ?? []

            return (
              <div
                key={col.key}
                onDragOver={(e) => e.preventDefault()}
                onDrop={() => {
                  const t = tasks.find((x) => x.id === dragId)
                  if (t) handleDrop(t, col.key)
                  setDragId(null)
                }}
                className="flex w-64 flex-shrink-0 flex-col rounded-lg bg-[var(--color-surface)]"
              >
                <div
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
                    const { owner, flow, depIds, unmetDeps, unmetColColor } = meta
                    const pending = t.id.startsWith('temp-')
                    return (
                      <div
                        key={t.id}
                        data-testid="task-card"
                        data-task-id={t.id}
                        draggable={!pending}
                        onDragStart={(e) => {
                          if (pending) return
                          setDragId(t.id)
                          e.dataTransfer.setData('application/x-tionswarm-task', t.id)
                          e.dataTransfer.effectAllowed = 'link'
                        }}
                        onClick={(e) => {
                          if (pending) return
                          if (sel.handleClick(e, t.id, orderedIds)) return
                          setModal({ mode: 'edit', taskId: t.id })
                        }}
                        onDragOver={(e) => {
                          // OS file drag over a card → offer to attach (a card being
                          // dragged internally carries no 'Files', so moves are
                          // unaffected and still bubble to the column).
                          if (pending || !Array.from(e.dataTransfer.types).includes('Files')) return
                          e.preventDefault()
                          e.stopPropagation()
                          if (fileDropId !== t.id) setFileDropId(t.id)
                        }}
                        onDragLeave={(e) => {
                          if (!Array.from(e.dataTransfer.types).includes('Files')) return
                          if (fileDropId === t.id) setFileDropId(null)
                        }}
                        onDrop={(e) => {
                          const files = Array.from(e.dataTransfer.files)
                          if (files.length === 0) return // not a file drop → let the column handle the move
                          e.preventDefault()
                          e.stopPropagation()
                          setFileDropId(null)
                          void attachFilesToTask(t, files)
                        }}
                        className={`rounded-lg border bg-[var(--color-surface-2)] p-2 text-sm shadow-[var(--shadow-sm)] transition ${
                          fileDropId === t.id
                            ? 'ring-2 ring-[var(--color-accent)] ring-offset-1'
                            : ''
                        } ${
                          pending
                            ? 'animate-pulse cursor-default border-[var(--color-border)] opacity-70'
                            : `cursor-pointer hover:shadow-[var(--shadow-md)] active:cursor-grabbing ${
                                sel.isSelected(t.id)
                                  ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] ring-1 ring-[var(--color-accent)]'
                                  : 'border-[var(--color-border)] hover:border-[var(--color-accent)]'
                              }`
                        }`}
                      >
                        <div className="font-medium">{t.title}</div>
                        {pending ? (
                          <div className="mt-1 text-[11px] text-[var(--color-text-dim)]">
                            başlık üretiliyor…
                          </div>
                        ) : (
                          t.description && (
                            <div className="mt-1 line-clamp-2 text-xs text-[var(--color-text-dim)]">
                              {t.description}
                            </div>
                          )
                        )}
                        {/* Rich attribute badges: due date, priority, tags. */}
                        {(t.dueDate || t.priority || (t.tags?.length ?? 0) > 0) && (
                          <div className="mt-1.5 flex flex-wrap items-center gap-1">
                            {t.dueDate && (
                              <span
                                title={`Bitiş: ${t.dueDate}`}
                                className={`inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium ${
                                  t.dueDate < today
                                    ? 'bg-[var(--color-danger)]/15 text-[var(--color-danger)]'
                                    : t.dueDate === today
                                      ? 'bg-[var(--color-warning)]/15 text-[var(--color-warning)]'
                                      : 'bg-[var(--color-surface)] text-[var(--color-text-dim)]'
                                }`}
                              >
                                ◷ {t.dueDate.slice(5)}
                              </span>
                            )}
                            {t.priority && PRIORITY_META[t.priority] && (
                              <span
                                className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium"
                                style={{
                                  backgroundColor: PRIORITY_META[t.priority].color + '22',
                                  color: PRIORITY_META[t.priority].color,
                                }}
                              >
                                ● {PRIORITY_META[t.priority].label}
                              </span>
                            )}
                            {t.tags?.map((tag) => (
                              <span
                                key={tag}
                                className="rounded-full bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]"
                              >
                                #{tag}
                              </span>
                            ))}
                          </div>
                        )}
                        {(owner ||
                          t.flowId ||
                          depIds.length > 0 ||
                          (t.artifactIds?.length ?? 0) > 0) && (
                          <div className="mt-2 flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
                            {owner && (
                              <AgentIdentity agent={owner} size="sm" className="max-w-[160px]" />
                            )}
                            {(t.artifactIds?.length ?? 0) > 0 && (
                              <span
                                className="inline-flex items-center gap-0.5 rounded bg-[var(--color-surface)] px-1.5 py-0.5 text-[10px]"
                                title={`${t.artifactIds!.length} ek (artifact)`}
                              >
                                <Paperclip size={10} /> {t.artifactIds!.length}
                              </span>
                            )}
                            {t.flowId && (
                              <span className="inline-flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]">
                                {normalizeAvatar(flow?.emoji) ?? '🔀'} {flow?.name ?? 'Akış'}
                              </span>
                            )}
                            {depIds.length > 0 &&
                              (() => {
                                // unmetColColor (the first unmet dependency's column
                                // colour) comes precomputed from cardMeta.
                                const chipStyle =
                                  unmetDeps.length > 0 && unmetColColor
                                    ? {
                                        backgroundColor: unmetColColor + '22',
                                        color: unmetColColor,
                                      }
                                    : undefined
                                return (
                                  <span
                                    className={`inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] ${
                                      unmetDeps.length > 0 && !unmetColColor
                                        ? 'bg-[var(--color-warning)]/15 text-[var(--color-warning)]'
                                        : unmetDeps.length > 0
                                          ? ''
                                          : 'bg-[var(--color-success)]/10 text-[var(--color-success)]'
                                    }`}
                                    style={chipStyle}
                                    title={
                                      unmetDeps.length > 0
                                        ? `${unmetDeps.length} bağımlılık tamamlanmadı`
                                        : 'Tüm bağımlılıklar tamamlandı'
                                    }
                                  >
                                    🔗 {depIds.length}
                                  </span>
                                )
                              })()}
                          </div>
                        )}
                      </div>
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
          onError={onError}
        />
      )}
    </div>
  )
}
