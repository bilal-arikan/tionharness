import { NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function ToolExecutionLimits({ draft, set }: PanelProps) {
  return (
    <>
      <NumberField
        label="Default shell timeout (seconds)"
        hint="Timeout used when a shell command does not specify one."
        min={1}
        max={3600}
        value={draft.shellDefaultTimeoutSec}
        onChange={(v) => set('shellDefaultTimeoutSec', v)}
      />
      <NumberField
        label="Maximum shell timeout (seconds)"
        hint="Upper limit for a command-specific shell timeout."
        min={1}
        max={3600}
        value={draft.shellMaxTimeoutSec}
        onChange={(v) => set('shellMaxTimeoutSec', v)}
      />
      <NumberField
        label="Tool output limit (KB)"
        hint="Truncate tool output beyond this size, including MCP output."
        min={1}
        max={4096}
        value={draft.maxToolOutputKB}
        onChange={(v) => set('maxToolOutputKB', v)}
      />
    </>
  )
}
