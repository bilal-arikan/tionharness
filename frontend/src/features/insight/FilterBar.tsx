import { Search, X, Layers } from 'lucide-react'
import type { InsightLens } from '@/types'
import type { FindingFilter } from './insightHelpers'

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
  const active =
    filter.channel ||
    filter.status ||
    filter.severity ||
    filter.lens ||
    filter.regressedOnly ||
    filter.search
  return (
    <div className="flex flex-wrap items-center gap-2">
      <div className="relative">
        <Search className="pointer-events-none absolute left-2 top-1/2 h-4 w-4 -translate-y-1/2 text-[var(--color-text-dim)]" />
        <input
          type="text"
          value={filter.search ?? ''}
          onChange={(e) => setFilter({ ...filter, search: e.target.value })}
          placeholder="Bulgu ara…"
          className="w-48 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] py-1 pl-8 pr-2 text-sm"
        />
      </div>

      <select
        className={selectCls}
        value={filter.channel ?? ''}
        onChange={(e) => setFilter({ ...filter, channel: e.target.value || undefined })}
      >
        <option value="">Tüm kanallar</option>
        <option value="app-fix">app-fix</option>
        <option value="workspace-opt">workspace-opt</option>
      </select>

      <select
        className={selectCls}
        value={filter.status ?? ''}
        onChange={(e) => setFilter({ ...filter, status: e.target.value || undefined })}
      >
        <option value="">Tüm statüler</option>
        <option value="new">yeni</option>
        <option value="accepted">kabul</option>
        <option value="applied">uygulandı</option>
        <option value="verified">doğrulandı</option>
        <option value="dismissed">yoksayıldı</option>
      </select>

      <select
        className={selectCls}
        value={filter.severity ?? ''}
        onChange={(e) => setFilter({ ...filter, severity: e.target.value || undefined })}
      >
        <option value="">Tüm önem</option>
        <option value="high">yüksek</option>
        <option value="med">orta</option>
        <option value="low">düşük</option>
      </select>

      <select
        className={selectCls}
        value={filter.lens ?? ''}
        onChange={(e) => setFilter({ ...filter, lens: e.target.value || undefined })}
      >
        <option value="">Tüm lensler</option>
        {lenses.map((l) => (
          <option key={l.id} value={l.id}>
            {l.id}
          </option>
        ))}
      </select>

      <button
        onClick={() => setFilter({ ...filter, regressedOnly: !filter.regressedOnly })}
        className={`rounded-md border px-2 py-1 text-sm ${
          filter.regressedOnly
            ? 'border-[var(--color-danger)] text-[var(--color-danger)]'
            : 'border-[var(--color-border)] hover:bg-[var(--color-surface-2)]'
        }`}
      >
        ⚠ Regresyon
      </button>

      {setCluster && (
        <button
          onClick={() => setCluster(!cluster)}
          className={`flex items-center gap-1 rounded-md border px-2 py-1 text-sm ${
            cluster
              ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
              : 'border-[var(--color-border)] hover:bg-[var(--color-surface-2)]'
          }`}
          title="Benzer bulguları kümele"
        >
          <Layers className="h-4 w-4" /> Kümele
        </button>
      )}

      {active && (
        <button
          onClick={() => setFilter({})}
          className="flex items-center gap-1 rounded-md px-2 py-1 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]"
        >
          <X className="h-4 w-4" /> Temizle
        </button>
      )}
    </div>
  )
}
