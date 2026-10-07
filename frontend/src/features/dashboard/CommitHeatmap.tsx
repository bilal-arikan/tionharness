import { useCallback, useEffect, useReducer, useRef, useState } from 'react'
import { Loader2, RefreshCw } from 'lucide-react'
import { api } from '@/api'
import { InfoPopover } from '@/shared/components'
import type { CommitActivity, CommitActivityDay } from '@/types'
import { dateFormat } from '@/shared/lib/intl'
import { count } from '@/shared/lib/format'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  commitCountLabel,
  commitLevel,
  heatmapMove,
  heatmapTooltipReducer,
  initialHeatmapTooltipState,
  isHeatmapTooltipVisible,
} from './commitHeatmapModel'

const WEEKS = 52
const DAYS_PER_WEEK = 7
const CELL_COUNT = WEEKS * DAYS_PER_WEEK

function dayLabel(day: string): string {
  const [year, month, date] = day.split('-').map(Number)
  return dateFormat({ dateStyle: 'long', timeZone: 'UTC' }).format(
    new Date(Date.UTC(year, month - 1, date)),
  )
}

function commitAriaLabel(point: CommitActivityDay, t: TFunction<'dashboard'>): string {
  return t('commits.dayLabel', {
    date: dayLabel(point.day),
    count: point.value,
    value: commitCountLabel(point.value),
  })
}

const LEVEL_CLASS = [
  'bg-[var(--color-surface-2)]',
  'bg-[color-mix(in_srgb,var(--color-success)_25%,var(--color-surface))]',
  'bg-[color-mix(in_srgb,var(--color-success)_45%,var(--color-surface))]',
  'bg-[color-mix(in_srgb,var(--color-success)_70%,var(--color-surface))]',
  'bg-[var(--color-success)]',
]

