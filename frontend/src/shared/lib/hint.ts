import type { ReactNode } from 'react'

// Explanations up to this many characters (on one line) read better shown than
// hidden; anything longer, multi-line or rich goes behind the (ⓘ) of InfoPopover.
export const SHORT_HINT_MAX = 80

export function isShortHint(text: ReactNode): text is string {
  return typeof text === 'string' && text.length <= SHORT_HINT_MAX && !text.includes('\n')
}
