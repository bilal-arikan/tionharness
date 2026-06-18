import { useEffect, useRef, useState } from 'react'
import { Play, Hourglass, Pencil, X, Timer } from 'lucide-react'
import { api } from '../../api'
import type { Agent, Schedule } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'
import { AgentAvatar } from '../agents/AgentAvatar'

interface Props {
  agents: Agent[]
  /** Deep-link target: scroll to and highlight this schedule once loaded. */
  focusId?: string | null
  onError: (msg: string) => void
}

// One-shot presets: label + a function that returns the fireAt unix timestamp.
const ONE_SHOT_PRESETS: { label: string; fireAt: () => number }[] = [
  { label: '30 dk sonra',   fireAt: () => Math.floor(Date.now() / 1000) + 30 * 60 },
  { label: '1 saat sonra',  fireAt: () => Math.floor(Date.now() / 1000) + 60 * 60 },
  { label: '2 saat sonra',  fireAt: () => Math.floor(Date.now() / 1000) + 2 * 60 * 60 },
  { label: '4 saat sonra',  fireAt: () => Math.floor(Date.now() / 1000) + 4 * 60 * 60 },
  { label: '8 saat sonra',  fireAt: () => Math.floor(Date.now() / 1000) + 8 * 60 * 60 },
  {
    label: 'Yarın 09:00', fireAt: () => {
      const d = new Date(); d.setDate(d.getDate() + 1); d.setHours(9, 0, 0, 0)
      return Math.floor(d.getTime() / 1000)
    },
  },
  {
    label: 'Yarın 18:00', fireAt: () => {
      const d = new Date(); d.setDate(d.getDate() + 1); d.setHours(18, 0, 0, 0)
      return Math.floor(d.getTime() / 1000)
    },
  },
]

