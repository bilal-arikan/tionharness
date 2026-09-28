import { NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function DelegationLimits({ draft, set }: PanelProps) {
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">
        Limits for synchronous agent delegation. Individual tool availability is managed in the
        Tools screen.
      </p>
      <NumberField
        label="Maximum delegation depth"
        hint="Maximum nesting for synchronous agent delegation (1–10)."
        min={1}
        max={10}
        value={draft.delegationMaxDepth}
        onChange={(v) => set('delegationMaxDepth', v)}
      />
      <NumberField
        label="Delegations per turn"
        hint="Maximum synchronous delegation calls in one user turn (1–100)."
        min={1}
        max={100}
        value={draft.delegationMaxCalls}
        onChange={(v) => set('delegationMaxCalls', v)}
      />
    </>
  )
}
