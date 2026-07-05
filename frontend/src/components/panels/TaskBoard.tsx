import { useEffect, useMemo, useState } from 'react'
import { Trash2 } from 'lucide-react'
import { api } from '../../api'
import type { Agent, Task, Flow, BoardColumnDef } from '../../types'
import { AgentIdentity } from '../agents/AgentIdentity'
import { normalizeAvatar } from '../../lib/avatar'
import { TaskFormModal } from './TaskFormModal'
import { BoardColumnEditor } from './BoardColumnEditor'
import { Button, SelectionBar, SelectionBarButton, PaneHeader } from '../common'
import { useMultiSelect } from '../../hooks/useMultiSelect'

// Fallback columns used until workspace settings are loaded.
const DEFAULT_COLUMNS: BoardColumnDef[] = [
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

// Parse a task's dependencies JSON string into an array of task IDs.
function parseDeps(raw: string): string[] {
  try {
    const arr = JSON.parse(raw || '[]')
    return Array.isArray(arr) ? (arr as string[]) : []
  } catch {
    return []
  }
}

// Compute topological levels so tasks with no blockers sort first (level 0).
// Cycles are broken by assigning level 0 to the repeated node.
function topoLevels(tasks: Task[]): Map<string, number> {
  const depsOf = new Map<string, string[]>()
  for (const t of tasks) depsOf.set(t.id, parseDeps(t.dependencies))
  const levels = new Map<string, number>()
  const visiting = new Set<string>()
  function level(id: string): number {
    if (levels.has(id)) return levels.get(id)!
    if (visiting.has(id)) return 0
    visiting.add(id)
    const deps = depsOf.get(id) ?? []
    const l = deps.length === 0 ? 0 : Math.max(...deps.map((d) => level(d) + 1))
    visiting.delete(id)
    levels.set(id, l)
    return l
  }
  for (const t of tasks) level(t.id)
  return levels
}

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
}

