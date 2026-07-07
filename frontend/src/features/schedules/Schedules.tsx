import { useEffect, useRef, useState } from 'react'
import { Play, Hourglass, Pencil, X, Clock, Workflow, Repeat, LayoutGrid } from 'lucide-react'
import { api } from '@/api'
import type { Agent, Flow, Schedule, AutomationTriggerKind } from '@/types'
import { AgentPicker } from '@/shared/components/agents/AgentPicker'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { Button, TagEditor, PaneHeader } from '@/shared/components'
import { normalizeAvatar } from '@/shared/lib/avatar'
import { Automations } from './Automations'

// TargetModeToggle is a small segmented control letting a schedule/automation
// target either a single agent or an orchestration flow.
export function TargetModeToggle({
  mode,
  onChange,
}: {
  mode: 'agent' | 'flow'
  onChange: (m: 'agent' | 'flow') => void
}) {
  return (
    <div className="inline-flex overflow-hidden rounded border border-[var(--color-border)] text-xs">
      {(['agent', 'flow'] as const).map((m) => (
        <button
          key={m}
          type="button"
          onClick={() => onChange(m)}
          className={`px-2 py-1 transition ${
            mode === m
              ? 'bg-[var(--color-accent)] text-white'
              : 'bg-[var(--color-bg)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
          }`}
        >
          {m === 'agent' ? 'Ajan' : 'Akış'}
        </button>
      ))}
    </div>
  )
}

