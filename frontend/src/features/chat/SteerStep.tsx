import { CornerDownRight } from 'lucide-react'
import type { TurnStep } from '@/types'

interface Props {
  step: TurnStep
}

// SteerStep renders live user guidance folded into a running turn — distinct
// from model narration so the conversation history stays legible.
export function SteerStep({ step }: Props) {
  return (
    <div className="flex items-start gap-2 rounded-md border border-[var(--color-accent)]/40 bg-[var(--color-accent)]/10 px-3 py-1.5 text-xs text-[var(--color-text)]">
      <CornerDownRight size={14} className="mt-0.5 shrink-0 text-[var(--color-accent)]" />
      <span className="min-w-0 flex-1 whitespace-pre-wrap break-words">
        <span className="mr-1 text-[var(--color-text-dim)]">Yönlendirme:</span>
        {step.text}
      </span>
    </div>
  )
}
