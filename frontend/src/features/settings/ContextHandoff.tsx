import { Toggle, NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function ContextHandoff({ draft, set }: PanelProps) {
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">
        Resume long autonomous tasks in a fresh context. Manual handoff remains available
        independently.
      </p>
      <Toggle
        label="Automatic context handoff"
        hint="Start a fresh session when an autonomous turn reaches the context limit. Manual chat is unaffected."
        checked={draft.handoffAuto}
        onChange={(v) => set('handoffAuto', v)}
      />
      <NumberField
        label="Maximum handoff chain"
        hint="Maximum consecutive resets before falling back to regular compaction."
        min={1}
        max={100}
        value={draft.handoffMaxChain || 20}
        onChange={(v) => set('handoffMaxChain', v)}
      />
      <Toggle
        label="Write a handoff file"
        hint="Also write .tionharness/handoff.md in the working directory."
        checked={draft.handoffWriteFile}
        onChange={(v) => set('handoffWriteFile', v)}
      />
    </>
  )
}
