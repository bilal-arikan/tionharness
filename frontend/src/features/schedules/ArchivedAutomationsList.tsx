import { useCallback, useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArchiveRestore } from 'lucide-react'
import { api } from '@/api'
import type { Agent, Automation } from '@/types'
import { ArchiveViewBanner, LoadingState, toast } from '@/shared/components'
import { relativeTime } from '@/shared/lib/time'

interface Props {
  agents: Agent[]
  onError: (msg: string) => void
  /** Called after a rule is restored, so the live lanes re-fetch. */
  onRestored: () => void
}

// ArchivedAutomationsList is the Otomasyon screen's archive view: the rules that
// were archived (by hand or by the curator). They never fire while archived; each
// row restores its rule back into its lane with its configuration and ledger.
export function ArchivedAutomationsList({ agents, onError, onRestored }: Props) {
  const { t } = useTranslation('schedules')
  const [rows, setRows] = useState<Automation[]>([])
  const [loading, setLoading] = useState(true)
  const [busyId, setBusyId] = useState<string | null>(null)

  const load = useCallback(() => {
    api
      .listArchivedAutomations()
      .then(setRows)
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
  }, [onError])

  useEffect(() => load(), [load])

  const restore = async (a: Automation) => {
    setBusyId(a.id)
    try {
      await api.setArchived('automations', a.id, false)
      setRows((prev) => prev.filter((x) => x.id !== a.id))
      toast.success(t('archive.restored'))
      onRestored()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setBusyId(null)
    }
  }

  const agentName = (id: string) => agents.find((a) => a.id === id)?.name ?? id

  return (
    <div data-testid="automations-archive-view" className="flex min-h-0 flex-1 flex-col">
      <ArchiveViewBanner
        testId="automations-archive-banner"
        count={rows.length}
        noun={t('archive.noun')}
        restoreHint={t('archive.restoreHint')}
      />
      {loading ? (
        <LoadingState label={t('archive.loading')} className="flex-1" />
      ) : (
        <ul className="min-h-0 flex-1 space-y-2 overflow-y-auto p-3">
          {rows.map((a) => (
            <li
              key={a.id}
              data-testid="archived-automation-row"
              data-automation-id={a.id}
              className="flex items-center gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm"
            >
              <div className="min-w-0 flex-1">
                <div className="truncate font-medium">{a.name || a.id}</div>
                <div className="truncate text-xs text-[var(--color-text-dim)]">
                  {t(`triggerKinds.${a.triggerKind ?? 'tag'}`, {
                    defaultValue: a.triggerKind ?? 'tag',
                  })}
                  {a.targetAgentId ? ` · → ${agentName(a.targetAgentId)}` : ''}
                  {a.updatedAt ? ` · ${relativeTime(a.updatedAt)}` : ''}
                </div>
              </div>
              <button
                type="button"
                data-testid="archived-automation-restore"
                disabled={busyId === a.id}
                onClick={() => restore(a)}
                className="flex flex-shrink-0 items-center gap-1 rounded border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-50"
              >
                <ArchiveRestore size={13} /> {t('archive.restore')}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
