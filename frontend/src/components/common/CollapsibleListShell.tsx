import type { ReactNode } from 'react'

interface Props {
  // Mobile drawer open state. On `md+` the list is ALWAYS visible regardless of
  // this value (a static column); `open` only drives the portrait-phone drawer.
  open: boolean
  onToggle: () => void
  // The list column (the panel's <aside>/<div> with its own width + border).
  children: ReactNode
  // Kept for API compatibility with older callers; no longer used (there is no
  // reopen rail — the list is always present on desktop and a drawer on mobile).
  label?: string
  testId?: string
  hideRail?: boolean
}

// CollapsibleListShell renders a screen's left list EXACTLY like the chat sessions
// sidebar:
//   • md+ (wide): a static, always-visible column — never collapses, no rail.
//   • < md (portrait phone): a left slide-in drawer over the content, toggled by an
//     external header hamburger (md:hidden). `open` drives only the drawer's slide;
//     a dim backdrop appears while it is open.
// The list is ALWAYS in the DOM (like the chat sidebar); mobile visibility is pure
// CSS transform, so on desktop the panel is always open by default.
export function CollapsibleListShell({ open, onToggle, children }: Props) {
  return (
    <>
      {/* Mobile-only dim backdrop while the drawer is open. */}
      {open && <div className="fixed inset-0 z-30 bg-black/50 md:hidden" onClick={onToggle} />}
      {/* Desktop: static column (always visible). Mobile: fixed left drawer that
          slides in/out with `open`. Solid surface + shadow so it never shows the
          content/backdrop through it. */}
      <div
        className={`relative flex shrink-0 bg-[var(--color-surface)] shadow-[var(--shadow-sm)] md:static md:translate-x-0 max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:z-40 max-md:shadow-xl max-md:transition-transform ${
          open ? 'max-md:translate-x-0' : 'max-md:-translate-x-full'
        }`}
      >
        {children}
      </div>
    </>
  )
}
