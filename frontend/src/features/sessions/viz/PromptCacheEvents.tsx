import { useMemo } from 'react'
import type { SessionDebugEvent } from '@/types'
import { buildPromptCacheSummary } from './flowVizData'

const KIND_META: Record<string, { label: string; cls: string }> = {
  frozen: { label: 'donduruldu', cls: 'bg-sky-500/15 text-sky-400' },
  adopted: { label: 'adopte', cls: 'bg-violet-500/15 text-violet-400' },
  stale: { label: 'stale', cls: 'bg-amber-500/15 text-amber-400' },
  refreshed: { label: 'yenilendi', cls: 'bg-emerald-500/15 text-emerald-400' },
  break: { label: 'kırılım', cls: 'bg-red-500/15 text-red-400' },
}

// PromptCacheEvents lists a session's prompt-cache lifecycle from the debug
// journal: prompt-epoch snapshots (freeze / adopt / held-back drift / explicit
// refresh — the prevention layer, _Docs/57) alongside detected cache breaks
// (cachebreak.go — the detection layer). With the epoch on, "kırılım" entries
// should only follow deliberate adopts; a break without an adopt next to it is
// the signal worth investigating. Renders nothing for a session with neither
// (a healthy cache is silent).
export function PromptCacheEvents({ events }: { events: SessionDebugEvent[] }) {
  const model = useMemo(() => buildPromptCacheSummary(events), [events])
  if (!model) {
    return <p className="text-[10px] text-[var(--color-text-dim)]">Bu oturumda prompt-cache olayı yok (sağlıklı: cache sessizce çalışıyor).</p>
  }
  return (
    <div>
      <div className="mb-1.5 flex flex-wrap gap-1.5">
        {Object.entries(model.counts)
          .filter(([, n]) => n > 0)
          .map(([kind, n]) => (
            <span key={kind} className={`rounded px-1.5 py-px text-[10px] font-medium ${KIND_META[kind].cls}`}>
              {KIND_META[kind].label}: {n}
            </span>
          ))}
      </div>
      <ul className="max-h-40 space-y-1 overflow-y-auto">
        {model.items.map((it, i) => (
          <li key={i} className="flex items-start gap-1.5 text-[10px] leading-snug">
            <span className={`mt-px shrink-0 rounded px-1 py-px font-medium ${KIND_META[it.kind].cls}`}>
              {KIND_META[it.kind].label}
            </span>
            <span className="text-[var(--color-text-dim)]">{new Date(it.ts).toLocaleTimeString('tr-TR')}</span>
            <span className={`min-w-0 break-words ${it.err ? 'text-[var(--color-danger)]' : 'text-[var(--color-text)]'}`}>
              {it.label}
              {it.detail && it.detail !== it.label ? ` — ${it.detail.slice(0, 160)}` : ''}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
