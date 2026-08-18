import { X, CheckSquare } from 'lucide-react'
import type { ReactNode } from 'react'

interface Props {
  /** Number of currently-selected items. The bar hides itself when this is 0. */
  count: number
  onClear: () => void
  /** Bulk-action buttons for this specific list (render <SelectionBarButton/>s). */
  children?: ReactNode
  /**
   * How many selected items are currently filtered out of view. Shown as a hint
   * so the user understands a bulk action still applies to off-screen items.
   */
  hiddenCount?: number
  /** Optional "select all (visible)" affordance. */
  onSelectAll?: () => void
  /** Pin the bar to the top instead of the bottom (default). */
  position?: 'top' | 'bottom'
}

// SelectionBar is the shared bulk-action strip that appears whenever a list has
// ≥1 multi-selected item. It owns no selection state itself — the count and the
// action buttons are supplied by the host list (driven by useMultiSelect).
export function SelectionBar({
  count,
  onClear,
  children,
  hiddenCount = 0,
  onSelectAll,
  position = 'bottom',
}: Props) {
  if (count === 0) return null
  return (
    <div
      className={`sticky ${position === 'top' ? 'top-0' : 'bottom-0'} z-10 flex flex-wrap items-center gap-2 border-t border-[var(--color-border)] bg-[var(--color-accent-soft)] px-3 py-2 text-sm`}
    >
      <button
        onClick={onClear}
        title="Seçimi temizle (Esc)"
        className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-text)]"
      >
        <X size={15} />
      </button>
      <span className="font-medium text-[var(--color-text)]">
        {count} seçili
        {hiddenCount > 0 && (
          <span className="ml-1 font-normal text-[var(--color-text-dim)]">
            (+{hiddenCount} filtre dışı)
          </span>
        )}
      </span>
      {onSelectAll && (
        <button
          onClick={onSelectAll}
          title="Görünen tümünü seç"
          className="flex items-center gap-1 rounded px-1.5 py-0.5 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
        >
          <CheckSquare size={13} /> Tümü
        </button>
      )}
      <div className="ml-auto flex flex-wrap items-center gap-1.5">{children}</div>
    </div>
  )
}

interface ButtonProps {
  onClick: () => void
  icon?: ReactNode
  children: ReactNode
  danger?: boolean
  disabled?: boolean
  title?: string
}

// SelectionBarButton is a compact action button styled for the SelectionBar.
export function SelectionBarButton({
  onClick,
  icon,
  children,
  danger,
  disabled,
  title,
}: ButtonProps) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      title={title}
      className={`flex items-center gap-1.5 rounded-md border px-2.5 py-1 text-xs font-medium transition disabled:cursor-not-allowed disabled:opacity-40 ${
        danger
          ? 'border-[color-mix(in_srgb,var(--color-danger)_40%,transparent)] text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_12%,transparent)]'
          : 'border-[var(--color-border)] text-[var(--color-text)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]'
      }`}
    >
      {icon}
      {children}
    </button>
  )
}
