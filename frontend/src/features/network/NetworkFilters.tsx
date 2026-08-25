// NetworkFilters — the board-style facet row for the Network screen. Owns the
// search box plus Ajan / Tür / Durum / Etiket dropdowns and the "arşivi göster"
// toggle. Everything mutates the panel's live filter; nothing is persisted.
//
// Facet options + counts are derived from the UNFILTERED graph (a count that
// shrank because of another facet would make the menu look broken), mirroring the
// board filter bar.

import { useEffect, useMemo, useRef, type ReactNode } from 'react'
import { Search, X, Archive } from 'lucide-react'
import type { BoardColumnDef, WorkspaceGraph } from '@/types'
import { FacetDropdown, type FacetOption } from '@/features/tasks/views/FacetDropdown'
import {
  KIND_LABEL,
  countActiveNetworkFacets,
  isNetworkFilterActive,
  type NetworkFilter,
} from './networkFilter'
import { compareText } from '@/shared/lib/intl'

interface Props {
  filter: NetworkFilter
  onChange: (next: NetworkFilter) => void
  onClear: () => void
  /** Unfiltered graph — facet options + counts come from here. */
  graph: WorkspaceGraph
  /** Nodes surviving the current filter, for the "N / M" counter. */
  visibleCount: number
  boardColumns: BoardColumnDef[]
  /** Extra controls (node-type layer chips + density) rendered inline in the same
      toolbar row, after a divider and before the far-right counter. */
  children?: ReactNode
}

// A node type carries an owning agent (agent instance / run / task).
const AGENT_BEARING = new Set(['agent', 'run', 'task'])
const KIND_BEARING = new Set(['agent', 'run'])

const STATUS_LABEL_FALLBACK: Record<string, string> = {
  todo: 'Yapılacak',
  in_progress: 'Sürüyor',
  review: 'İncelemede',
  done: 'Tamamlandı',
  failed: 'Başarısız',
}

