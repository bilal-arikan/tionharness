// Numeric-field validity tracking. A NumberField whose text is empty, not a
// number, or outside [min, max] never writes to the settings draft — the last
// valid value stays there. It registers itself here instead, so the screen
// hosting the fields can block Save while any of them is invalid (a silent 0 /
// null reaching the backend is exactly the data loss this prevents).
//
// Lives beside primitives.tsx (which renders NumberField and the provider) so
// that component file exports only components, as fast refresh requires.
import { createContext, useCallback, useMemo, useState } from 'react'

export interface NumberValidity {
  hasInvalid: boolean
  report: (id: string, invalid: boolean) => void
}

export const NumberValidityCtx = createContext<NumberValidity | null>(null)

// Owns the invalid-field set. Call in the screen that renders the fields AND the
// Save button, pass the result to NumberValidityProvider, and gate Save on
// `hasInvalid`.
export function useNumberValidity(): NumberValidity {
  const [invalid, setInvalid] = useState<ReadonlySet<string>>(() => new Set())
  const report = useCallback((id: string, bad: boolean) => {
    setInvalid((prev) => {
      if (prev.has(id) === bad) return prev
      const next = new Set(prev)
      if (bad) next.add(id)
      else next.delete(id)
      return next
    })
  }, [])
  return useMemo(() => ({ hasInvalid: invalid.size > 0, report }), [invalid, report])
}
