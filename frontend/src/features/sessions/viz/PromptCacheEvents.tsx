import { useMemo } from 'react'
import type { SessionDebugEvent } from '@/types'
import { buildPromptCacheSummary } from './flowVizData'
import { formatTime } from '@/shared/lib/intl'
import { useTranslation } from 'react-i18next'

const KIND_META: Record<string, { labelKey: string; cls: string }> = {
  // Every hue maps onto a theme token (accent/warning/success/danger/info) so the
  // badges re-theme with presets and stay legible in the light theme.
  frozen: {
    labelKey: 'visualization.promptCache.kind.frozen',
    cls: 'bg-[color-mix(in_srgb,var(--color-info)_15%,transparent)] text-[var(--color-info)]',
  },
  adopted: {
    labelKey: 'visualization.promptCache.kind.adopted',
    cls: 'bg-[color-mix(in_srgb,var(--color-accent)_15%,transparent)] text-[var(--color-accent)]',
  },
  stale: {
    labelKey: 'visualization.promptCache.kind.stale',
    cls: 'bg-[color-mix(in_srgb,var(--color-warning)_15%,transparent)] text-[var(--color-warning)]',
  },
  refreshed: {
    labelKey: 'visualization.promptCache.kind.refreshed',
    cls: 'bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] text-[var(--color-success)]',
  },
  break: {
    labelKey: 'visualization.promptCache.kind.break',
    cls: 'bg-[color-mix(in_srgb,var(--color-danger)_15%,transparent)] text-[var(--color-danger)]',
  },
}

// PromptCacheEvents lists a session's prompt-cache lifecycle from the debug
// journal: prompt-epoch snapshots (freeze / adopt / held-back drift / explicit
// refresh — the prevention layer, _Docs/57) alongside detected cache breaks
// (cachebreak.go — the detection layer). With the epoch on, "kırılım" entries
// should only follow deliberate adopts; a break without an adopt next to it is
// the signal worth investigating. Renders nothing for a session with neither
// (a healthy cache is silent).
export function PromptCacheEvents({ events }: { events: SessionDebugEvent[] }) {
  const { t } = useTranslation('sessions')
  const model = useMemo(() => buildPromptCacheSummary(events), [events])
  if (!model) {
    return (
      <p className="text-[10px] text-[var(--color-text-dim)]">
        {t('visualization.promptCache.empty')}
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
