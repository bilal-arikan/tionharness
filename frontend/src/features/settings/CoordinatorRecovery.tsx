import { Toggle, NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function CoordinatorRecovery({ draft, set }: PanelProps) {
  return (
    <>
      <Toggle
        label="Detect stalled coordinators"
        hint="Use a judge and periodic checks to recover coordinators that claim to have started workers without actually starting them."
        checked={draft.coordinatorStallGuard}
        onChange={(v) => set('coordinatorStallGuard', v)}
      />
      <NumberField
        label="Stall check window (minutes)"
        hint="0 uses the default window; -1 disables periodic checks while retaining the turn-end guard."
        min={-1}
        max={1440}
        value={draft.coordinatorStallSweepMin}
        onChange={(v) => set('coordinatorStallSweepMin', v)}
      />
      <NumberField
        label="Consecutive recovery nudges"
        hint="Maximum corrective nudges before reporting a visible failure. 0 uses the default."
        min={0}
        max={10}
        value={draft.coordinatorStallMaxNudges}
        onChange={(v) => set('coordinatorStallMaxNudges', v)}
      />
      <Toggle
        label="Show recovery notes in chat"
        hint="Only controls visibility. Recovery notes are always recorded and passed to the coordinator."
        checked={draft.coordinatorStallNoteVisible}
        onChange={(v) => set('coordinatorStallNoteVisible', v)}
      />
    </>
  )
}
