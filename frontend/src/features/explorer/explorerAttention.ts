import type { ViewGraphAttention, ViewGraphLiveResult, ViewGraphResult, ViewRef } from '@/types'
import { refToString } from '@/types'

// The map's attention layer (_Docs/68 §7.2): which nodes a human should look at
// right now. The backend derives it from the awareness open-loop scan — the
// same facts the agent's pulse reads — and ships it on the graph payload; this
// module is the pure glue between that payload, the filter chips and the
// spotlight (a node that just changed glows wider for a few seconds).

// Facets the status strip toggles. Each one is a group of backend reason codes.
export const ATTENTION_FACETS = ['waiting', 'stuck', 'failed', 'stale', 'errors'] as const
export type AttentionFacet = (typeof ATTENTION_FACETS)[number]

const FACET_REASONS: Record<AttentionFacet, readonly string[]> = {
  waiting: ['waiting-ask'],
  stuck: ['stuck', 'blocked'],
  failed: ['failed-card'],
  stale: ['stale-card'],
  errors: ['tool-errors'],
}

export function isAttentionFacet(value: string): value is AttentionFacet {
  return (ATTENTION_FACETS as readonly string[]).includes(value)
}

// facetsOf lists the facets an attention entry falls under.
export function facetsOf(entry: ViewGraphAttention | undefined): AttentionFacet[] {
  if (!entry) return []
  return ATTENTION_FACETS.filter((facet) =>
    entry.reasons.some((reason) => FACET_REASONS[facet].includes(reason)),
  )
}

// attentionMatches is true when the entry carries at least one reason of the
// selected facets (OR inside the facet set, like every other chip row).
export function attentionMatches(
  entry: ViewGraphAttention | undefined,
  facets: readonly string[],
): boolean {
  if (facets.length === 0) return true
  if (!entry) return false
  return facets.some((facet) =>
    isAttentionFacet(facet) ? entry.reasons.some((r) => FACET_REASONS[facet].includes(r)) : false,
  )
}

// bearsAttention: the kinds the backend marks today. Everything else is
// structure and is never dropped by an attention filter.
export function bearsAttention(ref: ViewRef): boolean {
  return ref.kind === 'session' || (ref.kind === 'board' && !!ref.sub)
}

// mergeLive lays a /graph/live payload over the current map: nodes and edges
// stay exactly as they are, the volatile layers are replaced.
export function mergeLive(graph: ViewGraphResult, live: ViewGraphLiveResult): ViewGraphResult {
  return {
    ...graph,
    live: live.live ?? [],
    meta: live.meta ?? graph.meta,
    attention: live.attention ?? {},
    status: live.status ?? graph.status,
  }
}

// changedKeys names the nodes whose live state or attention changed between two
// payloads: a session that started or stopped, a ring that appeared, moved
// level or gained a reason. The spotlight set.
export function changedKeys(prev: ViewGraphResult | null, next: ViewGraphResult): string[] {
  if (!prev) return []
  const out = new Set<string>()
  const before = prev.attention ?? {}
  const after = next.attention ?? {}
  for (const [key, entry] of Object.entries(after)) {
    const was = before[key]
    if (!was || was.level !== entry.level || was.reasons.join('|') !== entry.reasons.join('|')) {
      out.add(key)
    }
  }
  for (const key of Object.keys(before)) if (!after[key]) out.add(key)
  const liveBefore = new Map((prev.live ?? []).map((l) => [refToString(l.session), l.state]))
  const liveAfter = new Map((next.live ?? []).map((l) => [refToString(l.session), l.state]))
  for (const [key, state] of liveAfter) if (liveBefore.get(key) !== state) out.add(key)
  for (const key of liveBefore.keys()) if (!liveAfter.has(key)) out.add(key)
  return [...out]
}

// How long a changed node stays in the spotlight.
export const FLASH_MS = 10_000

// pruneFlashes drops expired entries. Returns the same map when nothing
// expired so a state setter can skip the re-render.
export function pruneFlashes(flashes: Map<string, number>, now: number): Map<string, number> {
  let stale = false
  for (const at of flashes.values()) {
    if (now - at >= FLASH_MS) {
      stale = true
      break
    }
  }
  if (!stale) return flashes
  const next = new Map<string, number>()
  for (const [key, at] of flashes) if (now - at < FLASH_MS) next.set(key, at)
  return next
}

// relativeAge buckets a unix-second stamp for the status strip's "last digest"
// label: minutes under an hour, hours under a day, days after that.
export function relativeAge(
  at: number | undefined,
  nowMs: number,
): { unit: 'now' | 'minutes' | 'hours' | 'days'; count: number } | null {
  if (!at) return null
  const seconds = Math.max(0, Math.floor(nowMs / 1000) - at)
  if (seconds < 60) return { unit: 'now', count: 0 }
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return { unit: 'minutes', count: minutes }
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return { unit: 'hours', count: hours }
  return { unit: 'days', count: Math.floor(hours / 24) }
}
