import { useCallback, useEffect, useRef, useState } from 'react'
import { Loader2, RefreshCw } from 'lucide-react'
import { api } from '@/api'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'
import { PaneHeader } from '@/shared/components'
import type { ActionItem, Dashboard } from '@/types'
import { StatTiles } from './StatTiles'
import { CostSummary } from './CostSummary'
import { ActionQueue } from './ActionQueue'
import { OutcomeSummary } from './OutcomeSummary'
import { CostRankBars, DayBars, StackedBar, RankBars } from './charts'
import { fmtUsd } from './chartFormat'
import { CommitHeatmap } from './CommitHeatmap'
import { useTranslation } from 'react-i18next'

const RANGES = [7, 14, 30, 90]

// DashboardNav lets an action-queue row jump to the screen it points at. The
// parent (App) supplies the actual routing.
export interface DashboardNav {
  openSession: (id: string) => void
  openView: (view: 'board' | 'flows' | 'schedules') => void
}

// DashboardPanel is the workspace overview: how much is running, how much is
// stuck, and the trend behind those numbers.
//
// The counters come out of the same projector pass an agent's
// get_view{kind:"workspace"} runs (_Docs/66), so the tiles and the agent's own
// summary can never disagree. The raw projection text itself is shown only on
// the Explorer screen; this screen draws the facts.
export function DashboardPanel({
  onError,
  nav,
}: {
  onError?: (msg: string) => void
  nav?: DashboardNav
}) {
  const { t } = useTranslation('dashboard')
  const [days, setDays] = useState(14)
  const [data, setData] = useState<Dashboard | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const request = useRef<AbortController | null>(null)

  // run lands results through callbacks only (so the effect may call it); load
  // is the retry entry point that re-arms the spinner. A window change re-arms
  // it too, before the refetch lands.
  const run = useCallback(() => {
    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    return api
      .getDashboard(days, controller.signal)
      .then((d) => {
        if (controller.signal.aborted) return
        setData(d)
        setError(null)
      })
      .catch((e) => {
        if (controller.signal.aborted) return
        // Keep the previous data on screen but say it failed: silently showing a
        // stale workspace as if current is worse than showing nothing.
        const msg = e instanceof Error ? e.message : String(e)
        setError(msg)
        onError?.(msg)
      })
      .finally(() => {
        if (request.current !== controller) return
        request.current = null
        if (!controller.signal.aborted) setLoading(false)
      })
  }, [days, onError])
  const load = useCallback(() => {
    setLoading(true)
    return run()
  }, [run])

  useKeyedReset(days, () => setLoading(true))
  useEffect(() => {
    void run()
    return () => {
      request.current?.abort()
      request.current = null
    }
  }, [run])

  // Route an action-queue click to the screen that owns the entity.
  const onNavigate = (kind: ActionItem['kind'], id: string) => {
    if (!nav) return
    if (kind === 'session') nav.openSession(id)
    else if (kind === 'run') nav.openView('flows')
    else if (kind === 'card') nav.openView('board')
    else if (kind === 'schedule') nav.openView('schedules')
  }

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <PaneHeader
        title={t('title')}
        right={
          <>
            <div className="flex overflow-hidden rounded-md border border-[var(--color-border)] text-xs">
              {RANGES.map((d) => (
                <button
                  key={d}
                  type="button"
                  onClick={() => setDays(d)}
                  className={`px-2 py-1 transition ${
                    days === d
                      ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
                      : 'text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
                  }`}
                >
                  {t('rangeDays', { count: d })}
                </button>
              ))}
            </div>
            <button
              type="button"
              onClick={() => void load()}
              title={t('refresh')}
              className="flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
            >
              {loading ? <Loader2 size={13} className="animate-spin" /> : <RefreshCw size={13} />}
              {t('refresh')}
            </button>
          </>
        }
      />

      <div className="min-h-0 flex-1 overflow-auto p-4">
        {error && (
          <p className="mb-3 rounded-md border border-[var(--color-danger)] px-3 py-2 text-xs text-[var(--color-danger)]">
            {error}
          </p>
        )}

        {!data ? (
          <p className="text-sm text-[var(--color-text-dim)]">
            {loading ? t('loading') : t('noData')}
          </p>
        ) : (
          <div className="flex flex-col gap-4">
            <StatTiles c={data.counters} />

            {/* Item 1 (cost) + item 2 (action queue): the two things a CEO reads
                first — what it costs and what needs a decision. */}
            <div className="grid gap-4 lg:grid-cols-2">
              <CostSummary cost={data.cost} costDelta={data.deltas.cost} />
              <ActionQueue items={data.actions} onNavigate={onNavigate} />
            </div>

            {/* Item 4: the completion side — throughput, cycle time, success rate. */}
            <OutcomeSummary o={data.outcomes} />

            {/* Git activity owns its request and state; the dashboard range does not filter it. */}
            <CommitHeatmap />

            {/* Item 3: every trend carries its period-over-period delta. */}
            <div className="grid gap-4 lg:grid-cols-2">
              <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
                <DayBars
                  points={data.sessionsByDay}
                  label={t('trends.openedSessions')}
                  delta={data.deltas.sessions}
                />
              </section>
              <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
                <DayBars
                  points={data.runsByDay}
                  label={t('trends.flowRuns')}
                  color="#a855f7"
                  delta={data.deltas.runs}
                />
              </section>
              <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
                <DayBars
                  points={data.tokensByDay}
                  label={t('trends.tokens')}
                  color="#06b6d4"
                  delta={data.deltas.tokens}
                />
              </section>
              <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
                {/* Cost trend: up is the bad direction, so its delta inverts. */}
                <DayBars
                  points={data.costByDay}
                  label={t('trends.cost')}
                  color="#22c55e"
                  delta={data.deltas.cost}
                  invert
                  format={(n) => fmtUsd(n, data.cost.estimated)}
                />
              </section>
            </div>

            <div className="grid gap-4 lg:grid-cols-2">
              <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
                <StackedBar items={data.boardByColumn} label={t('breakdowns.boardColumns')} />
              </section>
              <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
                <StackedBar items={data.runsByStatus} label={t('breakdowns.runStatuses')} />
              </section>
              <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
                <StackedBar items={data.sessionsByKind} label={t('breakdowns.sessionKinds')} />
              </section>
              <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
                <RankBars items={data.topAgents} label={t('breakdowns.busiestAgents')} />
              </section>
              <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
                <CostRankBars
                  items={data.topAgentsCost}
                  label={t('breakdowns.costliestAgents')}
                  estimated={data.cost.estimated}
                />
              </section>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
