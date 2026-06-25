export interface PillOption {
  value: string
  label: string
  hint?: string
  // A leading glyph (emoji or symbol) shown before the label, matching the
  // composer's per-turn picker icon language.
  icon?: string
}

interface Props {
  value: string
  onChange: (value: string) => void
  options: PillOption[]
  // Accessible group label + the per-option data-testid prefix ("foo" →
  // group data-testid="foo", each pill data-testid="foo-option" + data-value).
  ariaLabel: string
  testid?: string
}

// OptionPills is an always-visible, icon-labeled segmented control — a more
// readable alternative to a <select> for a small, fixed set of choices. It mirrors
// the chat composer's per-turn pickers (icon + label, accent highlight on the
// active choice) so the agent settings read the same way.
export function OptionPills({ value, onChange, options, ariaLabel, testid }: Props) {
  return (
    <div role="radiogroup" aria-label={ariaLabel} data-testid={testid} className="flex flex-wrap gap-1.5">
      {options.map((o) => {
        const active = o.value === value
        return (
          <button
            key={o.value || '_default'}
            type="button"
            role="radio"
            aria-checked={active}
            data-testid={testid ? `${testid}-option` : undefined}
            data-value={o.value}
            onClick={() => onChange(o.value)}
            title={o.hint}
            className={`flex items-center gap-1.5 rounded-lg border px-2.5 py-1.5 text-sm transition ${
              active
                ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:border-[var(--color-accent)] hover:text-[var(--color-text)]'
            }`}
          >
            {o.icon && <span className="text-base leading-none">{o.icon}</span>}
            <span className="font-medium">{o.label}</span>
          </button>
        )
      })}
    </div>
  )
}
