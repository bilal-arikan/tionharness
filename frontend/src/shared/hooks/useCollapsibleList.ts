import { useState } from 'react'

// useCollapsibleList tracks the MOBILE drawer state of a screen's left list — the
// exact model the chat sessions sidebar uses. On `md+` the list is always visible
// (a static column, handled by CSS in CollapsibleListShell), so this flag only
// matters on portrait phones, where it must default CLOSED and reset on every load
// (never persisted) so a phone always lands on the content, not an open drawer.
// The `storageKey` argument is accepted for call-site compatibility but unused.
export function useCollapsibleList(_storageKey?: string): {
  open: boolean
  toggle: () => void
  setOpen: (v: boolean) => void
} {
  const [open, setOpen] = useState(false)
  return { open, toggle: () => setOpen((v) => !v), setOpen }
}
