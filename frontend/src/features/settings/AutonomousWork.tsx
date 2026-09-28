import { Toggle, NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function AutonomousWork({ draft, set }: PanelProps) {
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">
        These controls apply to autonomous work. Interactive conversations are unaffected.
      </p>
      <Toggle
        label="Confine autonomous file operations"
        hint="Restrict supported file tools to the working directory and block git push. Shell commands and arbitrary scripts can still access other paths; this is not a sandbox."
        checked={draft.autonomousConfine}
        onChange={(v) => set('autonomousConfine', v)}
      />
      <Toggle
        label="Autonomous startup checklist"
        hint="Remind autonomous agents to orient themselves, verify a baseline, complete one task and close the loop."
        checked={draft.autonomousBootSeq}
        onChange={(v) => set('autonomousBootSeq', v)}
      />
      <Toggle
        label="Continue unfinished autonomous work"
        hint="Resume unfinished autonomous turns automatically. Manually stopped or guard-limited runs are excluded."
        checked={draft.autonomousAutoContinue}
        onChange={(v) => set('autonomousAutoContinue', v)}
      />
      {draft.autonomousAutoContinue && (
        <NumberField
          label="Maximum continuation turns"
          hint="0 uses the built-in default."
          min={0}
          value={draft.autonomousAutoContinueMax}
          onChange={(v) => set('autonomousAutoContinueMax', v)}
        />
      )}
    </>
  )
}
