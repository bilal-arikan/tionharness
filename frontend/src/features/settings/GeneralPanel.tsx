import { Toggle } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function GeneralPanel({ draft, set }: PanelProps) {
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">
        Applies to all workspaces after saving.
      </p>
      <Toggle
        label="Keep screen awake"
        hint="Prevent the screen from sleeping while the app is open. Applies to all workspaces after saving."
        checked={draft.keepAwake}
        onChange={(value) => set('keepAwake', value)}
      />
      <Toggle
        label="Generate titles automatically"
        hint="Name new conversations and tasks automatically."
        checked={draft.autoTitleEnabled}
        onChange={(value) => set('autoTitleEnabled', value)}
      />
      <Toggle
        label="Tag sessions automatically"
        hint="Add tags for errors, goals, completion and archival. Manual and agent-applied tags remain available."
        checked={draft.autoTagSessions}
        onChange={(v) => set('autoTagSessions', v)}
      />
    </>
  )
}
