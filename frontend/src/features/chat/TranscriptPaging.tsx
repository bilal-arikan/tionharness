import type { TranscriptSummary } from '@/types'
import { useTranslation } from 'react-i18next'

export interface TranscriptPagingState {
  offset: number
  total: number
  summary?: TranscriptSummary
  participants?: string[]
  hasOlder: boolean
  hasNewer: boolean
  loading: boolean
  loadOlder: () => void
  loadNewer: () => void
  loadLatest: () => void
}

export function TranscriptPaging({
  state,
  edge,
}: {
  state?: TranscriptPagingState
  edge: 'older' | 'newer'
}) {
  const { t } = useTranslation('chatStatus')
  if (!state || (edge === 'older' ? !state.hasOlder : !state.hasNewer)) return null
  return (
    <div className="flex justify-center gap-3 py-3 text-sm">
      <button
        disabled={state.loading}
        onClick={edge === 'older' ? state.loadOlder : state.loadNewer}
        className="rounded-lg border border-[var(--color-border)] px-3 py-2 disabled:opacity-50"
      >
        {state.loading
          ? t('transcript.loading')
          : edge === 'older'
            ? t('transcript.earlier')
            : t('transcript.newer')}
      </button>
      {edge === 'newer' && (
        <button disabled={state.loading} onClick={state.loadLatest} className="px-3 py-2">
          {t('transcript.latest')}
        </button>
      )}
    </div>
  )
}
