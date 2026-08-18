import type { ReactNode } from 'react'
import { Menu } from 'lucide-react'

interface Props {
  // Screen title shown in the header (e.g. "Ajanlar"). Optional: omit it when
  // passing `titleSlot` to render custom main content (e.g. a name input) in its
  // place.
  title?: string
  // When provided, replaces the title + subtitle block entirely with custom
  // content that fills the main (growing) area — used by detail editors that put
  // a name input + id chip directly in the top bar (e.g. the flow editor).
  titleSlot?: ReactNode
  // When provided, a left toggle button appears (like the chat sessions
  // hamburger) that shows/hides the screen's left list panel.
  listOpen?: boolean
  onToggleList?: () => void
  // Optional secondary line after the title (e.g. the selected agent name),
  // mirroring the chat header's "· Ajan" subtitle.
  subtitle?: ReactNode
  // Optional right-aligned actions.
  right?: ReactNode
  // Optional "secondary" group (chips / side-info + one movable control). On md+
  // it sits inline between the title and the right actions; on narrow screens it
  // wraps to a full-width SECOND ROW, while the title + right actions stay on the
  // first row. Used by detail headers (Skills/Artifacts) to keep the first row
  // uncluttered on phones.
  secondary?: ReactNode
  // When true, the `secondary` group ALWAYS sits on its own second row (at every
  // width), instead of only wrapping on narrow screens. Title + right actions
  // stay on the first row at all widths.
  secondaryAlwaysWrap?: boolean
}

// PaneHeader is the standard top bar for every list screen (Agents, Flows,
// Artifacts, Tools, Market, Skills, Executions …). It matches the chat header:
// a left toggle for the list panel + a title, optional subtitle, and right-side
// actions. Screens render it above their two-column body so the whole app shares
// one header look and one "open the left panel" affordance.
export function PaneHeader({
  title,
  titleSlot,
  listOpen,
  onToggleList,
  subtitle,
  right,
  secondary,
  secondaryAlwaysWrap,
}: Props) {
  const hamburger = onToggleList && (
    <button
      onClick={onToggleList}
      title={listOpen ? 'Listeyi gizle' : 'Listeyi göster'}
      aria-label="Liste panelini aç/kapat"
      aria-pressed={listOpen}
      data-testid="pane-list-toggle"
      className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] md:hidden"
    >
      <Menu size={18} />
    </button>
  )
  const titleContent = titleSlot ?? (
    <>
      {title && <span className="shrink-0 truncate text-sm font-semibold">{title}</span>}
      {subtitle && (
        <span className="truncate text-sm text-[var(--color-text-dim)]">{subtitle}</span>
      )}
    </>
  )

  // With a `secondary` group: three-slot flex-wrap layout. On md+ it's one row
  // (title · secondary · right); on narrow the secondary group breaks to a full
  // -width second row (basis-full + order-last) while title + right stay on row 1.
  if (secondary) {
    // secondaryAlwaysWrap: keep the second row at every width (no md: overrides).
    return (
      <header
        className={`flex shrink-0 flex-wrap items-center gap-x-2 gap-y-1.5 border-b border-[var(--color-border)] py-3 max-md:px-3 md:px-6 ${
          secondaryAlwaysWrap ? '' : 'md:flex-nowrap'
        }`}
      >
        <div
          className={`order-1 flex min-w-0 items-center gap-2 ${secondaryAlwaysWrap ? 'flex-1' : 'max-md:flex-1'}`}
        >
          {hamburger}
          {titleContent}
        </div>
        <div
          className={`order-last flex basis-full flex-wrap items-center gap-2 ${
            secondaryAlwaysWrap ? '' : 'md:order-2 md:min-w-0 md:flex-1 md:basis-auto'
          }`}
        >
          {secondary}
        </div>
        {right && (
          <div
            className={`order-2 flex shrink-0 items-center gap-2 ${secondaryAlwaysWrap ? '' : 'md:order-3'}`}
          >
            {right}
          </div>
        )}
      </header>
    )
  }

  return (
    <header className="flex shrink-0 items-center justify-between gap-2 border-b border-[var(--color-border)] py-3 max-md:px-3 md:px-6">
      <div className="flex min-w-0 flex-1 items-center gap-2">
        {hamburger}
        {titleContent}
      </div>
      {right && <div className="flex shrink-0 items-center gap-2">{right}</div>}
    </header>
  )
}
