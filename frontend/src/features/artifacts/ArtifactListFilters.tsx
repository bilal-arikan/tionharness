import type { Dispatch, SetStateAction } from 'react'
import { Archive, Search, X } from 'lucide-react'
import type { OriginFilter } from './artifactGrouping'

interface Props {
  query: string
  setQuery: Dispatch<SetStateAction<string>>
  originFilter: OriginFilter
  setOriginFilter: Dispatch<SetStateAction<OriginFilter>>
  showArchived: boolean
  setShowArchived: Dispatch<SetStateAction<boolean>>
  archivedCount: number
}

const ORIGIN_FACETS: ReadonlyArray<readonly [OriginFilter, string]> = [
  ['all', 'Tümü'],
  ['chat', 'Sohbet eki'],
  ['manual', 'Manuel'],
  ['agent', 'Ajan'],
  ['tool', 'Tool'],
  ['plan', 'Plan'],
]

// ArtifactListFilters is the list column's filter strip: title search + origin
// facet + the archived-view toggle. Presentational — every filter is owned by
// useArtifactList and applied server-side.
export function ArtifactListFilters({
  query,
  setQuery,
  originFilter,
  setOriginFilter,
  showArchived,
  setShowArchived,
  archivedCount,
}: Props) {
  return (
    <div className="flex flex-col gap-2 border-b border-[var(--color-border)] px-3 py-2">
      <div className="relative">
        <Search
          size={13}
          className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-text-dim)]"
        />
        <input
          data-testid="artifacts-search-input"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Başlıkta ara…"
          className="w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] py-1.5 pl-7 pr-7 text-xs outline-none focus:border-[var(--color-accent)]"
        />
        {query && (
          <button
            onClick={() => setQuery('')}
            title="Temizle"
            className="absolute right-1.5 top-1/2 -translate-y-1/2 rounded p-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
          >
            <X size={13} />
          </button>
        )}
      </div>
      <div className="flex flex-wrap gap-1">
        {ORIGIN_FACETS.map(([val, label]) => (
          <button
            key={val}
            data-testid="artifacts-filter"
            data-origin={val}
            onClick={() => setOriginFilter(val)}
            className={`rounded px-1.5 py-0.5 text-[10px] font-medium transition ${
              originFilter === val
                ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
                : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
            }`}
          >
            {label}
          </button>
        ))}
      </div>
      {/* Archived view toggle: flips the list between active and archived
          artifacts. Hidden until at least one artifact has been archived. */}
      {(archivedCount > 0 || showArchived) && (
        <button
          data-testid="artifacts-archived-toggle"
          data-active={showArchived}
          onClick={() => setShowArchived((v) => !v)}
          title={showArchived ? 'Aktif artifactlara dön' : 'Arşivlenen artifactları göster'}
          className={`flex items-center gap-1.5 self-start rounded px-1.5 py-0.5 text-[10px] font-medium transition ${
            showArchived
              ? 'bg-[var(--color-accent)] text-[var(--color-on-accent)]'
              : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
          }`}
        >
          <Archive size={12} />
          {showArchived ? 'Arşiv görünümü' : `Arşiv (${archivedCount})`}
        </button>
      )}
    </div>
  )
}
