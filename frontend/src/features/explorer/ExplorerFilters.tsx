import { Radio, X } from 'lucide-react'
import { avatarForeground } from '@/shared/lib/avatar'
import {
  countActiveExplorerFacets,
  SESSION_KIND_LABEL,
  toggleValue,
  type ExplorerFacets,
  type ExplorerFilter,
} from './explorerFilter'

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
  liveCount: number
  visibleCount: number
  totalCount: number
}

const chipBase = 'flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs transition'
const chipOff = `${chipBase} border-[var(--color-border)] text-[var(--color-text-dim)] opacity-70 hover:opacity-100`
const chipOn = `${chipBase} border-transparent bg-[var(--color-accent)] text-[var(--color-on-accent)]`

// ExplorerFilters is the toolbar row under the map header: the Network
// screen's facets, re-homed. Layer chips hide whole groups; the live toggle
// keeps only executing sessions; kind / agent / tag chips narrow sessions.
export function ExplorerFilters({
  filter,
  onChange,
  onClear,
  buckets,
  facets,
  liveCount,
  visibleCount,
  totalCount,
}: Props) {
  const active = countActiveExplorerFacets(filter)
  return (
    <div
      className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-[var(--color-border)] py-2 text-xs max-md:px-3 md:px-6"
      role="group"
      aria-label="Harita filtreleri"
    >
      <span className="text-[var(--color-text-dim)]">katmanlar:</span>
      {buckets.map((bucket) => {
        const on = !filter.hiddenBuckets.includes(bucket.key)
        return (
          <button
            key={bucket.key}
            onClick={() =>
              onChange({ ...filter, hiddenBuckets: toggleValue(filter.hiddenBuckets, bucket.key) })
            }
            aria-pressed={on}
            className={`${chipBase} ${
              on
                ? 'border-transparent'
                : 'border-[var(--color-border)] text-[var(--color-text-dim)] opacity-60'
            }`}
            style={
              on ? { background: bucket.color, color: avatarForeground(bucket.color) } : undefined
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

      <span className="mx-1 h-4 w-px bg-[var(--color-border)]" />
      <button
        onClick={() => onChange({ ...filter, liveOnly: !filter.liveOnly })}
        aria-pressed={filter.liveOnly}
        className={filter.liveOnly ? chipOn : chipOff}
        title="Yalnız şu an çalışan oturumlar"
      >
        <Radio size={11} />
        Canlı{liveCount > 0 ? ` · ${liveCount}` : ''}
      </button>

      {facets.kinds.length > 0 && (
        <>
          <span className="text-[var(--color-text-dim)]">tür:</span>
          {facets.kinds.map((kind) => (
            <button
              key={kind}
              onClick={() => onChange({ ...filter, kinds: toggleValue(filter.kinds, kind) })}
              aria-pressed={filter.kinds.includes(kind)}
              className={filter.kinds.includes(kind) ? chipOn : chipOff}
            >
              {SESSION_KIND_LABEL[kind] ?? kind}
            </button>
          ))}
        </>
      )}

      {facets.agents.length > 0 && (
        <label className="flex items-center gap-1 text-[var(--color-text-dim)]">
          ajan:
          <select
            value={filter.agentIds[0] ?? ''}
            onChange={(e) =>
              onChange({ ...filter, agentIds: e.target.value ? [e.target.value] : [] })
            }
            className="rounded-md border border-[var(--color-border)] bg-transparent px-1 py-0.5 text-xs text-[var(--color-text)]"
          >
            <option value="">tümü</option>
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
          <span className="text-[var(--color-text-dim)]">etiket:</span>
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

      <span className="ml-auto text-[var(--color-text-dim)]">
        {visibleCount}/{totalCount} düğüm
      </span>
      {active > 0 && (
        <button
          onClick={onClear}
          className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-0.5 text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
        >
          <X size={11} />
          {active} filtre · temizle
        </button>
      )}
    </div>
  )
}
