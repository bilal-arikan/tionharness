import { useState } from 'react'
import { RefreshCw, X } from 'lucide-react'
import { api } from '../../api'
import type { Agent, Task, Flow, BoardState, BoardColumnDef, TaskPriority } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'
import { DependencyPicker } from './DependencyPicker'
import { Button, ModalOverlay } from '../common'

function parseDeps(raw: string): string[] {
  try {
    const arr = JSON.parse(raw || '[]')
    return Array.isArray(arr) ? (arr as string[]) : []
  } catch {
    return []
  }
}

const PRIORITIES: { key: TaskPriority; label: string; color: string }[] = [
  { key: 'critical', label: 'Kritik', color: '#ef4444' },
  { key: 'high', label: 'Yüksek', color: '#f59e0b' },
  { key: 'medium', label: 'Orta', color: '#3b82f6' },
  { key: 'low', label: 'Düşük', color: '#6b7280' },
]

interface Props {
  // 'create' opens a blank form; 'edit' seeds from `task`.
  mode: 'create' | 'edit'
  task?: Task
  agents: Agent[]
  flows: Flow[]
  columns: BoardColumnDef[]
  tasks: Task[]
  defaultBoardState?: string
  onClose: () => void
  // Called with the persisted task (created or updated).
  onSaved: (task: Task) => void
  onDeleted?: (id: string) => void
  onError: (msg: string) => void
}

