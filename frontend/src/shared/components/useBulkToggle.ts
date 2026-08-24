// BulkToggle lets a parent broadcast an "expand all" / "collapse all" command to a
// group of CollapsibleSections. Bump `nonce` (with the desired `all` state) to snap
// every subscribing section open/closed; the user can still fold them individually
// afterwards. useBulkToggle returns the signal plus the two commands.
import { useState } from 'react'

export interface BulkToggle {
  all: boolean
  nonce: number
}

export function useBulkToggle(initial = true): {
  bulk: BulkToggle
  expandAll: () => void
  collapseAll: () => void
} {
  const [bulk, setBulk] = useState<BulkToggle>({ all: initial, nonce: 0 })
  return {
    bulk,
    expandAll: () => setBulk((b) => ({ all: true, nonce: b.nonce + 1 })),
    collapseAll: () => setBulk((b) => ({ all: false, nonce: b.nonce + 1 })),
  }
}
