import { useMemo } from 'react'
import type { SessionDebugEvent } from '@/types'
import { buildSelfHealingSummary } from './flowVizData'

const KIND_META: Record<string, { label: string; cls: string }> = {
  // sky has no semantic theme token, so it stays raw as a distinct categorical
  // hue; the rest map onto accent/warning/success so they re-theme with presets.
  recovery: { label: 'kurtarma', cls: 'bg-sky-500/15 text-sky-400' },
  repair: {
    label: 'onarım',
    cls: 'bg-[color-mix(in_srgb,var(--color-accent)_15%,transparent)] text-[var(--color-accent)]',
  },
  guardrail: {
    label: 'guardrail',
    cls: 'bg-[color-mix(in_srgb,var(--color-warning)_15%,transparent)] text-[var(--color-warning)]',
  },
  lesson: {
    label: 'ders',
    cls: 'bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] text-[var(--color-success)]',
  },
}

// SelfHealingEvents lists a session's self-healing activity (turn recoveries,
// sequence repairs, guardrail decisions, distilled lessons) from the debug
// journal — the visibility face of _Docs/56. Renders nothing for a session
// that healed nothing (the healthy common case).
export function SelfHealingEvents({ events }: { events: SessionDebugEvent[] }) {
  const model = useMemo(() => buildSelfHealingSummary(events), [events])
  if (!model) {
    return (
      <p className="text-[10px] text-[var(--color-text-dim)]">
        Bu oturumda self-healing olayı yok (sağlıklı).
      </p>
    )
  }
  return (
    <div>
      <div className="mb-1.5 flex flex-wrap gap-1.5">
        {Object.entries(model.counts)
          .filter(([, n]) => n > 0)
          .map(([kind, n]) => (
            <span
              key={kind}
              className={`rounded px-1.5 py-px text-[10px] font-medium ${KIND_META[kind].cls}`}
            >
              {KIND_META[kind].label}: {n}
            </span>
          ))}
      </div>
      <ul className="max-h-40 space-y-1 overflow-y-auto">
        {model.items.map((it, i) => (
          <li key={i} className="flex items-start gap-1.5 text-[10px] leading-snug">
            <span
              className={`mt-px shrink-0 rounded px-1 py-px font-medium ${KIND_META[it.kind].cls}`}
            >
              {KIND_META[it.kind].label}
            </span>
            <span className="text-[var(--color-text-dim)]">
              {new Date(it.ts).toLocaleTimeString('tr-TR')}
            </span>
            <span
              className={`min-w-0 break-words ${it.err ? 'text-[var(--color-danger)]' : 'text-[var(--color-text)]'}`}
            >
              {it.label}
              {it.detail && it.detail !== it.label ? ` — ${it.detail.slice(0, 160)}` : ''}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
