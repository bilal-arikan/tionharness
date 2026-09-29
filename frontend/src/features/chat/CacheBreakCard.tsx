import { useState } from 'react'
import { tokens as fmtTok } from '@/shared/lib/format'
import { RefreshCw } from 'lucide-react'
import type { TurnStep } from '@/types'
import { api } from '@/api'
import { toast } from '@/shared/components'
import { STEP_KIND_MAP } from '@/shared/stepKinds'
import { useTranslation } from 'react-i18next'

const HeaderIcon = STEP_KIND_MAP.cache_break.Icon

interface Props {
  step: TurnStep
  // The open session, needed for the "refresh context" remedy. Absent in
  // read-only/preview renders, where the card degrades to information only.
  sessionId?: string
}

// Human headline per attributed cause. The backend already ships a Turkish
// `detail` sentence; this is the SHORT form for the collapsed row so the card
// stays one line at rest.
// CacheBreakCard reports that THIS turn lost the session's warm prompt-cache
// prefix and re-paid it cold, with the attributed cause and — where one exists —
// the remedy. Only "something changed" causes reach the chat (a TTL cooldown is
// the normal price of a pause and is surfaced by the transcript's cold divider
// instead), so every card here is actionable or worth investigating.
//
// Note on `prompt-or-tools-changed`: with the prompt epoch on (default), a frozen
// session prefix should make this cause IMPOSSIBLE mid-session — the drift is
// held back until a deliberate adopt. Seeing it therefore means either the epoch
// is off for this workspace or something bypassed the snapshot, which is why the
// card says so rather than presenting it as routine.
export function CacheBreakCard({ step, sessionId }: Props) {
  const { t } = useTranslation('chat')
  const [open, setOpen] = useState(false)
  const [refreshing, setRefreshing] = useState(false)
  const reason = step.reason ?? ''
  const headline = ['model-changed', 'prompt-or-tools-changed'].includes(reason)
    ? t(`cacheBreak.headline.${reason}`)
    : (step.text?.trim() ?? t('cacheBreak.headline.generic'))
  const suspicious = reason === 'prompt-or-tools-changed'

  const refresh = async () => {
    if (!sessionId || refreshing) return
    setRefreshing(true)
    try {
      await api.summarizeSession(sessionId, 'refresh-context')
      toast.success(t('cacheBreak.refreshSuccess'))
    } catch (e) {
      toast.error(t('cacheBreak.refreshError', { error: (e as Error).message }))
    } finally {
      setRefreshing(false)
    }
  }

  return (
    <div className="my-0.5 rounded-md border border-[color-mix(in_srgb,var(--color-warning)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_8%,transparent)] text-xs">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[var(--color-warning)]"
      >
        <HeaderIcon size={14} className="shrink-0" />
        <span className="min-w-0 flex-1 truncate font-medium">{headline}</span>
        {!!step.coldTokens && (
          <span
            title={t('cacheBreak.coldPrefixSize')}
            className="shrink-0 font-mono text-[10px] opacity-80"
          >
            {t('common.tokenShort', { count: fmtTok(step.coldTokens) })}
          </span>
        )}
      </button>
      {open && (
        <div className="border-t border-[color-mix(in_srgb,var(--color-warning)_20%,transparent)] px-3 py-2 text-[11px] text-[var(--color-text-dim)]">
          {step.text?.trim() && <p>{step.text}</p>}
          {suspicious && (
            <p className="mt-1.5 text-[var(--color-danger)]">{t('cacheBreak.suspiciousDetail')}</p>
          )}
          {reason === 'model-changed' && (
            <p className="mt-1.5">{t('cacheBreak.modelChangedDetail')}</p>
          )}
          {sessionId && suspicious && (
            <button
              type="button"
              onClick={refresh}
              disabled={refreshing}
              className="mt-2 inline-flex items-center gap-1.5 rounded border border-[var(--color-border)] px-2 py-1 text-[11px] text-[var(--color-text)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-50"
            >
              <RefreshCw size={11} className={refreshing ? 'animate-spin' : undefined} />
              {t('cacheBreak.refresh')}
            </button>
          )}
        </div>
      )}
    </div>
  )
}
