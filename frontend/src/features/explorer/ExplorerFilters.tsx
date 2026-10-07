import { Clock, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import type { ViewGraphStatus } from '@/types'
import { avatarForeground } from '@/shared/lib/avatar'
import { FilterToggle, useFilterDisclosure } from '@/shared/components'
import {
  countActiveExplorerFacets,
  sessionKindLabel,
  TIME_WINDOWS,
  toggleValue,
  type ExplorerFacets,
  type ExplorerFilter,
} from './explorerFilter'
import { ExplorerStatusStrip } from './ExplorerStatusStrip'

export interface ExplorerBucket {
  key: string
  label: string
  color: string
}

interface Props {
  filter: ExplorerFilter
  onChange: (next: ExplorerFilter) => void
  onClear: () => void
  buckets: ExplorerBucket[]
  facets: ExplorerFacets
  status: ViewGraphStatus | null | undefined
  liveCount: number
  visibleCount: number
  totalCount: number
  // Canvas packing (useStoredDensity); a rarely-touched knob, so it lives in
  // the folded panel with the other facets.
  density: number
  onDensity: (value: number) => void
}

const chipBase = 'flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition'
const chipOff = `${chipBase} border-[var(--color-border)] text-[var(--color-text-dim)] opacity-70 hover:opacity-100`
const chipOn = `${chipBase} border-transparent bg-[var(--color-accent)] text-[var(--color-on-accent)]`

// ExplorerFilters is the map's single toolbar under the header. The first line
// is what a user reaches for every visit: the time window (the Rota toolbar's
// cutoff — the map's main lens), the live counters (each one a filter, the
// running one the live-only toggle), the folded-facets toggle and the visible
// node count. The folded panel holds the rarely-touched rest: layer chips that
// hide whole groups, session kind / agent / tag facets, and the density knob.
export function ExplorerFilters({
  filter,
  onChange,
  onClear,
  buckets,
  facets,
  status,
  liveCount,
  visibleCount,
  totalCount,
  density,
  onDensity,
}: Props) {
  const { t } = useTranslation('explorer')
  const active = countActiveExplorerFacets(filter)
  const [open, toggle] = useFilterDisclosure('explorer', false)
  return (
    <div
      className="flex flex-col gap-2 border-b border-[var(--color-border)] py-2 text-xs max-md:px-3 md:px-6"
      role="group"
      aria-label={t('filters.label')}
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <span
          className="flex items-center gap-1"
          role="group"
          aria-label={t('filters.window')}
          title={t('filters.windowHint')}
        >
          <Clock size={12} className="text-[var(--color-text-dim)]" />
          {TIME_WINDOWS.map((value) => (
            <button
              key={value}
              type="button"
              onClick={() => onChange({ ...filter, window: value })}
              aria-pressed={filter.window === value}
              className={`rounded px-1.5 py-0.5 ${
                filter.window === value
                  ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                  : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
              }`}
            >
              {t(`filters.windowOptions.${value}`)}
            </button>
          ))}
        </span>
        <ExplorerStatusStrip
          status={status}
          liveCount={liveCount}
          filter={filter}
          onChange={onChange}
        />
        <FilterToggle
          open={open}
          onToggle={toggle}
          activeCount={active}
          testId="explorer-filters-toggle"
        />
        <span className="ml-auto text-[var(--color-text-dim)]">
          {t('filters.nodeCount', { visible: visibleCount, total: totalCount })}
        </span>
        {active > 0 && (
          <button
            onClick={onClear}
            className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-0.5 text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
          >
            <X size={11} />
            {t('filters.clear', { count: active })}
          </button>
        )}
      </div>

      {open && (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <span className="text-[var(--color-text-dim)]">{t('filters.layers')}:</span>
          {buckets.map((bucket) => {
            const on = !filter.hiddenBuckets.includes(bucket.key)
            return (
              <button
                key={bucket.key}
                onClick={() =>
                  onChange({
                    ...filter,
                    hiddenBuckets: toggleValue(filter.hiddenBuckets, bucket.key),
                  })
                }
                aria-pressed={on}
                className={`${chipBase} ${
                  on
                    ? 'border-transparent'
                    : 'border-[var(--color-border)] text-[var(--color-text-dim)] opacity-60'
                }`}
                style={
                  on
                    ? { background: bucket.color, color: avatarForeground(bucket.color) }
                    : undefined
                }
              >
                <span
                  className="inline-block h-2 w-2 rounded-full"
                  style={{ background: on ? 'var(--color-text)' : bucket.color }}
                />
                {bucket.label}
              </button>
            )
          })}

          {facets.kinds.length > 0 && (
            <>
              <span className="text-[var(--color-text-dim)]">{t('filters.kind')}:</span>
              {facets.kinds.map((kind) => (
                <button
                  key={kind}
                  onClick={() => onChange({ ...filter, kinds: toggleValue(filter.kinds, kind) })}
                  aria-pressed={filter.kinds.includes(kind)}
                  className={filter.kinds.includes(kind) ? chipOn : chipOff}
                >
                  {sessionKindLabel(kind)}
                </button>
              ))}
            </>
          )}

          {facets.agents.length > 0 && (
            <label className="flex items-center gap-1 text-[var(--color-text-dim)]">
              {t('filters.agent')}:
              <select
                value={filter.agentIds[0] ?? ''}
                onChange={(e) =>
                  onChange({ ...filter, agentIds: e.target.value ? [e.target.value] : [] })
                }
                className="rounded-md border border-[var(--color-border)] bg-transparent px-1 py-0.5 text-xs text-[var(--color-text)]"
              >
                <option value="">{t('filters.all')}</option>
                {facets.agents.map((agent) => (
                  <option key={agent.id} value={agent.id}>
                    {agent.label}
                  </option>
                ))}
              </select>
            </label>
          )}

          {facets.tags.length > 0 && (
            <>
              <span className="text-[var(--color-text-dim)]">{t('filters.tag')}:</span>
              {facets.tags.map((tag) => (
                <button
                  key={tag}
                  onClick={() => onChange({ ...filter, tags: toggleValue(filter.tags, tag) })}
                  aria-pressed={filter.tags.includes(tag)}
                  className={filter.tags.includes(tag) ? chipOn : chipOff}
                >
                  #{tag}
                </button>
              ))}
            </>
          )}

          <label
            className="flex items-center gap-2 text-[var(--color-text-dim)]"
            title={t('densityHint')}
          >
            {t('density')}
            <input
              type="range"
              min={0.4}
              max={2}
              step={0.1}
              value={density}
              onChange={(e) => onDensity(parseFloat(e.target.value))}
              className="w-20 accent-[var(--color-accent)]"
            />
          </label>
        </div>
      )}
    </div>
  )
}
