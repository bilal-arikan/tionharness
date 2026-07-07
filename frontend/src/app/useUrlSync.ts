// useUrlSync keeps the URL hash and the app's navigation state in two-way sync.
//
//  state → URL: whenever the route changes, the canonical hash is written. The
//    first write uses replaceState (no spurious history entry on load); later
//    writes use pushState so the browser back/forward buttons walk the history.
//    Neither pushState nor replaceState fires hashchange/popstate, so there is
//    no write→read feedback loop.
//
//  URL → state: popstate (back/forward) and hashchange (manual address-bar edit
//    or an in-app link) re-parse the hash and hand the Route to onRoute, which
//    only mutates state when it actually differs — settling the loop.
import { useEffect, useRef } from 'react'
import { buildRoute, parseRoute, type Route } from './url'

export function useUrlSync(route: Route, ready: boolean, onRoute: (r: Route) => void) {
  const onRouteRef = useRef(onRoute)
  onRouteRef.current = onRoute
  const firstWrite = useRef(true)

  // state → URL
  useEffect(() => {
    if (!ready) return
    // The first sync after the app is ready only *canonicalises* the URL the user
    // loaded (e.g. fills in the default session) — it must not push a history
    // entry. Flip the flag on that first ready run regardless of whether a write
    // was needed, so a later genuine navigation never replaceState-clobbers a
    // real history entry the browser already created.
    const isFirst = firstWrite.current
    firstWrite.current = false
    const target = '#' + buildRoute(route)
    if (window.location.hash === target) return
    if (isFirst) {
      window.history.replaceState(null, '', target)
    } else {
      window.history.pushState(null, '', target)
    }
  }, [ready, route.workspaceId, route.view, route.id])

  // URL → state (mounted once; reads the latest onRoute via the ref)
  useEffect(() => {
    const handler = () => onRouteRef.current(parseRoute(window.location.hash))
    window.addEventListener('popstate', handler)
    window.addEventListener('hashchange', handler)
    return () => {
      window.removeEventListener('popstate', handler)
      window.removeEventListener('hashchange', handler)
    }
  }, [])
}
