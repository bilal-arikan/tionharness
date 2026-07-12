import type { TurnStep } from '@/types'
import { STEP_KIND_MAP } from '@/shared/stepKinds'

const HeaderIcon = STEP_KIND_MAP.tool_delta.Icon

interface Props {
  step: TurnStep
}

// ToolDeltaStep renders the live, streaming output of a long-running tool as a
// monospace block. Transient: replaced by the final tool card once the turn's
// persisted trace arrives.
export function ToolDeltaStep({ step }: Props) {
  return (
    <div className="overflow-hidden rounded-md">
      <div className="flex items-center gap-2 px-3 py-1 text-xs text-[var(--color-text-dim)]">
        <HeaderIcon size={14} className="shrink-0" />
        <span className="font-medium text-[var(--color-text)]">{step.tool || 'Araç'}</span>
        <span className="ml-auto animate-pulse">çalışıyor…</span>
      </div>
      {step.output && (
        <pre className="max-h-48 overflow-auto border-t border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-xs text-[var(--color-text)]">
          {step.output}
        </pre>
      )}
    </div>
  )
}
