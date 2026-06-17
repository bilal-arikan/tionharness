import { useEffect, useState } from 'react'
import { api } from '../../api'
import type { Agent, Task, Run, BoardState } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'
import { AgentAvatar } from '../agents/AgentAvatar'

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

// Cron presets for the "bind to schedule" card action (mirror Schedules.tsx).
const CRON_PRESETS: { label: string; expr: string }[] = [
  { label: 'Her 5 dakika', expr: '*/5 * * * *' },
  { label: 'Saat başı', expr: '0 * * * *' },
  { label: 'Her gün 09:00', expr: '0 9 * * *' },
  { label: 'Pazartesi 08:00', expr: '0 8 * * 1' },
]

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
}

export function TaskBoard({ agents, onError }: Props) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [prompt, setPrompt] = useState('')
  const [ownerAgentId, setOwnerAgentId] = useState('')
  const [runningId, setRunningId] = useState<string | null>(null)
  const [retitlingId, setRetitlingId] = useState<string | null>(null)
  const [runsFor, setRunsFor] = useState<string | null>(null)
  const [runs, setRuns] = useState<Run[]>([])
  const [dragId, setDragId] = useState<string | null>(null)
  // Schedule-binding popover: which task's form is open + its chosen cron expr.
  const [schedFor, setSchedFor] = useState<string | null>(null)
  const [schedCron, setSchedCron] = useState('*/5 * * * *')
  const [schedBusy, setSchedBusy] = useState(false)

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

  // Regenerate a task's title from its prompt on demand.
  const retitle = async (task: Task) => {
    setRetitlingId(task.id)
    try {
      const updated = await api.generateTaskTitle(task.id)
      setTasks((prev) => prev.map((t) => (t.id === task.id ? updated : t)))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setRetitlingId(null)
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

  const run = async (task: Task) => {
    setRunningId(task.id)
    try {
      const r = await api.runTask(task.id)
      setTasks((prev) =>
        prev.map((t) =>
          t.id === task.id
            ? { ...t, boardState: r.status === 'success' ? 'done' : 'failed', lastRunStatus: r.status }
            : t,
        ),
      )
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setRunningId(null)
    }
  }

  const remove = async (task: Task) => {
    if (!confirm(`"${task.title}" silinsin mi?`)) return
    setTasks((prev) => prev.filter((t) => t.id !== task.id))
    try {
      await api.deleteTask(task.id)
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  const showRuns = async (task: Task) => {
    if (runsFor === task.id) {
      setRunsFor(null)
      return
    }
    setRunsFor(task.id)
    try {
      setRuns(await api.listTaskRuns(task.id))
    } catch (e) {
      onError((e as Error).message)
    }
  }

  // Toggle the cron-binding popover for a task. Requires an owner agent, since a
  // schedule needs an agent and RunTask delivers to the task's owner.
  const toggleSched = (task: Task) => {
    if (schedFor === task.id) {
      setSchedFor(null)
      return
    }
    if (!task.ownerAgentId) {
      onError('Zamanlamak için önce göreve bir ajan atayın')
      return
    }
    setSchedCron('*/5 * * * *')
    setSchedFor(task.id)
  }

  // Create a cron schedule bound to this task (taskId set → scheduler RunTask).
  const bindSchedule = async (task: Task) => {
    if (!schedCron.trim()) return
    setSchedBusy(true)
    try {
      await api.createSchedule({
        agentId: task.ownerAgentId,
        taskId: task.id,
        cronExpr: schedCron.trim(),
        enabled: true,
      })
      setSchedFor(null)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSchedBusy(false)
    }
  }

  return (
    <div className="flex h-full flex-col">
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
                  return (
                  <div
                    key={t.id}
                    draggable
                    onDragStart={() => setDragId(t.id)}
                    className="cursor-grab rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2 text-sm active:cursor-grabbing"
                  >
                    <div className="flex items-start justify-between gap-1">
                      <div className="font-medium">{t.title}</div>
                      <button
                        onClick={() => retitle(t)}
                        disabled={retitlingId === t.id}
                        title="Başlığı yeniden oluştur"
                        className="shrink-0 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-accent)] disabled:opacity-30"
                      >
                        {retitlingId === t.id ? '…' : '⟳'}
                      </button>
                    </div>
                    {t.prompt && (
                      <div className="mt-1 line-clamp-2 text-xs text-[var(--color-text-dim)]">{t.prompt}</div>
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
                    <div className="mt-2 flex items-center gap-2">
                      <button
                        onClick={() => run(t)}
                        disabled={!t.ownerAgentId || runningId === t.id}
                        className="rounded bg-[var(--color-accent-soft)] px-2 py-0.5 text-xs text-[var(--color-text)] hover:opacity-90 disabled:opacity-30"
                        title={t.ownerAgentId ? 'Şimdi çalıştır' : 'Önce ajan ata'}
                      >
                        {runningId === t.id ? '…' : '▶ Çalıştır'}
                      </button>
                      <button
                        onClick={() => toggleSched(t)}
                        className={`text-xs hover:text-[var(--color-accent)] ${
                          schedFor === t.id
                            ? 'text-[var(--color-accent)]'
                            : 'text-[var(--color-text-dim)]'
                        }`}
                        title={t.ownerAgentId ? 'Cron zamanlamasına bağla' : 'Önce ajan ata'}
                      >
                        ⏰ Zamanla
                      </button>
                      <button
                        onClick={() => showRuns(t)}
                        className="text-xs text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
                      >
                        Geçmiş
                      </button>
                      <button
                        onClick={() => remove(t)}
                        className="ml-auto text-xs text-[var(--color-text-dim)] hover:text-red-400"
                        title="Sil"
                      >
                        ✕
                      </button>
                    </div>
                    {schedFor === t.id && (
                      <div className="mt-2 space-y-2 border-t border-[var(--color-border)] pt-2">
                        <div className="flex flex-wrap items-center gap-1.5">
                          <select
                            value={CRON_PRESETS.some((p) => p.expr === schedCron) ? schedCron : ''}
                            onChange={(e) => e.target.value && setSchedCron(e.target.value)}
                            className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-1.5 py-0.5 text-xs outline-none"
                          >
                            <option value="">Özel…</option>
                            {CRON_PRESETS.map((p) => (
                              <option key={p.expr} value={p.expr}>
                                {p.label}
                              </option>
                            ))}
                          </select>
                          <input
                            value={schedCron}
                            onChange={(e) => setSchedCron(e.target.value)}
                            placeholder="dk sa gün ay haftagünü"
                            className="w-28 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-1.5 py-0.5 font-mono text-xs outline-none focus:border-[var(--color-accent)]"
                          />
                        </div>
                        <div className="flex items-center gap-2">
                          <button
                            onClick={() => bindSchedule(t)}
                            disabled={schedBusy}
                            className="rounded bg-[var(--color-accent)] px-2 py-0.5 text-xs font-medium text-white hover:opacity-90 disabled:opacity-40"
                          >
                            {schedBusy ? '…' : 'Bağla'}
                          </button>
                          <button
                            onClick={() => setSchedFor(null)}
                            className="text-xs text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
                          >
                            İptal
                          </button>
                        </div>
                      </div>
                    )}
                    {runsFor === t.id && (
                      <div className="mt-2 space-y-1 border-t border-[var(--color-border)] pt-2">
                        {runs.length === 0 && (
                          <p className="text-xs text-[var(--color-text-dim)]">Henüz çalıştırma yok.</p>
                        )}
                        {runs.map((r) => (
                          <div key={r.id} className="rounded bg-[var(--color-bg)] p-1.5 text-xs">
                            <span className={STATUS_COLOR[r.status] ?? ''}>{r.status}</span>
                            <span className="ml-1 text-[var(--color-text-dim)]">({r.trigger})</span>
                            <div className="mt-0.5 text-[var(--color-text)] line-clamp-3">
                              {r.error || r.output}
                            </div>
                          </div>
                        ))}
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
  )
}
