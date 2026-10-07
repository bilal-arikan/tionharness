import type { ViewGraphResult, ViewGraphTimes } from '@/types'
import { refToString } from '@/types'
import { i18next } from '@/i18n'
import { attentionMatches, bearsAttention, isAttentionFacet } from './explorerAttention'
import { isLiveAgentRef } from './explorerLive'

// Explorer filtering — the Network screen's facets carried over to the map:
// group layers (hide a whole bucket subtree), live-only, session kind, owning
// agent and tags, plus the time window (the Rota screen's idle cutoff carried
// over). Facets combine with AND, values inside one facet with OR.
// Pure: takes the (live-augmented) graph, returns a smaller graph. Whatever the
// filter cuts loose from the root is dropped too, so no orphan floats around.

export interface ExplorerFilter {
  // Ref strings of the root's children (e.g. 'category:sessions') to hide.
  hiddenBuckets: string[]
  liveOnly: boolean
  kinds: string[] // session kinds: chat | task | worker | …
  agentIds: string[]
  tags: string[]
  // Attention facets from the status strip (explorerAttention.ATTENTION_FACETS):
  // keep only the sessions / cards that need a look for one of these reasons.
  attention: string[]
  // Time window in seconds (0 = all time): keep the nodes created, edited or
  // read by an agent within it — whichever is newest — plus their path to the root.
  window: TimeWindow
}

export type TimeWindow = 0 | 3600 | 21600 | 86400 | 259200
export const TIME_WINDOWS: readonly TimeWindow[] = [3600, 21600, 86400, 259200, 0]
// A browser that never picked a window opens on the last day.
export const DEFAULT_TIME_WINDOW: TimeWindow = 86400

// touchedAt is a node's newest stamp: created, edited or read by an agent.
export function touchedAt(times: ViewGraphTimes | undefined): number {
  if (!times) return 0
  return Math.max(times.created ?? 0, times.updated ?? 0, times.read ?? 0)
}

export const EXPLORER_FILTER_KEY = 'tionharness.explorerFilter'

// Short session-kind labels for the kind chips.
const SESSION_KIND_KEYS: Record<string, string> = {
  chat: 'chat',
  task: 'task',
  schedule: 'schedule',
  spawned: 'spawned',
  worker: 'worker',
  inbox: 'inbox',
}

export function sessionKindLabel(kind: string): string {
  const key = SESSION_KIND_KEYS[kind]
  return key ? i18next.t(`sessionKind.${key}`, { ns: 'explorer' }) : kind
}

export const emptyExplorerFilter = (): ExplorerFilter => ({
  hiddenBuckets: [],
  liveOnly: false,
  kinds: [],
  agentIds: [],
  tags: [],
  attention: [],
  window: 0,
})

// defaultExplorerFilter is what a browser with nothing stored opens on: no
// facets, the last day's window.
export const defaultExplorerFilter = (): ExplorerFilter => ({
  ...emptyExplorerFilter(),
  window: DEFAULT_TIME_WINDOW,
})

function stringList(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((v): v is string => typeof v === 'string') : []
}

export function parseExplorerFilter(raw: string | null | undefined): ExplorerFilter {
  if (!raw) return defaultExplorerFilter()
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch {
    return defaultExplorerFilter()
  }
  if (!parsed || typeof parsed !== 'object') return defaultExplorerFilter()
  const obj = parsed as Record<string, unknown>
  return {
    hiddenBuckets: stringList(obj.hiddenBuckets),
    liveOnly: obj.liveOnly === true,
    kinds: stringList(obj.kinds),
    agentIds: stringList(obj.agentIds),
    tags: stringList(obj.tags),
    attention: stringList(obj.attention).filter(isAttentionFacet),
    window: (TIME_WINDOWS as readonly unknown[]).includes(obj.window)
      ? (obj.window as TimeWindow)
      : DEFAULT_TIME_WINDOW,
  }
}

export function serializeExplorerFilter(filter: ExplorerFilter): string {
  return JSON.stringify(filter)
}

// countActiveExplorerFacets counts what narrows the map beyond the time window:
// the window is always set and always on screen, so it is not a "filter" the
// clear button would have to undo.
export function countActiveExplorerFacets(f: ExplorerFilter): number {
  return (
    (f.hiddenBuckets.length > 0 ? 1 : 0) +
    (f.liveOnly ? 1 : 0) +
    (f.kinds.length > 0 ? 1 : 0) +
    (f.agentIds.length > 0 ? 1 : 0) +
    (f.tags.length > 0 ? 1 : 0) +
    (f.attention.length > 0 ? 1 : 0)
  )
}

export function toggleValue(list: string[], value: string): string[] {
  return list.includes(value) ? list.filter((v) => v !== value) : [...list, value]
}

// applyExplorerFilter drops the nodes the facets exclude, then keeps only what
// the root still reaches. Sessions are the facet-bearing kind (cards too, for
// the attention facets); every other node is only ever removed as a hidden
// bucket or as an orphan.
export function applyExplorerFilter(
  graph: ViewGraphResult,
  filter: ExplorerFilter,
  liveState: ReadonlyMap<string, unknown>,
  rootKey: string,
  // Nodes folded from the side panel: kept themselves, but nothing is reached
  // through them, so a subtree with no other way in disappears.
  collapsed: ReadonlySet<string> = new Set(),
  nowMs: number = Date.now(),
): ViewGraphResult {
  const hidden = new Set(filter.hiddenBuckets)
  // A payload without stamps (an older backend) is not cut: an empty map would
  // read as a failed load.
  const inWindow =
    filter.window > 0 && graph.times
      ? recentWithAncestors(graph, filter.window, liveState, nowMs)
      : null
  const meta = graph.meta ?? {}
  const attention = graph.attention ?? {}
  const keep = (key: string, ref: ViewGraphResult['nodes'][number]['ref']): boolean => {
    if (hidden.has(key)) return false
    if (inWindow && key !== rootKey && !inWindow.has(key)) return false
    if (
      filter.attention.length > 0 &&
      bearsAttention(ref) &&
      !attentionMatches(attention[key], filter.attention)
    ) {
      return false
    }
    if (ref.kind !== 'session') return true
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
    if (keep(key, handle.ref)) allowed.add(key)
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

// recentWithAncestors is the time window's keep-set: every node touched inside
// the window (live sessions and their avatars count as touched now), plus every
// node above one, so a recent card keeps its column and the board, a recent
// session its kind group — and nothing else.
function recentWithAncestors(
  graph: ViewGraphResult,
  windowSecs: number,
  liveState: ReadonlyMap<string, unknown>,
  nowMs: number,
): Set<string> {
  const cutoff = Math.floor(nowMs / 1000) - windowSecs
  const times = graph.times ?? {}
  const parents = new Map<string, string[]>()
  for (const edge of graph.edges) {
    const target = refToString(edge.target)
    const list = parents.get(target) ?? []
    list.push(refToString(edge.source))
    parents.set(target, list)
  }
  const keep = new Set<string>()
  const queue: string[] = []
  for (const handle of graph.nodes) {
    const key = refToString(handle.ref)
    if (liveState.has(key) || isLiveAgentRef(handle.ref) || touchedAt(times[key]) >= cutoff) {
      keep.add(key)
      queue.push(key)
    }
  }
  while (queue.length > 0) {
    const current = queue.pop()!
    for (const parent of parents.get(current) ?? []) {
      if (keep.has(parent)) continue
      keep.add(parent)
      queue.push(parent)
    }
  }
  return keep
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
