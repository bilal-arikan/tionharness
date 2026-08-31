// Network filtering — pure functions over the workspace graph the panel already
// holds in state. Mirrors the board's client-side filter model (facets combine
// with AND; values within one facet combine with OR), adapted to graph nodes.
//
// The graph is small (agents = live instances, plus tasks/flows/skills/mcp and
// live session nodes), so filtering happens entirely client-side.

import type { WorkspaceGraph, WorkspaceGraphNode } from '@/types'

// A single facet set. Empty arrays / false mean "no constraint" for that facet.
export interface NetworkFilter {
  text: string
  agentIds: string[] // agent definition ids; '-' = no agent (unassigned)
  runKinds: string[] // chat|task|flow|schedule|worker|spawned|inbox|flow-coordinator
  statuses: string[] // task board states
  tags: string[]
  showArchived: boolean // archived run/agent nodes are hidden until this is on
}

export const emptyNetworkFilter = (): NetworkFilter => ({
  text: '',
  agentIds: [],
  runKinds: [],
  statuses: [],
  tags: [],
  showArchived: false,
})

// Short run-kind labels for the filter chips (the tooltip labels in
// relationGraph.ts are the long "Görev çalıştırması" form).
export const KIND_LABEL: Record<string, string> = {
  chat: 'Sohbet',
  task: 'Görev',
  flow: 'Akış',
  'flow-coordinator': 'Akış koordinatörü',
  schedule: 'Zamanlama',
  spawned: 'Spawn',
  worker: 'Worker',
  inbox: 'Inbox',
}

// foldForSearch normalises text for substring matching, folding Turkish dotted/
// dotless i onto plain 'i' so "LOGIN"/"istanbul" both match regardless of case.
// (Same behaviour as the board's filter; kept local to avoid a cross-feature dep.)
function foldForSearch(s: string): string {
  return s.replace(/[İI]/g, 'i').replace(/ı/g, 'i').toLowerCase().normalize('NFD').replace(/̇/g, '') // strip the combining dot above left by 'İ'
}

// Nodes that carry a runKind (and thus respond to the run-kind facet).
const KIND_BEARING = new Set(['agent', 'run'])
// Nodes that carry an owning agent (and thus respond to the agent facet).
const AGENT_BEARING = new Set(['agent', 'run', 'task'])
// Attachment nodes that only exist because a running agent uses them — prune them
// when the filter leaves them with no surviving edge (an orphan star/triangle).
const ATTACHMENT = new Set(['skill', 'mcp'])

// countActiveNetworkFacets counts how many facets are constraining the view, for
// the "N filtre" badge. showArchived counts because it changes the default view.
export function countActiveNetworkFacets(f: NetworkFilter): number {
  let n = 0
  if (f.text.trim()) n++
  if (f.agentIds.length) n++
  if (f.runKinds.length) n++
  if (f.statuses.length) n++
  if (f.tags.length) n++
  if (f.showArchived) n++
  return n
}

export function isNetworkFilterActive(f: NetworkFilter): boolean {
  return countActiveNetworkFacets(f) > 0
}

// keepNode applies the per-node facet predicates (AND across facets).
function keepNode(n: WorkspaceGraphNode, f: NetworkFilter, text: string): boolean {
  // Archived nodes stay hidden unless explicitly revealed.
  if (n.archived && !f.showArchived) return false

  if (text) {
    const hay = foldForSearch(
      [n.label, n.sub, n.desc, ...(n.tags ?? [])].filter(Boolean).join('\n'),
    )
    if (!hay.includes(text)) return false
  }
  if (f.runKinds.length && KIND_BEARING.has(n.type)) {
    if (!f.runKinds.includes(n.runKind ?? '')) return false
  }
  if (f.agentIds.length && AGENT_BEARING.has(n.type)) {
    if (!f.agentIds.includes(n.agentId || '-')) return false
  }
  if (f.statuses.length && n.type === 'task') {
    if (!f.statuses.includes(n.status ?? '')) return false
  }
  if (f.tags.length && AGENT_BEARING.has(n.type)) {
    const tags = n.tags ?? []
    if (!f.tags.some((want) => tags.includes(want))) return false
  }
  return true
}

// filterGraph returns a new WorkspaceGraph narrowed to the active facets. Edges
// to dropped nodes are removed, and skill/MCP attachment nodes left with no
// surviving edge are pruned so the canvas has no dangling icons. Stats are kept
// from the original graph (they describe the workspace totals, not the subset).
export function filterGraph(graph: WorkspaceGraph, f: NetworkFilter): WorkspaceGraph {
  if (!isNetworkFilterActive(f)) return graph
  const text = f.text.trim() ? foldForSearch(f.text.trim()) : ''

  const kept = graph.nodes.filter((n) => keepNode(n, f, text))
  let keptIds = new Set(kept.map((n) => n.id))
  let edges = graph.edges.filter((e) => keptIds.has(e.source) && keptIds.has(e.target))

  // Prune orphaned attachment nodes (skill/mcp with no incident edge).
  const incident = new Set<string>()
  for (const e of edges) {
    incident.add(e.source)
    incident.add(e.target)
  }
  const nodes = kept.filter((n) => !ATTACHMENT.has(n.type) || incident.has(n.id))
  keptIds = new Set(nodes.map((n) => n.id))
  edges = edges.filter((e) => keptIds.has(e.source) && keptIds.has(e.target))

  return { nodes, edges, stats: graph.stats }
}
