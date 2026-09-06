// GoalProposals — the evolver's cards for one goal (E2): run a pass now, see
// the last pass's bookkeeping, and accept / dismiss each proposal. Nothing is
// applied here; an accepted card waits for the human (or E3) to make the
// change and close it as applied.
import { useCallback, useEffect, useState } from 'react'
import { Check, Dna, Loader2, X } from 'lucide-react'
import { api } from '@/api'
import { Badge, Button, SectionHead } from '@/shared/components'
import { fullDateTime } from '@/shared/lib/time'
import type { Goal } from '@/types/goal'
import type { GoalEvolution } from '@/types/evolution'
import type { InsightFinding } from '@/types/insight'
import { metricLabel } from './goalMeta'
import type { GoalMetricDef } from '@/types/goal'

interface Props {
  goal: Goal
  metrics: GoalMetricDef[] | undefined
  onError: (msg: string) => void
}

const STATUS_LABEL: Record<string, string> = {
  new: 'yeni',
  triaged: 'incelendi',
  accepted: 'kabul',
  applied: 'uygulandı',
  verified: 'doğrulandı',
  dismissed: 'reddedildi',
}

export function GoalProposals({ goal, metrics, onError }: Props) {
  const [data, setData] = useState<GoalEvolution | null>(null)
  const [running, setRunning] = useState(false)
  const [note, setNote] = useState<string | null>(null)
  const [showClosed, setShowClosed] = useState(false)

  const load = useCallback(() => {
    api
      .goalEvolution(goal.id)
      .then(setData)
      .catch((e) => onError((e as Error).message))
  }, [goal.id, onError])

  useEffect(load, [load])

  const run = async () => {
    setRunning(true)
    setNote(null)
    try {
      const res = await api.evolveGoal(goal.id)
      if (!res.ran) setNote(`Atlandı: ${res.skipped}`)
      else
        setNote(
          `${res.proposals.length} öneri, ${res.dropped} elendi${
            res.dropReasons?.length ? ` (${res.dropReasons.join('; ')})` : ''
          }${res.lowConfidence ? ' · düşük güven' : ''}`,
        )
      load()
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e))
    } finally {
      setRunning(false)
    }
  }

  const setStatus = async (f: InsightFinding, status: 'accepted' | 'dismissed') => {
    try {
      await api.setInsightFindingStatus(f.id, status)
      load()
    } catch (e) {
      onError(e instanceof Error ? e.message : String(e))
    }
  }

  const findings = data?.findings ?? []
  const open = findings.filter((f) => !['dismissed', 'verified', 'applied'].includes(f.status))
  const closed = findings.filter((f) => ['dismissed', 'verified', 'applied'].includes(f.status))
  const canRun = goal.status === 'active' && goal.policy.mode !== 'off'

  return (
    <section className="flex flex-col gap-2" data-testid="goal-proposals">
      <div className="flex flex-wrap items-center gap-2">
        <SectionHead>
          <span className="inline-flex items-center gap-1">
            <Dna size={12} /> Öneriler
          </span>
        </SectionHead>
        {data?.ran && data.state && (
          <span className="text-xs text-[var(--color-text-dim)]">
            son geçiş {fullDateTime(data.state.lastAt)} · {data.state.trigger} ·{' '}
            {data.state.skipped
              ? `atlandı: ${data.state.skipped}`
              : `${data.state.proposals} öneri, ${data.state.dropped} elendi`}
          </span>
        )}
        <Button
          size="sm"
          variant="secondary"
          className="ml-auto"
          onClick={() => void run()}
          disabled={running || !canRun}
          title={
            canRun
              ? 'Evolver geçişini şimdi koştur (cooldown yok sayılır; eşik altındaysa düşük güven)'
              : 'Yalnız etkin ve politikası kapalı olmayan hedefler için'
          }
          data-testid="goal-evolve"
        >
          {running ? <Loader2 size={13} className="animate-spin" /> : <Dna size={13} />}
          Şimdi evrimleştir
        </Button>
      </div>
      {note && <p className="text-xs text-[var(--color-text-dim)]">{note}</p>}
      {open.length === 0 ? (
        <p className="text-xs text-[var(--color-text-dim)]">
          Açık öneri yok. Evolver, kapsamda en az {goal.policy.minRuns || data?.minRuns || 5} oturum
          birikince ya da bir guardrail ihlal edilince kendisi koşar; hemen görmek için "Şimdi
          evrimleştir".
        </p>
      ) : (
        <ul className="flex flex-col gap-2">
          {open.map((f) => (
            <ProposalCard key={f.id} f={f} metrics={metrics} onStatus={setStatus} />
          ))}
        </ul>
      )}
      {closed.length > 0 && (
        <button
          onClick={() => setShowClosed((v) => !v)}
          className="self-start text-xs text-[var(--color-text-dim)] hover:underline"
        >
          {showClosed ? 'Kapananları gizle' : `Kapanan ${closed.length} öneriyi göster`}
        </button>
      )}
      {showClosed && (
        <ul className="flex flex-col gap-2 opacity-70">
          {closed.map((f) => (
            <ProposalCard key={f.id} f={f} metrics={metrics} />
          ))}
        </ul>
      )}
    </section>
  )
}

