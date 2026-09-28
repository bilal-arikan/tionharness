import { Toggle, NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function DiagnosticsPanel({ draft, set }: PanelProps) {
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">
        Applies to all workspaces after saving.
      </p>
      <Toggle
        label="Record diagnostic events"
        hint="Record per-session timing, token usage, tool outcomes and recovery events."
        checked={draft.debugJournalEnabled}
        onChange={(v) => set('debugJournalEnabled', v)}
      />
      <NumberField
        label="Diagnostic events to retain"
        hint="Keep the newest events per session. 0 uses the default of 5,000."
        min={0}
        value={draft.debugJournalCap}
        onChange={(v) => set('debugJournalCap', v)}
      />
    </>
  )
}
