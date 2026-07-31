// coordination.ts — the single place the frontend decides what a session's part
// in a coordinator tree is (M2, _Docs/47).
//
// Since coordinator trees can nest arbitrarily deep, "coordinator" and "worker"
// are no longer alternatives: a mid-level node is BOTH. `role` carries only the
// lineage ('worker' when something spawned it) while `coordinatorMode` carries
// the capability. Testing `role === 'coordinator'` — which several places used to
// do — now silently misses every mid-level node, so route every check through
// these helpers instead.

/** The coordination fields any session-shaped object may carry. */
export type CoordinationFields = {
  role?: string
  coordinatorMode?: boolean
  coordinatorSessionId?: string
  rootCoordinatorSessionId?: string
  coordinatorDepth?: number
}

/**
 * True when the session may spawn and drive workers. Accepts the legacy
 * `role === 'coordinator'` value so sessions created before the rework keep
 * showing their coordination UI.
 */
export function isCoordinatorSession(s?: CoordinationFields | null): boolean {
  if (!s) return false
  return Boolean(s.coordinatorMode) || s.role === 'coordinator'
}

/** True when the session reports UP to a coordinator (it has a parent). */
export function isWorkerSession(s?: CoordinationFields | null): boolean {
  if (!s) return false
  return Boolean(s.coordinatorSessionId) || s.role === 'worker'
}

/**
 * True for a mid-level node: it drives its own workers AND owes a result to the
 * coordinator above it. Worth showing differently — its "finished" turn does not
 * mean its task is done.
 */
export function isSubCoordinatorSession(s?: CoordinationFields | null): boolean {
  return isCoordinatorSession(s) && isWorkerSession(s)
}

/** True when the session takes part in a coordinator tree at all. */
export function isInCoordinatorTree(s?: CoordinationFields | null): boolean {
  return isCoordinatorSession(s) || isWorkerSession(s)
}

/**
 * A short human label for the session's part in its tree, or '' when it is not in
 * one. Used for the sidebar/panel chips.
 */
export function coordinationLabel(s?: CoordinationFields | null): string {
  if (isSubCoordinatorSession(s)) {
    const depth = s?.coordinatorDepth ?? 0
    return depth > 0 ? `Alt-koordinatör · seviye ${depth}` : 'Alt-koordinatör'
  }
  if (isCoordinatorSession(s)) return 'Koordinatör'
  if (isWorkerSession(s)) {
    const depth = s?.coordinatorDepth ?? 0
    return depth > 1 ? `Worker · seviye ${depth}` : 'Worker'
  }
  return ''
}