// Common cron presets grouped by category.
const PRESET_GROUPS: { group: string; items: { label: string; expr: string }[] }[] = [
  {
    group: 'Dakikalar',
    items: [
      { label: 'Her dakika',    expr: '* * * * *'    },
      { label: 'Her 5 dakika',  expr: '*/5 * * * *'  },
      { label: 'Her 10 dakika', expr: '*/10 * * * *' },
      { label: 'Her 15 dakika', expr: '*/15 * * * *' },
      { label: 'Her 30 dakika', expr: '*/30 * * * *' },
    ],
  },
  {
    group: 'Saatler',
    items: [
      { label: 'Saat başı',    expr: '0 * * * *'    },
      { label: 'Her 2 saatte', expr: '0 */2 * * *'  },
      { label: 'Her 4 saatte', expr: '0 */4 * * *'  },
      { label: 'Her 6 saatte', expr: '0 */6 * * *'  },
      { label: 'Her 12 saatte',expr: '0 */12 * * *' },
    ],
  },
  {
    group: 'Günlük',
    items: [
      { label: 'Gece yarısı', expr: '0 0 * * *'  },
      { label: '06:00',       expr: '0 6 * * *'  },
      { label: '08:00',       expr: '0 8 * * *'  },
      { label: '09:00',       expr: '0 9 * * *'  },
      { label: '12:00',       expr: '0 12 * * *' },
      { label: '18:00',       expr: '0 18 * * *' },
      { label: '21:00',       expr: '0 21 * * *' },
    ],
  },
  {
    group: 'Haftalık',
    items: [
      { label: 'Hafta içi 09:00',  expr: '0 9 * * 1-5'  },
      { label: 'Pazartesi 08:00',  expr: '0 8 * * 1'    },
      { label: 'Pazartesi 09:00',  expr: '0 9 * * 1'    },
      { label: 'Cuma 17:00',       expr: '0 17 * * 5'   },
      { label: 'Hafta sonu 10:00', expr: '0 10 * * 6,0' },
    ],
  },
  {
    group: 'Aylık',
    items: [
      { label: "Ayın 1'i 09:00",  expr: '0 9 1 * *'  },
      { label: "Ayın 15'i 09:00", expr: '0 9 15 * *' },
      { label: "Ayın sonu 18:00", expr: '0 18 28-31 * *' },
    ],
  },
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
  // 'cron' = recurring schedule, 'oneshot' = single run
  const [mode, setMode] = useState<'cron' | 'oneshot'>('cron')
  // For one-shot: either a preset-computed timestamp or custom datetime string.
  const [oneShotFireAt, setOneShotFireAt] = useState<number>(0)
  const [oneShotCustom, setOneShotCustom] = useState('')

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
    if (!agentId) { onError('Ajan seçimi zorunlu'); return }
    if (!prompt.trim()) { onError('Prompt zorunlu'); return }

    if (mode === 'oneshot') {
      // Resolve fireAt: custom input takes precedence over preset.
      let fireAt = oneShotFireAt
      if (oneShotCustom) {
        const ms = new Date(oneShotCustom).getTime()
        if (isNaN(ms)) { onError('Geçersiz tarih/saat'); return }
        fireAt = Math.floor(ms / 1000)
      }
      if (!fireAt || fireAt <= Math.floor(Date.now() / 1000)) {
        onError('Çalışma zamanı gelecekte olmalı — bir taslak veya özel zaman seçin')
        return
      }
      try {
        const s = await api.createSchedule({ agentId, prompt: prompt.trim(), oneShot: true, fireAt, enabled: true })
        setSchedules((prev) => [s, ...prev])
        setPrompt(''); setOneShotFireAt(0); setOneShotCustom('')
      } catch (e) { onError((e as Error).message) }
      return
    }

    if (!cronExpr.trim()) { onError('Cron ifadesi zorunlu'); return }
    try {
      const s = await api.createSchedule({ agentId, cronExpr: cronExpr.trim(), prompt: prompt.trim(), enabled: true })
      setSchedules((prev) => [s, ...prev])
      setPrompt('')
    } catch (e) { onError((e as Error).message) }
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
    <div className="flex min-h-0 flex-1 flex-col p-4">
      {/* New schedule form */}
      <div className="mb-4 space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
        {/* Mode toggle */}
        <div className="flex gap-1 text-xs">
          <button
            onClick={() => setMode('cron')}
            className={`rounded px-2 py-0.5 ${mode === 'cron' ? 'bg-[var(--color-accent)] text-white' : 'border border-[var(--color-border)] text-[var(--color-text-dim)] hover:bg-[var(--color-bg)]'}`}
          >
            🔁 Tekrarlayan
          </button>
          <button
            onClick={() => setMode('oneshot')}
            className={`rounded px-2 py-0.5 ${mode === 'oneshot' ? 'bg-[var(--color-accent)] text-white' : 'border border-[var(--color-border)] text-[var(--color-text-dim)] hover:bg-[var(--color-bg)]'}`}
          >
            ⏱ Tek seferlik
          </button>
        </div>

        <div className="flex flex-wrap items-start gap-2">
          <AgentPicker agents={agents} value={agentId} onChange={setAgentId} />

          {mode === 'cron' ? (
            <>
              <select
                value={cronExpr}
                onChange={(e) => setCronExpr(e.target.value)}
                className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none"
              >
                {PRESET_GROUPS.map((g) => (
                  <optgroup key={g.group} label={g.group}>
                    {g.items.map((p) => (
                      <option key={p.expr} value={p.expr}>
                        {p.label} ({p.expr})
                      </option>
                    ))}
                  </optgroup>
                ))}
              </select>
              <input
                value={cronExpr}
                onChange={(e) => setCronExpr(e.target.value)}
                placeholder="cron: dk sa gün ay haftagünü"
                className="w-44 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 font-mono text-sm outline-none focus:border-[var(--color-accent)]"
              />
            </>
          ) : (
            <div className="flex flex-wrap gap-1">
              {ONE_SHOT_PRESETS.map((p) => {
                const fireAt = p.fireAt()
                const active = oneShotFireAt === fireAt && !oneShotCustom
                return (
                  <button
                    key={p.label}
                    onClick={() => { setOneShotFireAt(fireAt); setOneShotCustom('') }}
                    className={`rounded px-2 py-0.5 text-xs ${active ? 'bg-[var(--color-accent)] text-white' : 'border border-[var(--color-border)] hover:bg-[var(--color-bg)]'}`}
                  >
                    {p.label}
                  </button>
                )
              })}
              <input
                type="datetime-local"
                value={oneShotCustom}
                onChange={(e) => { setOneShotCustom(e.target.value); setOneShotFireAt(0) }}
                className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-0.5 text-xs outline-none focus:border-[var(--color-accent)]"
                title="Özel tarih/saat"
              />
            </div>
          )}
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
                  {PRESET_GROUPS.map((g) => (
                    <optgroup key={g.group} label={g.group}>
                      {g.items.map((p) => (
                        <option key={p.expr} value={p.expr}>
                          {p.label} ({p.expr})
                        </option>
                      ))}
                    </optgroup>
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
            {!s.oneShot && (
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
            )}
            {(() => {
              const owner = agents.find((a) => a.id === s.agentId)
              return owner ? (
                <AgentAvatar agent={owner} size={28} />
              ) : (
                <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--color-surface-2)] text-[10px] text-[var(--color-text-dim)]">?</span>
              )
            })()}
            <div className="flex-1">
              <div className="flex items-center gap-2">
                {s.oneShot ? (
                  <span className="flex items-center gap-1 text-xs font-medium text-[var(--color-accent)]">
                    <Timer size={12} /> Tek seferlik
                  </span>
                ) : (
                  <span className="font-mono text-[var(--color-accent)]">{s.cronExpr}</span>
                )}
                <span className="text-xs text-[var(--color-text-dim)]">{agentName(s.agentId)}</span>
              </div>
              <div className="text-xs text-[var(--color-text-dim)]">Prompt: {s.prompt}</div>
              <div className="text-xs text-[var(--color-text-dim)]">
                {s.oneShot ? (
                  <>Çalışma zamanı: {fmtTime(s.fireAt ?? 0)}</>
                ) : (
                  <>
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
                  </>
                )}
              </div>
            </div>
            {!s.oneShot && (
              <button
                onClick={() => runNow(s)}
                disabled={runningId === s.id}
                className="text-[var(--color-text-dim)] hover:text-[var(--color-success)] disabled:opacity-40"
                title="Şimdi çalıştır"
              >
                {runningId === s.id ? <Hourglass size={15} /> : <Play size={15} />}
              </button>
            )}
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
