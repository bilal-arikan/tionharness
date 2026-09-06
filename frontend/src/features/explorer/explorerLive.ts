import type { ViewGraphLive, ViewGraphResult, ViewRef } from '@/types'
import { refToString } from '@/types'

// The map's live layer. GET /api/views/graph names the sessions executing right
// now and the agent driving each; this module turns that into synthetic nodes
// the canvas can draw: one avatar node per live session, hanging off the
// session. Because the nodes are derived from the payload, they appear when a
// session starts and vanish — together with the session's glow — on the refresh
// after it stops. Pure, no DOM.

const LIVE_SUB_PREFIX = 'live:'

// liveAgentRef is the ref of the avatar node for a live session: the agent's
// id with a `live:<session>` sub, so the same agent driving two sessions gets
// two nodes and clicking either still projects the agent card.
export function liveAgentRef(live: ViewGraphLive): ViewRef {
  return { kind: 'agent', id: live.agent.id, sub: LIVE_SUB_PREFIX + live.session.id }
}

export function isLiveAgentRef(ref: ViewRef): boolean {
  return ref.kind === 'agent' && !!ref.sub && ref.sub.startsWith(LIVE_SUB_PREFIX)
}

// panelRefFor is what the side panel projects for a selected node: a live
// avatar node projects its agent (the backend knows no `live:` sub).
export function panelRefFor(ref: ViewRef): ViewRef {
  return isLiveAgentRef(ref) ? { kind: ref.kind, id: ref.id } : ref
}

export interface LiveLayer {
  graph: ViewGraphResult
  // session key -> live state
  liveState: Map<string, ViewGraphLive['state']>
  // live avatar node key -> its live entry
  liveAgents: Map<string, ViewGraphLive>
}

// augmentLive adds the avatar nodes and their session -> avatar edges. Nodes
// and edges of the input are kept as they are; live entries whose session is
// not on the map (filtered out upstream) are ignored.
export function augmentLive(graph: ViewGraphResult): LiveLayer {
  const present = new Set(graph.nodes.map((h) => refToString(h.ref)))
  const liveState = new Map<string, ViewGraphLive['state']>()
  const liveAgents = new Map<string, ViewGraphLive>()
  const nodes = [...graph.nodes]
  const edges = [...graph.edges]
  for (const live of graph.live ?? []) {
    const sessionKey = refToString(live.session)
    if (!present.has(sessionKey)) continue
    liveState.set(sessionKey, live.state)
    const ref = liveAgentRef(live)
    const key = refToString(ref)
    if (liveAgents.has(key)) continue
    liveAgents.set(key, live)
    nodes.push({ label: live.agent.name || live.agent.id, ref })
    edges.push({ source: live.session, target: ref })
  }
  return { graph: { ...graph, nodes, edges }, liveState, liveAgents }
}
