import type { ReactNode } from 'react'
import { Menu } from 'lucide-react'

interface Props {
  // Screen title shown in the header (e.g. "Ajanlar").
  title: string
  // When provided, a left toggle button appears (like the chat sessions
  // hamburger) that shows/hides the screen's left list panel.
  listOpen?: boolean
  onToggleList?: () => void
  // Optional secondary line after the title (e.g. the selected agent name),
  // mirroring the chat header's "· Ajan" subtitle.
  subtitle?: ReactNode
  // Optional right-aligned actions.
  right?: ReactNode
}

// PaneHeader is the standard top bar for every list screen (Agents, Flows,
// Artifacts, Tools, Market, Skills, Executions …). It matches the chat header:
// a left toggle for the list panel + a title, optional subtitle, and right-side
// actions. Screens render it above their two-column body so the whole app shares
// one header look and one "open the left panel" affordance.
export function PaneHeader({ title, listOpen, onToggleList, subtitle, right }: Props) {
  return (
    <header className="flex shrink-0 items-center justify-between gap-2 border-b border-[var(--color-border)] py-3 max-md:px-3 md:px-6">
      <div className="flex min-w-0 items-center gap-2">
        {onToggleList && (
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
        )}
        <span className="shrink-0 truncate text-sm font-semibold">{title}</span>
        {subtitle && (
          <span className="truncate text-sm text-[var(--color-text-dim)]">{subtitle}</span>
        )}
      </div>
      {right && <div className="flex shrink-0 items-center gap-2">{right}</div>}
    </header>
  )
}
