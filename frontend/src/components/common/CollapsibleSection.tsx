import { useEffect, useRef, useState, type ReactNode } from 'react'
import { ChevronRight } from 'lucide-react'

// BulkToggle lets a parent broadcast an "expand all" / "collapse all" command to a
// group of CollapsibleSections. Bump `nonce` (with the desired `all` state) to snap
// every subscribing section open/closed; the user can still fold them individually
// afterwards. `useBulkToggle` returns the signal plus the two commands.
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

// CollapsibleSection — a fold in/out wrapper for a titled block. The chevron +
// title toggle the body; an optional `right` node (cache tag, count, mode
// switch) sits outside the toggle so its own controls stay clickable. Used by
// the context-preview modals so each context segment (system prompt, dynamic
// suffix, message array, tools) can be collapsed independently.
interface Props {
  title: ReactNode
  // Trailing header content kept OUTSIDE the toggle button (may hold its own
  // interactive controls — nesting a button inside a button is invalid HTML).
  right?: ReactNode
  defaultOpen?: boolean
  // Optional broadcast signal from a parent "expand/collapse all" control (see
  // useBulkToggle). When its nonce changes, the section snaps to `all`.
  bulk?: BulkToggle
  children: ReactNode
  className?: string
}

export function CollapsibleSection({
  title,
  right,
  defaultOpen = true,
  bulk,
  children,
  className = '',
}: Props) {
  const [open, setOpen] = useState(defaultOpen)
  // Follow the parent's expand/collapse-all command when its nonce bumps — but NOT
  // on the initial mount (nonce unchanged), so defaultOpen is honoured until the
  // user actually clicks expand/collapse-all.
  const seenNonce = useRef(bulk?.nonce)
  useEffect(() => {
    if (!bulk || bulk.nonce === seenNonce.current) return
    seenNonce.current = bulk.nonce
    setOpen(bulk.all)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [bulk?.nonce])
  return (
    <div className={`mb-4 ${className}`}>
      <div className="flex items-center gap-1.5">
        <button
          type="button"
          onClick={() => setOpen((o) => !o)}
          aria-expanded={open}
          className="group flex min-w-0 flex-1 items-center gap-1.5 text-left"
        >
          <ChevronRight
            size={13}
            className={`shrink-0 text-[var(--color-text-dim)] transition-transform group-hover:text-[var(--color-text)] ${
              open ? 'rotate-90' : ''
            }`}
          />
          <h3 className="flex min-w-0 items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)] group-hover:text-[var(--color-text)]">
            {title}
          </h3>
        </button>
        {right}
      </div>
      {open && <div className="mt-1.5">{children}</div>}
    </div>
  )
}
