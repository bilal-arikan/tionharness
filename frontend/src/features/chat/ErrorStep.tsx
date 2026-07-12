import type { TurnStep } from '@/types'
import { STEP_KIND_MAP } from '@/shared/stepKinds'

const HeaderIcon = STEP_KIND_MAP.error.Icon

interface Props {
  step: TurnStep
}

// ErrorStep renders a turn-level failure (provider error, budget exceeded,
// cancellation) inline — visually stronger than a recovery notice.
export function ErrorStep({ step }: Props) {
  return (
    <div className="flex items-start gap-2 rounded-md border border-[color-mix(in_srgb,var(--color-danger)_45%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_12%,transparent)] px-3 py-1.5 text-xs text-[var(--color-danger)]">
      <HeaderIcon size={14} className="mt-0.5 shrink-0" />
      <span className="min-w-0 flex-1 whitespace-pre-wrap break-words">{step.text || 'Hata'}</span>
      {step.reason && (
        <span className="shrink-0 rounded bg-[color-mix(in_srgb,var(--color-danger)_22%,transparent)] px-1.5 py-0.5 font-mono text-[10px] opacity-80">
          {step.reason}
        </span>
      )}
    </div>
  )
}
