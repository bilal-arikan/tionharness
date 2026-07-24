import { useMemo } from 'react'
import type { SessionDebugEvent } from '@/types'

// ThinkingShareChart plots the hidden-reasoning share of each llm_call's output
// over the session, from the debug journal's `think`/`out` fields. The API bills
// extended thinking inside output_tokens without breaking it out, so `think` is
// the agent layer's derived estimate (out − visible); this panel makes the
// otherwise-invisible reasoning cost legible turn by turn. Renders nothing when
// the session did no measurable thinking (a thinking-off agent is silent).
export function ThinkingShareChart({ events }: { events: SessionDebugEvent[] }) {
  const model = useMemo(() => {
    const calls = events.filter((e) => e.type === 'llm_call' && (e.out ?? 0) > 0)
    let totalOut = 0
    let totalThink = 0
    const bars = calls.map((e) => {
      const out = e.out ?? 0
      const think = Math.min(e.think ?? 0, out)
      totalOut += out
      totalThink += think
      return { share: out > 0 ? think / out : 0, ts: e.ts, out, think }
    })
    if (totalThink === 0) return null
    return { bars, share: totalOut > 0 ? totalThink / totalOut : 0, totalThink }
  }, [events])

  if (!model) {
    return (
      <p className="text-[10px] text-[var(--color-text-dim)]">
        Bu oturumda ölçülebilir gizli akıl yürütme yok (thinking kapalı ajan sessizdir).
      </p>
    )
  }

  return (
    <div>
      <div className="mb-1.5 flex items-center gap-1.5 text-[10px]">
        <span className="rounded bg-violet-500/15 px-1.5 py-px font-medium text-violet-400">
          Ortalama düşünme: %{Math.round(model.share * 100)}
        </span>
        <span className="text-[var(--color-text-dim)]">
          {model.totalThink.toLocaleString('tr-TR')} tok gizli akıl yürütme
        </span>
      </div>
      {/* Per-call bars: height ∝ thinking share of that call's output. */}
      <div className="flex h-16 items-end gap-px overflow-x-auto">
        {model.bars.map((b, i) => (
          <div
            key={i}
            title={`${new Date(b.ts).toLocaleTimeString('tr-TR')} · %${Math.round(b.share * 100)} · ${b.think}/${b.out} tok`}
            className="w-1.5 shrink-0 rounded-sm bg-violet-500/60"
            style={{ height: `${Math.max(2, Math.round(b.share * 100))}%` }}
          />
        ))}
      </div>
      <p className="mt-1 text-[9px] text-[var(--color-text-dim)]">
        Her çubuk bir LLM çağrısı; yükseklik = o çağrının çıktısının düşünmeye giden oranı.
      </p>
    </div>
  )
}
