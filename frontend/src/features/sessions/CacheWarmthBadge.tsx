import { Flame, Snowflake } from 'lucide-react'
import { cacheRemaining, formatCountdown } from './sessionDetailFormat'
import { useTranslation } from 'react-i18next'

interface Props {
  // Session last-activity timestamp (unix seconds); the warm window starts here.
  updatedAt: number
  // Live tick (unix seconds) driving the countdown.
  nowSec: number
  // compact drops the wording, showing only the icon + countdown — for inline
  // placement in the context-window header where space is tight.
  compact?: boolean
}

// CacheWarmthBadge surfaces the prompt-cache warmth of a session: whether the
// Anthropic ephemeral cache (and the mirrored prompt epoch) is still warm and,
// if so, how long until it goes cold. Derived entirely from updatedAt + a live
// tick, so it needs no backend field. Once cold, the next turn pays a full
// cache-miss and pending epoch changes are adopted for free.
export function CacheWarmthBadge({ updatedAt, nowSec, compact }: Props) {
  const { t } = useTranslation('sessions')
  const remaining = cacheRemaining(updatedAt, nowSec)
  const warm = remaining > 0

  if (warm) {
    return (
      <span
        title={t('cache.warmTitle', { remaining: formatCountdown(remaining) })}
        className="inline-flex items-center gap-1 font-mono text-[11px] text-[var(--color-warning)]"
      >
        <Flame size={12} className="shrink-0" />
        {compact
          ? formatCountdown(remaining)
          : t('cache.warmRemaining', { remaining: formatCountdown(remaining) })}
      </span>
    )
  }

  return (
    <span
      title={t('cache.coldTitle')}
      className="inline-flex items-center gap-1 text-[11px] text-[var(--color-text-dim)]"
    >
      <Snowflake size={12} className="shrink-0" />
      {compact ? '—' : t('cache.cold')}
    </span>
  )
}
