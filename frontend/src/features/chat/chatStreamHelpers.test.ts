import { describe, expect, it } from 'vitest'
import { intersectWith } from './chatStreamHelpers'

describe('intersectWith', () => {
  it('drops latches the server no longer reports as running', () => {
    // SES12 outlived its turn (its completion event landed while another
    // workspace was active, so nothing cleared it); the server says only SES7
    // is still in flight here.
    const got = intersectWith(new Set(['SES7', 'SES12']), new Set(['SES7']))
    expect([...got]).toEqual(['SES7'])
  })

  it('returns the same set instance when nothing is dropped', () => {
    const prev = new Set(['SES7'])
    expect(intersectWith(prev, new Set(['SES7', 'SES9']))).toBe(prev)
  })

  it('never adds ids the caller was not already latching', () => {
    // reconcileActive must not raise a "streaming" latch for a detached turn
    // this window owns no run handle for — that is the pending set's job.
    const got = intersectWith(new Set<string>(), new Set(['SES7']))
    expect(got.size).toBe(0)
  })

  it('clears everything when the workspace has no running turn', () => {
    expect(intersectWith(new Set(['SES1', 'SES2']), new Set()).size).toBe(0)
  })
})
