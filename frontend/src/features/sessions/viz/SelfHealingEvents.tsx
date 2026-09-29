import { useMemo } from 'react'
import type { SessionDebugEvent } from '@/types'
import { buildSelfHealingSummary } from './flowVizData'
import { formatTime } from '@/shared/lib/intl'
import { useTranslation } from 'react-i18next'

const KIND_META: Record<string, { labelKey: string; cls: string }> = {
  // Every hue maps onto a theme token (accent/warning/success/info) so the badges
  // re-theme with presets and stay legible in the light theme.
  recovery: {
    labelKey: 'visualization.selfHealing.kind.recovery',
    cls: 'bg-[color-mix(in_srgb,var(--color-info)_15%,transparent)] text-[var(--color-info)]',
  },
  repair: {
    labelKey: 'visualization.selfHealing.kind.repair',
    cls: 'bg-[color-mix(in_srgb,var(--color-accent)_15%,transparent)] text-[var(--color-accent)]',
  },
  guardrail: {
    labelKey: 'visualization.selfHealing.kind.guardrail',
    cls: 'bg-[color-mix(in_srgb,var(--color-warning)_15%,transparent)] text-[var(--color-warning)]',
  },
  lesson: {
    labelKey: 'visualization.selfHealing.kind.lesson',
    cls: 'bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] text-[var(--color-success)]',
  },
}

// SelfHealingEvents lists a session's self-healing activity (turn recoveries,
// sequence repairs, guardrail decisions, distilled lessons) from the debug
// journal — the visibility face of _Docs/56. Renders nothing for a session
// that healed nothing (the healthy common case).
export function SelfHealingEvents({ events }: { events: SessionDebugEvent[] }) {
  const { t } = useTranslation('sessions')
  const model = useMemo(() => buildSelfHealingSummary(events), [events])
  if (!model) {
    return (
      <p className="text-[10px] text-[var(--color-text-dim)]">
        {t('visualization.selfHealing.empty')}
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
              {t(KIND_META[kind].labelKey)}: {n}
            </span>
          ))}
      </div>
      <ul className="max-h-40 space-y-1 overflow-y-auto">
        {model.items.map((it, i) => (
          <li key={i} className="flex items-start gap-1.5 text-[10px] leading-snug">
            <span
              className={`mt-px shrink-0 rounded px-1 py-px font-medium ${KIND_META[it.kind].cls}`}
            >
              {t(KIND_META[it.kind].labelKey)}
            </span>
            <span className="text-[var(--color-text-dim)]">
              {formatTime(new Date(it.ts), { timeStyle: 'medium' })}
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
