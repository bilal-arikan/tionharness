// Server-authoritative wall clock. Every elapsed-time reading in the UI must be
// measured against the SERVER's clock, not the browser's: turn start stamps come
// from the backend (hub event `time`, message `createdAt`), so subtracting a raw
// `Date.now()` from them silently folds in whatever skew the client machine has.
//
// The hub stream stamps every frame with the server's unix seconds, which gives
// us a free, continuously refreshed skew estimate. `serverNow()` is the only
// "current time" a duration calculation should use.
//
// Absolute timestamps (a message's clock label) are deliberately NOT shifted —
// those are rendered in the viewer's local timezone on purpose.

// skewSec = serverUnix - localUnix, as last observed. 0 until the first frame.
let skewSec = 0

// Resync threshold in seconds. Observations arrive after network latency, so the
// estimate jitters by a fraction of a second; only adopt a materially different
// value so the visible counter does not bounce.
const RESYNC_THRESHOLD = 2

// noteServerTime feeds an observation of the server's clock (unix seconds, as
// carried by a hub event or the stream hello). Ignores empty/absent stamps.
export function noteServerTime(serverUnixSec: number): void {
  if (!serverUnixSec) return
  const observed = serverUnixSec - Math.floor(Date.now() / 1000)
  if (Math.abs(observed - skewSec) >= RESYNC_THRESHOLD) skewSec = observed
}

// serverNow returns the current time in unix seconds ON THE SERVER's clock.
// Before any observation it degrades to the local clock (skew 0).
export function serverNow(): number {
  return Math.floor(Date.now() / 1000) + skewSec
}

// serverClockSkewSec exposes the current estimate (diagnostics / tests).
export function serverClockSkewSec(): number {
  return skewSec
}
