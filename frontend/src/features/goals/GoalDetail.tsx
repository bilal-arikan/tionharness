// GoalDetail — the read view of one goal: the user's own words next to the
// writer's normalized shape, the measurement (primary + guardrails), scope,
// policy, the writer's open questions and the goal's own change log. Sections
// are laid out so later phases (fitness trend, proposals, evolution ledger)
// slot in under the same header without moving anything.
import { AlertTriangle, History, MessageSquareQuote, Quote } from 'lucide-react'
import { Badge, SectionHead } from '@/shared/components'
import { fullDateTime } from '@/shared/lib/time'
import type { Goal, GoalCatalog, GoalMetricDef } from '@/types/goal'
import { GoalFitnessBlock } from './GoalFitnessBlock'
import { GoalProposals } from './GoalProposals'
import {
  DIRECTION_LABEL,
  KIND_LABEL,
  MODE_LABEL,
  PRIORITY_LABEL,
  SCOPE_LABEL,
  SURFACE_LABEL,
  authorLabel,
  formatMetricValue,
  metricLabel,
} from './goalMeta'

interface Props {
  goal: Goal
  catalog: GoalCatalog | null
  onError: (msg: string) => void
}

export function GoalDetail({ goal, catalog, onError }: Props) {
  const metrics = catalog?.metrics
  const unit = (key: string) => metrics?.find((m) => m.key === key)?.unit
  const nameOf = (kind: keyof GoalCatalog['candidates'], id: string) => {
    if (kind === 'tags') return id
    return catalog?.candidates[kind].find((c) => c.id === id)?.name ?? id
  }
  const scopeEntries = (Object.keys(SCOPE_LABEL) as (keyof typeof SCOPE_LABEL)[]).filter(
    (k) => (goal.scope[k]?.length ?? 0) > 0,
  )
  const questions = goal.questions?.filter((q) => q.trim() !== '') ?? []

  return (
    <div className="flex flex-col gap-6" data-testid="goal-detail">
      {questions.length > 0 && (
        <div className="flex gap-2 rounded-lg border border-[var(--color-warning)]/40 bg-[color-mix(in_srgb,var(--color-warning)_10%,transparent)] px-3 py-2 text-sm">
          <AlertTriangle size={16} className="mt-0.5 shrink-0 text-[var(--color-warning)]" />
          <div>
            <p className="font-medium">Hedef yazıcının açık soruları</p>
            <ul className="mt-1 list-disc pl-5 text-[var(--color-text-dim)]">
              {questions.map((q, i) => (
                <li key={i}>{q}</li>
              ))}
            </ul>
            <p className="mt-1 text-xs text-[var(--color-text-dim)]">
              Etkinleştirmeden önce düzenleyip soruları boşalt ya da "yeniden yaz" ile cevaplarını
              ekle.
            </p>
          </div>
        </div>
      )}

      {goal.summary && <p className="text-sm">{goal.summary}</p>}
      {goal.description && (
        <p className="whitespace-pre-wrap text-sm text-[var(--color-text-dim)]">
          {goal.description}
        </p>
      )}

      {goal.rawText && (
        <section className="flex flex-col gap-1.5">
          <SectionHead>
            <span className="inline-flex items-center gap-1">
              <Quote size={12} /> Senin sözlerin
            </span>
          </SectionHead>
          <blockquote className="whitespace-pre-wrap border-l-2 border-[var(--color-accent)] pl-3 text-sm italic text-[var(--color-text-dim)]">
            {goal.rawText}
          </blockquote>
        </section>
      )}

      <GoalFitnessBlock goal={goal} metrics={metrics} onError={onError} />

      <GoalProposals goal={goal} metrics={metrics} onError={onError} />

      <section className="flex flex-col gap-2">
        <SectionHead>Ölçüm tanımı</SectionHead>
        <MetricRow
          label="Ana metrik"
          metricKey={goal.primary.metric}
          metrics={metrics}
          value={
            <>
              <span className="font-medium">{DIRECTION_LABEL[goal.primary.direction]}</span>
              {goal.primary.target !== null && goal.primary.target !== undefined && (
                <span className="text-[var(--color-text-dim)]">
                  {' '}
                  · hedef {formatMetricValue(goal.primary.target, unit(goal.primary.metric))}
                </span>
              )}
            </>
          }
        />
        {goal.guardrails.length === 0 ? (
          <p className="text-xs text-[var(--color-warning)]">
            Guardrail yok — tek metrikli hedef, ölçülmeyen her şeyi feda etmeye açıktır.
          </p>
        ) : (
          goal.guardrails.map((r, i) => (
            <MetricRow
              key={i}
              label="Guardrail"
              metricKey={r.metric}
              metrics={metrics}
              value={
                <span className="text-[var(--color-text-dim)]">
                  {r.min !== null &&
                    r.min !== undefined &&
                    `≥ ${formatMetricValue(r.min, unit(r.metric))}`}
                  {r.min !== null &&
                    r.min !== undefined &&
                    r.max !== null &&
                    r.max !== undefined &&
                    ' · '}
                  {r.max !== null &&
                    r.max !== undefined &&
                    `≤ ${formatMetricValue(r.max, unit(r.metric))}`}
                </span>
              }
            />
          ))
        )}
        {goal.rubric && (
          <div className="mt-1">
            <p className="text-xs text-[var(--color-text-dim)]">Rubrik</p>
            <p className="whitespace-pre-wrap text-sm">{goal.rubric}</p>
          </div>
        )}
      </section>

      <section className="flex flex-col gap-2">
        <SectionHead>Kapsam</SectionHead>
        {scopeEntries.length === 0 ? (
          <p className="text-sm text-[var(--color-text-dim)]">Tüm workspace.</p>
        ) : (
          scopeEntries.map((k) => (
            <div key={k} className="flex flex-wrap items-center gap-1.5 text-sm">
              <span className="w-28 text-xs text-[var(--color-text-dim)]">{SCOPE_LABEL[k]}</span>
              {goal.scope[k]!.map((id) => (
                <Badge key={id} tone="accent">
                  {nameOf(k, id)}
                </Badge>
              ))}
            </div>
          ))
        )}
      </section>

      <section className="flex flex-col gap-2">
        <SectionHead>Politika</SectionHead>
        <div className="grid gap-x-6 gap-y-1 text-sm md:grid-cols-2">
          <Row k="Mod" v={MODE_LABEL[goal.policy.mode]} />
          <Row k="Tür" v={goal.kind ? KIND_LABEL[goal.kind] : '—'} />
          <Row k="Öncelik" v={`${goal.priority || 3} · ${PRIORITY_LABEL[goal.priority || 3]}`} />
          <Row
            k="Cooldown"
            v={goal.policy.cooldownHours ? `${goal.policy.cooldownHours} sa` : '—'}
          />
          <Row k="En az koşu" v={goal.policy.minRuns ? String(goal.policy.minRuns) : '—'} />
          {goal.policy.mode === 'auto' && (
            <Row
              k="Oto-uygulanır"
              v={
                (goal.policy.autoApplySurfaces ?? [])
                  .map((s) => SURFACE_LABEL[s] ?? s)
                  .join(', ') || '—'
              }
            />
          )}
        </div>
        {goal.notes && (
          <p className="mt-1 flex gap-1.5 text-xs text-[var(--color-text-dim)]">
            <MessageSquareQuote size={14} className="mt-0.5 shrink-0" />
            <span className="whitespace-pre-wrap">{goal.notes}</span>
          </p>
        )}
      </section>

      <section className="flex flex-col gap-2">
        <SectionHead>
          <span className="inline-flex items-center gap-1">
            <History size={12} /> Geçmiş
          </span>
        </SectionHead>
        <ol className="flex flex-col gap-1 text-xs" data-testid="goal-history">
          {[...goal.history].reverse().map((rev, i) => (
            <li key={i} className="flex flex-wrap gap-x-2 text-[var(--color-text-dim)]">
              <span className="tabular-nums">{fullDateTime(rev.at)}</span>
              <span className="text-[var(--color-text)]">{authorLabel(rev.by)}</span>
              {rev.note && <span>· {rev.note}</span>}
              {rev.fields && rev.fields.length > 0 && (
                <span className="text-[var(--color-text-dim)]">· {rev.fields.join(', ')}</span>
              )}
            </li>
          ))}
        </ol>
      </section>
    </div>
  )
}

function MetricRow({
  label,
  metricKey,
  metrics,
  value,
}: {
  label: string
  metricKey: string
  metrics: GoalMetricDef[] | undefined
  value: React.ReactNode
}) {
  return (
    <div className="flex flex-wrap items-baseline gap-x-2 text-sm">
      <span className="w-28 text-xs text-[var(--color-text-dim)]">{label}</span>
      <span title={metricKey}>{metricLabel(metricKey, metrics)}</span>
      <span>{value}</span>
    </div>
  )
}

function Row({ k, v }: { k: string; v: string }) {
  return (
    <div className="flex gap-2">
      <span className="w-28 shrink-0 text-xs text-[var(--color-text-dim)]">{k}</span>
      <span>{v}</span>
    </div>
  )
}
