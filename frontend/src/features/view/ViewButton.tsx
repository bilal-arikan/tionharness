import { useState } from 'react'
import { ModalOverlay } from '@/shared/components/ModalOverlay'
import type { ViewRef } from '@/types'
import { ViewPanel } from './ViewPanel'

interface Props {
  target: ViewRef
  // Extra classes for the trigger (sizing/placement varies by host screen).
  className?: string
  // Compact renders the glyph only — for headers that are already crowded.
  compact?: boolean
  // label overrides the default "Özet" text — used when the trigger names a
  // specific drill-down (a projection handle) rather than "this screen's summary".
  label?: string
  disabled?: boolean
}

// ViewButton is the "◱ Özet" trigger that opens the projection drawer for one
// entity. It owns the open/closed state so a host screen only has to say WHICH
// entity it is showing.
//
// Labelled "Özet", not "Bağlam": the chat header already has a "Bağlam" button
// that previews the raw prompt of the NEXT turn. Two buttons with one name in the
// same toolbar would be worse than a slightly less evocative word.
//
// The drawer is right-anchored (a side sheet, not a centered dialog): the point
// is to read the projection against the screen behind it, not to replace it.
export function ViewButton({ target, className = '', compact, label, disabled }: Props) {
  const [open, setOpen] = useState(false)

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        disabled={disabled}
        title="Bu ekranın ajana verilen kompakt özeti (get_view ile aynı çıktı)"
        className={`flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:cursor-not-allowed disabled:opacity-50 ${className}`}
      >
        <span className="leading-none">◱</span>
        {!compact && (label ?? 'Özet')}
      </button>
      {open && (
        <ModalOverlay onClose={() => setOpen(false)} padding="p-0" className="!justify-end">
          <div className="h-full" onMouseDown={(e) => e.stopPropagation()}>
            <ViewPanel target={target} onClose={() => setOpen(false)} />
          </div>
        </ModalOverlay>
      )}
    </>
  )
}
