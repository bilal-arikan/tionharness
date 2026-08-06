import type { OutcomeBlock } from '@/types'
import { DayBars } from './charts'

// fmtDuration renders a second span as the coarsest readable unit, matching the
// view layer's dur() idiom (31s, 2m, 3sa, 4g). Used for cycle time.
function fmtDuration(sec: number): string {
  if (sec <= 0) return '—'
  if (sec < 60) return `${Math.round(sec)}sn`
  if (sec < 3600) return `${Math.round(sec / 60)}dk`
  if (sec < 86400) return `${Math.round(sec / 3600)}sa`
  return `${Math.round(sec / 86400)}g`
}

// OutcomeSummary is the completion side of the overview: throughput (cards
// finishing per day), how long a card takes end to end, and the flow-run success
// rate. The volume trends say how much STARTED; this says how much FINISHED.
//
// Each chip states its own "no data" case in words: a rate off zero closed runs
// is undefined, and a cycle time with no finished card is not "0 seconds".
export function OutcomeSummary({ o }: { o: OutcomeBlock }) {
  const rate = o.runSuccessRate
  const chips = [
    {
      label: 'Biten kart',
      value: String(o.cardsDoneWindow),
      hint: 'bu aralıkta',
    },
    {
      label: 'Ort. tamamlanma süresi',
      value: o.cardsDoneWindow > 0 ? fmtDuration(o.cycleTimeAvgSec) : '—',
      hint: o.cardsDoneWindow > 0 ? 'oluşturma → bitti' : 'biten kart yok',
    },
    {
      label: 'Koşu başarı oranı',
      value: rate == null ? '—' : `%${Math.round(rate * 100)}`,
      hint: rate == null ? 'kapanan koşu yok' : `${o.runsClosedWindow} kapandı`,
      tone: rate != null && rate < 0.8 ? 'warn' : 'normal',
    },
  ]

  return (
    <section className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3">
      <div className="mb-2 text-sm font-medium">✅ Sonuçlar</div>
      <div className="mb-3 grid grid-cols-3 gap-2">
        {chips.map((c) => (
          <div
            key={c.label}
            className="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] p-2.5"
          >
            <div className="text-[11px] text-[var(--color-text-dim)]">{c.label}</div>
            <div
              className={`mt-0.5 text-xl font-semibold tabular-nums ${
                c.tone === 'warn' ? 'text-[var(--color-danger)]' : ''
              }`}
            >
              {c.value}
            </div>
            <div className="mt-0.5 truncate text-[10px] text-[var(--color-text-dim)]">{c.hint}</div>
          </div>
        ))}
      </div>
      <DayBars points={o.velocityByDay} label="Biten kart / gün" color="#22c55e" />
    </section>
  )
}
