// Rota F0 wait spans: the stretches of a coordinator's bar where it had handed
// work to a worker and was itself idle. Without these the bar reads as one
// unbroken run, hiding that most of a long coordinator session is spent waiting
// rather than working.
//
// Derived from the workers' own lifetimes rather than from liveness: the
// `awaiting_workers` liveness state only answers "right now", so it cannot
// paint a bar that spans yesterday. A member lane's [createdAt, end] is the
// window its coordinator was blocked on it, and overlapping members merge into
// one span (two workers in flight is still one wait).
//
// SCOPE: spawned workers only — lanes that exist as their own session. A
// `run_subagent` call runs inside the coordinator's own turn and creates no
// session, so nothing here can see it; those stretches stay solid. See
// _Docs/78 §13.
import type { LaneSession, LaneState } from '@/shared/lib/laneModel'
import { laneMembers } from '@/shared/lib/laneModel'

/** A stretch of a coordinator's bar spent waiting on its workers. */
export interface RotaWait {
  start: number
  end: number
  /** How many members were in flight at the peak of this span, for the tooltip. */
  peak: number
}

/** Waits shorter than this are not drawn: a sub-minute handoff adds a sliver of
 *  visual noise without telling the reader anything. */
const MIN_WAIT_SEC = 60

/** A member still in flight has no end yet, so its wait runs to `now`. The
 *  liveness signal is the fresher fact; `runState` is what the header carries
 *  when no liveness snapshot has landed (a seeded, never-streamed lane). */
function memberEnd(m: LaneSession, now: number): number {
  const live = !!m.live || m.runState === 'running'
  return live ? now : Math.max(m.updatedAt, m.createdAt)
}

/** The merged wait spans on `rootId`'s bar. Members are those the layout would
 *  draw (pass the same filter), so a wait never points at a hidden lane. */
export function waitSpans(
  state: LaneState,
  rootId: string,
  now: number,
  keep?: (s: LaneSession) => boolean,
): RotaWait[] {
  const members = laneMembers(state, rootId).filter((m) => !keep || keep(m))
  if (members.length === 0) return []

  // Merge the member lifetimes into continuous spans: two workers that touch
  // end-to-start are ONE wait, because the coordinator never got the turn back
  // in between.
  const lives = members
    .map((m) => ({ start: m.createdAt || m.updatedAt, end: memberEnd(m, now) }))
    .filter((l) => l.end > l.start)
    .sort((a, b) => a.start - b.start)

  const spans: RotaWait[] = []
  for (const l of lives) {
    const last = spans[spans.length - 1]
    if (last && l.start <= last.end) {
      if (l.end > last.end) last.end = l.end
    } else {
      spans.push({ start: l.start, end: l.end, peak: 0 })
    }
  }

  // Peak concurrency per span, measured separately from the merge: a handoff
  // (one worker ending exactly as the next starts) is one continuous wait but
  // never two workers at once, so counting it off the merge sweep would report
  // a concurrency that never happened. Here a lifetime counts toward a span
  // only while it strictly overlaps another.
  for (const span of spans) {
    const inSpan = lives.filter((l) => l.start < span.end && l.end > span.start)
    for (const l of inSpan) {
      const at = l.start
      const n = inSpan.filter((o) => o.start <= at && o.end > at).length
      if (n > span.peak) span.peak = n
    }
  }

  return spans.filter((s) => s.end - s.start >= MIN_WAIT_SEC)
}

/** Clip the spans to the coordinator's own bar, so a wait never draws past the
 *  bar's rounded end (a worker can outlive an archived coordinator). */
export function clipWaits(waits: RotaWait[], barStart: number, barEnd: number): RotaWait[] {
  const out: RotaWait[] = []
  for (const w of waits) {
    const start = Math.max(w.start, barStart)
    const end = Math.min(w.end, barEnd)
    if (end - start >= MIN_WAIT_SEC) out.push({ start, end, peak: w.peak })
  }
  return out
}

/** "2 worker · 18 dk" — what one wait segment hides. */
export function formatWait(w: RotaWait): string {
  const m = Math.round((w.end - w.start) / 60)
  const dur = m < 60 ? `${m} dk` : `${Math.floor(m / 60)} sa ${m % 60} dk`
  return `${w.peak} worker · ${dur} bekleme`
}
