import { useEffect, useState, type ComponentType } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, Hourglass, MessageCircleQuestion, Radio, XCircle } from 'lucide-react'
import type { ViewGraphStatus } from '@/types'
import { relativeAge, type AttentionFacet } from './explorerAttention'
import { toggleValue, type ExplorerFilter } from './explorerFilter'

interface Props {
  status: ViewGraphStatus | null | undefined
  // Sessions glowing on the map right now (the live layer's count; the same
  // number the backend puts in status.running, but this one moves with the
  // payload the canvas draws).
  liveCount: number
  filter: ExplorerFilter
  onChange: (next: ExplorerFilter) => void
}

type Tone = 'live' | 'danger' | 'warn'

const TONE_VAR: Record<Tone, string> = {
  live: 'var(--color-accent)',
  danger: 'var(--color-danger)',
  warn: 'var(--color-warning)',
}

interface Counter {
  key: string
  icon: ComponentType<{ size?: number }>
  label: string
  count: number
  tone: Tone
  pressed: boolean
  onToggle: () => void
}

// ExplorerStatusStrip is the row of live counters above the map: what a user
// reads before reading any node. Every counter is also a filter — pressing
// "takılan · 2" narrows the map to those two sessions — so the strip is the
// entry point for "what needs me", not only a readout. A zero counter is shown
// dimmed and cannot be pressed: an empty map would read as a failed load.
export function ExplorerStatusStrip({ status, liveCount, filter, onChange }: Props) {
  const { t } = useTranslation('explorer')
  // The digest age is relative to "now": refreshed on a slow clock so the label
  // does not freeze at "2 min ago" while the map sits open.
  const [nowMs, setNowMs] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNowMs(Date.now()), 30_000)
    return () => window.clearInterval(timer)
  }, [])
  if (!status) return null
  const facet = (name: AttentionFacet) => ({
    pressed: filter.attention.includes(name),
    onToggle: () => onChange({ ...filter, attention: toggleValue(filter.attention, name) }),
  })
  const counters: Counter[] = [
    {
      key: 'running',
      icon: Radio,
      label: t('status.running'),
      count: liveCount,
      tone: 'live',
      pressed: filter.liveOnly,
      onToggle: () => onChange({ ...filter, liveOnly: !filter.liveOnly }),
    },
    {
      key: 'waiting',
      icon: MessageCircleQuestion,
      label: t('status.waiting'),
      count: status.waiting,
      tone: 'warn',
      ...facet('waiting'),
    },
    {
      key: 'stuck',
      icon: AlertTriangle,
      label: t('status.stuck'),
      count: status.stuck,
      tone: 'danger',
      ...facet('stuck'),
    },
    {
      key: 'failed',
      icon: XCircle,
      label: t('status.failed'),
      count: status.failedCards,
      tone: 'danger',
      ...facet('failed'),
    },
    {
      key: 'stale',
      icon: Hourglass,
      label: t('status.stale'),
      count: status.stale,
      tone: 'warn',
      ...facet('stale'),
    },
  ]
  const age = relativeAge(status.lastDigestAt, nowMs)
  return (
    <div
      role="group"
      aria-label={t('status.label')}
      className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] py-1.5 text-xs max-md:px-3 md:px-6"
    >
      {counters.map(({ key, icon: Icon, label, count, tone, pressed, onToggle }) => {
        const empty = count === 0 && !pressed
        return (
          <button
            key={key}
            type="button"
            onClick={onToggle}
            disabled={empty}
            aria-pressed={pressed}
            title={t('status.filterHint')}
            className={`flex items-center gap-1 rounded-full border px-2 py-0.5 transition disabled:cursor-default ${
              empty ? 'opacity-45' : 'hover:brightness-110'
            }`}
            style={{
              borderColor: pressed ? TONE_VAR[tone] : 'var(--color-border)',
              color: empty ? 'var(--color-text-dim)' : TONE_VAR[tone],
              background: pressed
                ? `color-mix(in srgb, ${TONE_VAR[tone]} 18%, transparent)`
                : undefined,
            }}
          >
            <Icon size={12} />
            <span>
              {label} · {count}
            </span>
          </button>
        )
      })}
      {status.failedRuns > 0 && (
        <span className="text-[var(--color-text-dim)]">
          {t('status.failedRuns', { count: status.failedRuns })}
        </span>
      )}
      <span className="ml-auto truncate text-[var(--color-text-dim)]">
        {t('status.notesToday', { count: status.notesToday })}
        {' · '}
        {age ? t(`status.age.${age.unit}`, { count: age.count }) : t('status.noDigest')}
      </span>
    </div>
  )
}
