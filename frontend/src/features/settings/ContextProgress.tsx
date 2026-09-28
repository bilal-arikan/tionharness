import { Toggle } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function ContextProgress({ draft, set }: PanelProps) {
  return (
    <>
      <Toggle
        label="Save progress to disk"
        hint="Persist the task checklist so it can be reused across sessions."
        checked={draft.progressPersist}
        onChange={(v) => set('progressPersist', v)}
      />
      <Toggle
        label="Restore progress in new sessions"
        hint="Load unfinished progress when a new session has no checklist of its own."
        checked={draft.progressResume}
        onChange={(v) => set('progressResume', v)}
      />
    </>
  )
}
