// Splitting a session bar into the stretches it actually worked (_Docs/78 §16).
//
// A bar spans createdAt → updatedAt, so a conversation held in a few sittings
// hours apart draws as one solid block. The server reports the bouts (message
// timestamps folded on an idle threshold, GET /api/sessions/activity); this
// module turns them into the blocks the canvas draws, which is a different
// question: bouts are facts about the transcript, segments are facts about
// this bar — clipped to it, merged when clipping made them touch, and with the
// live tail extended so a turn that is streaming right now is not drawn as an
// idle gap (its message is only persisted when the turn ends).
import type { ActivitySpan } from '@/types'

// Segments closer together than this are one block: below it the gap cannot be
// drawn as anything but noise, whatever the zoom.
const MIN_GAP_SEC = 1

// barSegments clips the bouts to [barStart, barEnd] and returns the blocks to
// draw. An empty result means "draw the bar whole": either there is no activity
// data for this session, or it all collapses into a single stretch, and a
// one-segment bar is exactly the plain bar.
export function barSegments(
  spans: readonly ActivitySpan[] | undefined,
  barStart: number,
  barEnd: number,
  live: boolean,
): ActivitySpan[] {
  if (!spans || spans.length === 0 || barEnd <= barStart) return []

  const clipped: ActivitySpan[] = []
  for (const s of spans) {
    const start = Math.max(s.start, barStart)
    const end = Math.min(s.end, barEnd)
    if (end < start) continue
    const last = clipped[clipped.length - 1]
    // Merge into the previous block when clipping (or the source data) left
    // them touching, so the canvas never draws a zero-width gap.
    if (last && start - last.end <= MIN_GAP_SEC) {
      last.end = Math.max(last.end, end)
      continue
    }
    clipped.push({ start, end })
  }
  if (clipped.length === 0) return []

  // A live bar runs to "now" while its last message is older than that: the
  // turn in flight has not been persisted yet. Carrying the tail avoids
  // drawing an active session as idle.
  if (live) clipped[clipped.length - 1].end = barEnd

  return clipped.length > 1 ? clipped : []
}

// segmentGaps returns the idle stretches between consecutive segments, plus the
// head gap when the first segment starts after the bar does (a session created
// well before its first message). Used for the tooltip and to decide whether a
// connector is worth drawing; the connector itself spans the whole bar.
export function segmentGaps(segments: readonly ActivitySpan[], barStart: number): ActivitySpan[] {
  if (segments.length === 0) return []
  const gaps: ActivitySpan[] = []
  if (segments[0].start - barStart > MIN_GAP_SEC) {
    gaps.push({ start: barStart, end: segments[0].start })
  }
  for (let i = 1; i < segments.length; i++) {
    gaps.push({ start: segments[i - 1].end, end: segments[i].start })
  }
  return gaps
}

// formatSegments — "4 oturuş · 3 sa 12 dk çalışma · 16 sa boşluk", the line the
// bar's tooltip adds once it is split.
export function formatSegments(segments: readonly ActivitySpan[], barStart: number): string {
  if (segments.length === 0) return ''
  const work = segments.reduce((sum, s) => sum + (s.end - s.start), 0)
  const idle = segmentGaps(segments, barStart).reduce((sum, g) => sum + (g.end - g.start), 0)
  return `${segments.length} oturuş · ${fmtDur(work)} çalışma · ${fmtDur(idle)} boşluk`
}

function fmtDur(sec: number): string {
  const m = Math.round(sec / 60)
  if (m < 60) return `${m} dk`
  const h = Math.floor(m / 60)
  return `${h} sa ${m % 60} dk`
}
