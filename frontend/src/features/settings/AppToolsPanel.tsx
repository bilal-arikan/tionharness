import { Toggle } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function ToolsPanel({ draft, set }: PanelProps) {
  return (
    <>
      <Toggle
        label="Shell access"
        hint="Allow agents to run commands through the application shell. Applies to all workspaces after saving."
        checked={draft.enableShell}
        onChange={(v) => set('enableShell', v)}
      />
      {/* Individual tool visibility is managed in the workspace Tools screen. */}
      <Toggle
        label="Local code mode"
        hint="Let the native agent loop call MCP tools through generated Python bindings. Requires shell access; tool permission checks still apply."
        checked={draft.enableCodeMode}
        onChange={(v) => set('enableCodeMode', v)}
      />
      {draft.enableCodeMode && !draft.enableShell && (
        <div className="rounded-lg border border-[var(--color-warning)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
          <b>Code mode requires shell access.</b> Enable Shell access above to make this capability
          available.
        </div>
      )}
    </>
  )
}
