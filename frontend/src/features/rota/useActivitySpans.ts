// useActivitySpans — the Rota canvas's source for "when was this session
// actually working" (_Docs/78 §16). The lane store carries a session's
// lifetime, not its turns, so the bouts a bar is split along come from a
// dedicated read: GET /api/sessions/activity.
//
// The read is lazy and incremental on purpose. It is asked only for the lanes
// the canvas drew, only for the ones whose transcript moved since the last
// answer (updatedAt is the fingerprint), and behind a debounce so a burst of
// stream events during a live turn does not turn into a burst of requests.
import { useEffect, useRef, useState } from 'react'
import { api } from '@/api'
import type { ActivitySpan } from '@/types'
import type { LaneSession } from '@/shared/lib/laneModel'

// Matches the server's per-call id cap; asking for more is a 400.
const MAX_IDS = 200
const DEBOUNCE_MS = 400

interface Entry {
  updatedAt: number
  // The threshold this answer was computed with: the same transcript split at
  // a different gap is a different answer, so it has to invalidate the entry.
  gapSec: number
  spans: ActivitySpan[]
}

export function useActivitySpans(
  sessions: readonly LaneSession[],
  gapSec: number,
): ReadonlyMap<string, ActivitySpan[]> {
  // The answers we hold, keyed by session id. A session absent from a reply
  // (no transcript) is cached as an empty list so it is not asked again until
  // it moves.
  const cache = useRef(new Map<string, Entry>())
  const [spans, setSpans] = useState<ReadonlyMap<string, ActivitySpan[]>>(new Map())

  // The request key: which sessions, at which transcript version. Built as a
  // string so the effect re-runs on a real change, not on every new array.
  // The gap rides along: zooming past a ladder rung (rotaActivityGap.ts) has
  // to re-ask, the same way a moved transcript does.
  const want = sessions.slice(0, MAX_IDS).map((s) => `${s.id}:${s.updatedAt}`)
  const key = `${gapSec}|${want.join(',')}`

  useEffect(() => {
    if (want.length === 0) return
    const stale = want
      .map((w) => w.split(':'))
      .filter(([id, updatedAt]) => {
        const entry = cache.current.get(id)
        return entry?.updatedAt !== Number(updatedAt) || entry.gapSec !== gapSec
      })
      .map(([id]) => id)
    if (stale.length === 0) return

    const controller = new AbortController()
    const timer = setTimeout(() => {
      const asked = new Map(
        want.map((w) => {
          const [id, updatedAt] = w.split(':')
          return [id, Number(updatedAt)]
        }),
      )
      api
        .sessionActivity(stale, gapSec, controller.signal)
        .then((resp) => {
          if (controller.signal.aborted) return
          for (const id of stale) {
            // Record the version we asked about, so an empty answer (a session
            // with no transcript) is remembered rather than re-requested.
            cache.current.set(id, {
              updatedAt: asked.get(id) ?? 0,
              gapSec,
              spans: resp.sessions[id] ?? [],
            })
          }
          const next = new Map<string, ActivitySpan[]>()
          for (const [id, entry] of cache.current) {
            if (entry.spans.length > 0) next.set(id, entry.spans)
          }
          setSpans(next)
        })
        .catch(() => {
          // A failed read leaves the bars whole — the canvas is still correct,
          // it just does not show the split. The next lane change retries.
        })
    }, DEBOUNCE_MS)

    return () => {
      controller.abort()
      clearTimeout(timer)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key])

  return spans
}
