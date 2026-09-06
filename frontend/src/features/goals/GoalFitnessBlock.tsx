// GoalFitnessBlock — the goal's numbers (E1): the primary metric and every
// guardrail over the window, then the same per configuration snapshot the
// sessions ran under, with what changed between snapshots. LLM-free; the
// later phases (proposals, ledger) attach under this block.
import { useEffect, useState } from 'react'
import { Activity, GitCommitHorizontal, RefreshCw } from 'lucide-react'
import { api } from '@/api'
import { Badge, SectionHead } from '@/shared/components'
import { fullDateTime } from '@/shared/lib/time'
import type { Goal, GoalMetricDef } from '@/types/goal'
import type { GoalFitness, GuardrailStatus, MetricValue, SnapshotFitness } from '@/types/evolution'
import { formatMetricValue, metricLabel } from './goalMeta'
import { WINDOWS, changeLabel, deltaLabel, primaryVerdict, sinceFor } from './fitnessMeta'

interface Props {
  goal: Goal
  metrics: GoalMetricDef[] | undefined
  onError: (msg: string) => void
}

export function GoalFitnessBlock({ goal, metrics, onError }: Props) {
  const [win, setWin] = useState('30d')
  const [fit, setFit] = useState<GoalFitness | null>(null)
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    api
      .goalFitness(goal.id, sinceFor(win))
      .then((f) => {
        if (!cancelled) setFit(f)
      })
      .catch((e) => onError((e as Error).message))
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
    // Refetch when the goal's measurement definition changes.
  }, [goal.id, goal.updatedAt, win, onError])

  const label = (m: MetricValue) => metricLabel(m.metric, metrics)
  const fmt = (m: MetricValue) => formatMetricValue(m.value, m.unit)
  const verdict = fit ? primaryVerdict(fit) : 'none'

  return (
    <section className="flex flex-col gap-2" data-testid="goal-fitness">
      <div className="flex flex-wrap items-center gap-2">
        <SectionHead>
          <span className="inline-flex items-center gap-1">
            <Activity size={12} /> Ölçüm
          </span>
        </SectionHead>
        <div className="ml-auto flex gap-1 text-xs">
          {WINDOWS.map((w) => (
            <button
              key={w.key}
              onClick={() => setWin(w.key)}
              aria-pressed={win === w.key}
              className={`rounded-full border px-2 py-0.5 ${
                win === w.key
                  ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
                  : 'border-[var(--color-border)] text-[var(--color-text-dim)]'
              }`}
            >
              {w.label}
            </button>
          ))}
          <RefreshCw
            size={12}
            className={`ml-1 self-center opacity-50 ${loading ? 'animate-spin' : ''}`}
          />
        </div>
      </div>

      {!fit ? (
        <p className="text-xs text-[var(--color-text-dim)]">Yükleniyor…</p>
      ) : (
        <>
          <div className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-sm">
            <span className="w-28 text-xs text-[var(--color-text-dim)]">Ana metrik</span>
            <span>{label(fit.primary)}</span>
            <MetricCell m={fit.primary} />
            {fit.target !== null && fit.target !== undefined && (
              <Badge tone={verdict === 'ok' ? 'success' : verdict === 'off' ? 'warning' : 'muted'}>
                hedef {formatMetricValue(fit.target, fit.primary.unit)} ·{' '}
                {verdict === 'ok' ? 'tutuyor' : verdict === 'off' ? 'tutmuyor' : 'veri yok'}
              </Badge>
            )}
            <span className="text-xs text-[var(--color-text-dim)]">
              {fit.sessions} oturum · pencere {fit.since ? fullDateTime(fit.since) : 'tümü'}
            </span>
          </div>
          {fit.guardrails.map((g, i) => (
            <div key={i} className="flex flex-wrap items-baseline gap-x-3 gap-y-1 text-sm">
              <span className="w-28 text-xs text-[var(--color-text-dim)]">Guardrail</span>
              <span>{label(g)}</span>
              <MetricCell m={g} />
              <GuardBadge g={g} />
            </div>
          ))}

          <SectionHead className="mt-3">
            <span className="inline-flex items-center gap-1">
              <GitCommitHorizontal size={12} /> Konfigürasyon sürümlerine göre
            </span>
          </SectionHead>
          {fit.bySnapshot.length === 0 ? (
            <p className="text-xs text-[var(--color-text-dim)]">
              Bu pencerede kapsamda oturum yok.
            </p>
          ) : (
            <ol className="flex flex-col gap-2" data-testid="goal-fitness-snapshots">
              {fit.bySnapshot.map((s, i) => (
                <SnapshotRow
                  key={s.hash || 'unstamped'}
                  s={s}
                  prev={i > 0 ? fit.bySnapshot[i - 1] : undefined}
                  direction={fit.direction}
                  fmt={fmt}
                />
              ))}
            </ol>
          )}
        </>
      )}
    </section>
  )
}

