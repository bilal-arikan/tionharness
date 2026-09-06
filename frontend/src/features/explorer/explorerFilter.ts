import type { ViewGraphResult } from '@/types'
import { refToString } from '@/types'

// Explorer filtering — the Network screen's facets carried over to the map:
// group layers (hide a whole bucket subtree), live-only, session kind, owning
// agent and tags. Facets combine with AND, values inside one facet with OR.
// Pure: takes the (live-augmented) graph, returns a smaller graph. Whatever the
// filter cuts loose from the root is dropped too, so no orphan floats around.

export interface ExplorerFilter {
  // Ref strings of the root's children (e.g. 'category:sessions') to hide.
  hiddenBuckets: string[]
  liveOnly: boolean
  kinds: string[] // session kinds: chat | task | flow | worker | …
  agentIds: string[]
  tags: string[]
}

export const EXPLORER_FILTER_KEY = 'tionharness.explorerFilter'

// Short session-kind labels for the kind chips.
export const SESSION_KIND_LABEL: Record<string, string> = {
  chat: 'Sohbet',
  task: 'Görev',
  flow: 'Akış',
  'flow-coordinator': 'Akış koordinatörü',
  schedule: 'Zamanlama',
  spawned: 'Spawn',
  worker: 'Worker',
  inbox: 'Inbox',
}

export const emptyExplorerFilter = (): ExplorerFilter => ({
  hiddenBuckets: [],
  liveOnly: false,
  kinds: [],
  agentIds: [],
  tags: [],
})

function stringList(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((v): v is string => typeof v === 'string') : []
}

export function parseExplorerFilter(raw: string | null | undefined): ExplorerFilter {
  if (!raw) return emptyExplorerFilter()
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return emptyExplorerFilter()
  }
  if (!parsed || typeof parsed !== 'object') return emptyExplorerFilter()
  const obj = parsed as Record<string, unknown>
  return {
    hiddenBuckets: stringList(obj.hiddenBuckets),
    liveOnly: obj.liveOnly === true,
    kinds: stringList(obj.kinds),
    agentIds: stringList(obj.agentIds),
    tags: stringList(obj.tags),
  }
}

export function serializeExplorerFilter(filter: ExplorerFilter): string {
  return JSON.stringify(filter)
}

export function countActiveExplorerFacets(f: ExplorerFilter): number {
  return (
    (f.hiddenBuckets.length > 0 ? 1 : 0) +
    (f.liveOnly ? 1 : 0) +
    (f.kinds.length > 0 ? 1 : 0) +
    (f.agentIds.length > 0 ? 1 : 0) +
    (f.tags.length > 0 ? 1 : 0)
  )
}

export function toggleValue(list: string[], value: string): string[] {
  return list.includes(value) ? list.filter((v) => v !== value) : [...list, value]
}

// applyExplorerFilter drops the nodes the facets exclude, then keeps only what
// the root still reaches. Sessions are the facet-bearing kind; every other node
// is only ever removed as a hidden bucket or as an orphan.
export function applyExplorerFilter(
  graph: ViewGraphResult,
  filter: ExplorerFilter,
  liveState: ReadonlyMap<string, unknown>,
  rootKey: string,
  // Nodes folded from the side panel: kept themselves, but nothing is reached
  // through them, so a subtree with no other way in disappears.
  collapsed: ReadonlySet<string> = new Set(),
): ViewGraphResult {
  const hidden = new Set(filter.hiddenBuckets)
  const meta = graph.meta ?? {}
  const keep = (key: string, kind: string): boolean => {
    if (hidden.has(key)) return false
    if (kind !== 'session') return true
    const m = meta[key] ?? {}
    if (filter.liveOnly && !liveState.has(key)) return false
    if (filter.kinds.length > 0 && !filter.kinds.includes(m.kind ?? '')) return false
    if (filter.agentIds.length > 0 && !filter.agentIds.includes(m.agentId ?? '')) return false
    if (filter.tags.length > 0 && !(m.tags ?? []).some((t) => filter.tags.includes(t))) {
      return false
    }
    return true
  }

  const allowed = new Set<string>()
  for (const handle of graph.nodes) {
    const key = refToString(handle.ref)
    if (keep(key, handle.ref.kind)) allowed.add(key)
  }
  const children = new Map<string, string[]>()
  for (const edge of graph.edges) {
    const source = refToString(edge.source)
    const target = refToString(edge.target)
    if (!allowed.has(source) || !allowed.has(target)) continue
    const list = children.get(source) ?? []
    list.push(target)
    children.set(source, list)
  }
  const reachable = new Set<string>()
  const queue = allowed.has(rootKey) ? [rootKey] : []
  while (queue.length > 0) {
    const current = queue.shift()!
    if (reachable.has(current)) continue
    reachable.add(current)
    if (collapsed.has(current)) continue
    for (const next of children.get(current) ?? []) if (!reachable.has(next)) queue.push(next)
  }

  return {
    ...graph,
    nodes: graph.nodes.filter((h) => reachable.has(refToString(h.ref))),
    edges: graph.edges.filter(
      (e) => reachable.has(refToString(e.source)) && reachable.has(refToString(e.target)),
    ),
  }
}

// childCounts is how many distinct children each node has (self-loops do not
// count): what the fold toggle reports and the canvas badges on a folded node.
export function childCounts(graph: ViewGraphResult): Map<string, number> {
  const seen = new Map<string, Set<string>>()
  for (const edge of graph.edges) {
    const source = refToString(edge.source)
    const target = refToString(edge.target)
    if (source === target) continue
    const set = seen.get(source) ?? new Set<string>()
    set.add(target)
    seen.set(source, set)
  }
  return new Map([...seen].map(([key, set]) => [key, set.size]))
}

// Facet vocabularies the filter bar offers, read off the unfiltered graph so a
// chip always corresponds to something on the map.
export interface ExplorerFacets {
  kinds: string[]
  agents: { id: string; label: string }[]
  tags: string[]
}

export function explorerFacets(graph: ViewGraphResult): ExplorerFacets {
  const kinds = new Set<string>()
  const tags = new Set<string>()
  const agentIds = new Set<string>()
  for (const m of Object.values(graph.meta ?? {})) {
    if (m.kind) kinds.add(m.kind)
    for (const t of m.tags ?? []) tags.add(t)
    if (m.agentId) agentIds.add(m.agentId)
  }
  const agents = graph.nodes
    .filter((h) => h.ref.kind === 'agent' && !h.ref.sub && agentIds.has(h.ref.id))
    .map((h) => ({ id: h.ref.id, label: h.label.replace(/^agent:\S+\s*/, '') || h.ref.id }))
    .sort((a, b) => a.label.localeCompare(b.label))
  return {
    kinds: [...kinds].sort(),
    agents,
    tags: [...tags].sort(),
  }
}
