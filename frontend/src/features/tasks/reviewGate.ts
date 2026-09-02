/**
 * REVIEW_ROUND_BUDGET mirrors `agent.ReviewRoundBudget`
 * (internal/agent/coordination_situation.go). The backend counts a card's failed
 * verification rounds in `Task.ReviewBounces` and injects a hard
 * `<review-gate-exhausted>` block into a coordinator's turn once the count
 * reaches this budget. The board shows the same number so a human sees the
 * treadmill at the moment the coordinator is told to stop.
 *
 * Keep the two in sync: a UI that says "3 / 3" while the backend escalates at 4
 * is worse than no badge at all.
 */
export const REVIEW_ROUND_BUDGET = 3

/** Presentation for the review-round badge, or null when the card has none. */
export interface ReviewGateBadge {
  /** Failed verification rounds so far. */
  count: number
  /** The card has reached the budget — the coordinator is being told to stop. */
  exhausted: boolean
  /** Compact chip text, e.g. "3/3". */
  label: string
  /** Hover explanation. */
  title: string
  /** CSS colour token the chip is tinted with. */
  color: string
}

/**
 * reviewGateBadge derives the badge for a card's review-bounce count.
 *
 * A card that has never failed a review round gets NO badge: the board is
 * already dense, and "0 failed rounds" is the normal state of every card on it.
 * The chip only appears once there is something to notice, and turns red exactly
 * when the backend stops accepting another review round.
 */
export function reviewGateBadge(reviewBounces: number | undefined): ReviewGateBadge | null {
  const count = reviewBounces ?? 0
  if (count <= 0) return null
  const exhausted = count >= REVIEW_ROUND_BUDGET
  return {
    count,
    exhausted,
    label: `${count}/${REVIEW_ROUND_BUDGET}`,
    title: exhausted
      ? `Doğrulama bütçesi doldu: bu kart incelemeden ${count} kez geri döndü (bütçe ${REVIEW_ROUND_BUDGET}). Yeni bir reviewer turu yerine kartı daralt veya karar ver.`
      : `Bu kart incelemeden ${count} kez geri döndü (bütçe ${REVIEW_ROUND_BUDGET}).`,
    color: exhausted ? 'var(--color-danger)' : 'var(--color-warning)',
  }
}
