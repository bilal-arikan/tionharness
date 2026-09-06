import type { ReactNode } from 'react'
import { PanelLeftOpen } from 'lucide-react'
import { Backdrop } from './Backdrop'

interface Props {
  // Open state from useCollapsibleList: on md+ whether the docked column shows
  // (else the reopen rail); on narrow whether the drawer is slid in.
  open: boolean
  onToggle: () => void
  // The list column (the panel's <aside>/<div> with its own width + border).
  children: ReactNode
  // Label on the collapsed reopen rail (e.g. "Artifactlar"). Also its tooltip.
  label: string
  testId?: string
}

// CollapsibleListShell renders a screen's left list EXACTLY like the chat sessions
// sidebar, the ONE standard for every list column:
//   • md+ (docked): `open` shows the column; collapsed swaps it for a slim
//     vertical reopen rail (icon + rotated label) so the content pane gets the
//     width. The state is persisted by useCollapsibleList (default open).
//   • < md (drawer): a left slide-in drawer over the content, toggled by the
//     screen header's list button; `open` drives only the drawer's slide and a
//     dim backdrop appears while it is open.
// The column stays in the DOM on narrow (pure CSS transform) so the drawer can
// animate; on md+ it is unmounted while collapsed and re-enters with
// `.th-pane-enter` (styles/layout.css).
export function CollapsibleListShell({ open, onToggle, children, label, testId }: Props) {
  return (
    <>
      {/* Narrow-only dim backdrop while the drawer is open. */}
      {open && <Backdrop onClick={onToggle} className="md:hidden" />}

      {/* Docked reopen rail (md+ only, while collapsed). */}
      {!open && (
        <button
          type="button"
          onClick={onToggle}
          title={`${label} panelini aç`}
          aria-label={`${label} panelini aç`}
          data-testid={testId ? `${testId}-rail` : undefined}
          className="th-rail hidden h-full w-9 shrink-0 flex-col items-center gap-3 border-r border-[var(--color-border)] bg-[var(--color-surface)] pt-3 text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-accent)] md:flex"
        >
          <PanelLeftOpen size={16} className="shrink-0" />
          <span className="text-[10px] font-medium uppercase tracking-wide [writing-mode:vertical-rl]">
            {label}
          </span>
        </button>
      )}

      {/* Docked column (md+) / fixed left drawer (narrow). Solid surface + shadow
          so the drawer never shows the content through it. The drawer is
          inset-y-0 (full viewport height) so it reserves the MobileNavBar height
          at the bottom, keeping its last rows clickable. */}
      <div
        data-testid={testId}
        data-list-open={open ? '1' : '0'}
        className={`relative flex shrink-0 bg-[var(--color-surface)] shadow-[var(--shadow-sm)] md:static md:translate-x-0 max-md:fixed max-md:inset-y-0 max-md:left-0 max-md:z-40 max-md:pb-[calc(3.25rem+env(safe-area-inset-bottom))] max-md:shadow-xl th-drawer ${
          open ? 'th-pane-enter max-md:translate-x-0' : 'md:hidden max-md:-translate-x-full'
        }`}
      >
        {children}
      </div>
    </>
  )
}
