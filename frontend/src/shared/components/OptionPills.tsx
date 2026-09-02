import { useRef } from 'react'

export interface PillOption {
  value: string
  label: string
  hint?: string
  // A leading glyph (emoji or symbol) shown before the label, matching the
  // composer's per-turn picker icon language.
  icon?: string
  // When true the pill is shown greyed and non-selectable (e.g. a reasoning tier
  // the current model can't honour). `hint` then carries the reason (tooltip).
  disabled?: boolean
}

interface Props {
  value: string
  onChange: (value: string) => void
  options: PillOption[]
  // Accessible group label + the per-option data-testid prefix ("foo" →
  // group data-testid="foo", each pill data-testid="foo-option" + data-value).
  ariaLabel: string
  ariaDescribedBy?: string
  testid?: string
}

// OptionPills is an always-visible, icon-labeled segmented control — a more
// readable alternative to a <select> for a small, fixed set of choices. It mirrors
// the chat composer's per-turn pickers (icon + label, accent highlight on the
// active choice) so the agent settings read the same way.
export function OptionPills({
  value,
  onChange,
  options,
  ariaLabel,
  ariaDescribedBy,
  testid,
}: Props) {
  const optionRefs = useRef<Array<HTMLButtonElement | null>>([])
  const enabledIndexes = options.flatMap((option, index) => (option.disabled ? [] : [index]))
  const selectedIndex = options.findIndex((option) => option.value === value && !option.disabled)
  const tabStopIndex = selectedIndex >= 0 ? selectedIndex : (enabledIndexes[0] ?? -1)

  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      aria-describedby={ariaDescribedBy}
      data-testid={testid}
      className="flex flex-wrap gap-1.5"
    >
      {options.map((o, index) => {
        const active = o.value === value
        const disabled = !!o.disabled
        return (
          <button
            key={o.value || '_default'}
            ref={(node) => {
              optionRefs.current[index] = node
            }}
            type="button"
            role="radio"
            aria-checked={active}
            aria-disabled={disabled || undefined}
            disabled={disabled}
            tabIndex={index === tabStopIndex ? 0 : -1}
            data-testid={testid ? `${testid}-option` : undefined}
            data-value={o.value}
            onClick={() => {
              if (!disabled) onChange(o.value)
            }}
            onKeyDown={(event) => {
              if (
                !['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'].includes(
                  event.key,
                )
              ) {
                return
              }
              event.preventDefault()
              const current = enabledIndexes.indexOf(index)
              const next =
                event.key === 'Home'
                  ? 0
                  : event.key === 'End'
                    ? enabledIndexes.length - 1
                    : event.key === 'ArrowRight' || event.key === 'ArrowDown'
                      ? (current + 1) % enabledIndexes.length
                      : (current - 1 + enabledIndexes.length) % enabledIndexes.length
              const nextIndex = enabledIndexes[next]
              if (nextIndex === undefined) return
              onChange(options[nextIndex].value)
              optionRefs.current[nextIndex]?.focus()
            }}
            title={o.hint}
            className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-sm transition ${
              disabled
                ? 'cursor-not-allowed border-[var(--color-border)] text-[var(--color-text-dim)] opacity-40'
                : active
                  ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                  : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-text)]'
            }`}
          >
            {o.icon && (
              <span aria-hidden="true" className="text-base leading-none">
                {o.icon}
              </span>
            )}
            <span className="font-medium">{o.label}</span>
          </button>
        )
      })}
    </div>
  )
}
