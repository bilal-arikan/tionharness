import { Search, X, Layers, TriangleAlert } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { InsightLens } from '@/types'
import type { FindingFilter } from './insightHelpers'
import { FilterToggle, useFilterDisclosure } from '@/shared/components'

interface Props {
  filter: FindingFilter
  setFilter: (f: FindingFilter) => void
  lenses: InsightLens[]
  // Clustering is only offered in the list view; the kanban omits it (columns are
  // the grouping). When setCluster is undefined the "Kümele" button is hidden.
  cluster?: boolean
  setCluster?: (v: boolean) => void
}

const selectCls =
  'rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm'

export function FilterBar({ filter, setFilter, lenses, cluster, setCluster }: Props) {
  const { t } = useTranslation('insight')
  const active =
    filter.channel ||
    filter.status ||
    filter.severity ||
    filter.lens ||
    filter.regressedOnly ||
    filter.search
  // Filters narrowing the board besides the search box, badged on the toggle so
  // a folded strip never hides rows silently.
  const facetCount = [
    filter.channel,
    filter.status,
    filter.severity,
    filter.lens,
    filter.regressedOnly,
  ].filter(Boolean).length
  // The search box always shows; the selects fold behind one toggle on a phone
  // (always inline from `md:` up), so the board is not pushed down by three
  // rows of controls.
  const [open, toggle] = useFilterDisclosure('insight')
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative">
        <Search className="pointer-events-none absolute left-2 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-text-dim)]" />
        <input
          type="text"
          value={filter.search ?? ''}
          onChange={(e) => setFilter({ ...filter, search: e.target.value })}
          placeholder={t('filters.searchPlaceholder')}
          className="w-48 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] py-1 pl-8 pr-2 text-sm"
        />
      </div>

      <FilterToggle
        open={open}
        onToggle={toggle}
        activeCount={facetCount}
        className="md:hidden"
        testId="insight-filters-toggle"
      />
      <div className={`${open ? 'contents' : 'hidden'} md:contents`}>
        <select
          className={selectCls}
          value={filter.channel ?? ''}
          onChange={(e) => setFilter({ ...filter, channel: e.target.value || undefined })}
        >
          <option value="">{t('filters.allChannels')}</option>
          <option value="app-fix">{t('channel.appFix')}</option>
          <option value="workspace-opt">{t('channel.workspaceOpt')}</option>
        </select>

        <select
          className={selectCls}
          value={filter.status ?? ''}
          onChange={(e) => setFilter({ ...filter, status: e.target.value || undefined })}
        >
          <option value="">{t('filters.allStatuses')}</option>
          <option value="new">{t('status.new')}</option>
          <option value="accepted">{t('status.accepted')}</option>
          <option value="applied">{t('status.applied')}</option>
          <option value="verified">{t('status.verified')}</option>
          <option value="dismissed">{t('status.dismissed')}</option>
        </select>

        <select
          className={selectCls}
          value={filter.severity ?? ''}
          onChange={(e) => setFilter({ ...filter, severity: e.target.value || undefined })}
        >
          <option value="">{t('filters.allSeverities')}</option>
          <option value="high">{t('severity.high')}</option>
          <option value="med">{t('severity.medium')}</option>
          <option value="low">{t('severity.low')}</option>
        </select>

        <select
          className={selectCls}
          value={filter.lens ?? ''}
          onChange={(e) => setFilter({ ...filter, lens: e.target.value || undefined })}
        >
          <option value="">{t('filters.allLenses')}</option>
          {lenses.map((l) => (
            <option key={l.id} value={l.id}>
              {l.id}
            </option>
          ))}
        </select>

        <button
          onClick={() => setFilter({ ...filter, regressedOnly: !filter.regressedOnly })}
          className={`flex items-center gap-1 rounded-md border px-2 py-1 text-sm ${
            filter.regressedOnly
              ? 'border-[var(--color-danger)] text-[var(--color-danger)]'
              : 'border-[var(--color-border)] hover:bg-[var(--color-surface-2)]'
          }`}
        >
          <TriangleAlert className="h-3.5 w-3.5" /> {t('filters.regression')}
        </button>

        {setCluster && (
          <button
            onClick={() => setCluster(!cluster)}
            className={`flex items-center gap-1 rounded-md border px-2 py-1 text-sm ${
              cluster
                ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
                : 'border-[var(--color-border)] hover:bg-[var(--color-surface-2)]'
            }`}
            title={t('filters.clusterTitle')}
          >
            <Layers className="h-4 w-4" /> {t('filters.cluster')}
          </button>
        )}
      </div>

      {active && (
        <button
          onClick={() => setFilter({})}
          className="flex items-center gap-1 rounded-md px-2 py-1 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
        >
          <X className="h-4 w-4" /> {t('filters.clear')}
        </button>
      )}
    </div>
  )
}
