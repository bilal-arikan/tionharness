// A waiting banner shown above the composer while a session is paused on a
// pending self-wake (schedule_wake): the agent ended its turn and armed an
// automatic re-invocation after a delay. Without this the session would look
// finished. It surfaces the agent's reason, a live countdown to the wake, and a
// "Durdur" control that disarms the wake (POST /api/chat/wake/cancel).
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import { AlarmClock, Square } from 'lucide-react'
import { serverNow } from '@/shared/lib/serverClock'
import { ComposerCard } from './ComposerCard'
import { BTN_STOP_COMPACT } from './composer/buttonStyles'

interface Props {
  // The agent's stated reason for waiting (may be empty).
  reason: string
  // Unix seconds when the wake fires (0 if unknown — countdown is then hidden).
  fireAt: number
  // Durdur: disarm the pending wake.
  onCancel: () => void
  // Read-only run logs (schedule/automation) surface the countdown for context
  // but must not let a viewer disarm the automation's own self-wake — the Durdur
  // control is hidden for them.
  hideCancel?: boolean
}

// remainingLabel renders the seconds left until fireAt as a compact "~Xs" / "~Xm Ys".
function remainingLabel(t: TFunction<'chatStatus'>, fireAt: number, nowSec: number): string | null {
  if (fireAt <= 0) return null
  const left = fireAt - nowSec
  if (left <= 0) return t('wake.soon')
  if (left < 60) return t('wake.seconds', { seconds: left })
  const m = Math.floor(left / 60)
  const s = left % 60
  return s
    ? t('wake.minutesSeconds', { minutes: m, seconds: s })
    : t('wake.minutes', { minutes: m })
}

// WakeWaitBanner shows the "waiting to auto-resume" state for a session with a
// pending self-wake, plus a Durdur control.
export function WakeWaitBanner({ reason, fireAt, onCancel, hideCancel }: Props) {
  const { t } = useTranslation('chatStatus')
  // Tick once a second so the countdown stays live without a parent re-render.
  const [now, setNow] = useState(() => serverNow())
  useEffect(() => {
    const t = setInterval(() => setNow(serverNow()), 1000)
    return () => clearInterval(t)
  }, [])

  const left = remainingLabel(t, fireAt, now)
  return (
    <ComposerCard tone="wake" className="flex items-center gap-2 px-3 py-2 text-sm">
      <AlarmClock size={15} className="shrink-0 animate-pulse text-[var(--color-warning)]" />
      <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">
        <span className="font-medium text-[var(--color-warning)]">{t('wake.waiting')}</span>
        {left && (
          <span className="text-[var(--color-text-dim)]">
            {t('wake.countdownSuffix', { value: left })}
          </span>
        )}
        {reason && (
          <span className="text-[var(--color-text-dim)]">{t('wake.reasonSuffix', { reason })}</span>
        )}
      </span>
      {!hideCancel && (
        <button onClick={onCancel} title={t('wake.stopTitle')} className={BTN_STOP_COMPACT}>
          <Square size={13} />
          {t('wake.stop')}
        </button>
      )}
    </ComposerCard>
  )
}
