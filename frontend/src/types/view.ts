// Projection layer types — mirrors internal/view (see _Docs/66-VIEW-KATMANI.md).
//
// A view is the compact, deterministic summary of a large entity. The SAME bytes
// go to the agent and to this UI: the panel renders `text` verbatim instead of
// re-composing it from header/body, so a wrong or stale projection is visible to
// the user rather than hidden behind a prettier rendering.

export type ViewKind =
  | 'flowrun'
  | 'session'
  | 'board'
  | 'workspace'
  | 'schedule'
  | 'agent'
  | 'budget'
  | 'tools'
  | 'category'
  // Explorer map extension kinds (TSK66): artifact/automation/skill/insight are
  // category-member leaves, logs is an inline-tail leaf like budget/tools.
  | 'artifact'
  | 'automation'
  | 'skill'
  | 'insight'
  | 'logs'
  // Trajectory ("Rota"): reached from a root session's children (_Docs/77 R9).
  | 'trajectory'
  // Workspace memory note, a category-member leaf under category:notes (_Docs/94).
  | 'note'

// Budget tiers. tiny is one dense line (safe to push into a prompt suffix), card
// is the default, full adds per-item detail.
export type ViewLevel = 'tiny' | 'card' | 'full'

export interface ViewRef {
  kind: ViewKind
  id: string
  sub?: string
}

// A drill-down pointer: the projection stayed small, and this is how to open the
// part it left out.
export interface ViewHandle {
  label: string
  ref: ViewRef
  level?: ViewLevel
}

export interface ViewResult {
  ref: ViewRef
  level: ViewLevel
  header: string
  body: string
  // The rendered projection exactly as an agent receives it.
  text: string
  handles: ViewHandle[] | null
  asOf: string
  // The revision projected (status/updatedAt/trace length) — two views with the
  // same source describe the same state.
  source: string
  // How many items the projection deliberately hid. Always rendered.
  elided: number
  // What was hidden ("kart", "eski mesaj", "node"). A bare count is ambiguous —
  // 174 hidden messages and 174 hidden cards mean very different things.
  elidedUnit?: string
  // Approximate token cost (chars/4) of header+body.
  tokens: number
}

// ViewChildrenResult is the Explorer map's structural drill-down: the child
// handles a node expands into (GET /api/views/{kind}/{id}/children). Separate
// from a full projection — it lists what a node drills into without rendering a
// card for each child. The node's own summary (with its elision count) comes from
// getView.
export interface ViewChildrenResult {
  ref: ViewRef
  children: ViewHandle[]
}

export interface ViewNeighborhoodResult {
  focus: ViewHandle
  parents: ViewHandle[]
  children: ViewHandle[]
  hiddenParentCount: number
  hiddenChildCount: number
}

// ViewGraphEdge is one parent -> child structural relationship of the whole map.
interface ViewGraphEdge {
  source: ViewRef
  target: ViewRef
}

// ViewGraphResult is the entire structural map of the workspace
// (GET /api/views/graph): every node reachable from the root plus every edge,
// uncapped. The Explorer screen lays it out as one force-directed network.
// ViewGraphLive is one session executing right now: running, or a coordinator
// idle while one of its direct workers runs. The map glows these and hangs the
// driving agent's avatar off them; the entry (and the glow) is gone once the
// session stops.
interface ViewGraphLiveAgent {
  id: string
  name: string
  emoji?: string
  color?: string
}
export interface ViewGraphLive {
  session: ViewRef
  state: 'running' | 'awaiting-workers'
  agent: ViewGraphLiveAgent
}

// ViewGraphMeta is a session's facet data for the map's filters.
export interface ViewGraphMeta {
  kind?: string
  agentId?: string
  tags?: string[]
  archived?: boolean
}

// ViewGraphTimes is a node's recency facets (unix seconds; absent = unknown):
// created, last edited, last read by an agent and who read it. The map's time
// window keeps a node whose newest stamp falls inside it.
export interface ViewGraphTimes {
  created?: number
  updated?: number
  read?: number
  readBy?: string
}

// ViewGraphAttention marks a node a human should look at: the strongest level
// among its reasons (danger > warn > notice) and the reason codes themselves
// (waiting-ask, stuck, blocked, failed-card, stale-card, tool-errors). Derived
// on the backend from the awareness open-loop scan, so the map and the agent's
// pulse agree on what is stuck, waiting or failed.
export type ViewAttentionLevel = 'danger' | 'warn' | 'notice'
export interface ViewGraphAttention {
  level: ViewAttentionLevel
  reasons: string[]
  at?: number
}

// ViewGraphStatus is the status strip above the map: the counters a user reads
// before reading any node.
export interface ViewGraphStatus {
  at: number
  running: number
  waiting: number
  stuck: number
  failedCards: number
  failedRuns: number
  stale: number
  notesToday: number
  lastDigestAt?: number
}

export interface ViewGraphResult {
  nodes: ViewHandle[]
  edges: ViewGraphEdge[]
  live?: ViewGraphLive[]
  // Keyed by refToString of the session.
  meta?: Record<string, ViewGraphMeta>
  // Keyed by refToString of the node (sessions and board cards today).
  attention?: Record<string, ViewGraphAttention>
  // Keyed by refToString of the node (timed entities, plus anything an agent read).
  times?: Record<string, ViewGraphTimes>
  status?: ViewGraphStatus | null
}

// ViewGraphLiveResult is the map's volatile half alone (GET /api/views/graph/live):
// re-pulled on state events so the node/edge set, and with it the physics,
// stays put while rings, glows and the status strip update.
export interface ViewGraphLiveResult {
  live: ViewGraphLive[]
  meta: Record<string, ViewGraphMeta>
  attention: Record<string, ViewGraphAttention>
  times?: Record<string, ViewGraphTimes>
  status: ViewGraphStatus | null
}

// refToString spells a ref the way handles and the get_view tool do.
export function refToString(ref: ViewRef): string {
  return `${ref.kind}:${ref.id}${ref.sub ? `#${ref.sub}` : ''}`
}

const VIEW_KINDS: ViewKind[] = [
  'flowrun',
  'session',
  'board',
  'workspace',
  'schedule',
  'agent',
  'budget',
  'tools',
  'category',
  'artifact',
  'automation',
  'skill',
  'trajectory',
  'insight',
  'logs',
  'note',
]

// parseRef is refToString's inverse: "category:col:in_progress" → {kind, id},
// "board:board#T1" → {kind, id, sub}. Splits kind at the FIRST colon (a category
// id like "col:in_progress" keeps its own colon) and sub at the "#". Returns null
// for a string that does not name a known kind — used to restore a deep-linked map
// selection whose node has not been fetched yet.
export function parseRef(s: string): ViewRef | null {
  const colon = s.indexOf(':')
  if (colon < 0) return null
  const kind = s.slice(0, colon)
  if (!(VIEW_KINDS as string[]).includes(kind)) return null
  let rest = s.slice(colon + 1)
  let sub: string | undefined
  const hash = rest.indexOf('#')
  if (hash >= 0) {
    sub = rest.slice(hash + 1)
    rest = rest.slice(0, hash)
  }
  if (!rest) return null
  return { kind: kind as ViewKind, id: rest, ...(sub ? { sub } : {}) }
}
