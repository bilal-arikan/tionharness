import type { LucideIcon } from 'lucide-react'

interface Props {
  // Colour/semantic class from ./buttonStyles (BTN_PRIMARY, BTN_DANGER, …).
  className: string
  icon: LucideIcon
  // Visible on wide viewports; below the `sm` breakpoint only the icon shows and
  // this text survives as the accessible name + tooltip.
  label: string
  // Tooltip. Falls back to `label` so a narrow-screen icon is never unexplained.
  title?: string
  testId?: string
  disabled?: boolean
  onClick?: () => void
}

// ActionButton is the composer's text action (Gönder / Durdur / Sıraya / …).
// It renders an icon plus a label, and hides the label below the `sm` breakpoint
// so the send cluster degrades to icon-only buttons on narrow screens rather than
// wrapping onto a second toolbar row.
//
// The collapse is pure CSS (a `hidden sm:inline` span), not a measured breakpoint:
// no resize listener, no re-render on viewport change. The label stays in the DOM
// as `aria-label` + `title`, so screen readers and hover tooltips keep working when
// only the glyph is visible.
export function ActionButton({
  className,
  icon: Icon,
  label,
  title,
  testId,
  disabled,
  onClick,
}: Props) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={title ?? label}
      aria-label={label}
      data-testid={testId}
      className={className}
    >
      <Icon size={18} className="shrink-0" />
      <span className="hidden sm:inline">{label}</span>
    </button>
  )
}
