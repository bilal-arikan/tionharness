import { useEffect, useRef, useState } from 'react'
import { Play, Hourglass, Pencil, X } from 'lucide-react'
import { api } from '../../api'
import type { Agent, Schedule } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'

interface Props {
  agents: Agent[]
  /** Deep-link target: scroll to and highlight this schedule once loaded. */
  focusId?: string | null
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

export function Schedules({ agents, focusId, onError }: Props) {
  const [schedules, setSchedules] = useState<Schedule[]>([])
  // Briefly highlight a deep-linked schedule once it is present in the list.
  const [highlightId, setHighlightId] = useState<string | null>(null)
  const focusRef = useRef<HTMLDivElement | null>(null)
  const [agentId, setAgentId] = useState('')
  const [cronExpr, setCronExpr] = useState('*/5 * * * *')
  const [prompt, setPrompt] = useState('')

  // Inline edit state (one schedule edited at a time).
  const [editId, setEditId] = useState<string | null>(null)
  const [editAgentId, setEditAgentId] = useState('')
  const [editCronExpr, setEditCronExpr] = useState('')
  const [editPrompt, setEditPrompt] = useState('')

  // Id of the schedule currently being run manually (disables its Run button).
  const [runningId, setRunningId] = useState<string | null>(null)

  const reload = () =>
    api.listSchedules().then(setSchedules).catch((e) => onError(e.message))

  useEffect(() => {
    reload()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // When a deep-link target is present and loaded, scroll it into view and flash
  // a highlight ring that fades after a moment.
  useEffect(() => {
    if (!focusId || !schedules.some((s) => s.id === focusId)) return
    setHighlightId(focusId)
    focusRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    const t = setTimeout(() => setHighlightId(null), 2500)
    return () => clearTimeout(t)
  }, [focusId, schedules])

  const agentName = (id: string) => agents.find((a) => a.id === id)?.name ?? '—'

  const create = async () => {
    if (!agentId || !cronExpr.trim()) {
      onError('Ajan ve cron ifadesi zorunlu')
      return
    }
    if (!prompt.trim()) {
      onError('Prompt zorunlu')
      return
    }
    try {
      const s = await api.createSchedule({
        agentId,
        cronExpr: cronExpr.trim(),
        prompt: prompt.trim(),
        enabled: true,
      })
      setSchedules((prev) => [s, ...prev])
      setPrompt('')
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

  const startEdit = (s: Schedule) => {
    setEditId(s.id)
    setEditAgentId(s.agentId)
    setEditCronExpr(s.cronExpr)
    setEditPrompt(s.prompt)
  }

  const cancelEdit = () => setEditId(null)

  const saveEdit = async (s: Schedule) => {
    if (!editAgentId || !editCronExpr.trim()) {
      onError('Ajan ve cron ifadesi zorunlu')
      return
    }
    if (!editPrompt.trim()) {
      onError('Prompt zorunlu')
      return
    }
    try {
      const updated = await api.updateSchedule(s.id, {
        agentId: editAgentId,
        cronExpr: editCronExpr.trim(),
        prompt: editPrompt.trim(),
      })
      setSchedules((prev) => prev.map((x) => (x.id === s.id ? updated : x)))
      setEditId(null)
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const runNow = async (s: Schedule) => {
    setRunningId(s.id)
    try {
      const updated = await api.runSchedule(s.id)
      setSchedules((prev) => prev.map((x) => (x.id === s.id ? updated : x)))
      if (updated.lastDeliveryStatus === 'failure') {
        onError(`Çalıştırma başarısız: ${updated.lastDeliveryError || 'bilinmeyen hata'}`)
      }
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setRunningId(null)
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
        <div className="flex flex-wrap items-start gap-2">
          <AgentPicker agents={agents} value={agentId} onChange={setAgentId} />
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
        <div className="flex flex-wrap items-end gap-2">
          <input
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="Prompt (zorunlu) — ajana gönderilecek talimat"
            className="flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
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
        {schedules.map((s) =>
          editId === s.id ? (
            <div
              key={s.id}
              className="space-y-2 rounded-lg border border-[var(--color-accent)] bg-[var(--color-surface)] p-3 text-sm"
            >
              <div className="flex flex-wrap items-start gap-2">
                <AgentPicker agents={agents} value={editAgentId} onChange={setEditAgentId} />
                <select
                  value={editCronExpr}
                  onChange={(e) => setEditCronExpr(e.target.value)}
                  className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none"
                >
                  <option value={editCronExpr}>Hazır ifade seç…</option>
                  {PRESETS.map((p) => (
                    <option key={p.expr} value={p.expr}>
                      {p.label} ({p.expr})
                    </option>
                  ))}
                </select>
                <input
                  value={editCronExpr}
                  onChange={(e) => setEditCronExpr(e.target.value)}
                  placeholder="cron: dk sa gün ay haftagünü"
                  className="w-44 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 font-mono text-sm outline-none focus:border-[var(--color-accent)]"
                />
              </div>
              <div className="flex flex-wrap items-end gap-2">
                <input
                  value={editPrompt}
                  onChange={(e) => setEditPrompt(e.target.value)}
                  placeholder="Prompt (zorunlu) — ajana gönderilecek talimat"
                  className="flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
                />
                <button
                  onClick={() => saveEdit(s)}
                  className="rounded bg-[var(--color-accent)] px-3 py-1 text-sm font-medium text-white hover:opacity-90"
                >
                  Kaydet
                </button>
                <button
                  onClick={cancelEdit}
                  className="rounded border border-[var(--color-border)] px-3 py-1 text-sm hover:bg-[var(--color-bg)]"
                >
                  İptal
                </button>
              </div>
            </div>
          ) : (
          <div
            key={s.id}
            ref={s.id === focusId ? focusRef : undefined}
            className={`flex items-center gap-3 rounded-lg border bg-[var(--color-surface)] px-3 py-2 text-sm transition ${
              highlightId === s.id
                ? 'border-[var(--color-accent)] ring-2 ring-[var(--color-accent)]'
                : 'border-[var(--color-border)]'
            }`}
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
              <div className="text-xs text-[var(--color-text-dim)]">Prompt: {s.prompt}</div>
              <div className="text-xs text-[var(--color-text-dim)]">
                Sonraki: {fmtTime(s.nextRunAt)} · Son:{' '}
                {s.lastDeliveryStatus ? (
                  <span
                    className={
                      s.lastDeliveryStatus === 'success'
                        ? 'text-[var(--color-success)]'
                        : 'text-[var(--color-danger)]'
                    }
                  >
                    {s.lastDeliveryStatus} {fmtTime(s.lastRunAt)}
                  </span>
                ) : (
                  '—'
                )}
                {s.lastDeliveryError && (
                  <span className="text-[var(--color-danger)]"> ({s.lastDeliveryError})</span>
                )}
              </div>
            </div>
            <button
              onClick={() => runNow(s)}
              disabled={runningId === s.id}
              className="text-[var(--color-text-dim)] hover:text-[var(--color-success)] disabled:opacity-40"
              title="Şimdi çalıştır"
            >
              {runningId === s.id ? <Hourglass size={15} /> : <Play size={15} />}
            </button>
            <button
              onClick={() => startEdit(s)}
              className="text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
              title="Düzenle"
            >
              <Pencil size={15} />
            </button>
            <button
              onClick={() => remove(s)}
              className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
              title="Sil"
            >
              <X size={15} />
            </button>
          </div>
          ),
        )}
      </div>
    </div>
  )
}
