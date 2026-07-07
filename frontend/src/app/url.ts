// Hash-based deep-link routing helpers. The app addresses its full navigation
// state through the URL hash so any workspace/view/entity is reachable by URL
// (shareable, restorable on reload, back/forward aware).
//
// Scheme: #/w/{workspaceId}/{view}[/{entityId}]
//   - workspaceId scopes the request to an isolated backend database.
//   - view is one of the NavRail views.
//   - entityId is meaningful per view: chat→sessionId, executions→sessionId,
//     agents→agentId, artifacts→artifactId, schedules→scheduleId,
//     settings→category key. Other views ignore it.
import type { View } from './NavRail'

const VIEWS: View[] = [
  'chat', 'executions', 'agents', 'network', 'board', 'schedules',
  'flows', 'artifacts', 'skills', 'tools', 'market', 'budget',
  'logs', 'workspace', 'settings',
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

// isView reports whether a string is a known NavRail view.
export function isView(v: string | null | undefined): v is View {
  return !!v && (VIEWS as string[]).includes(v)
}

// routeFromEvent derives a deep-link Route from an autonomous AppEvent's target
// hints (view + sessionId/agentId/...). Returns null when the event carries no
// usable view, so callers can no-op. Used to navigate on notification click:
// the click sets the URL hash to this route and the URL→state machinery does the
// rest (including a workspace switch).
export function routeFromEvent(e: {
  workspaceId?: string
  target?: Record<string, string>
}): Route | null {
  const t = e.target
  if (!t || !isView(t.view)) return null
  const view = t.view
  let id: string | null = null
  if (view === 'chat' || view === 'executions') id = t.sessionId ?? null
  else if (view === 'agents') id = t.agentId ?? null
  return { workspaceId: e.workspaceId ?? null, view, id }
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
    settingsCat: string | null
    workspaceTab: string | null
    executionId: string | null
  },
): string | null {
  switch (view) {
    case 'chat':
      return state.sessionId
    case 'executions':
      return state.executionId
    case 'agents':
      return state.agentId
    case 'artifacts':
      return state.artifactId
    case 'schedules':
      return state.scheduleId
    case 'settings':
      return state.settingsCat
    case 'workspace':
      return state.workspaceTab
    default:
      return null
  }
}