// FlowPicker is a simple dropdown of the workspace flows (mirrors AgentPicker).
export function FlowPicker({
  flows,
  value,
  onChange,
}: {
  flows: Flow[]
  value: string
  onChange: (id: string) => void
}) {
  // Selected flow's icon (mojibake-safe emoji, or a Workflow glyph fallback),
  // shown next to the dropdown — mirrors the AgentPicker's leading avatar.
  const selected = flows.find((f) => f.id === value)
  const selectedEmoji = normalizeAvatar(selected?.emoji)
  return (
    <div className="flex items-center gap-1.5 rounded border border-[var(--color-border)] bg-[var(--color-bg)] pl-1.5 focus-within:border-[var(--color-accent)]">
      <span className="flex h-5 w-5 shrink-0 items-center justify-center rounded bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
        {selectedEmoji ? <span className="text-sm leading-none">{selectedEmoji}</span> : <Workflow size={13} />}
      </span>
      <select
        data-testid="flow-picker"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="rounded bg-transparent py-1 pr-2 text-sm outline-none"
      >
        <option value="">Akış seç…</option>
        {flows.map((f) => (
          <option key={f.id} value={f.id}>
            {normalizeAvatar(f.emoji) ? `${normalizeAvatar(f.emoji)} ${f.name}` : f.name}
          </option>
        ))}
      </select>
    </div>
  )
}

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
  // Workspace flows, for flow-backed schedules/automations (target = a flow).
  const [flows, setFlows] = useState<Flow[]>([])
  // Briefly highlight a deep-linked schedule once it is present in the list.
  const [highlightId, setHighlightId] = useState<string | null>(null)
  const focusRef = useRef<HTMLDivElement | null>(null)
  const [agentId, setAgentId] = useState('')
  // Create-form target: an agent (prompt delivery) or a flow (orchestration run).
  const [targetMode, setTargetMode] = useState<'agent' | 'flow'>('agent')
  const [flowId, setFlowId] = useState('')
  const [cronExpr, setCronExpr] = useState('*/5 * * * *')
  const [prompt, setPrompt] = useState('')
  // Optional end date for the new schedule (datetime-local string; '' = none).
  const [expiresAt, setExpiresAt] = useState('')

  // Inline edit state (one schedule edited at a time).
  const [editId, setEditId] = useState<string | null>(null)
  const [editAgentId, setEditAgentId] = useState('')
  const [editTargetMode, setEditTargetMode] = useState<'agent' | 'flow'>('agent')
  const [editFlowId, setEditFlowId] = useState('')
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

  // Top-level tab: cron schedules vs the two automation kinds. The Automations
  // component reports its per-kind counts up (autoCounts) for the tab badges.
  const [tab, setTab] = useState<'schedules' | AutomationTriggerKind>('schedules')
  const [autoCounts, setAutoCounts] = useState({ tag: 0, board: 0 })

  const reload = () =>
    api.listSchedules().then(setSchedules).catch((e) => onError(e.message))

  useEffect(() => {
    reload()
    api.listFlows().then(setFlows).catch((e) => onError((e as Error).message))
    api
      .getWorkspaceSettings()
      .then((s) => setPauseAutonomy(s.pauseAutonomy))
      .catch((e) => onError((e as Error).message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const flowName = (id?: string) => flows.find((f) => f.id === id)?.name ?? id ?? '—'
  // Resolved (mojibake-safe) flow emoji, or null when the flow has none.
  const flowEmoji = (id?: string) => normalizeAvatar(flows.find((f) => f.id === id)?.emoji)

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
    if (!cronExpr.trim()) {
      onError('Cron ifadesi zorunlu')
      return
    }
    if (targetMode === 'flow') {
      if (!flowId) {
        onError('Akış seçilmeli')
        return
      }
    } else {
      if (!agentId) {
        onError('Ajan zorunlu')
        return
      }
      if (!prompt.trim()) {
        onError('Prompt zorunlu')
        return
      }
    }
    const expUnix = localInputToUnix(expiresAt)
    if (expUnix && expUnix <= Math.floor(Date.now() / 1000)) {
      onError('Son tarih gelecekte olmalı')
      return
    }
    try {
      const s = await api.createSchedule({
        ...(targetMode === 'flow' ? { flowId } : { agentId }),
        cronExpr: cronExpr.trim(),
        prompt: prompt.trim(),
        enabled: true,
        expiresAt: expUnix,
      })
      setSchedules((prev) => [s, ...prev])
      setPrompt('')
      setExpiresAt('')
      setFlowId('')
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
    setEditTargetMode(s.flowId ? 'flow' : 'agent')
    setEditFlowId(s.flowId ?? '')
    setEditCronExpr(s.cronExpr)
    setEditPrompt(s.prompt)
    setEditExpiresAt(unixToLocalInput(s.expiresAt))
  }

  const cancelEdit = () => setEditId(null)

  const saveEdit = async (s: Schedule) => {
    if (!editCronExpr.trim()) {
      onError('Cron ifadesi zorunlu')
      return
    }
    if (editTargetMode === 'flow') {
      if (!editFlowId) {
        onError('Akış seçilmeli')
        return
      }
    } else {
      if (!editAgentId) {
        onError('Ajan zorunlu')
        return
      }
      if (!editPrompt.trim()) {
        onError('Prompt zorunlu')
        return
      }
    }
    const expUnix = localInputToUnix(editExpiresAt)
    if (expUnix && expUnix <= Math.floor(Date.now() / 1000)) {
      onError('Son tarih gelecekte olmalı')
      return
    }
    try {
      const updated = await api.updateSchedule(s.id, {
        // Send the active target explicitly; the other is cleared server-side.
        ...(editTargetMode === 'flow'
          ? { flowId: editFlowId, agentId: '' }
          : { agentId: editAgentId, flowId: '' }),
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
    <div className="flex min-h-0 flex-1 flex-col">
      <PaneHeader
        title="Otomasyon"
        right={
          pauseAutonomy !== null ? (
            <button
              data-testid="workspace-pause-autonomy-toggle"
              onClick={togglePauseAutonomy}
              disabled={savingPause}
              aria-pressed={pauseAutonomy}
              title={
                pauseAutonomy
                  ? 'Bu workspace’te otonomi duraklatıldı — yalnız zamanlama çağrılarını bloklar (manuel sohbet + “şimdi çalıştır” etkilenmez). Tıkla: sürdür.'
                  : 'Bu workspace’te otonomiyi duraklat — yalnız zamanlama çağrılarını bloklar (manuel sohbet + “şimdi çalıştır” etkilenmez).'
              }
              className={`flex items-center gap-2 rounded-lg border px-2.5 py-1 text-xs transition disabled:opacity-40 ${
                pauseAutonomy
                  ? 'border-[var(--color-danger)] text-[var(--color-danger)]'
                  : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
              }`}
            >
              <span
                className={`h-4 w-8 flex-shrink-0 rounded-full transition ${
                  pauseAutonomy ? 'bg-[var(--color-danger)]' : 'bg-[var(--color-border)]'
                }`}
              >
                <span
                  className={`block h-4 w-4 rounded-full bg-white transition ${
                    pauseAutonomy ? 'translate-x-4' : ''
                  }`}
                />
              </span>
              <span className="hidden sm:inline">
                {pauseAutonomy ? 'Otonomi duraklatıldı' : 'Otonomiyi duraklat'}
              </span>
            </button>
          ) : undefined
        }
      />
      {/* Single scroll region: the section header + create form + list + the
          Automations section all scroll together (previously the header/form were
          pinned outside the scroll and ate vertical space). */}
      <div className="min-h-0 flex-1 overflow-y-auto p-4">

      {/* Unified tab bar: cron schedules + the two automation kinds. */}
      <div className="mb-4 flex items-center gap-1 border-b border-[var(--color-border)]">
        {([
          { key: 'schedules', label: 'Zamanlamalar', icon: Clock, color: 'text-[#6b8e23]', count: schedules.length },
          { key: 'tag', label: 'Etiket otomasyonları', icon: Repeat, color: 'text-violet-500', count: autoCounts.tag },
          { key: 'board', label: 'Pano otomasyonları', icon: LayoutGrid, color: 'text-sky-500', count: autoCounts.board },
        ] as const).map((t) => {
          const active = tab === t.key
          const Icon = t.icon
          return (
            <button
              key={t.key}
              type="button"
              onClick={() => setTab(t.key)}
              className={`-mb-px flex items-center gap-1.5 border-b-2 px-3 py-2 text-sm font-medium transition ${
                active
                  ? 'border-[var(--color-accent)] text-[var(--color-text)]'
                  : 'border-transparent text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
              }`}
            >
              <Icon size={15} className={active ? t.color : ''} />
              <span className="hidden sm:inline">{t.label}</span>
              <span
                className={`rounded-full px-1.5 py-0.5 text-[10px] ${
                  active
                    ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                    : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]'
                }`}
              >
                {t.count}
              </span>
            </button>
          )
        })}
      </div>

      {tab === 'schedules' && (
      <>
      {/* Section header — schedules are cron/time based (sky accent), distinct
          from the tag-/board-triggered Automations in their own tabs. */}
      <div className="mb-2 flex items-center gap-2 text-sm font-semibold text-[var(--color-text)]">
        <Clock size={15} className="text-[#6b8e23]" />
        Zamanlamalar (cron / zaman tabanlı)
      </div>

      {/* New schedule form */}
      <div className="mb-4 space-y-2 rounded-lg border border-l-4 border-[var(--color-border)] border-l-[#6b8e23] bg-[var(--color-surface)] p-3">
        <div className="flex flex-wrap items-start gap-2">
          <TargetModeToggle mode={targetMode} onChange={setTargetMode} />
          {targetMode === 'flow' ? (
            <div data-testid="schedule-create-flow-wrap">
              <FlowPicker flows={flows} value={flowId} onChange={setFlowId} />
            </div>
          ) : (
            <div data-testid="schedule-create-agent-wrap">
              <AgentPicker agents={agents} value={agentId} onChange={setAgentId} />
            </div>
          )}
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
            placeholder={
              targetMode === 'flow'
                ? 'Akış girdisi (opsiyonel)'
                : 'Prompt (zorunlu) — ajana gönderilecek talimat'
            }
            className="flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
          />
          <div data-testid="schedule-create-submit">
            <Button onClick={create}>+ Zamanlama</Button>
          </div>
        </div>
      </div>

      {/* Schedule list */}
      <div className="space-y-2">
        {schedules.length === 0 && (
          <p className="text-sm text-[var(--color-text-dim)]">Henüz zamanlama yok.</p>
        )}
        {schedules.map((s) =>
          editId === s.id ? (
            <div
              key={s.id}
              className="space-y-2 rounded-lg border border-l-4 border-[var(--color-accent)] border-l-[#6b8e23] bg-[var(--color-surface)] p-3 text-sm"
            >
              <div className="flex flex-wrap items-start gap-2">
                <TargetModeToggle mode={editTargetMode} onChange={setEditTargetMode} />
                {editTargetMode === 'flow' ? (
                  <FlowPicker flows={flows} value={editFlowId} onChange={setEditFlowId} />
                ) : (
                  <AgentPicker agents={agents} value={editAgentId} onChange={setEditAgentId} />
                )}
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
            className={`flex items-center gap-3 rounded-lg border border-l-4 border-l-[#6b8e23] bg-[var(--color-surface)] px-3 py-2 text-sm transition ${
              highlightId === s.id
                ? 'border-[var(--color-accent)] ring-2 ring-[var(--color-accent)]'
                : 'border-[var(--color-border)]'
            }`}
          >
            {/* Enable toggle on top, agent/flow icon below (stacked vertically). */}
            <div className="flex shrink-0 flex-col items-center gap-2">
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
                if (s.flowId) {
                  const fe = flowEmoji(s.flowId)
                  return (
                    <span
                      className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--color-accent-soft)] text-[var(--color-accent)]"
                      title="Akış tabanlı zamanlama"
                    >
                      {fe ? <span className="text-base leading-none">{fe}</span> : <Workflow size={15} />}
                    </span>
                  )
                }
                const owner = agents.find((a) => a.id === s.agentId)
                return owner ? (
                  <AgentAvatar agent={owner} size={28} />
                ) : (
                  <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--color-surface-2)] text-[10px] text-[var(--color-text-dim)]">?</span>
                )
              })()}
            </div>
            <div className="flex-1">
              <div className="flex items-center gap-2">
                <span className="font-mono text-[var(--color-accent)]">{s.cronExpr}</span>
                <span className="text-xs text-[var(--color-text-dim)]">
                  {s.flowId ? `${flowEmoji(s.flowId) ?? '🔀'} ${flowName(s.flowId)}` : agentName(s.agentId)}
                </span>
                <span className="ml-auto font-mono text-[10px] text-[var(--color-text-dim)] opacity-60" title="Zamanlama ID">
                  {s.id}
                </span>
              </div>
              {s.prompt || !s.flowId ? (
                <div className="text-xs text-[var(--color-text-dim)]">
                  {s.flowId ? 'Girdi' : 'Prompt'}: {s.prompt}
                </div>
              ) : null}
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
      </div>
      </>
      )}

      {/* Automations live in their own tabs. The component stays mounted (even on
          the schedules tab, activeKind=null → renders nothing) so its item fetch
          and per-kind counts stay live for the tab badges. */}
      <Automations
        agents={agents}
        flows={flows}
        onError={onError}
        activeKind={tab === 'schedules' ? null : tab}
        onCounts={setAutoCounts}
      />
      </div>
    </div>
  )
}
