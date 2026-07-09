import { useEffect, useRef, useState } from 'react'
import { ChevronDown, ChevronRight, RefreshCw, X } from 'lucide-react'
import { api } from '@/api'
import type { Agent, Task, Flow, BoardState, BoardColumnDef, TaskPriority, Artifact } from '@/types'
import { AgentPicker } from '@/shared/components/agents/AgentPicker'
import { DependencyPicker } from './DependencyPicker'
import { TaskArtifactRefs } from './TaskArtifactRefs'
import { Button, ModalOverlay } from '@/shared/components'
import { normalizeAvatar } from '@/shared/lib/avatar'

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
  // Called with the persisted task (created or updated). On optimistic create it
  // is first called with a temporary card (id `temp-…`) so the board renders it
  // instantly in the selected column.
  onSaved: (task: Task) => void
  // Reconciles an optimistic create: replaces the temp card with the server row,
  // or removes it (real = null) when the create failed.
  onReplaceTemp?: (tempId: string, real: Task | null) => void
  onDeleted?: (id: string) => void
  onError: (msg: string) => void
}

// excerpt returns a short, single-line preview of the description used as the
// placeholder title until the async AI title lands ("ilk başta içeriğin belli
// miktarını başlıkta göster").
function excerpt(text: string, max = 60): string {
  const line = text.trim().split(/\r?\n/, 1)[0] ?? ''
  return line.length > max ? line.slice(0, max).trimEnd() + '…' : line
}

// TaskFormModal is the obsidian-pm-style card editor: a centered popup that
// creates and edits a board task with its attributes (priority, tags, assignee,
// flow, dependencies). Replaces the old right-hand TaskDetailPanel and the
// inline create form.
export function TaskFormModal({
  mode, task, agents, flows, columns, tasks, defaultBoardState, onClose, onSaved, onReplaceTemp, onDeleted, onError,
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
  const [artifactIds, setArtifactIds] = useState<string[]>(task?.artifactIds ?? [])
  // Workspace artifacts, loaded once to resolve refs to titles/kinds and feed the
  // "link existing" picker; drag-dropped files append newly created artifacts.
  const [allArtifacts, setAllArtifacts] = useState<Artifact[]>([])
  const [saving, setSaving] = useState(false)
  const [retitling, setRetitling] = useState(false)
  // Dependencies picker is collapsible; open by default only when the task
  // already has dependencies, so the section stays out of the way otherwise.
  const [depsOpen, setDepsOpen] = useState(depIds.length > 0)

  // On create, focus the description field immediately — it is the primary input
  // (the title is auto-generated from it), so the user can start typing at once.
  const descRef = useRef<HTMLTextAreaElement>(null)
  useEffect(() => {
    if (mode === 'create') descRef.current?.focus()
  }, [mode])

  // Load workspace artifacts for the reference picker + chip resolution.
  useEffect(() => {
    api.listArtifacts().then(setAllArtifacts).catch(() => {
      /* non-fatal: picker just shows "no artifacts" */
    })
  }, [])

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
    artifactIds,
  })

  const save = async () => {
    if (mode === 'create' && !title.trim() && !description.trim()) {
      onError('Başlık veya açıklama gerekli')
      return
    }
    if (mode === 'edit') {
      setSaving(true)
      try {
        onSaved(await api.updateTask(task!.id, payload()))
        onClose()
      } catch (e) {
        onError((e as Error).message)
      } finally {
        setSaving(false)
      }
      return
    }

    // Create: optimistic. Render the card instantly in the SELECTED column with a
    // placeholder title (typed title, else a content excerpt), close the modal,
    // then reconcile with the server row. This guarantees the chosen board column
    // is honored and lets the AI title fill in asynchronously (temp cards show a
    // "başlık üretiliyor…" hint via their `temp-` id).
    const p = payload()
    const tempId = `temp-${Date.now()}`
    const nowSec = Math.floor(Date.now() / 1000)
    const optimistic: Task = {
      id: tempId,
      title: p.title || excerpt(p.description),
      description: p.description,
      prompt: '',
      ownerAgentId: p.ownerAgentId,
      flowId: p.flowId,
      boardState: p.boardState,
      dependencies: p.dependencies,
      priority: p.priority,
      tags: p.tags,
      artifactIds: p.artifactIds,
      lastRunId: '',
      lastRunStatus: '',
      lastRunAt: 0,
      createdAt: nowSec,
      updatedAt: nowSec,
    }
    onSaved(optimistic)
    onClose()
    try {
      const saved = await api.createTask(p)
      onReplaceTemp?.(tempId, saved)
    } catch (e) {
      onReplaceTemp?.(tempId, null)
      onError((e as Error).message)
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
              ref={descRef}
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
                <AgentPicker agents={agents} value={ownerAgentId} onChange={setOwnerAgentId} placeholder="Ajan seç (opsiyonel)" clearable />
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
                  <option key={f.id} value={f.id}>{normalizeAvatar(f.emoji) ?? '🔀'} {f.name}</option>
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

          <Field label="Ekler — Artifact referansları">
            <TaskArtifactRefs
              value={artifactIds}
              onChange={setArtifactIds}
              artifacts={allArtifacts}
              onArtifactsChanged={(created) => setAllArtifacts((prev) => [...created, ...prev])}
              bucket={task?.id ?? 'board'}
              onError={onError}
            />
          </Field>

          <div className="flex flex-col gap-1.5">
            <button
              type="button"
              onClick={() => setDepsOpen((v) => !v)}
              aria-expanded={depsOpen}
              className="flex items-center gap-1.5 text-left text-[10px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] opacity-70 transition hover:opacity-100"
            >
              {depsOpen ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
              <span>Bağımlılıklar — önce tamamlanması gereken görevler</span>
              {depIds.length > 0 && (
                <span className="rounded-full bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]">
                  {depIds.length}
                </span>
              )}
            </button>
            {depsOpen && (
              <DependencyPicker tasks={parentOptions} value={depIds} onChange={setDepIds} />
            )}
          </div>
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
