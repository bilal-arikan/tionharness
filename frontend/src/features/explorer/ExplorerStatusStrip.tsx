import { useEffect, useState, type ComponentType } from 'react'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, Hourglass, Info, MessageCircleQuestion, Radio, XCircle } from 'lucide-react'
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

// ExplorerStatusStrip is the map toolbar's live counters: what a user reads
// before reading any node. Every counter is also a filter — pressing
// "takılan · 2" narrows the map to those two sessions — so the strip is the
// entry point for "what needs me", not only a readout. Only counters with
// something to show are drawn (a zero would only narrow to an empty map), plus a
// pressed one so it can be released; the running counter is the live-only
// filter. Today's notes and the digest age ride in the info icon's tooltip.
export function ExplorerStatusStrip({ status, liveCount, filter, onChange }: Props) {
  const { t } = useTranslation('explorer')
  // The digest age is relative to "now": refreshed on a slow clock so the label
  // does not freeze at "2 min ago" while the map sits open.
  const [nowMs, setNowMs] = useState(() => Date.now())
  useEffect(() => {
    const timer = window.setInterval(() => setNowMs(Date.now()), 30_000)
    return () => window.clearInterval(timer)
  }, [])
  const facet = (name: AttentionFacet) => ({
    pressed: filter.attention.includes(name),
    onToggle: () => onChange({ ...filter, attention: toggleValue(filter.attention, name) }),
  })
  const all: Counter[] = [
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
      count: status?.waiting ?? 0,
      tone: 'warn',
      ...facet('waiting'),
    },
    {
      key: 'stuck',
      icon: AlertTriangle,
      label: t('status.stuck'),
      count: status?.stuck ?? 0,
      tone: 'danger',
      ...facet('stuck'),
    },
    {
      key: 'failed',
      icon: XCircle,
      label: t('status.failed'),
      count: status?.failedCards ?? 0,
      tone: 'danger',
      ...facet('failed'),
    },
    {
      key: 'stale',
      icon: Hourglass,
      label: t('status.stale'),
      count: status?.stale ?? 0,
      tone: 'warn',
      ...facet('stale'),
    },
  ]
  const counters = all.filter((c) => c.count > 0 || c.pressed)
  const age = relativeAge(status?.lastDigestAt, nowMs)
  const info = status
    ? `${t('status.notesToday', { count: status.notesToday })} · ${
        age ? t(`status.age.${age.unit}`, { count: age.count }) : t('status.noDigest')
      }`
    : ''
  return (
    <span role="group" aria-label={t('status.label')} className="flex flex-wrap items-center gap-2">
      {counters.map(({ key, icon: Icon, label, count, tone, pressed, onToggle }) => (
        <button
          key={key}
          type="button"
          onClick={onToggle}
          aria-pressed={pressed}
          title={t('status.filterHint')}
          className="flex items-center gap-1 rounded-full border px-2 py-0.5 transition hover:brightness-110"
          style={{
            borderColor: pressed ? TONE_VAR[tone] : 'var(--color-border)',
            color: TONE_VAR[tone],
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
      ))}
      {status && status.failedRuns > 0 && (
        <span className="text-[var(--color-text-dim)]">
          {t('status.failedRuns', { count: status.failedRuns })}
        </span>
      )}
      {info && (
        <span
          className="flex items-center text-[var(--color-text-dim)]"
          title={info}
          aria-label={info}
          role="img"
        >
          <Info size={12} />
        </span>
      )}
    </span>
  )
}
