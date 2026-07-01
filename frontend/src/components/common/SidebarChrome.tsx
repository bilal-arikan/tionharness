import type { ReactNode } from 'react'
import { Plus, RefreshCw } from 'lucide-react'

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
// and optional actions (typically a RefreshButton) on the right.
export function SidebarHeader({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="flex items-center justify-between px-4 pt-4 pb-1">
      <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
        {title}
      </span>
      {children && <div className="flex items-center gap-1">{children}</div>}
    </div>
  )
}

// RefreshButton is the standard icon-only reload control for a sidebar header.
export function RefreshButton({ onClick, title = 'Yenile' }: { onClick: () => void; title?: string }) {
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

// ResizeHandle is the invisible drag strip on the right edge of a list column.
// Wire onMouseDown to useResizableSidebar's startDrag.
export function ResizeHandle({ onMouseDown }: { onMouseDown: (e: React.MouseEvent) => void }) {
  return (
    <div
      onMouseDown={onMouseDown}
      title="Genişliği ayarla"
      className="absolute right-0 top-0 h-full w-1 cursor-col-resize bg-transparent transition hover:bg-[var(--color-accent)]"
    />
  )
}