export function CommitHeatmap() {
  const { t } = useTranslation('dashboard')
  const [data, setData] = useState<CommitActivity | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [focused, setFocused] = useState(0)
  const [tooltip, dispatchTooltip] = useReducer(heatmapTooltipReducer, initialHeatmapTooltipState)
  const cells = useRef<Array<HTMLButtonElement | null>>([])
  const request = useRef<AbortController | null>(null)

  // run lands results through callbacks only (so the mount effect may call it;
  // loading starts true); load is the retry entry point that re-arms the spinner.
  const run = useCallback(() => {
    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    return api
      .getCommitActivity(WEEKS, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return
        setData(result)
      })
      .catch((reason) => {
        if (!controller.signal.aborted) {
          setError(reason instanceof Error ? reason.message : String(reason))
        }
      })
      .finally(() => {
        if (request.current !== controller) return
        request.current = null
        if (!controller.signal.aborted) setLoading(false)
      })
  }, [])
  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    return run()
  }, [run])

  useEffect(() => {
    void run()
    return () => {
      request.current?.abort()
      request.current = null
    }
  }, [run])

  const points = data?.commitsByDay.slice(-CELL_COUNT) ?? []
  const peak = Math.max(0, ...points.map((point) => point.value))
  const total = points.reduce((sum, point) => sum + point.value, 0)

  return (
    <section
      aria-labelledby="commit-activity-title"
      className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3"
    >
      <div className="mb-3 flex items-center justify-between gap-3">
        <div>
          <div className="flex items-center gap-1">
            <h2 id="commit-activity-title" className="text-sm font-medium">
              {t('commits.title')}
            </h2>
            <InfoPopover text={t('commits.interactionHint')} />
          </div>
          <p className="text-[11px] text-[var(--color-text-dim)]">{t('commits.period')}</p>
        </div>
        {!loading && data?.isGitRepo !== false && total > 0 && (
          <span className="text-xs tabular-nums text-[var(--color-text-dim)]">
            {t('commits.total', { count: total, value: count(total) })}
          </span>
        )}
      </div>

      {loading ? (
        <p className="flex items-center gap-2 text-xs text-[var(--color-text-dim)]" role="status">
          <Loader2 size={14} className="animate-spin" /> {t('commits.loading')}
        </p>
      ) : error ? (
        <div
          role="alert"
          className="flex flex-wrap items-center gap-2 text-xs text-[var(--color-danger)]"
        >
          <span>{t('commits.error', { error })}</span>
          <button
            type="button"
            onClick={() => void load()}
            className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-1 text-[var(--color-text)] hover:border-[var(--color-accent)]"
          >
            <RefreshCw size={12} /> {t('commits.retry')}
          </button>
        </div>
      ) : data?.isGitRepo === false ? (
        <p className="text-xs text-[var(--color-text-dim)]">{t('commits.notRepository')}</p>
      ) : points.length === 0 || total === 0 ? (
        <p className="text-xs text-[var(--color-text-dim)]">{t('commits.empty')}</p>
      ) : (
        <>
          <div className="overflow-x-auto pb-2" tabIndex={0} aria-label={t('commits.chartLabel')}>
            <div
              className="grid w-max grid-flow-col grid-rows-7 gap-1"
              role="grid"
              aria-label={t('commits.gridLabel', { count: total, value: count(total) })}
            >
              {points.map((point, index) => {
                const label = commitAriaLabel(point, t)
                const tooltipId = `commit-tooltip-${index}`
                const tooltipVisible = isHeatmapTooltipVisible(tooltip, index)
                return (
                  <span key={point.day} role="presentation" className="relative">
                    <button
                      ref={(node) => {
                        cells.current[index] = node
                      }}
                      type="button"
                      role="gridcell"
                      tabIndex={focused === index ? 0 : -1}
                      aria-label={label}
                      aria-describedby={tooltipVisible ? tooltipId : undefined}
                      onFocus={() => {
                        setFocused(index)
                        dispatchTooltip({ type: 'focus', index })
                      }}
                      onBlur={() => dispatchTooltip({ type: 'blur', index })}
                      onMouseEnter={() => dispatchTooltip({ type: 'mouseenter', index })}
                      onMouseLeave={() => dispatchTooltip({ type: 'mouseleave', index })}
                      onKeyDown={(event) => {
                        if (event.key === 'Escape') {
                          if (tooltipVisible) {
                            event.preventDefault()
                            dispatchTooltip({ type: 'escape', index })
                          }
                          return
                        }
                        if (
                          !event.key.startsWith('Arrow') &&
                          event.key !== 'Home' &&
                          event.key !== 'End'
                        ) {
                          return
                        }
                        event.preventDefault()
                        const next = heatmapMove(index, event.key, points.length)
                        setFocused(next)
                        cells.current[next]?.focus()
                      }}
                      className={`block h-3 w-3 rounded-[2px] border border-[var(--color-border)] focus:outline-2 focus:outline-offset-1 focus:outline-[var(--color-accent)] ${LEVEL_CLASS[commitLevel(point.value, peak)]}`}
                    >
                      <span className="sr-only">{count(point.value)}</span>
                    </button>
                    {tooltipVisible && (
                      <span
                        id={tooltipId}
                        role="tooltip"
                        className="pointer-events-none absolute bottom-full left-1/2 z-10 mb-2 w-max -translate-x-1/2 rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-1 text-[10px] text-[var(--color-text)] shadow-lg"
                      >
                        {label}
                      </span>
                    )}
                  </span>
                )
              })}
            </div>
          </div>
          <div className="mt-1 flex flex-wrap items-center justify-end gap-2 text-[10px] text-[var(--color-text-dim)]">
            <div className="flex items-center gap-1" aria-label={t('commits.intensityLabel')}>
              <span>{t('commits.less')}</span>
              {LEVEL_CLASS.map((className) => (
                <span
                  key={className}
                  className={`h-3 w-3 rounded-[2px] border border-[var(--color-border)] ${className}`}
                  aria-hidden="true"
                />
              ))}
              <span>{t('commits.more')}</span>
            </div>
          </div>
        </>
      )}
    </section>
  )
}
