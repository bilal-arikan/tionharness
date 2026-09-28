import { Toggle, NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function ContextRecovery({ draft, set }: PanelProps) {
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">
        Recovery controls for the native provider loop. CLI providers manage their own loops.
      </p>
      <Toggle
        label="Compact after context overflow"
        hint="Summarize older messages and retry when the context window overflows."
        checked={draft.reactiveCompact}
        onChange={(v) => set('reactiveCompact', v)}
      />
      <NumberField
        label="Output continuation attempts"
        hint="Continue responses that reach the output limit. 0 keeps the partial response without retrying."
        min={0}
        value={draft.maxTokenRetries}
        onChange={(v) => set('maxTokenRetries', v)}
      />
      <NumberField
        label="Recent messages to retain on recovery"
        hint="Messages retained verbatim during reactive compaction (2–50)."
        min={2}
        max={50}
        value={draft.reactiveKeepRecent}
        onChange={(v) => set('reactiveKeepRecent', v)}
      />
      <NumberField
        label="Output token limit"
        hint="0 selects a model-specific limit. A positive value applies a fixed cap."
        min={0}
        value={draft.maxOutputTokens}
        onChange={(v) => set('maxOutputTokens', v)}
      />
      <NumberField
        label="Provider retry budget"
        hint="Retry temporary provider errors within a turn (0–5). Permanent authentication or quota errors are not retried."
        min={0}
        max={5}
        value={draft.maxProviderRetries}
        onChange={(v) => set('maxProviderRetries', v)}
      />
    </>
  )
}
