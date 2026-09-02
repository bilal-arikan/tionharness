import { describe, it, expect } from 'vitest'
import { reviewGateBadge, REVIEW_ROUND_BUDGET } from './reviewGate'

describe('reviewGateBadge', () => {
  it('renders nothing for a card that never failed a review round', () => {
    expect(reviewGateBadge(undefined)).toBeNull()
    expect(reviewGateBadge(0)).toBeNull()
  })

  it('warns below the budget without calling the card exhausted', () => {
    const badge = reviewGateBadge(1)
    expect(badge).not.toBeNull()
    expect(badge!.exhausted).toBe(false)
    expect(badge!.label).toBe(`1/${REVIEW_ROUND_BUDGET}`)
    expect(badge!.color).toBe('var(--color-warning)')
  })

  it('turns danger exactly at the budget — the point the backend stops accepting rounds', () => {
    const badge = reviewGateBadge(REVIEW_ROUND_BUDGET)
    expect(badge!.exhausted).toBe(true)
    expect(badge!.color).toBe('var(--color-danger)')
    expect(badge!.title).toContain('bütçesi doldu')
  })

  it('keeps counting past the budget instead of clamping the label', () => {
    const badge = reviewGateBadge(REVIEW_ROUND_BUDGET + 4)
    expect(badge!.count).toBe(REVIEW_ROUND_BUDGET + 4)
    expect(badge!.label).toBe(`${REVIEW_ROUND_BUDGET + 4}/${REVIEW_ROUND_BUDGET}`)
    expect(badge!.exhausted).toBe(true)
  })

  it('never treats a negative count as a bounce', () => {
    expect(reviewGateBadge(-1)).toBeNull()
  })
})
