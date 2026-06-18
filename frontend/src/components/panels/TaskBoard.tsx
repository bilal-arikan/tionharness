import { useEffect, useState } from 'react'
import { api } from '../../api'
import type { Agent, Task, BoardState, Flow } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'
import { AgentAvatar } from '../agents/AgentAvatar'
import { TaskDetailPanel } from './TaskDetailPanel'

// Current unix time in seconds, matching the backend's task timestamps — used
// for optimistic createdAt/updatedAt so cards sort consistently before reload.
const nowSec = () => Math.floor(Date.now() / 1000)

const COLUMNS: { key: BoardState; label: string }[] = [
  { key: 'todo', label: 'Yapılacak' },
  { key: 'in_progress', label: 'Devam Eden' },
  { key: 'review', label: 'İnceleme' },
  { key: 'done', label: 'Bitti' },
  { key: 'failed', label: 'Başarısız' },
]

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
}

export function TaskBoard({ agents, onError }: Props) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [flows, setFlows] = useState<Flow[]>([])
  const [description, setDescription] = useState('')
  const [ownerAgentId, setOwnerAgentId] = useState('')
  const [newFlowId, setNewFlowId] = useState('')
  const [dragId, setDragId] = useState<string | null>(null)
  // Right-hand detail/editor drawer: which task is currently open (null = closed).
  const [selectedId, setSelectedId] = useState<string | null>(null)

  const reload = () =>
    api.listTasks().then(setTasks).catch((e) => onError(e.message))

  useEffect(() => {
    reload()
    api.listFlows().then(setFlows).catch((e) => onError(e.message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // A task is a passive board item: created from a description (title is
  // auto-generated from it). Agent and flow are optional informational tags;
  // the board itself never runs anything — flows/schedules/agent sessions read
  // and update tasks from outside.
  //
  // The backend generates the title (an AI call) before returning, so we insert
  // an optimistic card immediately (description as a placeholder title) and swap
  // it for the persisted task on success — the click registers instantly.
  const createTask = async () => {
    const desc = description.trim()
    if (!desc) return
    const tempId = `temp-${Date.now()}`
    const owner = ownerAgentId
    const flow = newFlowId
    const optimistic: Task = {
      id: tempId,
      title: desc.length > 60 ? desc.slice(0, 60) + '…' : desc,
      description: desc,
      prompt: '',
      ownerAgentId: owner,
      flowId: flow,
      boardState: 'todo',
      dependencies: '[]',
      lastRunId: '',
      lastRunStatus: '',
      lastRunAt: 0,
      createdAt: nowSec(),
      updatedAt: nowSec(),
    }
    setTasks((prev) => [optimistic, ...prev])
    setDescription('')
    setNewFlowId('')
    try {
      const t = await api.createTask({
        description: desc,
        ownerAgentId: owner || undefined,
        flowId: flow || undefined,
      })
      // Swap the placeholder for the persisted task (real id + AI title).
      setTasks((prev) => prev.map((x) => (x.id === tempId ? t : x)))
    } catch (e) {
      setTasks((prev) => prev.filter((x) => x.id !== tempId))
      onError((e as Error).message)
    }
  }

  const move = async (task: Task, boardState: BoardState) => {
    if (task.boardState === boardState) return
    // Bump updatedAt locally so the moved card sorts to the top of the column
    // immediately (the backend bumps it too on persist).
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

  // Persist edits/run-outcomes from the detail drawer back into the board list.
  const onSaved = (updated: Task) => {
    setTasks((prev) => prev.map((t) => (t.id === updated.id ? updated : t)))
  }

  // Drop a deleted task from the board and close the drawer if it was open.
  const onDeleted = (id: string) => {
    setTasks((prev) => prev.filter((t) => t.id !== id))
    if (selectedId === id) setSelectedId(null)
  }

  const selected = tasks.find((t) => t.id === selectedId) ?? null

  return (
    <div className="flex min-h-0 flex-1">
      <div className="flex h-full flex-1 flex-col overflow-hidden">
        {/* New task form */}
        <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-4 py-3">
          <input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') createTask()
            }}
            placeholder="Görev açıklaması — başlık otomatik oluşturulur"
            className="min-w-40 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
          />
          <AgentPicker
            agents={agents}
            value={ownerAgentId}
            onChange={setOwnerAgentId}
            placeholder="Ajan (opsiyonel, bilgi)"
          />
          {flows.length > 0 && (
            <select
              value={newFlowId}
              onChange={(e) => setNewFlowId(e.target.value)}
              title="Akış etiketi (opsiyonel, bilgi amaçlı)"
              className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none"
            >
              <option value="">🔀 Akış yok</option>
              {flows.map((f) => (
                <option key={f.id} value={f.id}>
                  🔀 {f.name}
                </option>
              ))}
            </select>
          )}
          <button
            onClick={createTask}
            className="rounded bg-[var(--color-accent)] px-3 py-1 text-sm font-medium text-white hover:opacity-90"
          >
            + Görev
          </button>
        </div>

        {/* Board */}
        <div className="flex flex-1 gap-3 overflow-x-auto p-4">
          {COLUMNS.map((col) => {
            // Within a column, most-recently-updated first — so a card that moves
            // here (by drag or by an agent's move_task, both bump updatedAt) lands
            // at the top.
            const colTasks = tasks
              .filter((t) => t.boardState === col.key)
              .sort((a, b) => b.updatedAt - a.updatedAt)
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
                <div className="flex items-center justify-between px-3 py-2 text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
                  <span>{col.label}</span>
                  <span className="rounded bg-[var(--color-surface-2)] px-1.5">{colTasks.length}</span>
                </div>
                <div className="flex-1 space-y-2 overflow-y-auto px-2 pb-2">
                  {colTasks.map((t) => {
                    const owner = agents.find((a) => a.id === t.ownerAgentId)
                    const flow = t.flowId ? flows.find((f) => f.id === t.flowId) : undefined
                    // An optimistic (not-yet-persisted) card while its title is
                    // being generated server-side; not clickable/draggable yet.
                    const pending = t.id.startsWith('temp-')
                    // Card is a summary: click anywhere to open the detail drawer;
                    // clicking the already-selected card toggles it closed.
                    return (
                      <div
                        key={t.id}
                        draggable={!pending}
                        onDragStart={() => !pending && setDragId(t.id)}
                        onClick={() => !pending && setSelectedId((cur) => (cur === t.id ? null : t.id))}
                        className={`rounded-lg border bg-[var(--color-surface-2)] p-2 text-sm shadow-[var(--shadow-sm)] transition ${
                          pending
                            ? 'animate-pulse cursor-default border-[var(--color-border)] opacity-70'
                            : `cursor-pointer hover:shadow-[var(--shadow-md)] active:cursor-grabbing ${
                                selectedId === t.id
                                  ? 'border-[var(--color-accent)]'
                                  : 'border-[var(--color-border)] hover:border-[var(--color-accent)]'
                              }`
                        }`}
                      >
                        <div className="font-medium">{t.title}</div>
                        {pending ? (
                          <div className="mt-1 text-[11px] text-[var(--color-text-dim)]">başlık üretiliyor…</div>
                        ) : (
                          t.description && (
                            <div className="mt-1 line-clamp-2 text-xs text-[var(--color-text-dim)]">
                              {t.description}
                            </div>
                          )
                        )}
                        {/* Optional informational tags: owner agent + flow. */}
                        {(owner || t.flowId) && (
                          <div className="mt-2 flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
                            {owner && (
                              <span className="flex items-center gap-1.5">
                                <AgentAvatar agent={owner} size={16} />
                                <span className="truncate">{owner.name}</span>
                              </span>
                            )}
                            {t.flowId && (
                              <span className="inline-flex items-center gap-1 rounded bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]">
                                🔀 {flow?.name ?? 'Akış'}
                              </span>
                            )}
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
      </div>

      {selected && (
        <TaskDetailPanel
          task={selected}
          agents={agents}
          flows={flows}
          onClose={() => setSelectedId(null)}
          onSaved={onSaved}
          onDeleted={onDeleted}
          onError={onError}
        />
      )}
    </div>
  )
}
