import type { TurnStep } from '@/types'
import { STEP_KIND_MAP } from '@/shared/stepKinds'

interface Props {
  step: TurnStep
}

// HookStep renders a PreToolUse/PostToolUse hook firing around a tool call as a
// subtle single-line audit notice. A blocking hook is shown in the danger tone;
// modify/allow/context actions in a neutral accent tone. The machine decision
// tag (hook_block / hook_modify / hook_allow / hook_context) sits in reason.
export function HookStep({ step }: Props) {
  const blocked = step.isError || step.reason === 'hook_block'
  const text = step.text?.trim() || step.reason || 'Hook'
  const tone = blocked
    ? 'border-[color-mix(in_srgb,var(--color-danger)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_12%,transparent)] text-[var(--color-danger)]'
    : 'border-[color-mix(in_srgb,var(--color-accent)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_10%,transparent)] text-[var(--color-accent)]'
  const Icon = blocked ? STEP_KIND_MAP.error.Icon : STEP_KIND_MAP.hook.Icon
  return (
    <div className={`flex items-center gap-2 rounded-md border px-3 py-1.5 text-xs ${tone}`}>
      <Icon size={14} className="shrink-0" />
      <span className="min-w-0 flex-1">
        {text}
        {step.tool && <span className="ml-1 opacity-70">· {step.tool}</span>}
      </span>
      {step.reason && (
        <span className="shrink-0 rounded bg-[color-mix(in_srgb,currentColor_18%,transparent)] px-1.5 py-0.5 font-mono text-[10px] opacity-80">
          {step.reason}
        </span>
      )}
    </div>
  )
}
