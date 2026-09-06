import type { ReactNode } from 'react'
import { PanelLeftClose, Plus, RefreshCw } from 'lucide-react'

// Shared chrome for every secondary sidebar (the per-screen list column), so the
// header row, the "new item" button, the refresh control and the drag handle all
// look and behave identically across screens instead of each panel rolling its
// own. See useResizableSidebar for the width side.

// SELECTED_ITEM_CLS is the ONE selected-list-item look used app-wide: a soft
// accent tint. Multi-select adds SELECTED_ITEM_RING on top so a range selection
// stays distinguishable from the single focused item.
export const SELECTED_ITEM_CLS = 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
export const SELECTED_ITEM_RING = 'ring-1 ring-[var(--color-accent)]'

// SidebarHeader is the top row of a list column: an uppercase title on the left
// and optional actions (typically a RefreshButton) on the right. Pass `onCollapse`
// (the useCollapsibleList toggle) to get the standard in-panel collapse button:
// on md+ it folds the docked column into the reopen rail, on narrow it closes
// the drawer (the backdrop does the same).
export function SidebarHeader({
  title,
  onCollapse,
  children,
}: {
  title: string
  onCollapse?: () => void
  children?: ReactNode
}) {
  return (
    <div className="flex items-center justify-between px-4 pt-4 pb-1">
      <span className="min-w-0 truncate text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
        {title}
      </span>
      {(children || onCollapse) && (
        <div className="flex shrink-0 items-center gap-1">
          {children}
          {onCollapse && <CollapseListButton onClick={onCollapse} />}
        </div>
      )}
    </div>
  )
}

// CollapseListButton is the icon-only "hide this list" control used by every
// list-column header (SidebarHeader and the custom sessions/insight headers).
export function CollapseListButton({
  onClick,
  title = 'Listeyi gizle',
}: {
  onClick: () => void
  title?: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={title}
      aria-label={title}
      data-testid="list-collapse"
      className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
    >
      <PanelLeftClose size={14} />
    </button>
  )
}

// RefreshButton is the standard icon-only reload control for a sidebar header.
export function RefreshButton({
  onClick,
  title = 'Yenile',
}: {
  onClick: () => void
  title?: string
}) {
  return (
    <button
      onClick={onClick}
      title={title}
      className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
    >
      <RefreshCw size={14} />
    </button>
  )
}

// NewItemButton is the prominent full-width "create" button that sits at the top
// of a list column (new chat / agent / flow / artifact / skill / market item …).
export function NewItemButton({
  onClick,
  label,
  disabled,
  title,
  testId,
  // bare = render just the button (no px-3 wrapper); caller controls spacing.
  bare,
  className,
}: {
  onClick: () => void
  label: string
  disabled?: boolean
  title?: string
  testId?: string
  bare?: boolean
  className?: string
}) {
  const btn = (
    <button
      onClick={onClick}
      disabled={disabled}
      data-testid={testId}
      title={title ?? label}
      className={`flex w-full items-center justify-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:cursor-not-allowed disabled:opacity-40 ${bare ? (className ?? '') : ''}`}
    >
      <Plus size={15} /> {label}
    </button>
  )
  if (bare) return btn
  return <div className={`px-3 pb-1 pt-1 ${className ?? ''}`}>{btn}</div>
}

// ResizeHandle is the invisible drag strip on the edge of a resizable column.
// Wire onMouseDown to useResizableSidebar's startDrag. `side` picks the edge:
// 'right' (default) for a left-hand list column, 'left' for a right-hand panel.
export function ResizeHandle({
  onMouseDown,
  side = 'right',
}: {
  onMouseDown: (e: React.MouseEvent) => void
  side?: 'left' | 'right'
}) {
  return (
    <div
      onMouseDown={onMouseDown}
      title="Genişliği ayarla"
      className={`absolute top-0 h-full w-1 cursor-col-resize bg-transparent transition hover:bg-[var(--color-accent)] ${
        side === 'left' ? 'left-0' : 'right-0'
      }`}
    />
  )
}
