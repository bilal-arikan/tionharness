import { useRef, useState } from 'react'
import { Info } from 'lucide-react'

// InfoPopover is a small (ⓘ) button that reveals explanatory text on click,
// keeping long help notes out of the way until the user asks for them. Closes on
// blur (click-away) so it behaves like a lightweight tooltip/popover. Position the
// popover relative to the button's inline-flex wrapper.
export function InfoPopover({
  text,
  label = 'Bilgi',
  align = 'left',
  fixed = false,
}: {
  text: string
  label?: string
  // Which edge the popover aligns to (avoids clipping near the panel's right edge).
  align?: 'left' | 'right'
  // Render the bubble in viewport (fixed) coordinates instead of absolutely
  // inside the wrapper. Needed when an ancestor scrolls/clips (e.g. the narrow
  // flow node palette), where an absolute bubble would be cut off.
  fixed?: boolean
}) {
  const [open, setOpen] = useState(false)
  const [pos, setPos] = useState<{ top: number; left: number } | null>(null)
  const btnRef = useRef<HTMLButtonElement>(null)

  const toggle = () => {
    if (fixed && !open && btnRef.current) {
      const r = btnRef.current.getBoundingClientRect()
      setPos({ top: r.bottom + 6, left: r.left })
    }
    setOpen((v) => !v)
  }

  // In fixed mode the bubble is clamped into the viewport so a button near the
  // right/bottom edge still shows the whole note.
  const fixedStyle = pos
    ? {
        top: Math.min(pos.top, Math.max(8, window.innerHeight - 180)),
        left: Math.min(pos.left, Math.max(8, window.innerWidth - 340)),
      }
    : undefined

  return (
    <span className="relative inline-flex align-middle">
      <button
        ref={btnRef}
        type="button"
        onClick={toggle}
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
          style={fixed ? fixedStyle : undefined}
          className={`z-50 w-80 max-w-[80vw] whitespace-pre-line rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-[11px] leading-relaxed text-[var(--color-text-dim)] shadow-[var(--shadow-lg)] ${
            fixed ? 'fixed' : `absolute top-5 ${align === 'right' ? 'right-0' : 'left-0'}`
          }`}
        >
          {text}
        </span>
      )}
    </span>
  )
}
