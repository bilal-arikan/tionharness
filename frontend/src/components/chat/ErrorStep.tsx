import type { TurnStep } from '../../types'

interface Props {
  step: TurnStep
}

// ErrorStep renders a turn-level failure (provider error, budget exceeded,
// cancellation) inline — visually stronger than a recovery notice.
export function ErrorStep({ step }: Props) {
  return (
    <div className="flex items-start gap-2 rounded-md border border-red-500/40 bg-red-500/10 px-3 py-1.5 text-xs text-red-300">
      <span className="shrink-0">⛔</span>
      <span className="min-w-0 flex-1 whitespace-pre-wrap break-words">{step.text || 'Hata'}</span>
      {step.reason && (
        <span className="shrink-0 rounded bg-red-500/20 px-1.5 py-0.5 font-mono text-[10px] opacity-80">
          {step.reason}
        </span>
      )}
    </div>
  )
}
