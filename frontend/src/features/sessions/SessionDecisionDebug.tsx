import { useEffect, useState } from 'react'
import { api } from '@/api'
import { getActiveWorkspace } from '@/api/client'
import { DeciderDebug } from '@/features/decider/DeciderDebug'
import type { DeciderView } from '@/types/decider'
import { LoadingState } from '@/shared/components'
import { useTranslation } from 'react-i18next'

export function SessionDecisionDebug({ sessionId }: { sessionId: string }) {
  const { t } = useTranslation('sessions')
  const [view, setView] = useState<DeciderView | null>(null)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  useEffect(() => {
    let alive = true
    api
      .getDecider()
      .then((result) => {
        if (alive) {
          setView(result)
          setError('')
        }
      })
      .catch((error: Error) => {
        if (alive) setError(error.message)
      })
    return () => {
      alive = false
    }
  }, [revision])
  if (error)
    return (
      <div className="space-y-3">
        <p role="alert">{error}</p>
        <button
          type="button"
          className="min-h-11 rounded border border-[var(--color-border)] px-3"
          onClick={() => setRevision((value) => value + 1)}
        >
          {t('decisions.refresh')}
        </button>
      </div>
    )
  if (!view) return <LoadingState label={t('decisions.loading')} />
  return (
    <DeciderDebug
      key={`${getActiveWorkspace() ?? ''}:${sessionId}`}
      view={view}
      sessionId={sessionId}
      workspaceId={getActiveWorkspace() ?? undefined}
    />
  )
}
