import { Toggle, NumberField } from './primitives'
import type { PanelProps } from './settingsPanelShared'

export function AnthropicOptions({ draft, set }: PanelProps) {
  return (
    <>
      <p className="text-xs text-[var(--color-text-dim)]">
        These options apply only to supported Anthropic API models, not Claude CLI.
      </p>
      <Toggle
        label="Extended prompt cache (1 hour)"
        hint="Keep reusable prompt prefixes cached for longer."
        checked={draft.extendedPromptCache}
        onChange={(v) => set('extendedPromptCache', v)}
      />
      <Toggle
        label="Server-side context editing"
        hint="Trim older tool results while preserving the cached prefix."
        checked={draft.anthropicContextEditing}
        onChange={(v) => set('anthropicContextEditing', v)}
      />
      <Toggle
        label="Native tool search"
        hint="Let supported Anthropic models discover deferred tools on the server."
        checked={draft.anthropicNativeToolSearch}
        onChange={(v) => set('anthropicNativeToolSearch', v)}
      />
      <Toggle
        label="Programmatic tool calls"
        hint="Run tool orchestration in the provider container. Local code mode is a separate capability; permission checks still apply."
        checked={draft.anthropicProgrammaticTools}
        onChange={(v) => set('anthropicProgrammaticTools', v)}
      />
      <Toggle
        label="Server-side web tools"
        hint="Enable provider-hosted search and page retrieval for agents with tools enabled. Usage may incur additional charges."
        checked={draft.anthropicWebTools}
        onChange={(v) => set('anthropicWebTools', v)}
      />
      <Toggle
        label="Server-side compaction"
        hint="Compact long tool loops on the server. Client-side conversation compaction remains separate."
        checked={draft.anthropicServerCompaction}
        onChange={(v) => set('anthropicServerCompaction', v)}
      />
      <Toggle
        label="Refusal fallback"
        hint="Use the configured API fallback for supported Fable/Mythos requests."
        checked={draft.anthropicRefusalFallback}
        onChange={(v) => set('anthropicRefusalFallback', v)}
      />
      <NumberField
        label="Autonomous task budget (tokens)"
        hint="0 disables the API task budget. Supported adaptive models use at least 20,000 tokens when enabled."
        min={0}
        step={1000}
        value={draft.autonomousTaskBudgetTokens}
        onChange={(v) => set('autonomousTaskBudgetTokens', v)}
      />
    </>
  )
}
