import { useEffect, useState } from 'react'
import { api } from '../../api'
import type { Agent, Task, BoardState } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'
import { AgentAvatar } from '../agents/AgentAvatar'
import { TaskDetailPanel } from './TaskDetailPanel'

const COLUMNS: { key: BoardState; label: string }[] = [
  { key: 'todo', label: 'Yapılacak' },
  { key: 'in_progress', label: 'Devam Eden' },
  { key: 'review', label: 'İnceleme' },
  { key: 'done', label: 'Bitti' },
  { key: 'failed', label: 'Başarısız' },
]

const STATUS_COLOR: Record<string, string> = {
  success: 'text-emerald-400',
  failure: 'text-red-400',
  running: 'text-amber-400',
  pending: 'text-[var(--color-text-dim)]',
}

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
}

export function TaskBoard({ agents, onError }: Props) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [prompt, setPrompt] = useState('')
  const [ownerAgentId, setOwnerAgentId] = useState('')
  const [dragId, setDragId] = useState<string | null>(null)
  // Right-hand detail/editor drawer: which task is currently open (null = closed).
  const [selectedId, setSelectedId] = useState<string | null>(null)

  const reload = () =>
    api.listTasks().then(setTasks).catch((e) => onError(e.message))

  useEffect(() => {
    reload()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Tasks are created from a prompt alone; the backend auto-generates the title.
  const createTask = async () => {
    if (!prompt.trim()) return
    try {
      const t = await api.createTask({
        prompt: prompt.trim(),
        ownerAgentId: ownerAgentId || undefined,
      })
      setTasks((prev) => [t, ...prev])
      setPrompt('')
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const move = async (task: Task, boardState: BoardState) => {
    if (task.boardState === boardState) return
    setTasks((prev) =>
      prev.map((t) => (t.id === task.id ? { ...t, boardState } : t)),
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
    <div className="flex h-full">
      <div className="flex h-full flex-1 flex-col overflow-hidden">
        {/* New task form */}
        <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-4 py-3">
          <input
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') createTask()
            }}
            placeholder="Ajana verilecek talimat (prompt) — başlık otomatik oluşturulur"
            className="min-w-40 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
          />
          <AgentPicker
            agents={agents}
            value={ownerAgentId}
            onChange={setOwnerAgentId}
            placeholder="Ajan seç (opsiyonel)"
          />
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
            const colTasks = tasks.filter((t) => t.boardState === col.key)
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
                    // Card is a summary: click anywhere to open the detail drawer.
                    return (
                      <div
                        key={t.id}
                        draggable
                        onDragStart={() => setDragId(t.id)}
                        onClick={() => setSelectedId(t.id)}
                        className={`cursor-pointer rounded-lg border bg-[var(--color-surface-2)] p-2 text-sm transition active:cursor-grabbing ${
                          selectedId === t.id
                            ? 'border-[var(--color-accent)]'
                            : 'border-[var(--color-border)] hover:border-[var(--color-accent)]'
                        }`}
                      >
                        <div className="font-medium">{t.title}</div>
                        {t.prompt && (
                          <div className="mt-1 line-clamp-2 text-xs text-[var(--color-text-dim)]">
                            {t.prompt}
                          </div>
                        )}
                        <div className="mt-2 flex items-center justify-between text-xs">
                          <span className="flex items-center gap-1.5 text-[var(--color-text-dim)]">
                            {owner ? (
                              <>
                                <AgentAvatar agent={owner} size={16} />
                                <span className="truncate">{owner.name}</span>
                              </>
                            ) : (
                              '—'
                            )}
                          </span>
                          {t.lastRunStatus && (
                            <span className={STATUS_COLOR[t.lastRunStatus] ?? ''}>● {t.lastRunStatus}</span>
                          )}
                        </div>
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
          onClose={() => setSelectedId(null)}
          onSaved={onSaved}
          onDeleted={onDeleted}
          onError={onError}
        />
      )}
    </div>
  )
}
