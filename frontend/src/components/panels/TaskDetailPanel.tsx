import { useEffect, useState } from 'react'
import { RefreshCw } from 'lucide-react'
import { api } from '../../api'
import type { Agent, Task, Flow, BoardState, BoardColumnDef } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'
import { DependencyPicker } from './DependencyPicker'
import { useResizableWidth } from '../../hooks/useResizableWidth'

function parseDeps(raw: string): string[] {
  try {
    const arr = JSON.parse(raw || '[]')
    return Array.isArray(arr) ? (arr as string[]) : []
  } catch {
    return []
  }
}

interface Props {
  task: Task
  agents: Agent[]
  flows: Flow[]
  // Workspace column definitions — drives the status pill selector.
  columns: BoardColumnDef[]
  // All board tasks — used for the dependency picker and chip navigation.
  tasks?: Task[]
  onClose: () => void
  // Called with the persisted task so the board can update its copy in place.
  onSaved: (task: Task) => void
  // Called after a successful delete so the board can drop the card.
  onDeleted: (id: string) => void
  onError: (msg: string) => void
  // Open another task's detail panel (dependency chip click).
  onSelectTask?: (id: string) => void
}

// TaskDetailPanel is the right-hand inspector/editor for a single Kanban card.
// The board is a passive status surface: a task is described, columned, and
// optionally tagged with an agent and a flow (informational). It is never run
// from here — flows, schedules and agent sessions read and update tasks from
// outside. So this panel only edits title/description/owner/flow/column/deps.
export function TaskDetailPanel({ task, agents, flows, columns, tasks = [], onClose, onSaved, onDeleted, onError, onSelectTask }: Props) {
  // Drag-to-resize width (left-edge handle, right-docked panel), persisted.
  const { width, dragging, onHandleDown } = useResizableWidth({
    storageKey: 'taskDetailPanelWidth',
    defaultWidth: 384,
    min: 320,
    max: 760,
  })
  const [title, setTitle] = useState(task.title)
  const [description, setDescription] = useState(task.description)
  const [ownerAgentId, setOwnerAgentId] = useState(task.ownerAgentId)
  const [flowId, setFlowId] = useState(task.flowId)
  const [boardState, setBoardState] = useState<BoardState>(task.boardState)
  const [depIds, setDepIds] = useState<string[]>(() => parseDeps(task.dependencies))
  const [saving, setSaving] = useState(false)
  const [retitling, setRetitling] = useState(false)

  // Reseed the form when the selected card changes (panel stays mounted).
  useEffect(() => {
    setTitle(task.title)
    setDescription(task.description)
    setOwnerAgentId(task.ownerAgentId)
    setFlowId(task.flowId)
    setBoardState(task.boardState)
    setDepIds(parseDeps(task.dependencies))
  }, [task])

  const currentDepsJSON = JSON.stringify(depIds.slice().sort())
  const savedDepsJSON = JSON.stringify(parseDeps(task.dependencies).slice().sort())

  const dirty =
    title !== task.title ||
    description !== task.description ||
    ownerAgentId !== task.ownerAgentId ||
    flowId !== task.flowId ||
    boardState !== task.boardState ||
    currentDepsJSON !== savedDepsJSON

  const save = async () => {
    setSaving(true)
    try {
      const updated = await api.updateTask(task.id, {
        title: title.trim(),
        description: description.trim(),
        ownerAgentId,
        flowId,
        boardState,
        dependencies: JSON.stringify(depIds),
      })
      onSaved(updated)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  // Regenerate the title from the task's description (server-side AI title).
  const retitle = async () => {
    setRetitling(true)
    try {
      const updated = await api.generateTaskTitle(task.id)
      onSaved(updated)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setRetitling(false)
    }
  }

  const remove = async () => {
    if (!confirm(`"${task.title}" silinsin mi?`)) return
    try {
      await api.deleteTask(task.id)
      onDeleted(task.id)
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // Tasks available as dependencies: all tasks except the current one.
  const depCandidates = tasks.filter((t) => t.id !== task.id)

  return (
    <aside
      style={{ width }}
      className="relative flex h-full shrink-0 flex-col overflow-y-auto border-l border-[var(--color-border)] bg-[var(--color-surface)]"
    >
      {/* Drag handle on the left edge to resize the panel. */}
      <div
        onPointerDown={onHandleDown}
        title="Sürükleyerek genişliği ayarla"
        className={`absolute left-0 top-0 z-10 h-full w-1 cursor-col-resize transition-colors hover:bg-[var(--color-accent)] ${
          dragging ? 'bg-[var(--color-accent)]' : 'bg-transparent'
        }`}
      />
      <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
        <span className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
          Görev detayı
        </span>
        <button
          onClick={onClose}
          title="Paneli kapat"
          className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
        >
          ✕
        </button>
      </div>

      <div className="flex flex-1 flex-col gap-4 px-4 py-4">
        <Field label="Başlık">
          <div className="flex items-center gap-1.5">
            <input
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
            />
            <button
              onClick={retitle}
              disabled={retitling}
              title="AI ile başlığı açıklamadan yeniden oluştur"
              className="shrink-0 rounded border border-[var(--color-border)] px-2 py-1.5 text-sm text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:opacity-30"
            >
              {retitling ? '…' : <RefreshCw size={14} />}
            </button>
          </div>
        </Field>

        <Field label="Açıklama — başlık bundan üretilir">
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={6}
            className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </Field>

        <Field label="Ajan (opsiyonel, bilgi)">
          <AgentPicker
            agents={agents}
            value={ownerAgentId}
            onChange={setOwnerAgentId}
            placeholder="Ajan seç (opsiyonel)"
          />
        </Field>

        <Field label="Akış (opsiyonel, bilgi)">
          <select
            value={flowId}
            onChange={(e) => setFlowId(e.target.value)}
            className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          >
            <option value="">🔀 Akış yok</option>
            {flows.map((f) => (
              <option key={f.id} value={f.id}>
                🔀 {f.name}
              </option>
            ))}
          </select>
        </Field>

        {/* Dependencies: tasks that must complete before this one. */}
        <Field label="Bağımlılıklar — önce tamamlanması gereken görevler">
          <DependencyPicker
            tasks={depCandidates}
            value={depIds}
            onChange={setDepIds}
          />
          {depIds.length > 0 && (
            <div className="mt-1.5 flex flex-wrap gap-1.5">
              {depIds.map((depId) => {
                const dep = tasks.find((t) => t.id === depId)
                if (!dep) return null
                const done = dep.boardState === 'done'
                return (
                  <button
                    key={depId}
                    onClick={() => onSelectTask?.(depId)}
                    title="Bu göreve git"
                    className={`inline-flex items-center gap-1 rounded px-2 py-0.5 text-[11px] transition hover:opacity-75 ${
                      done
                        ? 'bg-green-500/15 text-green-400'
                        : 'bg-[var(--color-warning)]/15 text-[var(--color-warning)]'
                    }`}
                  >
                    {done ? '✓' : '⏳'}{' '}
                    {dep.title || dep.description || 'Görev'}
                  </button>
                )
              })}
            </div>
          )}
        </Field>

        <Field label="Durum (kolon)">
          <div className="flex flex-wrap gap-1.5">
            {columns.map((col) => {
              const active = boardState === col.key
              return (
                <button
                  key={col.key}
                  onClick={() => setBoardState(col.key as BoardState)}
                  className={`rounded-full px-2.5 py-1 text-xs transition ${
                    active
                      ? 'text-white'
                      : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
                  }`}
                  style={
                    active
                      ? { backgroundColor: col.color || 'var(--color-accent)' }
                      : col.color
                      ? { borderLeft: `3px solid ${col.color}`, paddingLeft: '6px' }
                      : undefined
                  }
                >
                  {col.label}
                </button>
              )
            })}
          </div>
        </Field>

        <div className="flex items-center gap-2 border-t border-[var(--color-border)] pt-3">
          <button
            onClick={save}
            disabled={!dirty || saving}
            className="rounded bg-[var(--color-accent)] px-3 py-1.5 text-sm font-medium text-white transition hover:opacity-90 disabled:opacity-40"
          >
            {saving ? 'Kaydediliyor…' : 'Kaydet'}
          </button>
          {dirty && !saving && (
            <span className="text-[11px] text-[var(--color-warning)]">kaydedilmemiş değişiklik</span>
          )}
        </div>

        {/* Danger zone */}
        <div className="mt-auto border-t border-[var(--color-border)] pt-3">
          <button
            onClick={remove}
            className="w-full rounded px-3 py-1.5 text-sm text-[var(--color-danger)] transition hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]"
          >
            🗑 Görevi sil
          </button>
        </div>
      </div>
    </aside>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="flex flex-col gap-1.5">
      <span className="text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
        {label}
      </span>
      {children}
    </label>
  )
}
