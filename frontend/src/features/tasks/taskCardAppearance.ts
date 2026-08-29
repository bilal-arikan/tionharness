const DEFAULT_CARD_SHADOW = 'shadow-[var(--shadow-sm)]'
const CHANGED_CARD_SHADOW =
  'shadow-[0_0_0_1px_var(--color-accent),0_0_14px_2px_var(--color-accent)]'

// Tailwind shadow utilities all write box-shadow, so a card must receive exactly
// one of them. Combining both makes the generated stylesheet order decide which
// visual wins, regardless of their order in the class attribute.
export function taskCardShadowClass(recentlyChanged: boolean, pending: boolean): string {
  return recentlyChanged && !pending ? CHANGED_CARD_SHADOW : DEFAULT_CARD_SHADOW
}
