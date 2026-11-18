// Complete class sets per state: two bg-* utilities on one element are resolved
// by stylesheet order, not class-string order, so "base + override" would lose.
const TRACK_ON = 'block h-4 w-8 rounded-full bg-[var(--color-accent)] transition'
const TRACK_OFF = 'block h-4 w-8 rounded-full bg-[var(--color-border)] transition'
const KNOB_ON = 'block h-4 w-4 translate-x-4 rounded-full bg-[var(--color-text)] transition'
const KNOB_OFF = 'block h-4 w-4 rounded-full bg-[var(--color-text)] transition'

interface Props {
  enabled: boolean
  onToggle: () => void
  labelOn: string
  labelOff: string
  testId?: string
  // Rendered as data-schedule-id, the hook the cards' other controls use too.
  entityId?: string
}

// EnableToggle is the on/off switch of a board card (cron rule or automation).
// The visible track is always 32x16; below `md` the button around it is a 36px
// touch target, from `md` up it shrinks to the track so dense lanes stay dense.
export function EnableToggle({ enabled, onToggle, labelOn, labelOff, testId, entityId }: Props) {
  const label = enabled ? labelOn : labelOff
  return (
    <button
      type="button"
      role="switch"
      aria-checked={enabled}
      aria-label={label}
      title={label}
      onClick={onToggle}
      data-testid={testId}
      data-schedule-id={entityId}
      className="flex h-9 w-9 shrink-0 items-center justify-center md:h-4 md:w-8"
    >
      <span className={enabled ? TRACK_ON : TRACK_OFF}>
        <span className={enabled ? KNOB_ON : KNOB_OFF} />
      </span>
    </button>
  )
}
