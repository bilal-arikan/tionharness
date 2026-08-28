import { describe, expect, it } from 'vitest'
import { costHint, costLabel } from './toolCostLabel'

describe('costLabel', () => {
  it('returns null when the backend sent no estimate', () => {
    expect(costLabel({})).toBeNull()
    expect(costLabel({ fullTokens: 900 })).toBeNull()
  })

  it('renders the per-turn badge from the CURRENT cost', () => {
    expect(costLabel({ currentTokens: 420, fullTokens: 3100 })).toBe('~420 tok/tur')
    expect(costLabel({ currentTokens: 3100, fullTokens: 3100 })).toBe('~3.1k tok/tur')
  })

  it('renders a free group as zero rather than hiding it', () => {
    expect(costLabel({ currentTokens: 0, fullTokens: 2000 })).toBe('~0 tok/tur')
  })
})

describe('costHint', () => {
  it('is empty without an estimate', () => {
    expect(costHint({})).toBe('')
  })

  it('spells out the delta of promoting the group to full', () => {
    const hint = costHint({ currentTokens: 400, fullTokens: 3400 })
    expect(hint).toContain('≈400')
    expect(hint).toContain('≈3.4k')
    expect(hint).toContain('+3.0k')
  })

  it('says there is nothing to promote when already all-full', () => {
    expect(costHint({ currentTokens: 3100, fullTokens: 3100 })).toContain('zaten')
  })
})
