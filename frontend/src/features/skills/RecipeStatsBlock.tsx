// RecipeStatsBlock — what the Rota index says about a coordinator recipe
// (F3): per version, how many runs finished / failed / were abandoned, the
// average duration / tokens / cost, and what the plan declared but never
// happened (unfired watchers, unreached phases). Computed server-side from the
// index rows' summaries; no LLM.
import { useEffect, useState } from 'react'
import { Loader2, Sparkles } from 'lucide-react'
import { api } from '@/api'
import type { RecipeStats } from '@/types/trajectory'
import { Badge, toast } from '@/shared/components'
import { fmtTime } from '@/features/schedules/timeUtils'
import { fmtDurationSec, fmtTokens } from '@/features/rota/trajectoryFormat'
import { useTranslation } from 'react-i18next'

interface Props {
  slug: string
  onOpenTrajectory?: (id: string) => void
  // Opens the recorded optimizer exchange in Chat.
  onOpenSession?: (sessionId: string) => void
}

export function RecipeStatsBlock({ slug, onOpenTrajectory, onOpenSession }: Props) {
  const { t } = useTranslation('skills')
  const [rows, setRows] = useState<RecipeStats[] | null>(null)
  const [openProposals, setOpenProposals] = useState<number>(0)
  const [lastPass, setLastPass] = useState<string>('')
  const [optimizing, setOptimizing] = useState(false)
  const [optimizerSession, setOptimizerSession] = useState<string | null>(null)
  const [nonce, setNonce] = useState(0)
  useEffect(() => {
    let cancelled = false
    api
      .recipeStats(slug)
      .then((r) => {
        if (!cancelled) setRows(r)
      })
      .catch(() => {
        if (!cancelled) setRows([])
      })
    // Open optimizer proposals for this recipe + the last pass (F4).
    api
      .listInsightFindings({ channel: 'recipe-opt' })
      .then((fs) => {
        if (cancelled) return
        setOpenProposals(
          fs.filter((f) => f.filePointer === `skill/${slug}` && (f.status || 'new') === 'new')
            .length,
        )
      })
      .catch(() => {})
    api
      .recipeOptimizerState(slug)
      .then((s) => {
        if (cancelled || !s.ran) return
        setLastPass(
          t('stats.lastPass', {
            time: fmtTime(s.state.lastAt),
            trigger:
              s.state.trigger === 'manual'
                ? t('stats.triggerManual')
                : s.state.trigger === 'failed'
                  ? t('stats.triggerFailed')
                  : t('stats.triggerThreshold'),
            proposals: s.state.proposals,
            skipped: s.state.skipped ? t('stats.skippedReason', { reason: s.state.skipped }) : '',
          }),
        )
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [slug, nonce, t])
  const optimize = async () => {
    setOptimizing(true)
    try {
      const r = await api.optimizeRecipe(slug)
      setOptimizerSession(r.sessionId ?? null)
      if (r.ran)
        toast.info(
          t('stats.optimizerResult', {
            proposals: r.proposals.length,
            dropped: r.dropped,
            applied: r.applied ? t('stats.applied', { count: r.applied }) : '',
          }),
        )
      else toast.warning(t('stats.optimizerSkipped', { reason: r.skipped ?? '—' }))
      setNonce((n) => n + 1)
    } catch (e) {
      toast.error(e instanceof Error ? e.message : String(e))
    } finally {
      setOptimizing(false)
    }
  }
  if (rows === null) return null
  return (
    <div className="mt-2 text-xs" data-testid="recipe-stats">
      <div className="mb-1 flex flex-wrap items-center gap-2">
        <span className="font-medium text-[var(--color-text-dim)]">{t('stats.title')}</span>
        {optimizerSession && onOpenSession && (
          <button
            type="button"
            onClick={() => onOpenSession(optimizerSession)}
            className="rounded border border-[var(--color-border)] px-1.5 py-0.5 text-[11px] hover:bg-[var(--color-surface-2)]"
            title={t('stats.openSessionHint')}
            data-testid="optimizer-open-session"
          >
            {t('stats.optimizerSession')}
          </button>
        )}
        <button
          type="button"
          onClick={optimize}
          disabled={optimizing}
          className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-accent)] disabled:opacity-40"
          title={t('stats.optimizeHint')}
          data-testid="recipe-optimize"
        >
          {optimizing ? <Loader2 size={11} className="animate-spin" /> : <Sparkles size={11} />}
          {t('stats.optimizeNow')}
        </button>
        {openProposals > 0 && (
          <Badge tone="accent" className="normal-case">
            {t('stats.openProposalChannel', { count: openProposals })}
          </Badge>
        )}
        {lastPass && (
          <span className="text-[var(--color-text-dim)]">
            {t('stats.lastPassLabel')}: {lastPass}
          </span>
        )}
      </div>
      {rows.length === 0 ? (
        <p className="text-[var(--color-text-dim)]">{t('stats.empty')}</p>
      ) : (
        <ul className="flex flex-col gap-1">
          {rows.map((r) => (
            <li
              key={r.templateRef}
              className="flex flex-wrap items-center gap-2 rounded border border-[var(--color-border)] px-2 py-1"
            >
              <span className="font-mono">
                {r.version ? `v${r.version}` : t('stats.unversioned')}
              </span>
              <Badge tone="muted">{t('stats.runs', { count: r.runs })}</Badge>
              {r.done > 0 && <Badge tone="success">{t('stats.done', { count: r.done })}</Badge>}
              {r.failed > 0 && (
                <Badge tone="danger">{t('stats.failed', { count: r.failed })}</Badge>
              )}
              {r.abandoned > 0 && (
                <Badge tone="muted">{t('stats.abandoned', { count: r.abandoned })}</Badge>
              )}
              {r.live > 0 && <Badge tone="accent">{t('stats.live', { count: r.live })}</Badge>}
              {r.summarized > 0 && (
                <span className="text-[var(--color-text-dim)]">
                  {[
                    t('stats.averageDuration', { duration: fmtDurationSec(r.avgDurationSec) }),
                    t('stats.averageTokens', { tokens: fmtTokens(r.avgTokens) }),
                    r.avgCostUsd > 0 ? `$${r.avgCostUsd.toFixed(2)}${r.priced ? '' : '~'}` : null,
                    t('stats.workerCount', { value: r.avgSessions.toFixed(1) }),
                  ]
                    .filter(Boolean)
                    .join(' · ')}
                </span>
              )}
              {r.unfiredWatchers &&
                Object.entries(r.unfiredWatchers).map(([w, n]) => (
                  <span
                    key={w}
                    className="rounded bg-[color-mix(in_srgb,var(--color-warning)_16%,transparent)] px-1.5 py-0.5 text-[10px] text-[var(--color-warning)]"
                    title={t('stats.watcherMissedHint', { count: n, total: r.summarized })}
                  >
                    ⚡ {w} {n}/{r.summarized} {t('stats.silent')}
                  </span>
                ))}
              {r.ghostPhases &&
                Object.entries(r.ghostPhases).map(([p, n]) => (
                  <span
                    key={p}
                    className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]"
                    title={t('stats.ghostPhaseHint', { count: n, total: r.summarized })}
                  >
                    {t('stats.ghostLabel', { phase: p, count: n, total: r.summarized })}
                  </span>
                ))}
              {r.latestId && onOpenTrajectory && (
                <button
                  type="button"
                  onClick={() => onOpenTrajectory(r.latestId!)}
                  className="ml-auto text-[var(--color-accent)] hover:underline"
                  title={t('stats.openLatestHint')}
                >
                  {t('stats.latest')}: {r.latestId}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
