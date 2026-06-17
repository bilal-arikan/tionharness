// Hash-based deep-link routing helpers. The app addresses its full navigation
// state through the URL hash so any workspace/view/entity is reachable by URL
// (shareable, restorable on reload, back/forward aware).
//
// Scheme: #/w/{workspaceId}/{view}[/{entityId}]
//   - workspaceId scopes the request to an isolated backend database.
//   - view is one of the NavRail views.
//   - entityId is meaningful per view: chat→sessionId, agents/memory/tools→agentId,
//     artifacts→artifactId, schedules→scheduleId. Other views ignore it.
import type { View } from '../components/NavRail'

const VIEWS: View[] = [
  'chat', 'agents', 'board', 'schedules', 'memory',
  'tools', 'flows', 'artifacts', 'logs', 'settings',
]

export interface Route {
  workspaceId: string | null
  view: View
  id: string | null
}

// parseRoute turns a location.hash string into a Route. Unknown views fall back
// to 'chat'; malformed input degrades gracefully to the default route.
export function parseRoute(hash: string): Route {
  const clean = hash.replace(/^#\/?/, '')
  const parts = clean.split('/').filter(Boolean).map(decodeURIComponent)

  let workspaceId: string | null = null
  let rest = parts
  if (parts[0] === 'w' && parts[1]) {
    workspaceId = parts[1]
    rest = parts.slice(2)
  }

  const view: View = rest[0] && (VIEWS as string[]).includes(rest[0]) ? (rest[0] as View) : 'chat'
  const id = rest[1] ?? null
  return { workspaceId, view, id }
}

// buildRoute serialises a Route back into a hash path (without the leading '#').
export function buildRoute(r: Route): string {
  const segs: string[] = []
  if (r.workspaceId) segs.push('w', encodeURIComponent(r.workspaceId))
  segs.push(r.view)
  if (r.id) segs.push(encodeURIComponent(r.id))
  return '/' + segs.join('/')
}

// routeIdForView returns the entity id that belongs in the URL for a given view,
// picking the right piece of app state. Views without an addressable entity
// return null.
export function routeIdForView(
  view: View,
  state: {
    sessionId: string | null
    agentId: string | null
    artifactId: string | null
    scheduleId: string | null
  },
): string | null {
  switch (view) {
    case 'chat':
      return state.sessionId
    case 'agents':
    case 'memory':
    case 'tools':
      return state.agentId
    case 'artifacts':
      return state.artifactId
    case 'schedules':
      return state.scheduleId
    default:
      return null
  }
}
