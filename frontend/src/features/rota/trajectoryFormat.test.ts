import { describe, expect, it } from 'vitest'
import { fmtDurationSec, fmtTokens } from './trajectoryFormat'

describe('trajectoryFormat', () => {
  it('formats durations at the natural unit', () => {
    expect(fmtDurationSec(0)).toBe('0 sn')
    expect(fmtDurationSec(42)).toBe('42 sn')
    expect(fmtDurationSec(600)).toBe('10 dk')
    expect(fmtDurationSec(3600)).toBe('1 sa')
    expect(fmtDurationSec(3600 + 900)).toBe('1 sa 15 dk')
    expect(fmtDurationSec(2 * 86400 + 3 * 3600)).toBe('2 g 3 sa')
  })

  it('formats token counts compactly', () => {
    expect(fmtTokens(0)).toBe('0')
    expect(fmtTokens(950)).toBe('950')
    expect(fmtTokens(12_345)).toBe('12.3k')
    expect(fmtTokens(250_000)).toBe('250k')
    expect(fmtTokens(2_400_000)).toBe('2.4M')
  })
})
