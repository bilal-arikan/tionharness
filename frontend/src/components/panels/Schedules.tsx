import { useEffect, useState } from 'react'
import { api } from '../../api'
import type { Agent, Task, Schedule } from '../../types'

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
}

// Common cron presets to spare the user from memorising the 5-field syntax.
const PRESETS: { label: string; expr: string }[] = [
  { label: 'Her dakika', expr: '* * * * *' },
  { label: 'Her 5 dakika', expr: '*/5 * * * *' },
  { label: 'Saat başı', expr: '0 * * * *' },
  { label: 'Her gün 09:00', expr: '0 9 * * *' },
  { label: 'Pazartesi 08:00', expr: '0 8 * * 1' },
]

function fmtTime(unix: number): string {
  if (!unix) return '—'
  return new Date(unix * 1000).toLocaleString('tr-TR')
}

export function Schedules({ agents, onError }: Props) {
  const [schedules, setSchedules] = useState<Schedule[]>([])
  const [tasks, setTasks] = useState<Task[]>([])
  const [agentId, setAgentId] = useState('')
  const [cronExpr, setCronExpr] = useState('*/5 * * * *')
  const [taskId, setTaskId] = useState('')
  const [prompt, setPrompt] = useState('')

  const reload = () =>
    api.listSchedules().then(setSchedules).catch((e) => onError(e.message))

  useEffect(() => {
    reload()
    api.listTasks().then(setTasks).catch((e) => onError(e.message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const agentName = (id: string) => agents.find((a) => a.id === id)?.name ?? '—'

  const create = async () => {
    if (!agentId || !cronExpr.trim()) {
      onError('Ajan ve cron ifadesi zorunlu')
      return
    }
    if (!taskId && !prompt.trim()) {
      onError('Görev veya prompt gerekli')
      return
    }
    try {
      const s = await api.createSchedule({
        agentId,
        cronExpr: cronExpr.trim(),
        taskId: taskId || undefined,
        prompt: prompt.trim() || undefined,
        enabled: true,
      })
      setSchedules((prev) => [s, ...prev])
      setPrompt('')
      setTaskId('')
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const toggle = async (s: Schedule) => {
    setSchedules((prev) =>
      prev.map((x) => (x.id === s.id ? { ...x, enabled: !x.enabled } : x)),
    )
    try {
      await api.toggleSchedule(s.id, !s.enabled)
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  const remove = async (s: Schedule) => {
    if (!confirm('Zamanlama silinsin mi?')) return
    setSchedules((prev) => prev.filter((x) => x.id !== s.id))
    try {
      await api.deleteSchedule(s.id)
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }

  return (
    <div className="flex h-full flex-col p-4">
      {/* New schedule form */}
      <div className="mb-4 space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
        <div className="flex flex-wrap gap-2">
          <select
            value={agentId}
            onChange={(e) => setAgentId(e.target.value)}
            className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none"
          >
            <option value="">Ajan seç</option>
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name}
              </option>
            ))}
          </select>
          <select
            value={cronExpr}
            onChange={(e) => setCronExpr(e.target.value)}
            className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none"
          >
            {PRESETS.map((p) => (
              <option key={p.expr} value={p.expr}>
                {p.label} ({p.expr})
              </option>
            ))}
          </select>
          <input
            value={cronExpr}
            onChange={(e) => setCronExpr(e.target.value)}
            placeholder="cron: dk sa gün ay haftagünü"
            className="w-44 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 font-mono text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </div>
        <div className="flex flex-wrap gap-2">
          <select
            value={taskId}
            onChange={(e) => setTaskId(e.target.value)}
            className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none"
          >
            <option value="">Görev bağlama (opsiyonel)</option>
            {tasks.map((t) => (
              <option key={t.id} value={t.id}>
                {t.title}
              </option>
            ))}
          </select>
          <input
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="veya doğrudan prompt gönder"
            disabled={!!taskId}
            className="flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)] disabled:opacity-40"
          />
          <button
            onClick={create}
            className="rounded bg-[var(--color-accent)] px-3 py-1 text-sm font-medium text-white hover:opacity-90"
          >
            + Zamanlama
          </button>
        </div>
      </div>

      {/* Schedule list */}
      <div className="flex-1 space-y-2 overflow-y-auto">
        {schedules.length === 0 && (
          <p className="text-sm text-[var(--color-text-dim)]">Henüz zamanlama yok.</p>
        )}
        {schedules.map((s) => (
          <div
            key={s.id}
            className="flex items-center gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm"
          >
            <button
              onClick={() => toggle(s)}
              className={`h-4 w-8 flex-shrink-0 rounded-full transition ${
                s.enabled ? 'bg-[var(--color-accent)]' : 'bg-[var(--color-border)]'
              }`}
              title={s.enabled ? 'Etkin' : 'Pasif'}
            >
              <span
                className={`block h-4 w-4 rounded-full bg-white transition ${
                  s.enabled ? 'translate-x-4' : ''
                }`}
              />
            </button>
            <div className="flex-1">
              <div className="flex items-center gap-2">
                <span className="font-mono text-[var(--color-accent)]">{s.cronExpr}</span>
                <span className="text-[var(--color-text-dim)]">→ {agentName(s.agentId)}</span>
              </div>
              <div className="text-xs text-[var(--color-text-dim)]">
                {s.taskId
                  ? `Görev: ${tasks.find((t) => t.id === s.taskId)?.title ?? s.taskId}`
                  : `Prompt: ${s.prompt}`}
              </div>
              <div className="text-xs text-[var(--color-text-dim)]">
                Sonraki: {fmtTime(s.nextRunAt)} · Son:{' '}
                {s.lastDeliveryStatus ? (
                  <span
                    className={
                      s.lastDeliveryStatus === 'success'
                        ? 'text-emerald-400'
                        : 'text-red-400'
                    }
                  >
                    {s.lastDeliveryStatus} {fmtTime(s.lastRunAt)}
                  </span>
                ) : (
                  '—'
                )}
                {s.lastDeliveryError && (
                  <span className="text-red-400"> ({s.lastDeliveryError})</span>
                )}
              </div>
            </div>
            <button
              onClick={() => remove(s)}
              className="text-[var(--color-text-dim)] hover:text-red-400"
              title="Sil"
            >
              ✕
            </button>
          </div>
        ))}
      </div>
    </div>
  )
}
