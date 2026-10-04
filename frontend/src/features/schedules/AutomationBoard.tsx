import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
import { Brush, Clock, Flag, LayoutGrid, Repeat, Waypoints, Workflow } from 'lucide-react'
import { api } from '@/api'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { SIGNAL_AUTOMATIONS, SIGNAL_SCHEDULES } from '@/app/eventToRefreshSignals'
import type { Agent, Automation, AutomationTriggerKind, BoardColumnDef, Schedule } from '@/types'
import { ArchiveViewToggle, PaneHeader, toast } from '@/shared/components'
import { ArchivedAutomationsList } from './ArchivedAutomationsList'
import { AutomationCard } from './AutomationCard'
import { AutomationModal } from './AutomationModal'
import { BoardColumn } from './BoardColumn'
import { COLUMN_ACCENT, DEFAULT_COLUMNS } from './automationMeta'
import { ScheduleCard } from './ScheduleCard'
import { ScheduleModal } from './ScheduleModal'
import { CuratorPanel } from './CuratorPanel'

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
  const { t } = useTranslation('schedules')
  const [curatorOpen, setCuratorOpen] = useState(false)
  // Archive view (the kanban board's pattern): the lanes give way to the list of
  // archived automations, each restorable back into its lane.
  const [showArchived, setShowArchived] = useState(false)
  const schedulesTick = useRefreshTrigger(SIGNAL_SCHEDULES)
  const automationsTick = useRefreshTrigger(SIGNAL_AUTOMATIONS)
  const [schedules, setSchedules] = useState<Schedule[]>([])
  const [automations, setAutomations] = useState<Automation[]>([])
  // Workspace board columns, for the board-trigger source/target filters.
  const [columns, setColumns] = useState<BoardColumnDef[]>(DEFAULT_COLUMNS)

  const [loadingSchedules, setLoadingSchedules] = useState(true)
  const [loadingAutomations, setLoadingAutomations] = useState(true)

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

  // When a deep-link target is present and loaded, scroll it into view and flash
  // a highlight ring that fades after a moment. Keyed on the target's presence,
  // not on every schedules refresh, so a poll does not re-scroll the board.
  const focusPresent = !!focusId && schedules.some((s) => s.id === focusId)
  useKeyedReset(`${focusId}|${focusPresent}`, () => {
    if (focusPresent) setHighlightId(focusId)
  })
  useEffect(() => {
    if (!focusPresent) return
    focusRef.current?.scrollIntoView({ behavior: 'smooth', block: 'center' })
    const t = setTimeout(() => setHighlightId(null), 2500)
    return () => clearTimeout(t)
  }, [focusId, focusPresent])

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
        onError(
          t('board.runFailed', {
            error: updated.lastDeliveryError || t('common.unknownError'),
          }),
        )
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
    if (!confirm(t('board.confirmDeleteSchedule'))) return
    setEditor(null) // the delete button lives in the edit popup
    setSchedules((prev) => prev.filter((x) => x.id !== s.id))
    try {
      await api.deleteSchedule(s.id)
      toast.success(t('board.scheduleDeleted'))
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

  // Archive: the rule leaves the lanes and stops firing but keeps its config and
  // ledger; the header's "Arşiv" view lists it and restores it.
  const archiveAutomation = async (a: Automation) => {
    if (!confirm(t('board.confirmArchiveAutomation'))) return
    setAutomations((prev) => prev.filter((x) => x.id !== a.id))
    try {
      await api.setArchived('automations', a.id, true)
      toast.success(t('board.automationArchived'))
    } catch (e) {
      onError((e as Error).message)
      reloadAutomations()
    }
  }

  // Pin (Rota F3): exempt the rule from the curator's automatic passes.
  const pinAutomation = async (a: Automation) => {
    const pinned = !a.pinned
    setAutomations((prev) => prev.map((x) => (x.id === a.id ? { ...x, pinned } : x)))
    try {
      await api.pinAutomation(a.id, pinned)
    } catch (e) {
      onError((e as Error).message)
      reloadAutomations()
    }
  }

  const removeAutomation = async (a: Automation) => {
    if (!confirm(t('board.confirmDeleteAutomation'))) return
    setEditor(null) // the delete button lives in the edit popup
    setAutomations((prev) => prev.filter((x) => x.id !== a.id))
    try {
      await api.deleteAutomation(a.id)
      toast.success(t('board.automationDeleted'))
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
      title: t('lanes.tag.title'),
      icon: Repeat,
      accent: COLUMN_ACCENT.tag,
      description: t('lanes.tag.description'),
      addLabel: t('lanes.tag.add'),
      emptyLabel: t('lanes.tag.empty'),
    },
    board: {
      title: t('lanes.board.title'),
      icon: LayoutGrid,
      accent: COLUMN_ACCENT.board,
      description: t('lanes.board.description'),
      addLabel: t('lanes.board.add'),
      emptyLabel: t('lanes.board.empty'),
    },
    phase: {
      title: t('lanes.phase.title'),
      icon: Waypoints,
      accent: COLUMN_ACCENT.phase,
      description: t('lanes.phase.description'),
      addLabel: t('lanes.phase.add'),
      emptyLabel: t('lanes.phase.empty'),
    },
    trajectory_end: {
      title: t('lanes.trajectoryEnd.title'),
      icon: Flag,
      accent: COLUMN_ACCENT.trajectory_end,
      description: t('lanes.trajectoryEnd.description'),
      addLabel: t('lanes.trajectoryEnd.add'),
      emptyLabel: t('lanes.trajectoryEnd.empty'),
    },
    flow: {
      title: t('lanes.flow.title'),
      icon: Workflow,
      accent: COLUMN_ACCENT.flow,
      description: t('lanes.flow.description'),
      addLabel: t('lanes.flow.add'),
      emptyLabel: t('lanes.flow.empty'),
    },
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
        onAdd={() => setEditor({ lane: kind, editing: null })}
        addLabel={meta.addLabel}
        loading={loadingAutomations}
        loadingLabel={t('board.automationsLoading')}
        emptyLabel={meta.emptyLabel}
      >
        {items.map((a) => (
          <AutomationCard
            key={a.id}
            automation={a}
            isBoardKind={kind === 'board'}
            agents={agents}
            columns={columns}
            onToggle={() => toggleAutomation(a)}
            onReset={() => resetAutomation(a)}
            onArchive={() => archiveAutomation(a)}
            onPin={() => pinAutomation(a)}
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
        title={showArchived ? t('board.archiveTitle') : t('board.title')}
        right={
          <>
            <ArchiveViewToggle
              testId="automations-archived-toggle"
              active={showArchived}
              onToggle={() => setShowArchived((v) => !v)}
              backLabel={t('board.backToBoard')}
              backTitle={t('board.backToBoardTitle')}
            />
            <button
              type="button"
              onClick={() => setCuratorOpen(true)}
              title={t('board.curatorTitle')}
              className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
              data-testid="curator-open"
            >
              <Brush size={14} />
              <span className="hidden sm:inline">{t('curator.title')}</span>
            </button>
            {pauseAutonomy !== null ? (
              <button
                data-testid="workspace-pause-autonomy-toggle"
                onClick={togglePauseAutonomy}
                disabled={savingPause}
                aria-pressed={pauseAutonomy}
                title={
                  pauseAutonomy ? t('board.autonomyPausedTitle') : t('board.pauseAutonomyTitle')
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
                  {pauseAutonomy ? t('board.autonomyPaused') : t('board.pauseAutonomy')}
                </span>
              </button>
            ) : undefined}
          </>
        }
      />
      {curatorOpen && (
        <CuratorPanel
          onClose={() => setCuratorOpen(false)}
          onError={onError}
          onChanged={() => {
            reloadAutomations()
            reloadSchedules()
          }}
        />
      )}

      {/* Eight lanes side by side, each with its own vertical card scroll. The
          row scrolls HORIZONTALLY at every width: below `md` it is a snapping
          carousel (one near-full-width lane per screen); on md+ every lane keeps
          a readable minimum width (BoardColumn) and only grows when there is
          spare room, so a narrow window scrolls sideways instead of squeezing
          all lanes into unreadable slivers. */}
      {showArchived && (
        <ArchivedAutomationsList agents={agents} onError={onError} onRestored={reloadAutomations} />
      )}
      {/* Kept mounted (hidden) in the archive view so lane scroll survives. */}
      <div
        className={`${showArchived ? 'hidden' : 'flex'} min-h-0 flex-1 snap-x snap-mandatory gap-3 overflow-x-auto overscroll-x-contain p-3 md:snap-none`}
      >
        <BoardColumn
          testId="automation-lane-schedules"
          title={t('lanes.schedules.title')}
          icon={Clock}
          accent={COLUMN_ACCENT.schedules}
          count={schedules.length}
          description={t('lanes.schedules.description')}
          onAdd={() => setEditor({ lane: 'schedules', editing: null })}
          addLabel={t('lanes.schedules.add')}
          loading={loadingSchedules}
          loadingLabel={t('lanes.schedules.loading')}
          emptyLabel={t('lanes.schedules.empty')}
        >
          {schedules.map((s) => (
            <ScheduleCard
              key={s.id}
              ref={s.id === focusId ? focusRef : undefined}
              schedule={s}
              agents={agents}
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
        {renderAutomationLane('phase')}
        {renderAutomationLane('trajectory_end')}
        {renderAutomationLane('flow')}
      </div>

      {editor?.lane === 'schedules' && (
        <ScheduleModal
          agents={agents}
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
