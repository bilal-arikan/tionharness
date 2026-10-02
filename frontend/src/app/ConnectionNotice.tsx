import { useEffect, useState, useSyncExternalStore } from 'react'
import { useTranslation } from 'react-i18next'
import {
  getLiveConnectionStatus,
  reconnectLiveConnection,
  subscribeLiveConnectionStatus,
} from '@/api/liveConnection'

// Keep the saved conversation visible during a disconnect. A delayed notice
// avoids flashing during ordinary session/workspace switches.
export function ConnectionNotice() {
  const { t } = useTranslation('shared')
  const status = useSyncExternalStore(subscribeLiveConnectionStatus, getLiveConnectionStatus)
  const [visible, setVisible] = useState(false)
  useEffect(() => {
    if (status === 'connected') return
    const timer = setTimeout(() => setVisible(true), 2000)
    return () => clearTimeout(timer)
  }, [status])
  useEffect(() => {
    if (status === 'connected') setVisible(false)
  }, [status])
  if (!visible || status === 'connected') return null
  return (
    <div
      role="status"
      aria-live="polite"
      className="flex shrink-0 flex-wrap items-center justify-between gap-2 border-b border-[var(--color-border)] bg-[var(--color-bg)] px-4 py-2 text-xs text-[var(--color-text-dim)]"
    >
      <span>{t('api.network.reconnecting')}</span>
      <button
        type="button"
        onClick={reconnectLiveConnection}
        className="shrink-0 text-[var(--color-accent)] hover:underline"
      >
        {t('api.network.retryConnection')}
      </button>
    </div>
  )
}
