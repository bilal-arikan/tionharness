// A tiny module-level store of monotonically-increasing "tick" counters, one
// per signal key. App.tsx's central SSE handler calls bumpSignal(key) when an
// event type wants to nudge the panels subscribed to that key; the panels read
// the current tick via useRefreshTrigger(key) and re-fetch on every change.
//
// This replaces the per-panel api.subscribeEvents dance (TaskBoard, NetworkPanel,
// ExecutionsPanel, useActivity) with a single dispatcher in App.tsx and a
// "dumb consumer" hook in each panel — the panels no longer need to know about
// event types, debounce, or the EventSource itself.
//
// A module store (vs React context) avoids provider plumbing and works across
// the whole tree. The pattern mirrors dirtySignals.ts so the codebase stays
// consistent.

const ticks = new Map<string, number>()
// Listeners keyed by signal name. Each entry is a Set so an unsubscribe leaves
// the rest of the subscribers intact.
const listeners = new Map<string, Set<() => void>>()
// Cached immutable snapshot per key so useSyncExternalStore sees a stable
// identity between changes (required — returning a fresh number each call would
// loop forever).
const snapshots = new Map<string, number>()

function refresh(key: string) {
  const t = ticks.get(key) ?? 0
  snapshots.set(key, t)
  listeners.get(key)?.forEach((l) => l())
}

// bumpSignal advances the tick for `key` and notifies all subscribers. The
// bump is unconditional — callers (App.tsx) debounce as they see fit to avoid
// hammering subscribers on a burst of events.
export function bumpSignal(key: string): void {
  ticks.set(key, (ticks.get(key) ?? 0) + 1)
  refresh(key)
}

// getSignalTick returns the current tick for `key`. Useful in effects that
// don't need reactivity (manual reads) — useSyncExternalStore consumers should
// prefer the hook.
export function getSignalTick(key: string): number {
  return ticks.get(key) ?? 0
}

// subscribeSignal registers `cb` to be invoked on every tick change for `key`.
// Returns an unsubscribe function. Cached snapshot is required for
// useSyncExternalStore to avoid spurious re-renders.
export function subscribeSignal(key: string, cb: () => void): () => void {
  let set = listeners.get(key)
  if (!set) {
    set = new Set()
    listeners.set(key, set)
  }
  set.add(cb)
  return () => {
    set?.delete(cb)
  }
}

// getSnapshot is the read function paired with subscribeSignal in
// useSyncExternalStore. Same identity requirement: must be referentially equal
// between calls if the underlying value hasn't changed.
export function getSnapshot(key: string): number {
  let s = snapshots.get(key)
  if (s === undefined) {
    s = ticks.get(key) ?? 0
    snapshots.set(key, s)
  }
  return s
}
