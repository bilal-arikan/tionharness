// Hash-based deep-link routing helpers. The app addresses its full navigation
// state through the URL hash so any workspace/view/entity is reachable by URL
// (shareable, restorable on reload, back/forward aware).
//
// Scheme: #/w/{workspaceId}/{view}[/{entityId}][?k=v&…]
//   - workspaceId scopes the request to an isolated backend database.
//   - view is one of the NavRail views.
//   - entityId is meaningful per view: chat→sessionId, agents→agentId,
//     artifacts→artifactId, schedules→scheduleId, settings→category key.
//     Other views ignore it.
//   - the query carries sub-state that cannot claim the single entity slot —
//     chat already spends it on the sessionId, so its list tabs ride here
//     (?list=workers&kind=task). Defaults are omitted so the common URL stays clean.
import type { View } from './NavRail'

const VIEWS: View[] = [
  'chat',
  'agents',
  'network',
  'explorer',
  'board',
  'schedules',
  'flows',
  'artifacts',
  'skills',
  'tools',
  'market',
  'budget',
  'logs',
  'insights',
  'workspace',
  'settings',
]

// Retired view slugs kept alive as redirects so bookmarked / notification URLs
// still land somewhere sensible. 'executions' was the standalone Activity screen,
// folded into the unified chat transcript view; its entity id was already a
// sessionId, so the mapping is a pure rename.
const LEGACY_VIEWS: Record<string, View> = {
  executions: 'chat',
}

// resolveView maps a raw hash segment to a live View, honouring legacy slugs.
// Unknown segments fall back to 'chat'.
function resolveView(seg: string | undefined): View {
  if (!seg) return 'chat'
  if ((VIEWS as string[]).includes(seg)) return seg as View
  return LEGACY_VIEWS[seg] ?? 'chat'
}

export interface Route {
  workspaceId: string | null
  view: View
  id: string | null
  /** Per-view sub-state from the hash query string. Absent = "no sub-state given". */
  query?: Record<string, string>
}

// parseRoute turns a location.hash string into a Route. Unknown views fall back
// to 'chat'; malformed input degrades gracefully to the default route.
export function parseRoute(hash: string): Route {
  const raw = hash.replace(/^#\/?/, '')
  const q = raw.indexOf('?')
  const clean = q < 0 ? raw : raw.slice(0, q)
  const query: Record<string, string> = {}
  if (q >= 0) {
    for (const [k, v] of new URLSearchParams(raw.slice(q + 1))) query[k] = v
  }
  const parts = clean.split('/').filter(Boolean).map(decodeURIComponent)

  let workspaceId: string | null = null
  let rest = parts
  if (parts[0] === 'w' && parts[1]) {
    workspaceId = parts[1]
    rest = parts.slice(2)
  }

  const view = resolveView(rest[0])
  const id = rest[1] ?? null
  return { workspaceId, view, id, query }
}

// isView reports whether a string is a known NavRail view. Legacy slugs are NOT
// views — routeFromEvent maps them separately so a stale backend event target
// (target.view === 'executions') still routes instead of being dropped.
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
  if (!t?.view) return null
  // A legacy target ('executions', emitted by older builds and by events already
  // persisted in a log) still resolves — to the chat transcript it now lives in.
  if (!isView(t.view) && !(t.view in LEGACY_VIEWS)) return null
  const view = resolveView(t.view)
  let id: string | null = null
  if (view === 'chat') id = t.sessionId ?? null
  else if (view === 'agents') id = t.agentId ?? null
  return { workspaceId: e.workspaceId ?? null, view, id }
}

// buildRoute serialises a Route back into a hash path (without the leading '#').
// Query keys are sorted and empty values dropped so the same state always yields
// byte-identical output — useUrlSync compares the built string to the live hash.
export function buildRoute(r: Route): string {
  const segs: string[] = []
  if (r.workspaceId) segs.push('w', encodeURIComponent(r.workspaceId))
  segs.push(r.view)
  if (r.id) segs.push(encodeURIComponent(r.id))
  const path = '/' + segs.join('/')

  const pairs = Object.entries(r.query ?? {})
    .filter(([, v]) => v !== '' && v != null)
    .sort(([a], [b]) => a.localeCompare(b))
  if (pairs.length === 0) return path
  const qs = pairs.map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(v)}`).join('&')
  return `${path}?${qs}`
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
    insightTab: string | null
    flowsTab: string | null
    explorerNode: string | null
  },
): string | null {
  switch (view) {
    case 'chat':
      return state.sessionId
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
    case 'insights':
      return state.insightTab
    case 'explorer':
      // The selected map node's ref string (e.g. "category:sessions"). The root
      // carries no segment so a plain #/…/explorer stays clean.
      return state.explorerNode
    case 'flows':
      // Default "flows" tab carries no URL segment (clean #/w/{ws}/flows); only
      // the runs/templates tabs add /{tab}.
      return state.flowsTab && state.flowsTab !== 'flows' ? state.flowsTab : null
    default:
      return null
  }
}

// routeQueryForView returns the hash-query sub-state that belongs in the URL for
// a given view. Only chat has any today: its entity slot is taken by the
// sessionId, so the sidebar's list/kind tabs ride in the query instead. Default
// tab values are omitted, keeping the everyday URL identical to before.
export function routeQueryForView(
  view: View,
  state: { sessionListTab: string; sessionKindTab: string },
): Record<string, string> {
  if (view !== 'chat') return {}
  const q: Record<string, string> = {}
  if (state.sessionListTab && state.sessionListTab !== 'active') q.list = state.sessionListTab
  if (state.sessionKindTab) q.kind = state.sessionKindTab
  return q
}
