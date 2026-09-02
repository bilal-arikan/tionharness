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

interface Props {
  slug: string
  onOpenTrajectory?: (id: string) => void
}

export function RecipeStatsBlock({ slug, onOpenTrajectory }: Props) {
  const [rows, setRows] = useState<RecipeStats[] | null>(null)
  const [openProposals, setOpenProposals] = useState<number>(0)
  const [lastPass, setLastPass] = useState<string>('')
  const [optimizing, setOptimizing] = useState(false)
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
          `${fmtTime(s.state.lastAt)} · ${s.state.trigger === 'manual' ? 'elle' : s.state.trigger === 'failed' ? 'başarısız koşu' : 'eşik'} · ${s.state.proposals} öneri${s.state.skipped ? ` · atlandı: ${s.state.skipped}` : ''}`,
        )
      })
      .catch(() => {})
    return () => {
      cancelled = true
    }
  }, [slug, nonce])
  const optimize = async () => {
    setOptimizing(true)
    try {
      const r = await api.optimizeRecipe(slug)
      if (r.ran)
        toast.info(
          `✦ Optimizer: ${r.proposals.length} öneri (${r.dropped} elendi${r.applied ? `, ${r.applied} oto-uygulandı` : ''})`,
        )
      else toast.info(`Optimizer çalışmadı: ${r.skipped ?? '—'}`)
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
        <span className="font-medium text-[var(--color-text-dim)]">Rota istatistikleri</span>
        <button
          type="button"
          onClick={optimize}
          disabled={optimizing}
          className="flex items-center gap-1 rounded border border-[var(--color-border)] px-2 py-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-accent)] disabled:opacity-40"
          title="Reçete optimizer'ı şimdi çalıştır: koşu istatistiklerinden ölçülü değişiklik önerileri üretir (recipe-opt kanalı); hiçbir şeyi kendisi uygulamaz"
          data-testid="recipe-optimize"
        >
          {optimizing ? <Loader2 size={11} className="animate-spin" /> : <Sparkles size={11} />}
          Şimdi optimize et
        </button>
        {openProposals > 0 && (
          <Badge tone="accent" className="normal-case">
            ✦ {openProposals} açık öneri · İçgörü ▸ recipe-opt
          </Badge>
        )}
        {lastPass && <span className="text-[var(--color-text-dim)]">son geçiş: {lastPass}</span>}
      </div>
      {rows.length === 0 ? (
        <p className="text-[var(--color-text-dim)]">Bu reçeteyle henüz rota koşmadı.</p>
      ) : (
        <ul className="flex flex-col gap-1">
          {rows.map((r) => (
            <li
              key={r.templateRef}
              className="flex flex-wrap items-center gap-2 rounded border border-[var(--color-border)] px-2 py-1"
            >
              <span className="font-mono">{r.version ? `v${r.version}` : 'sürümsüz'}</span>
              <Badge tone="muted">{r.runs} koşu</Badge>
              {r.done > 0 && <Badge tone="success">{r.done} bitti</Badge>}
              {r.failed > 0 && <Badge tone="danger">{r.failed} başarısız</Badge>}
              {r.abandoned > 0 && <Badge tone="muted">{r.abandoned} terk</Badge>}
              {r.live > 0 && <Badge tone="accent">{r.live} canlı</Badge>}
              {r.summarized > 0 && (
                <span className="text-[var(--color-text-dim)]">
                  ort {fmtDurationSec(r.avgDurationSec)} · {fmtTokens(r.avgTokens)} token
                  {r.avgCostUsd > 0 ? ` · $${r.avgCostUsd.toFixed(2)}${r.priced ? '' : '~'}` : ''}
                  {' · '}
                  {r.avgSessions.toFixed(1)} worker
                </span>
              )}
              {r.unfiredWatchers &&
                Object.entries(r.unfiredWatchers).map(([w, n]) => (
                  <span
                    key={w}
                    className="rounded bg-[color-mix(in_srgb,var(--color-warning)_16%,transparent)] px-1.5 py-0.5 text-[10px] text-[var(--color-warning)]"
                    title={`İzleyici ${n}/${r.summarized} koşuda ateşlenmedi`}
                  >
                    ⚡ {w} {n}/{r.summarized} sessiz
                  </span>
                ))}
              {r.ghostPhases &&
                Object.entries(r.ghostPhases).map(([p, n]) => (
                  <span
                    key={p}
                    className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]"
                    title={`Faz ${n}/${r.summarized} koşuda hiç başlamadı`}
                  >
                    ◌ {p} {n}/{r.summarized} hayalet
                  </span>
                ))}
              {r.latestId && onOpenTrajectory && (
                <button
                  type="button"
                  onClick={() => onOpenTrajectory(r.latestId!)}
                  className="ml-auto text-[var(--color-accent)] hover:underline"
                  title="Son rotayı Rota ekranında aç"
                >
                  son: {r.latestId}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