function ProposalCard({
  f,
  metrics,
  onStatus,
}: {
  f: InsightFinding
  metrics: GoalMetricDef[] | undefined
  onStatus?: (f: InsightFinding, status: 'accepted' | 'dismissed') => void
}) {
  const e = f.evolution
  const tone =
    e?.kind === 'conflict' || e?.kind === 'escalation'
      ? 'danger'
      : f.status === 'accepted'
        ? 'success'
        : f.status === 'dismissed'
          ? 'muted'
          : 'accent'
  return (
    <li
      className="rounded-lg border border-[var(--color-border)] px-3 py-2 text-sm"
      data-testid={`goal-proposal-${f.id}`}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{f.title}</span>
        <Badge tone={tone}>{STATUS_LABEL[f.status] ?? f.status}</Badge>
        {f.severity === 'high' && <Badge tone="danger">yüksek</Badge>}
        {f.occurrences > 1 && <Badge tone="muted">×{f.occurrences}</Badge>}
        {onStatus && f.status !== 'accepted' && (
          <Button
            size="sm"
            variant="secondary"
            className="ml-auto"
            onClick={() => onStatus(f, 'accepted')}
            title="Kabul et (değişikliği sen yaparsın; uygulama E3'te)"
          >
            <Check size={13} /> Kabul
          </Button>
        )}
        {onStatus && (
          <Button
            size="sm"
            variant="danger"
            className={f.status === 'accepted' ? 'ml-auto' : ''}
            onClick={() => onStatus(f, 'dismissed')}
            title="Reddet — evolver bunu bir daha önermez"
          >
            <X size={13} /> Reddet
          </Button>
        )}
      </div>
      {e && (
        <div className="mt-1 flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
          <code className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5">
            {e.surface}
            {e.entityId ? `/${e.entityId}` : ''}
          </code>
          <span>
            {e.field} · {e.action}
          </span>
          {e.value && <code className="max-w-[28rem] truncate">{e.value}</code>}
          {e.removes && <span className="text-[var(--color-warning)]">kaldırır {e.removes}</span>}
          {e.expectedMetric && (
            <span>
              beklenen {metricLabel(e.expectedMetric, metrics)}{' '}
              {e.expectedDelta !== undefined && e.expectedDelta > 0 ? '+' : ''}
              {e.expectedDelta}
            </span>
          )}
        </div>
      )}
      {f.proposedFix && (
        <p className="mt-1 whitespace-pre-wrap text-xs text-[var(--color-text-dim)]">
          {f.proposedFix}
        </p>
      )}
      {f.rootCause && (
        <p className="mt-1 text-xs text-[var(--color-text-dim)]">Kanıt: {f.rootCause}</p>
      )}
    </li>
  )
}
