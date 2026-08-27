import { useCallback, useEffect, useRef, useState } from 'react'
import { Clock, Hash, LayoutGrid, Repeat, Zap } from 'lucide-react'
import { api } from '@/api'
import { useVisiblePoll } from '@/shared/hooks/useVisiblePoll'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { SIGNAL_AUTOMATIONS, SIGNAL_SCHEDULES } from '@/app/eventToRefreshSignals'
import type {
  Agent,
  Automation,
  AutomationTriggerKind,
  BoardColumnDef,
  Flow,
  Schedule,
} from '@/types'
import { PaneHeader, toast } from '@/shared/components'
import { AutomationCard } from './AutomationCard'
import { AutomationModal } from './AutomationModal'
import { BoardColumn } from './BoardColumn'
import { COLUMN_ACCENT, DEFAULT_COLUMNS } from './automationMeta'
import { ScheduleCard } from './ScheduleCard'
import { ScheduleModal } from './ScheduleModal'
import { count } from '@/shared/lib/format'

// Backstop refresh for the lane-header metrics; visibility-gated.
const LIVE_STATS_POLL_MS = 15000

// compact renders a large count as a short human string (1_240_000 → "1.2M",
// 850_000 → "850k"), for the token lane's live workspace total.
function compact(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${Math.round(n / 1_000)}k`
  return String(n)
}

interface Props {
  agents: Agent[]
  /** Deep-link target: scroll to and highlight this schedule once loaded. */
  focusId?: string | null
  onError: (msg: string) => void
}

// Which popup is open: a schedule form, or an automation form of one kind.
type Editor =
  | { lane: 'schedules'; editing: Schedule | null }
  | { lane: AutomationTriggerKind; editing: Automation | null }

// AutomationBoard is the Otomasyon screen: a three-lane board where each lane is
// one rule kind — cron schedules, tag-triggered automations and board-triggered
// automations. Rules are read-only cards; creating and editing happen in a popup
// (ScheduleModal / AutomationModal) so the lanes stay compact.
export function AutomationBoard({ agents, focusId, onError }: Props) {
  const schedulesTick = useRefreshTrigger(SIGNAL_SCHEDULES)
  const automationsTick = useRefreshTrigger(SIGNAL_AUTOMATIONS)
  const [schedules, setSchedules] = useState<Schedule[]>([])
  const [automations, setAutomations] = useState<Automation[]>([])
  // Workspace flows, for flow-backed schedules/automations (target = a flow).
  const [flows, setFlows] = useState<Flow[]>([])
  // Workspace board columns, for the board-trigger source/target filters.
  const [columns, setColumns] = useState<BoardColumnDef[]>(DEFAULT_COLUMNS)

  const [loadingSchedules, setLoadingSchedules] = useState(true)
  const [loadingAutomations, setLoadingAutomations] = useState(true)

  // Live workspace metrics shown in the token/counter lane headers so the user can
  // see how close the workspace is to the next fire (and calibrate intervals). null
  // until first fetch. Polled while the screen is open.
  const [liveStats, setLiveStats] = useState<{
    tokensToday: number
    messages: number
    tools: number
  } | null>(null)

  const [editor, setEditor] = useState<Editor | null>(null)
  // Id of the schedule currently being run manually (disables its Run button).
  const [runningId, setRunningId] = useState<string | null>(null)
  // Briefly highlight a deep-linked schedule once it is present in the list.
  const [highlightId, setHighlightId] = useState<string | null>(null)
  const focusRef = useRef<HTMLDivElement | null>(null)

  // Per-workspace autonomy brake: when on, this workspace's scheduled calls are
  // blocked before reaching a model. Manual chat / run-now are unaffected.
  // null = not loaded yet (hide the toggle until we know the real value).
  const [pauseAutonomy, setPauseAutonomy] = useState<boolean | null>(null)
  const [savingPause, setSavingPause] = useState(false)

  const reloadSchedules = useCallback(() => {
    void api
      .listSchedules()
      .then(setSchedules)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoadingSchedules(false))
  }, [onError])

  const reloadAutomations = useCallback(() => {
    void api
      .listAutomations()
      .then(setAutomations)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoadingAutomations(false))
  }, [onError])

  useEffect(() => {
    api
      .listFlows()
      .then(setFlows)
      .catch((e) => onError((e as Error).message))
    api
      .getWorkspaceSettings()
      .then((s) => {
        setPauseAutonomy(s.pauseAutonomy)
        if (s.boardColumns && s.boardColumns.length > 0) setColumns(s.boardColumns)
      })
      .catch((e) => onError((e as Error).message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => reloadSchedules(), [reloadSchedules, schedulesTick])

  useEffect(() => reloadAutomations(), [reloadAutomations, automationsTick])

  // Live workspace metrics for the lane headers: fetch on mount, then refresh on
  // an interval while the screen is open AND this window is visible. Silent on
  // failure — a stale/absent stat must never break the board.
  const loadLiveStats = useCallback(
    () =>
      api
        .getAutomationLiveStats()
        .then(setLiveStats)
        .catch(() => {}),
    [],
  )
  useEffect(() => {
    let alive = true
    api
      .getAutomationLiveStats()
      .then((s) => alive && setLiveStats(s))
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])
  useVisiblePoll(loadLiveStats, LIVE_STATS_POLL_MS, [loadLiveStats])

  // When a deep-link target is present and loaded, scroll it into view and flash
  // a highlight ring that fades after a moment.
  useEffect(() => {
    if (!focusId || !schedules.some((s) => s.id === focusId)) return
    setHighlightId(focusId)
    focusRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    const t = setTimeout(() => setHighlightId(null), 2500)
    return () => clearTimeout(t)
  }, [focusId, schedules])

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

  // ---- schedule actions -----------------------------------------------------

  const toggleSchedule = async (s: Schedule) => {
    setSchedules((prev) => prev.map((x) => (x.id === s.id ? { ...x, enabled: !x.enabled } : x)))
    try {
      await api.toggleSchedule(s.id, !s.enabled)
    } catch (e) {
      onError((e as Error).message)
      reloadSchedules()
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

  const setScheduleTags = async (s: Schedule, tags: string[]) => {
    setSchedules((prev) => prev.map((x) => (x.id === s.id ? { ...x, tags } : x)))
    try {
      await api.setScheduleTags(s.id, tags)
    } catch (e) {
      onError((e as Error).message)
      reloadSchedules()
    }
  }

  const removeSchedule = async (s: Schedule) => {
    if (!confirm('Zamanlama silinsin mi?')) return
    setEditor(null) // the delete button lives in the edit popup
    setSchedules((prev) => prev.filter((x) => x.id !== s.id))
    try {
      await api.deleteSchedule(s.id)
      toast.success('Zamanlama silindi')
    } catch (e) {
      onError((e as Error).message)
      reloadSchedules()
    }
  }

  // ---- automation actions ---------------------------------------------------

  const toggleAutomation = async (a: Automation) => {
    setAutomations((prev) => prev.map((x) => (x.id === a.id ? { ...x, enabled: !x.enabled } : x)))
    try {
      await api.toggleAutomation(a.id, !a.enabled)
      reloadAutomations() // toggling on resets the counter server-side; resync
    } catch (e) {
      onError((e as Error).message)
      reloadAutomations()
    }
  }

  const resetAutomation = async (a: Automation) => {
    try {
      await api.resetAutomation(a.id)
      reloadAutomations()
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const removeAutomation = async (a: Automation) => {
    if (!confirm('Otomasyon silinsin mi?')) return
    setEditor(null) // the delete button lives in the edit popup
    setAutomations((prev) => prev.filter((x) => x.id !== a.id))
    try {
      await api.deleteAutomation(a.id)
      toast.success('Otomasyon silindi')
    } catch (e) {
      onError((e as Error).message)
      reloadAutomations()
    }
  }

  const setSpawnTags = async (a: Automation, spawnTags: string[]) => {
    setAutomations((prev) => prev.map((x) => (x.id === a.id ? { ...x, spawnTags } : x)))
    try {
      await api.updateAutomation(a.id, { spawnTags })
    } catch (e) {
      onError((e as Error).message)
      reloadAutomations()
    }
  }

  // ---- render ---------------------------------------------------------------

  const byKind = (kind: AutomationTriggerKind) =>
    automations.filter((a) => (a.triggerKind ?? 'tag') === kind)

  const laneMeta: Record<
    AutomationTriggerKind,
    {
      title: string
      icon: typeof Repeat
      accent: string
      description: string
      addLabel: string
      emptyLabel: string
    }
  > = {
    tag: {
      title: 'Etiket otomasyonları',
      icon: Repeat,
      accent: COLUMN_ACCENT.tag,
      description: 'Tetikleyici etiketli bir oturum turu bitince son yanıtla çalışır.',
      addLabel: 'Yeni etiket otomasyonu',
      emptyLabel: 'Henüz etiket otomasyonu yok.',
    },
    board: {
      title: 'Pano otomasyonları',
      icon: LayoutGrid,
      accent: COLUMN_ACCENT.board,
      description: 'Bir kanban kartı taşınınca/oluşunca/değişince kart bağlamıyla çalışır.',
      addLabel: 'Yeni pano otomasyonu',
      emptyLabel: 'Henüz pano otomasyonu yok.',
    },
    token: {
      title: 'Token otomasyonları',
      icon: Zap,
      accent: COLUMN_ACCENT.token,
      description: 'Oturum/workspace token harcaması eşiği geçince bakım/temizlik için çalışır.',
      addLabel: 'Yeni token otomasyonu',
      emptyLabel: 'Henüz token otomasyonu yok.',
    },
    counter: {
      title: 'Sayaç otomasyonları',
      icon: Hash,
      accent: COLUMN_ACCENT.counter,
      description: 'Oturumun mesaj/tool sayısı aralığı geçince çalışır (token’dan kararlı ritim).',
      addLabel: 'Yeni sayaç otomasyonu',
      emptyLabel: 'Henüz sayaç otomasyonu yok.',
    },
  }

  // Live lane stat: shown only for token/counter lanes that actually hold a
  // workspace-scoped rule — the metric that rule keys on. Session-scoped rules vary
  // per session and have no single workspace-level value, so no stat for them.
  const laneStat = (kind: AutomationTriggerKind): React.ReactNode => {
    if (!liveStats) return undefined
    if (kind === 'token' && byKind('token').some((a) => a.tokenScope === 'workspace')) {
      return <span>bugün {compact(liveStats.tokensToday)} token</span>
    }
    if (kind === 'counter' && byKind('counter').some((a) => a.counterScope === 'workspace')) {
      return (
        <span>
          {count(liveStats.messages)} mesaj · {count(liveStats.tools)} tool
        </span>
      )
    }
    return undefined
  }

  const renderAutomationLane = (kind: AutomationTriggerKind) => {
    const items = byKind(kind)
    const meta = laneMeta[kind]
    return (
      <BoardColumn
        testId={`automation-lane-${kind}`}
        title={meta.title}
        icon={meta.icon}
        accent={meta.accent}
        count={items.length}
        description={meta.description}
        stat={laneStat(kind)}
        onAdd={() => setEditor({ lane: kind, editing: null })}
        addLabel={meta.addLabel}
        loading={loadingAutomations}
        loadingLabel="Otomasyonlar yükleniyor…"
        emptyLabel={meta.emptyLabel}
      >
        {items.map((a) => (
          <AutomationCard
            key={a.id}
            automation={a}
            isBoardKind={kind === 'board'}
            isTokenKind={kind === 'token'}
            isCounterKind={kind === 'counter'}
            agents={agents}
            flows={flows}
            columns={columns}
            onToggle={() => toggleAutomation(a)}
            onReset={() => resetAutomation(a)}
            onEdit={() => setEditor({ lane: kind, editing: a })}
            onSpawnTags={(tags) => setSpawnTags(a, tags)}
          />
        ))}
      </BoardColumn>
    )
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
                  className={`block h-4 w-4 rounded-full bg-[var(--color-text)] transition ${pauseAutonomy ? 'translate-x-4' : ''}`}
                />
              </span>
              <span className="hidden sm:inline">
                {pauseAutonomy ? 'Otonomi duraklatıldı' : 'Otonomiyi duraklat'}
              </span>
            </button>
          ) : undefined
        }
      />

      {/* Three lanes side by side. Below `md` (portrait phones / narrow windows)
          they become a snap-scrolling carousel: one near-full-width lane per
          screen, each with its own vertical card scroll. */}
      <div className="flex min-h-0 flex-1 snap-x snap-mandatory gap-3 overflow-x-auto p-3 md:snap-none md:overflow-x-hidden">
        <BoardColumn
          testId="automation-lane-schedules"
          title="Zamanlamalar"
          icon={Clock}
          accent={COLUMN_ACCENT.schedules}
          count={schedules.length}
          description="Cron / zaman tabanlı — ifade dolduğunda ajan ya da akış çalışır."
          onAdd={() => setEditor({ lane: 'schedules', editing: null })}
          addLabel="Yeni zamanlama"
          loading={loadingSchedules}
          loadingLabel="Zamanlamalar yükleniyor…"
          emptyLabel="Henüz zamanlama yok."
        >
          {schedules.map((s) => (
            <ScheduleCard
              key={s.id}
              ref={s.id === focusId ? focusRef : undefined}
              schedule={s}
              agents={agents}
              flows={flows}
              highlighted={highlightId === s.id}
              running={runningId === s.id}
              onToggle={() => toggleSchedule(s)}
              onRunNow={() => runNow(s)}
              onEdit={() => setEditor({ lane: 'schedules', editing: s })}
              onTags={(tags) => setScheduleTags(s, tags)}
            />
          ))}
        </BoardColumn>

        {renderAutomationLane('tag')}
        {renderAutomationLane('board')}
        {renderAutomationLane('token')}
        {renderAutomationLane('counter')}
      </div>

      {editor?.lane === 'schedules' && (
        <ScheduleModal
          agents={agents}
          flows={flows}
          editing={editor.editing}
          onClose={() => setEditor(null)}
          onSaved={(s, isNew) =>
            setSchedules((prev) =>
              isNew ? [s, ...prev] : prev.map((x) => (x.id === s.id ? s : x)),
            )
          }
          onDelete={editor.editing ? () => removeSchedule(editor.editing!) : undefined}
          onError={onError}
        />
      )}
      {editor && editor.lane !== 'schedules' && (
        <AutomationModal
          kind={editor.lane}
          agents={agents}
          flows={flows}
          columns={columns}
          editing={editor.editing}
          onClose={() => setEditor(null)}
          onSaved={(a, isNew) => {
            // Create returns the new entity; update returns only an ack → resync.
            if (isNew && a) setAutomations((prev) => [a, ...prev])
            else reloadAutomations()
          }}
          onDelete={editor.editing ? () => removeAutomation(editor.editing!) : undefined}
          onError={onError}
        />
      )}
    </div>
  )
}
