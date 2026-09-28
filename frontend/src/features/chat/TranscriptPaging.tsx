import type { TranscriptSummary } from '@/types'

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
  if (!state || (edge === 'older' ? !state.hasOlder : !state.hasNewer)) return null
  return (
    <div className="flex justify-center gap-3 py-3 text-sm">
      <button
        disabled={state.loading}
        onClick={edge === 'older' ? state.loadOlder : state.loadNewer}
        className="rounded-lg border border-[var(--color-border)] px-3 py-2 disabled:opacity-50"
      >
        {state.loading
          ? 'Loading…'
          : edge === 'older'
            ? 'Load earlier messages'
            : 'Load newer messages'}
      </button>
      {edge === 'newer' && (
        <button disabled={state.loading} onClick={state.loadLatest} className="px-3 py-2">
          Jump to latest
        </button>
      )}
    </div>
  )
}
