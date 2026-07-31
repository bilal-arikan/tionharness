// FacetDropdown — one filter facet rendered as a labelled button that opens a
// checklist (multi-select) or a radio list (single-select). Shared by every
// facet in the board filter bar so they behave identically.

import { useState, type ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'
import { useOutsideClick } from '@/shared/hooks/useOutsideClick'

export interface FacetOption {
  value: string
  label: string
  /** Optional hex accent drawn as a dot before the label. */
  color?: string
  /** Cards currently matching this option, shown greyed after the label. */
  count?: number
  /** Rendered instead of the plain label (e.g. an agent identity row). */
  node?: ReactNode
}

interface Props {
  label: string
  options: FacetOption[]
  /** Selected values. For single-select facets this holds 0 or 1 entry. */
  selected: string[]
  onChange: (next: string[]) => void
  /** 'single' turns the list into a radio group that toggles off on re-click. */
  mode?: 'multi' | 'single'
  /** Shown in place of the list when there is nothing to choose from. */
  emptyHint?: string
}

export function FacetDropdown({
  label,
  options,
  selected,
  onChange,
  mode = 'multi',
  emptyHint = 'Seçenek yok',
}: Props) {
  const [open, setOpen] = useState(false)
  const ref = useOutsideClick<HTMLDivElement>(() => setOpen(false), open)
  const active = selected.length > 0

  const toggle = (value: string) => {
    if (mode === 'single') {
      onChange(selected[0] === value ? [] : [value])
      setOpen(false)
      return
    }
    onChange(selected.includes(value) ? selected.filter((v) => v !== value) : [...selected, value])
  }

  // Active facets show what they are narrowing to, not just their name — the
  // whole point of the bar is that the current subset is legible at a glance.
  const summary = () => {
    if (!active) return label
    if (selected.length === 1) {
      const opt = options.find((o) => o.value === selected[0])
      return `${label}: ${opt?.label ?? selected[0]}`
    }
    return `${label}: ${selected.length}`
  }

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        className={`flex items-center gap-1 whitespace-nowrap rounded border px-2 py-1 text-xs transition ${
          active
            ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
            : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]'
        }`}
      >
        {summary()}
        <ChevronDown size={12} />
      </button>
      {open && (
        <div className="absolute left-0 z-30 mt-1 max-h-72 w-56 overflow-y-auto rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-1 shadow-[var(--shadow-md)]">
          {options.length === 0 ? (
            <div className="px-2 py-3 text-center text-xs text-[var(--color-text-dim)]">
              {emptyHint}
            </div>
          ) : (
            options.map((opt) => {
              const on = selected.includes(opt.value)
              return (
                <button
                  key={opt.value}
                  onClick={() => toggle(opt.value)}
                  className={`flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-xs transition hover:bg-[var(--color-surface-2)] ${
                    on ? 'text-[var(--color-accent)]' : 'text-[var(--color-text)]'
                  }`}
                >
                  <span
                    className={`flex h-3 w-3 flex-shrink-0 items-center justify-center border text-[9px] ${
                      mode === 'single' ? 'rounded-full' : 'rounded-[3px]'
                    } ${
                      on
                        ? 'border-[var(--color-accent)] bg-[var(--color-accent)] text-[var(--color-bg)]'
                        : 'border-[var(--color-border)]'
                    }`}
                  >
                    {on ? '✓' : ''}
                  </span>
                  {opt.color && (
                    <span
                      className="h-2 w-2 flex-shrink-0 rounded-full"
                      style={{ backgroundColor: opt.color }}
                    />
                  )}
                  <span className="min-w-0 flex-1 truncate">{opt.node ?? opt.label}</span>
                  {opt.count !== undefined && (
                    <span className="flex-shrink-0 text-[10px] text-[var(--color-text-dim)]">
                      {opt.count}
                    </span>
                  )}
                </button>
              )
            })
          )}
          {active && (
            <button
              onClick={() => {
                onChange([])
                setOpen(false)
              }}
              className="mt-1 w-full rounded px-2 py-1.5 text-left text-xs text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)]"
            >
              ✕ Temizle
            </button>
          )}
        </div>
      )}
    </div>
  )
}
