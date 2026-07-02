import { useEffect, useRef, useState } from 'react'
import { Play, Hourglass, Pencil, X } from 'lucide-react'
import { api } from '../../api'
import type { Agent, Schedule } from '../../types'
import { AgentPicker } from '../agents/AgentPicker'
import { AgentAvatar } from '../agents/AgentAvatar'
import { Button, TagEditor } from '../common'
import { Automations } from './Automations'

interface Props {
  agents: Agent[]
  /** Deep-link target: scroll to and highlight this schedule once loaded. */
  focusId?: string | null
  onError: (msg: string) => void
}

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

// Convert a unix-seconds timestamp to the "YYYY-MM-DDTHH:mm" string a
// datetime-local input expects (in local time). 0/undefined → empty string.
function unixToLocalInput(unix?: number): string {
  if (!unix) return ''
  const d = new Date(unix * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// Parse a datetime-local input string back to unix seconds. Empty → 0.
function localInputToUnix(s: string): number {
  if (!s) return 0
  const ms = new Date(s).getTime()
  return isNaN(ms) ? 0 : Math.floor(ms / 1000)
}

export function Schedules({ agents, focusId, onError }: Props) {
  const [schedules, setSchedules] = useState<Schedule[]>([])
  // Briefly highlight a deep-linked schedule once it is present in the list.
  const [highlightId, setHighlightId] = useState<string | null>(null)
  const focusRef = useRef<HTMLDivElement | null>(null)
  const [agentId, setAgentId] = useState('')
  const [cronExpr, setCronExpr] = useState('*/5 * * * *')
  const [prompt, setPrompt] = useState('')
  // Optional end date for the new schedule (datetime-local string; '' = none).
  const [expiresAt, setExpiresAt] = useState('')

  // Inline edit state (one schedule edited at a time).
  const [editId, setEditId] = useState<string | null>(null)
  const [editAgentId, setEditAgentId] = useState('')
  const [editCronExpr, setEditCronExpr] = useState('')
  const [editPrompt, setEditPrompt] = useState('')
  const [editExpiresAt, setEditExpiresAt] = useState('')

  // Id of the schedule currently being run manually (disables its Run button).
  const [runningId, setRunningId] = useState<string | null>(null)

  // Per-workspace autonomy brake. Moved here from the Workspace settings screen
  // (the app-global pause was removed): when on, this workspace's scheduled calls
  // are blocked before reaching a model. Manual chat / run-now are unaffected.
  // null = not loaded yet (hide the toggle until we know the real value).
  const [pauseAutonomy, setPauseAutonomy] = useState<boolean | null>(null)
  const [savingPause, setSavingPause] = useState(false)

  const reload = () =>
    api.listSchedules().then(setSchedules).catch((e) => onError(e.message))

  useEffect(() => {
    reload()
    api
      .getWorkspaceSettings()
      .then((s) => setPauseAutonomy(s.pauseAutonomy))
      .catch((e) => onError((e as Error).message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const togglePauseAutonomy = async () => {
    if (pauseAutonomy === null) return
    const next = !pauseAutonomy
    setPauseAutonomy(next) // optimistic
    setSavingPause(true)
    try {
      const updated = await api.updateWorkspaceSettings({ pauseAutonomy: next })
      setPauseAutonomy(updated.pauseAutonomy)
    } catch (e) {
      setPauseAutonomy(!next) // rollback
      onError((e as Error).message)
    } finally {
      setSavingPause(false)
    }
  }

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
    const expUnix = localInputToUnix(expiresAt)
    if (expUnix && expUnix <= Math.floor(Date.now() / 1000)) {
      onError('Son tarih gelecekte olmalı')
      return
    }
    try {
      const s = await api.createSchedule({
        agentId,
        cronExpr: cronExpr.trim(),
        prompt: prompt.trim(),
        enabled: true,
        expiresAt: expUnix,
      })
      setSchedules((prev) => [s, ...prev])
      setPrompt('')
      setExpiresAt('')
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
    setEditExpiresAt(unixToLocalInput(s.expiresAt))
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
    const expUnix = localInputToUnix(editExpiresAt)
    if (expUnix && expUnix <= Math.floor(Date.now() / 1000)) {
      onError('Son tarih gelecekte olmalı')
      return
    }
    try {
      const updated = await api.updateSchedule(s.id, {
        agentId: editAgentId,
        cronExpr: editCronExpr.trim(),
        prompt: editPrompt.trim(),
        expiresAt: expUnix,
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

  const setTags = async (s: Schedule, tags: string[]) => {
    setSchedules((prev) => prev.map((x) => (x.id === s.id ? { ...x, tags } : x)))
    try {
      await api.setScheduleTags(s.id, tags)
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
    <div className="flex min-h-0 flex-1 flex-col p-4">
      {/* Per-workspace autonomy pause (moved here from Workspace settings). */}
      {pauseAutonomy !== null && (
        <div
          data-testid="workspace-pause-autonomy"
          className={`mb-3 flex items-center gap-3 rounded-lg border px-3 py-2 text-sm ${
            pauseAutonomy
              ? 'border-[var(--color-danger)] bg-[color-mix(in_srgb,var(--color-danger)_8%,transparent)]'
              : 'border-[var(--color-border)] bg-[var(--color-surface)]'
          }`}
        >
          <button
            data-testid="workspace-pause-autonomy-toggle"
            onClick={togglePauseAutonomy}
            disabled={savingPause}
            className={`h-4 w-8 flex-shrink-0 rounded-full transition disabled:opacity-40 ${
              pauseAutonomy ? 'bg-[var(--color-danger)]' : 'bg-[var(--color-border)]'
            }`}
            title={pauseAutonomy ? 'Otonomi duraklatıldı' : 'Otonomi etkin'}
          >
            <span
              className={`block h-4 w-4 rounded-full bg-white transition ${
                pauseAutonomy ? 'translate-x-4' : ''
              }`}
            />
          </button>
          <div className="flex-1">
            <div className="font-medium">
              {pauseAutonomy ? 'Bu workspace’te otonomi duraklatıldı' : 'Bu workspace’te otonomiyi duraklat'}
            </div>
            <div className="text-xs text-[var(--color-text-dim)]">
              Yalnızca bu workspace’in zamanlama çağrılarını bloklar. Manuel sohbet ve “şimdi çalıştır” etkilenmez.
            </div>
          </div>
        </div>
      )}

      {/* New schedule form */}
      <div className="mb-4 space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
        <div className="flex flex-wrap items-start gap-2">
          <div data-testid="schedule-create-agent-wrap">
            <AgentPicker agents={agents} value={agentId} onChange={setAgentId} />
          </div>
          <select
            data-testid="schedule-create-cron-preset-select"
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
            data-testid="schedule-create-cron-input"
            value={cronExpr}
            onChange={(e) => setCronExpr(e.target.value)}
            placeholder="cron: dk sa gün ay haftagünü"
            className="w-44 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 font-mono text-sm outline-none focus:border-[var(--color-accent)]"
          />
          <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
            Son tarih (ops.):
            <input
              data-testid="schedule-create-expires-input"
              type="datetime-local"
              value={expiresAt}
              onChange={(e) => setExpiresAt(e.target.value)}
              className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
              title="Bu tarihten sonra zamanlama çalışmaz (opsiyonel)"
            />
            {expiresAt && (
              <button
                onClick={() => setExpiresAt('')}
                className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
                title="Son tarihi temizle"
                type="button"
              >
                <X size={13} />
              </button>
            )}
          </label>
        </div>
        <div className="flex flex-wrap items-end gap-2">
          <input
            data-testid="schedule-create-prompt-input"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder="Prompt (zorunlu) — ajana gönderilecek talimat"
            className="flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
          />
          <div data-testid="schedule-create-submit">
            <Button onClick={create}>+ Zamanlama</Button>
          </div>
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
                  data-testid="schedule-edit-cron-input"
                  data-schedule-id={s.id}
                  value={editCronExpr}
                  onChange={(e) => setEditCronExpr(e.target.value)}
                  placeholder="cron: dk sa gün ay haftagünü"
                  className="w-44 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 font-mono text-sm outline-none focus:border-[var(--color-accent)]"
                />
                <label className="flex items-center gap-1 text-xs text-[var(--color-text-dim)]">
                  Son tarih (ops.):
                  <input
                    type="datetime-local"
                    value={editExpiresAt}
                    onChange={(e) => setEditExpiresAt(e.target.value)}
                    className="rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
                    title="Bu tarihten sonra zamanlama çalışmaz (opsiyonel)"
                  />
                  {editExpiresAt && (
                    <button
                      onClick={() => setEditExpiresAt('')}
                      className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
                      title="Son tarihi temizle"
                      type="button"
                    >
                      <X size={13} />
                    </button>
                  )}
                </label>
              </div>
              <div className="flex flex-wrap items-end gap-2">
                <input
                  data-testid="schedule-edit-prompt-input"
                  data-schedule-id={s.id}
                  value={editPrompt}
                  onChange={(e) => setEditPrompt(e.target.value)}
                  placeholder="Prompt (zorunlu) — ajana gönderilecek talimat"
                  className="flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
                />
                <div data-testid="schedule-edit-save" data-schedule-id={s.id}>
                  <Button onClick={() => saveEdit(s)}>Kaydet</Button>
                </div>
                <div data-testid="schedule-edit-cancel" data-schedule-id={s.id}>
                  <Button variant="secondary" onClick={cancelEdit}>
                    İptal
                  </Button>
                </div>
              </div>
            </div>
          ) : (
          <div
            key={s.id}
            data-testid="schedule-row"
            data-schedule-id={s.id}
            ref={s.id === focusId ? focusRef : undefined}
            className={`flex items-center gap-3 rounded-lg border bg-[var(--color-surface)] px-3 py-2 text-sm transition ${
              highlightId === s.id
                ? 'border-[var(--color-accent)] ring-2 ring-[var(--color-accent)]'
                : 'border-[var(--color-border)]'
            }`}
          >
            <button
              data-testid="schedule-enable-toggle"
              data-schedule-id={s.id}
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
                <span className="font-mono text-[var(--color-accent)]">{s.cronExpr}</span>
                <span className="text-xs text-[var(--color-text-dim)]">{agentName(s.agentId)}</span>
                <span className="ml-auto font-mono text-[10px] text-[var(--color-text-dim)] opacity-60" title="Zamanlama ID">
                  {s.id}
                </span>
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
              {s.expiresAt ? (
                <div className="text-xs text-[var(--color-text-dim)]">
                  Son tarih:{' '}
                  <span
                    className={
                      s.expiresAt <= Math.floor(Date.now() / 1000)
                        ? 'text-[var(--color-danger)]'
                        : ''
                    }
                  >
                    {fmtTime(s.expiresAt)}
                    {s.expiresAt <= Math.floor(Date.now() / 1000) && ' (süresi doldu)'}
                  </span>
                </div>
              ) : null}
              <div className="mt-1">
                <TagEditor tags={s.tags ?? []} onChange={(tags) => setTags(s, tags)} className="py-1" />
              </div>
            </div>
            <button
              data-testid="schedule-run-now"
              data-schedule-id={s.id}
              onClick={() => runNow(s)}
              disabled={runningId === s.id}
              className="text-[var(--color-text-dim)] hover:text-[var(--color-success)] disabled:opacity-40"
              title="Şimdi çalıştır"
            >
              {runningId === s.id ? <Hourglass size={15} /> : <Play size={15} />}
            </button>
            <button
              data-testid="schedule-edit"
              data-schedule-id={s.id}
              onClick={() => startEdit(s)}
              className="text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
              title="Düzenle"
            >
              <Pencil size={15} />
            </button>
            <button
              data-testid="schedule-delete"
              data-schedule-id={s.id}
              onClick={() => remove(s)}
              className="text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
              title="Sil"
            >
              <X size={15} />
            </button>
          </div>
          ),
        )}

        {/* Tag-triggered automations (event-driven loops) live in the same screen. */}
        <Automations agents={agents} onError={onError} />
      </div>
    </div>
  )
}
