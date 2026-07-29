import { describe, expect, it, vi, afterEach } from 'vitest'
import { noteServerTime, serverClockSkewSec, serverNow } from './serverClock'
import { formatDurationMs } from './time'

afterEach(() => {
  vi.useRealTimers()
  // Re-align the module-level skew so one test cannot leak into the next.
  noteServerTime(Math.floor(Date.now() / 1000))
})

describe('serverClock', () => {
  it('adopts the server clock when it differs materially from the local one', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-28T12:00:00Z'))
    const local = Math.floor(Date.now() / 1000)

    // Client clock runs 90 s behind the server.
    noteServerTime(local + 90)
    expect(serverClockSkewSec()).toBe(90)
    expect(serverNow()).toBe(local + 90)
  })

  it('ignores sub-threshold jitter so the visible counter does not bounce', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-28T12:00:00Z'))
    const local = Math.floor(Date.now() / 1000)

    noteServerTime(local + 90)
    noteServerTime(local + 91) // 1 s of network jitter — below the threshold
    expect(serverClockSkewSec()).toBe(90)
  })

  it('ignores an absent stamp', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-07-28T12:00:00Z'))
    const local = Math.floor(Date.now() / 1000)

    noteServerTime(local + 90)
    noteServerTime(0)
    expect(serverClockSkewSec()).toBe(90)
  })
})

describe('formatDurationMs', () => {
  it('keeps one decimal for short, server-measured turns', () => {
    expect(formatDurationMs(800)).toBe('0.8 sn')
    expect(formatDurationMs(3400)).toBe('3.4 sn')
  })

  it('falls back to whole-second formatting from 10 s up', () => {
    expect(formatDurationMs(10_000)).toBe('10 sn')
    expect(formatDurationMs(135_000)).toBe('2 dk 15 sn')
    expect(formatDurationMs(3_900_000)).toBe('1 sa 5 dk')
  })

  it('clamps a negative span to zero', () => {
    expect(formatDurationMs(-5)).toBe('0.0 sn')
  })
})