export function TaskBoard({ agents, onError }: Props) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [flows, setFlows] = useState<Flow[]>([])
  const [columns, setColumns] = useState<BoardColumnDef[]>(DEFAULT_COLUMNS)
  const [dragId, setDragId] = useState<string | null>(null)
  // Create/edit popup state: null = closed.
  const [modal, setModal] = useState<{ mode: 'create' | 'edit'; taskId: string | null } | null>(null)
  // Left-side column editor panel.
  const [editorOpen, setEditorOpen] = useState(false)
  // When true, cards sort by topological dependency order (no-blocker tasks first).
  const [depSort, setDepSort] = useState(false)

  const reload = () =>
    api.listTasks().then(setTasks).catch((e) => onError(e.message))

  const loadColumns = () =>
    api
      .getWorkspaceSettings()
      .then((s) => {
        if (s.boardColumns && s.boardColumns.length > 0) {
          setColumns(s.boardColumns)
        }
      })
      .catch(() => {
        // non-fatal: keep defaults
      })

  useEffect(() => {
    reload()
    loadColumns()
    api.listFlows().then(setFlows).catch((e) => onError(e.message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const saveColumns = async (cols: BoardColumnDef[]) => {
    const updated = await api.updateWorkspaceSettings({ boardColumns: cols })
    setColumns(updated.boardColumns ?? cols)
    setEditorOpen(false)
    // Tell the Network screen so its live-mode column anchors can refresh
    // immediately (without waiting for an autonomous task event).
    window.dispatchEvent(new CustomEvent('tionswarm:board-columns-changed'))
  }

  const move = async (task: Task, boardState: string) => {
    if (task.boardState === boardState) return
    setTasks((prev) =>
      prev.map((t) =>
        t.id === task.id ? { ...t, boardState, updatedAt: nowSec() } : t,
      ),
    )
    try {
      await api.updateTask(task.id, { boardState })
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

  const modalTask = modal?.taskId ? tasks.find((t) => t.id === modal.taskId) ?? null : null

  // Multi-select (Ctrl/Cmd+Click, Shift-range) for bulk move/assign/delete.
  // The ordered id list mirrors the on-screen render order (column by column,
  // each column in its current sort) so Shift+Click ranges are predictable.
  const sel = useMultiSelect()
  const orderedIds = useMemo(() => {
    const lv = depSort ? topoLevels(tasks) : null
    return columns.flatMap((col) => {
      const arr = tasks.filter((t) => t.boardState === col.key)
      arr.sort((a, b) => {
        if (lv) {
          const la = lv.get(a.id) ?? 0
          const lb = lv.get(b.id) ?? 0
          if (la !== lb) return la - lb
        }
        return b.updatedAt - a.updatedAt
      })
      return arr.map((t) => t.id)
    })
  }, [tasks, columns, depSort])

  const bulkMove = async (boardState: string) => {
    if (!boardState) return
    const ids = [...sel.selected]
    setTasks((prev) => prev.map((t) => (sel.selected.has(t.id) ? { ...t, boardState, updatedAt: nowSec() } : t)))
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
    setTasks((prev) => prev.map((t) => (sel.selected.has(t.id) ? { ...t, ownerAgentId, updatedAt: nowSec() } : t)))
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

  // Precompute topo levels once when dep-sort is active.
  const levels = depSort ? topoLevels(tasks) : null

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
        {/* Top bar: title + board actions (Sütunlar / + Görev / Sırala). */}
        <PaneHeader
          title="Görevler"
          right={
            <>
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
              <div data-testid="task-create-submit">
                <Button onClick={() => setModal({ mode: 'create', taskId: null })}>+ Görev</Button>
              </div>
              <button
                data-testid="task-sort-by-deps"
                onClick={() => {
                  if (!depSort && !confirm('Görevler bağımlılık sırasına göre yeniden dizilecek. Devam edilsin mi?')) return
                  setDepSort((v) => !v)
                }}
                title={depSort ? 'Bağımlılık sıralamasını kapat' : 'Bağımlılığa göre sırala — önce bağımlısı olmayanlar'}
                className={`rounded border px-2 py-1 text-xs transition ${
                  depSort
                    ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                    : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]'
                }`}
              >
                🔗 Sırala
              </button>
            </>
          }
        />

        {/* Board */}
        <div className="flex flex-1 gap-3 overflow-x-auto p-4">
          {columns.map((col) => {
            const colTasksRaw = tasks.filter((t) => t.boardState === col.key)
            const colTasks = depSort && levels
              ? [...colTasksRaw].sort((a, b) => {
                  const la = levels.get(a.id) ?? 0
                  const lb = levels.get(b.id) ?? 0
                  if (la !== lb) return la - lb
                  return b.updatedAt - a.updatedAt
                })
              : [...colTasksRaw].sort((a, b) => b.updatedAt - a.updatedAt)

            return (
              <div
                key={col.key}
                onDragOver={(e) => e.preventDefault()}
                onDrop={() => {
                  const t = tasks.find((x) => x.id === dragId)
                  if (t) move(t, col.key)
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
                    const owner = agents.find((a) => a.id === t.ownerAgentId)
                    const flow = t.flowId ? flows.find((f) => f.id === t.flowId) : undefined
                    const pending = t.id.startsWith('temp-')
                    const depIds = parseDeps(t.dependencies)
                    const unmetDeps = depIds.filter((id) => {
                      const dep = tasks.find((x) => x.id === id)
                      return dep && dep.boardState !== 'done'
                    })
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
                        className={`rounded-lg border bg-[var(--color-surface-2)] p-2 text-sm shadow-[var(--shadow-sm)] transition ${
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
                        {/* Rich attribute badges: priority, tags. */}
                        {(t.priority || (t.tags?.length ?? 0) > 0) && (
                          <div className="mt-1.5 flex flex-wrap items-center gap-1">
                            {t.priority && PRIORITY_META[t.priority] && (
                              <span
                                className="inline-flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium"
                                style={{ backgroundColor: PRIORITY_META[t.priority].color + '22', color: PRIORITY_META[t.priority].color }}
                              >
                                ● {PRIORITY_META[t.priority].label}
                              </span>
                            )}
                            {t.tags?.map((tag) => (
                              <span key={tag} className="rounded-full bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]">#{tag}</span>
                            ))}
                          </div>
                        )}
                        {(owner || t.flowId || depIds.length > 0) && (
                          <div className="mt-2 flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
                            {owner && <AgentIdentity agent={owner} size="sm" className="max-w-[160px]" />}
                            {t.flowId && (
                              <span className="inline-flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]">
                                {normalizeAvatar(flow?.emoji) ?? '🔀'} {flow?.name ?? 'Akış'}
                              </span>
                            )}
                            {depIds.length > 0 && (() => {
                              // Color the chip based on the column of the first unmet dependency.
                              const firstUnmetTask = unmetDeps.length > 0
                                ? tasks.find((x) => x.id === unmetDeps[0])
                                : null
                              const unmetColColor = firstUnmetTask
                                ? (columns.find((c) => c.key === firstUnmetTask.boardState)?.color ?? null)
                                : null
                              const chipStyle = unmetDeps.length > 0 && unmetColColor
                                ? { backgroundColor: unmetColColor + '22', color: unmetColColor }
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
                                title={unmetDeps.length > 0 ? `${unmetDeps.length} bağımlılık tamamlanmadı` : 'Tüm bağımlılıklar tamamlandı'}
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
          onDeleted={onDeleted}
          onError={onError}
        />
      )}
    </div>
  )
}
