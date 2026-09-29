// AutomationFires — the fire ledger (R5, GET /api/automations/{id}/fires)
// folded into an automation card: every attempt, fired / skipped (with the
// reason) / failed, newest first. Loaded on demand so the board stays cheap.
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, ChevronRight } from 'lucide-react'
import { api } from '@/api'
import type { AutomationFireRecord } from '@/types'
import { fmtTime } from './timeUtils'
import { fireGlyph, SKIP_REASON_LABEL } from './fireMeta'

interface Props {
  automationId: string
  // Bumps when the automation fired again (lastFiredAt) so an open list refreshes.
  refreshKey?: number
}

const LIMIT = 25

export function AutomationFires({ automationId, refreshKey }: Props) {
  const { t } = useTranslation('schedules')
  const [open, setOpen] = useState(false)
  const [rows, setRows] = useState<AutomationFireRecord[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    let cancelled = false
    api
      .listAutomationFires(automationId, LIMIT)
      .then((r) => {
        if (!cancelled) setRows([...r].sort((a, b) => b.at - a.at))
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : String(e))
      })
    return () => {
      cancelled = true
    }
  }, [open, automationId, refreshKey])

  return (
    <div className="mt-1 text-[11px]">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex items-center gap-1 text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
        title={t('fires.title')}
        data-testid="automation-fires-toggle"
      >
        {open ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        {t('fires.label')}
      </button>
      {open && (
        <div className="mt-1 max-h-40 overflow-auto rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1">
          {error ? (
            <div className="text-[var(--color-danger)]">{error}</div>
          ) : rows === null ? (
            <div className="text-[var(--color-text-dim)]">{t('common.loading')}</div>
          ) : rows.length === 0 ? (
            <div className="text-[var(--color-text-dim)]">{t('fires.empty')}</div>
          ) : (
            <ul className="flex flex-col gap-0.5">
              {rows.map((f, i) => (
                <li key={`${f.at}-${i}`} className="flex items-center gap-1.5 truncate">
                  <span
                    className={
                      f.outcome === 'fired'
                        ? 'text-[var(--color-success)]'
                        : f.outcome === 'failed'
                          ? 'text-[var(--color-danger)]'
                          : 'text-[var(--color-text-dim)]'
                    }
                  >
                    {fireGlyph(f.outcome)}
                  </span>
                  <span className="text-[var(--color-text-dim)]">{fmtTime(f.at)}</span>
                  {f.triggerKind && (
                    <span className="text-[var(--color-text-dim)]">
                      {t(`triggerKinds.${f.triggerKind}`, { defaultValue: f.triggerKind })}
                    </span>
                  )}
                  {f.outcome === 'skipped' && f.reason && (
                    <span title={f.reason}>{SKIP_REASON_LABEL[f.reason] ?? f.reason}</span>
                  )}
                  {f.outcome === 'failed' && f.error && (
                    <span className="truncate text-[var(--color-danger)]" title={f.error}>
                      {f.error}
                    </span>
                  )}
                  {f.sessionId && (
                    <span
                      className="font-mono text-[var(--color-text-dim)]"
                      title={t('fires.openedSession')}
                    >
                      {t('common.targetArrow', { target: f.sessionId })}
                    </span>
                  )}
                  {f.iteration !== undefined && f.iteration > 0 && (
                    <span className="text-[var(--color-text-dim)]">#{f.iteration}</span>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  )
}
