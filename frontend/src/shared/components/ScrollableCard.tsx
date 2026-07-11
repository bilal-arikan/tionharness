import type { HTMLAttributes } from 'react'

// ScrollableCard — a vertically scrollable content region for interactive chat
// cards (ask/plan/permission/todo). Tall cards otherwise overflow the viewport on
// small/phone screens and their top gets clipped with no way to scroll to it. This
// caps the height viewport-relative and lets the inner content scroll. Keep action
// buttons (submit/approve/deny) OUTSIDE this wrapper so they stay visible.
interface Props extends HTMLAttributes<HTMLDivElement> {
  // Tailwind max-height class controlling the scroll threshold. Viewport-relative
  // (e.g. 'max-h-[55vh]') so it adapts to screen size.
  maxH?: string
}

export function ScrollableCard({ maxH = 'max-h-[55vh]', className = '', ...props }: Props) {
  return <div className={`${maxH} overflow-y-auto ${className}`} {...props} />
}