export function NetworkFilters({
  filter: f,
  onChange,
  onClear,
  graph,
  visibleCount,
  boardColumns,
  children,
}: Props) {
  const searchRef = useRef<HTMLInputElement>(null)
  const patch = (part: Partial<NetworkFilter>) => onChange({ ...f, ...part })

  // "/" focuses search — ignored while the user is already typing elsewhere.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey) return
      const el = document.activeElement
      if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) return
      if (el instanceof HTMLElement && el.isContentEditable) return
      e.preventDefault()
      searchRef.current?.focus()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  // Facet options, derived once per graph.
  const { agentOptions, kindOptions, statusOptions, tagOptions } = useMemo(() => {
    const agentMeta = new Map<string, { name: string; color?: string }>()
    const agentCount = new Map<string, number>()
    const kindCount = new Map<string, number>()
    const statusCount = new Map<string, number>()
    const tagCount = new Map<string, number>()
    let unassigned = 0

    for (const n of graph.nodes) {
      if (KIND_BEARING.has(n.type) && n.runKind) {
        kindCount.set(n.runKind, (kindCount.get(n.runKind) ?? 0) + 1)
      }
      if (AGENT_BEARING.has(n.type)) {
        const id = n.agentId || ''
        if (id) {
          agentCount.set(id, (agentCount.get(id) ?? 0) + 1)
          // Prefer an agent-instance node for the display name/color; a run node
          // only carries the agent name in `sub`.
          if (n.type === 'agent') agentMeta.set(id, { name: n.label, color: n.color })
          else if (!agentMeta.has(id)) agentMeta.set(id, { name: n.sub || id })
        } else {
          unassigned++
        }
        for (const t of n.tags ?? []) tagCount.set(t, (tagCount.get(t) ?? 0) + 1)
      }
      if (n.type === 'task' && n.status) {
        statusCount.set(n.status, (statusCount.get(n.status) ?? 0) + 1)
      }
    }

    const agentOptions: FacetOption[] = [...agentMeta.entries()]
      .map(([id, meta]) => ({
        value: id,
        label: meta.name,
        color: meta.color,
        count: agentCount.get(id),
      }))
      .sort((a, b) => compareText(a.label, b.label))
    if (unassigned > 0) agentOptions.push({ value: '-', label: 'Atanmamış', count: unassigned })

    const kindOptions: FacetOption[] = [...kindCount.keys()]
      .sort((a, b) => compareText(KIND_LABEL[a] ?? a, KIND_LABEL[b] ?? b))
      .map((k) => ({ value: k, label: KIND_LABEL[k] ?? k, count: kindCount.get(k) }))

    // Status options follow the workspace's Kanban columns (for label + color +
    // order); any leftover status not in a column is appended.
    const statusOptions: FacetOption[] = []
    const seen = new Set<string>()
    for (const c of boardColumns) {
      const count = statusCount.get(c.key) ?? 0
      if (count === 0) continue
      seen.add(c.key)
      statusOptions.push({ value: c.key, label: c.label, color: c.color || undefined, count })
    }
    for (const [st, count] of statusCount) {
      if (seen.has(st)) continue
      statusOptions.push({ value: st, label: STATUS_LABEL_FALLBACK[st] ?? st, count })
    }

    const tagOptions: FacetOption[] = [...tagCount.keys()]
      .sort((a, b) => compareText(a, b))
      .map((t) => ({ value: t, label: `#${t}`, count: tagCount.get(t) }))

    return { agentOptions, kindOptions, statusOptions, tagOptions }
  }, [graph, boardColumns])

  const activeFacets = countActiveNetworkFacets(f)
  const filtering = isNetworkFilterActive(f)
  const total = graph.nodes.length

  return (
    <div className="flex flex-wrap items-center gap-1.5 border-b border-[var(--color-border)] px-4 py-2 text-xs">
      {/* Search */}
      <div className="relative">
        <Search
          size={12}
          className="pointer-events-none absolute left-2 top-1/2 -translate-y-1/2 text-[var(--color-text-dim)]"
        />
        <input
          ref={searchRef}
          value={f.text}
          onChange={(e) => patch({ text: e.target.value })}
          onKeyDown={(e) => {
            if (e.key === 'Escape') {
              patch({ text: '' })
              e.currentTarget.blur()
            }
          }}
          placeholder="ara…  /"
          className="w-36 rounded border border-[var(--color-border)] bg-[var(--color-bg)] py-1 pl-6 pr-2 text-xs outline-none transition focus:border-[var(--color-accent)]"
        />
      </div>

      <FacetDropdown
        label="Ajan"
        options={agentOptions}
        selected={f.agentIds}
        onChange={(v) => patch({ agentIds: v })}
        emptyHint="Çalışan ajan yok"
      />
      <FacetDropdown
        label="Tür"
        options={kindOptions}
        selected={f.runKinds}
        onChange={(v) => patch({ runKinds: v })}
      />
      <FacetDropdown
        label="Durum"
        options={statusOptions}
        selected={f.statuses}
        onChange={(v) => patch({ statuses: v })}
        emptyHint="Görev yok"
      />
      <FacetDropdown
        label="Etiket"
        options={tagOptions}
        selected={f.tags}
        onChange={(v) => patch({ tags: v })}
        emptyHint="Etiket yok"
      />

      {/* Archived toggle — off by default so finished archived work stays out of
          the way; turning it on reveals archived runs/instances (flagged nodes). */}
      <button
        onClick={() => patch({ showArchived: !f.showArchived })}
        title="Arşivlenmiş oturumları göster"
        className={`flex items-center gap-1 whitespace-nowrap rounded border px-2 py-1 transition ${
          f.showArchived
            ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
            : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]'
        }`}
      >
        <Archive size={12} /> Arşiv
      </button>

      {/* Layer chips + density live here (passed as children) so filters and
          node-type toggles share one toolbar row. */}
      {children && (
        <>
          <span className="mx-0.5 h-4 w-px bg-[var(--color-border)]" />
          {children}
        </>
      )}

      <div className="ml-auto flex items-center gap-1.5">
        <span className={filtering ? 'text-[var(--color-accent)]' : 'text-[var(--color-text-dim)]'}>
          {filtering ? `${visibleCount} / ${total} düğüm` : `${total} düğüm`}
        </span>
        {filtering && (
          <button
            onClick={onClear}
            title="Tüm filtreleri temizle"
            className="flex items-center gap-1 rounded border border-[var(--color-accent)] bg-[var(--color-accent-soft)] px-1.5 py-1 text-[var(--color-accent)]"
          >
            {activeFacets} filtre <X size={11} />
          </button>
        )}
      </div>
    </div>
  )
}