// TaskFormModal is the obsidian-pm-style card editor: a centered popup that
// creates and edits a board task with its attributes (priority, tags, assignee,
// flow, dependencies). Replaces the old right-hand TaskDetailPanel and the
// inline create form.
export function TaskFormModal({
  mode, task, agents, flows, columns, tasks, defaultBoardState, onClose, onSaved, onDeleted, onError,
}: Props) {
  const firstCol = defaultBoardState ?? columns[0]?.key ?? 'todo'
  const [title, setTitle] = useState(task?.title ?? '')
  const [description, setDescription] = useState(task?.description ?? '')
  const [boardState, setBoardState] = useState<BoardState>(task?.boardState ?? firstCol)
  const [priority, setPriority] = useState<TaskPriority>(task?.priority ?? 'medium')
  const [ownerAgentId, setOwnerAgentId] = useState(task?.ownerAgentId ?? '')
  const [flowId, setFlowId] = useState(task?.flowId ?? '')
  const [tags, setTags] = useState<string[]>(task?.tags ?? [])
  const [tagInput, setTagInput] = useState('')
  const [depIds, setDepIds] = useState<string[]>(() => parseDeps(task?.dependencies ?? '[]'))
  const [saving, setSaving] = useState(false)
  const [retitling, setRetitling] = useState(false)

  const addTag = () => {
    const t = tagInput.trim()
    if (t && !tags.includes(t)) setTags((prev) => [...prev, t])
    setTagInput('')
  }

  const payload = () => ({
    title: title.trim(),
    description: description.trim(),
    ownerAgentId,
    flowId,
    boardState,
    dependencies: JSON.stringify(depIds),
    priority,
    tags,
  })

  const save = async () => {
    if (mode === 'create' && !title.trim() && !description.trim()) {
      onError('Başlık veya açıklama gerekli')
      return
    }
    setSaving(true)
    try {
      const saved = mode === 'create'
        ? await api.createTask(payload())
        : await api.updateTask(task!.id, payload())
      onSaved(saved)
      onClose()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  // Regenerate the title from the description (server-side AI title). Edit only.
  const retitle = async () => {
    if (!task) return
    setRetitling(true)
    try {
      const updated = await api.generateTaskTitle(task.id)
      setTitle(updated.title)
      onSaved(updated)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setRetitling(false)
    }
  }

  const remove = async () => {
    if (!task || !onDeleted) return
    if (!confirm(`"${task.title}" silinsin mi?`)) return
    try {
      await api.deleteTask(task.id)
      onDeleted(task.id)
      onClose()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const parentOptions = tasks.filter((t) => t.id !== task?.id)

  return (
    <ModalOverlay onClose={onClose}>
      <div
        data-testid="task-form-modal"
        className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-2xl"
      >
        {/* Header: title input + close */}
        <div className="flex items-center gap-2 border-b border-[var(--color-border)] px-4 py-3">
          <input
            data-testid="task-title-input"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Başlık (boşsa açıklamadan üretilir)"
            className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-base font-medium outline-none focus:border-[var(--color-accent)]"
          />
          {mode === 'edit' && (
            <button
              data-testid="task-retitle-ai"
              onClick={retitle}
              disabled={retitling}
              title="AI ile başlığı açıklamadan üret"
              className="shrink-0 rounded border border-[var(--color-border)] p-2 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:opacity-30"
            >
              {retitling ? '…' : <RefreshCw size={15} />}
            </button>
          )}
          <button
            data-testid="task-detail-close"
            onClick={onClose}
            title="Kapat"
            className="shrink-0 rounded p-2 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
          >
            <X size={16} />
          </button>
        </div>

        {/* Body */}
        <div className="flex flex-1 flex-col gap-4 overflow-y-auto px-4 py-4">
          <Field label="Açıklama">
            <textarea
              data-testid="task-description-textarea"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              rows={4}
              className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
            />
          </Field>

          <div className="grid grid-cols-2 gap-4">
            <Field label="Durum (kolon)">
              <div data-testid="task-boardstate-select" className="flex flex-wrap gap-1.5">
                {columns.map((col) => {
                  const active = boardState === col.key
                  return (
                    <button
                      key={col.key}
                      onClick={() => setBoardState(col.key as BoardState)}
                      className={`rounded-full px-2.5 py-1 text-xs transition ${
                        active ? 'text-white' : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
                      }`}
                      style={active ? { backgroundColor: col.color || 'var(--color-accent)' } : undefined}
                    >
                      {col.label}
                    </button>
                  )
                })}
              </div>
            </Field>

            <Field label="Öncelik">
              <div data-testid="task-priority-select" className="flex flex-wrap gap-1.5">
                {PRIORITIES.map((p) => {
                  const active = priority === p.key
                  return (
                    <button
                      key={p.key}
                      onClick={() => setPriority(active ? '' : p.key)}
                      className="rounded-full px-2.5 py-1 text-xs transition"
                      style={
                        active
                          ? { backgroundColor: p.color, color: '#fff' }
                          : { backgroundColor: 'var(--color-surface-2)', color: 'var(--color-text-dim)' }
                      }
                    >
                      {p.label}
                    </button>
                  )
                })}
              </div>
            </Field>
          </div>

          <div className="grid grid-cols-2 gap-4">
            <Field label="Ajan (atanan)">
              <div data-testid="task-detail-owner-wrap">
                <AgentPicker agents={agents} value={ownerAgentId} onChange={setOwnerAgentId} placeholder="Ajan seç (opsiyonel)" />
              </div>
            </Field>
            <Field label="Akış (opsiyonel)">
              <select
                value={flowId}
                onChange={(e) => setFlowId(e.target.value)}
                className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
              >
                <option value="">🔀 Akış yok</option>
                {flows.map((f) => (
                  <option key={f.id} value={f.id}>🔀 {f.name}</option>
                ))}
              </select>
            </Field>
          </div>

          <Field label="Etiketler">
            <div className="flex flex-wrap items-center gap-1.5">
              {tags.map((t) => (
                <span key={t} className="inline-flex items-center gap-1 rounded-full bg-[var(--color-accent-soft)] px-2 py-0.5 text-xs text-[var(--color-accent)]">
                  #{t}
                  <button onClick={() => setTags((prev) => prev.filter((x) => x !== t))} className="opacity-60 hover:opacity-100" title="Kaldır">×</button>
                </span>
              ))}
              <input
                data-testid="task-tag-input"
                value={tagInput}
                onChange={(e) => setTagInput(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ',') {
                    e.preventDefault()
                    addTag()
                  }
                }}
                onBlur={addTag}
                placeholder="+ etiket"
                className="min-w-24 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
              />
            </div>
          </Field>

          <Field label="Bağımlılıklar — önce tamamlanması gereken görevler">
            <DependencyPicker tasks={parentOptions} value={depIds} onChange={setDepIds} />
          </Field>
        </div>

        {/* Footer */}
        <div className="flex items-center gap-2 border-t border-[var(--color-border)] px-4 py-3">
          <div data-testid="task-detail-save">
            <Button onClick={save} disabled={saving}>
              {saving ? 'Kaydediliyor…' : mode === 'create' ? '+ Oluştur' : 'Kaydet'}
            </Button>
          </div>
          <button onClick={onClose} className="rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] transition hover:text-[var(--color-text)]">
            İptal
          </button>
          {mode === 'edit' && onDeleted && (
            <button
              data-testid="task-detail-delete"
              onClick={remove}
              className="ml-auto rounded px-3 py-1.5 text-sm text-[var(--color-danger)] transition hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]"
            >
              🗑 Sil
            </button>
          )}
        </div>
      </div>
    </ModalOverlay>
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
