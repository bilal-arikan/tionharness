import type { ReactNode } from 'react'
import { useState } from 'react'
import { useOutsideClick } from '@/shared/hooks/useOutsideClick'
import type { PickerOption } from './pickerOptions'

interface Props {
  value: string
  onChange?: (v: string) => void
  options: PickerOption[]
  // Menu heading + the trigger button's title() text.
  header: string
  title: (current: PickerOption) => string
  // Optional fixed leading glyph for the trigger button (e.g. a Brain icon). When
  // omitted the current option's own `icon` is used instead.
  triggerIcon?: ReactNode
  // When true the trigger shows only the icon (no text label) — the current value
  // is conveyed by the glyph alone; the full label/hint stays in the tooltip.
  iconOnly?: boolean
  menuWidthClass?: string
}

// ComposerPicker is the shared compact dropdown used by the composer's per-turn
// selectors (reasoning level, permission mode): a small trigger button that opens
// an option menu above it. The choice applies to the next message; the menu closes
// on select or outside click. Active (non-default) selections get an accent look.
export function ComposerPicker({
  value,
  onChange,
  options,
  header,
  title,
  triggerIcon,
  iconOnly,
  menuWidthClass = 'w-56',
}: Props) {
  const [open, setOpen] = useState(false)
  const rootRef = useOutsideClick<HTMLDivElement>(() => setOpen(false), open)
  const current = options.find((o) => o.value === value) ?? options[0]
  const leading = triggerIcon ?? (current.icon ? <span>{current.icon}</span> : null)

  return (
    <div ref={rootRef} className="relative shrink-0">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        title={title(current)}
        className={`flex items-center gap-1 rounded-xl border px-2.5 py-3 text-sm transition ${
          value
            ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
            : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
        }`}
      >
        {leading}
        {!iconOnly && <span className="hidden sm:inline">{current.label}</span>}
      </button>

      {open && (
        <div
          className={`absolute bottom-full left-0 mb-2 ${menuWidthClass} rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-2)] p-1 shadow-xl`}
        >
          <div className="px-2 py-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
            {header}
          </div>
          {options.map((o) => (
            <button
              key={o.value || 'default'}
              type="button"
              disabled={o.disabled}
              aria-disabled={o.disabled || undefined}
              title={o.disabled ? o.hint : undefined}
              onClick={() => {
                if (o.disabled) return
                onChange?.(o.value)
                setOpen(false)
              }}
              className={`flex w-full items-center justify-between gap-2 rounded-lg px-2 py-1.5 text-left text-sm ${
                o.disabled
                  ? 'cursor-not-allowed opacity-40'
                  : o.value === value
                    ? 'bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                    : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface)]'
              }`}
            >
              <span className="flex items-center gap-1.5 font-medium text-[var(--color-text)]">
                {o.icon && <span>{o.icon}</span>}
                {o.label}
              </span>
              <span className="truncate text-xs opacity-60">{o.hint}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
