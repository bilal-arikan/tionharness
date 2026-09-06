// Small pure helpers shared by the chat-stream modules.

// Immutable Set/Record helpers for the per-session streaming state.
export function withAdded(prev: ReadonlySet<string>, v: string): ReadonlySet<string> {
  if (prev.has(v)) return prev
  const n = new Set(prev)
  n.add(v)
  return n
}
export function withRemoved(prev: ReadonlySet<string>, v: string): ReadonlySet<string> {
  if (!prev.has(v)) return prev
  const n = new Set(prev)
  n.delete(v)
  return n
}
// intersectWith narrows a running-latch set to the ids the server still reports
// as in flight, returning `prev` unchanged when nothing is dropped (so React
// skips the re-render). See useChatStream.reconcileActive for why a latch has to
// be prunable and not just additive.
export function intersectWith(
  prev: ReadonlySet<string>,
  live: ReadonlySet<string>,
): ReadonlySet<string> {
  const n = new Set<string>()
  for (const id of prev) if (live.has(id)) n.add(id)
  return n.size === prev.size ? prev : n
}
export function withoutKey<T>(prev: Record<string, T>, key: string): Record<string, T> {
  if (!(key in prev)) return prev
  const n = { ...prev }
  delete n[key]
  return n
}