function MetricCell({ m }: { m: MetricValue }) {
  if (!m.available) {
    return <span className="text-xs text-[var(--color-text-dim)]">henüz ölçülmüyor</span>
  }
  if (m.value === null || m.value === undefined) {
    return <span className="text-xs text-[var(--color-text-dim)]">veri yok</span>
  }
  return (
    <span className="font-medium tabular-nums">
      {formatMetricValue(m.value, m.unit)}
      <span className="ml-1 text-xs font-normal text-[var(--color-text-dim)]">n={m.n}</span>
    </span>
  )
}

function GuardBadge({ g }: { g: GuardrailStatus }) {
  const bound = [
    g.min !== null && g.min !== undefined ? `≥ ${formatMetricValue(g.min, g.unit)}` : '',
    g.max !== null && g.max !== undefined ? `≤ ${formatMetricValue(g.max, g.unit)}` : '',
  ]
    .filter(Boolean)
    .join(' · ')
  if (!g.available || g.value === null) return <Badge tone="muted">{bound}</Badge>
  return (
    <Badge tone={g.violated ? 'danger' : 'success'}>
      {g.violated ? `ihlal · ${bound}` : bound}
    </Badge>
  )
}

function SnapshotRow({
  s,
  prev,
  direction,
  fmt,
}: {
  s: SnapshotFitness
  prev: SnapshotFitness | undefined
  direction: 'min' | 'max'
  fmt: (m: MetricValue) => string
}) {
  const delta = deltaLabel(s.primary, prev?.primary, direction)
  return (
    <li className="rounded-lg border border-[var(--color-border)] px-3 py-2 text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono">{s.hash ? s.hash.slice(0, 10) : 'damgasız'}</span>
        {s.current && <Badge tone="accent">şu anki</Badge>}
        <span className="text-[var(--color-text-dim)]">
          {fullDateTime(s.from)}
          {s.to !== s.from && ` → ${fullDateTime(s.to)}`} · {s.sessions} oturum
        </span>
        <span className="ml-auto tabular-nums">
          {s.primary.value === null || s.primary.value === undefined ? (
            <span className="text-[var(--color-text-dim)]">veri yok</span>
          ) : (
            <>
              <strong>{fmt(s.primary)}</strong>
              {delta.text && (
                <span
                  className={`ml-1 ${
                    delta.good === true
                      ? 'text-[var(--color-success)]'
                      : delta.good === false
                        ? 'text-[var(--color-danger)]'
                        : 'text-[var(--color-text-dim)]'
                  }`}
                >
                  {delta.text}
                </span>
              )}
            </>
          )}
        </span>
        {s.guardrails.some((g) => g.violated) && <Badge tone="danger">guardrail ihlali</Badge>}
      </div>
      {s.changes && s.changes.length > 0 && (
        <ul className="mt-1 list-disc pl-5 text-[var(--color-text-dim)]">
          {s.changes.slice(0, 8).map((c, i) => (
            <li key={i}>{changeLabel(c)}</li>
          ))}
          {s.changes.length > 8 && <li>… {s.changes.length - 8} değişiklik daha</li>}
        </ul>
      )}
    </li>
  )
}
