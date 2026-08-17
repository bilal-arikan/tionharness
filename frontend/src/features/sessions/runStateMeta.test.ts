import { describe, it, expect } from 'vitest'
import { runStateMeta } from './runStateMeta'

// The badge's decision, not its markup: which runState values render a badge, and
// which deliberately render nothing. The empty case is the one that matters most —
// every session written before Session.RunState existed carries '', and showing
// those a "tamamlandı" badge would invent a run that never happened.
describe('runStateMeta', () => {
  it('renders no badge for a session that never ran a background turn', () => {
    expect(runStateMeta(undefined)).toBeUndefined()
    expect(runStateMeta('')).toBeUndefined()
  })

  it('renders no badge for a value this UI does not know', () => {
    // Forward-compatible: a runState added to the runtime vocabulary later must
    // fall back to nothing rather than crash or render a raw enum string.
    expect(runStateMeta('some-future-status')).toBeUndefined()
  })

  it('labels every outcome in the runtime vocabulary', () => {
    for (const [state, label] of [
      ['completed', 'tamamlandı'],
      ['failed', 'başarısız'],
      ['killed', 'durduruldu'],
      ['timeout', 'zaman aşımı'],
      ['incomplete', 'yarım kaldı'],
    ] as const) {
      expect(runStateMeta(state)?.label).toBe(label)
    }
  })

  it('keeps the three bad outcomes distinguishable, and completed quiet', () => {
    // They call for different actions (retry / it-was-stopped / give-it-longer), so
    // they must not collapse into one tone; completed is the common case and stays
    // visually calm.
    expect(runStateMeta('failed')?.tone).toBe('danger')
    expect(runStateMeta('killed')?.tone).toBe('warning')
    expect(runStateMeta('timeout')?.tone).toBe('warning')
    expect(runStateMeta('completed')?.tone).toBe('muted')
    expect(runStateMeta('failed')?.tone).not.toBe(runStateMeta('completed')?.tone)
  })

  it('gives every badge a hover title explaining the outcome', () => {
    for (const s of ['completed', 'failed', 'killed', 'timeout', 'incomplete']) {
      expect(runStateMeta(s)?.title).toMatch(/arka plan turu/)
    }
  })
})
