import { NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function CoordinatorLimits({ draft, set }: PanelProps) {
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">
        Worker capacity and nesting limits for multi-agent coordination. Automatic turns and total
        subtree sessions remain unlimited.
      </p>
      <NumberField
        label="Workers per coordinator"
        hint="Maximum active workers for each coordinator (1–64)."
        min={1}
        max={64}
        value={draft.coordinatorMaxWorkers}
        onChange={(v) => set('coordinatorMaxWorkers', v)}
      />
      <NumberField
        label="Coordinator depth"
        hint="Root depth is 0. Use -1 for unlimited nesting."
        min={-1}
        max={12}
        value={draft.coordinatorMaxDepth}
        onChange={(v) => set('coordinatorMaxDepth', v)}
      />
      <NumberField
        label="Report grace period (seconds)"
        hint="Wait this long after a branch becomes idle before sending a missing completion report. Allow enough time for synthesis."
        min={5}
        max={1800}
        value={draft.coordinatorSettleGraceSec}
        onChange={(v) => set('coordinatorSettleGraceSec', v)}
      />
    </>
  )
}
