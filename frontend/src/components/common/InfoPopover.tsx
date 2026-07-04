import { useState } from 'react'
import { Info } from 'lucide-react'

// InfoPopover is a small (ⓘ) button that reveals explanatory text on click,
// keeping long help notes out of the way until the user asks for them. Closes on
// blur (click-away) so it behaves like a lightweight tooltip/popover. Position the
// popover relative to the button's inline-flex wrapper.
export function InfoPopover({
  text,
  label = 'Bilgi',
  align = 'left',
}: {
  text: string
  label?: string
  // Which edge the popover aligns to (avoids clipping near the panel's right edge).
  align?: 'left' | 'right'
}) {
  const [open, setOpen] = useState(false)
  return (
    <span className="relative inline-flex align-middle">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        onBlur={() => setOpen(false)}
        title={label}
        aria-label={label}
        aria-expanded={open}
        className="inline-flex h-4 w-4 items-center justify-center rounded-full text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
      >
        <Info size={13} />
      </button>
      {open && (
        <span
          role="tooltip"
          className={`absolute top-5 z-50 w-80 max-w-[80vw] whitespace-pre-line rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-[11px] leading-relaxed text-[var(--color-text-dim)] shadow-[var(--shadow-lg)] ${
            align === 'right' ? 'right-0' : 'left-0'
          }`}
        >
          {text}
        </span>
      )}
    </span>
  )
}
