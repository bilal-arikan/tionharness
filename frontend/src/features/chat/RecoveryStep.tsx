import type { TurnStep } from '@/types'
import { STEP_KIND_MAP } from '@/shared/stepKinds'

const HeaderIcon = STEP_KIND_MAP.recovery.Icon

interface Props {
  step: TurnStep
}

// RecoveryStep renders a non-happy-path branch of the agent loop (e.g. hitting
// the tool-iteration cap) as a subtle, single-line notice — distinct from
// regular narration so the user understands why a turn ended early.
export function RecoveryStep({ step }: Props) {
  const text = step.text?.trim() || step.reason || 'Kurtarma'
  return (
    <div className="flex items-center gap-2 rounded-md border border-[color-mix(in_srgb,var(--color-warning)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_12%,transparent)] px-3 py-1.5 text-xs text-[var(--color-warning)]">
      <HeaderIcon size={14} className="shrink-0" />
      <span className="min-w-0 flex-1">{text}</span>
      {step.reason && (
        <span className="shrink-0 rounded bg-[color-mix(in_srgb,var(--color-warning)_22%,transparent)] px-1.5 py-0.5 font-mono text-[10px] opacity-80">
          {step.reason}
        </span>
      )}
    </div>
  )
}
