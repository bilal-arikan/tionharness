import { ArrowRight, RotateCcw } from 'lucide-react'
import type { AgentToolEntry, AgentToolTier } from '@/types'
import { AgentTierBadge, AgentTierSelector } from '@/features/tools/VisibilityControls'

interface Props {
  tool: AgentToolEntry
  // The tier this agent pins the tool to. Always differs from tool.defaultVisibility
  // (an override equal to the default is deleted rather than stored).
  tier: AgentToolTier
  busy: boolean
  onSelect: (tier: AgentToolTier) => void
  onReset: () => void
}

// AgentToolOverrideRow renders ONE per-agent override as a before/after pair:
// the workspace-effective default on the left, the agent's pinned tier on the
// right, plus the selector to change it and a reset back to the default. Only
// overridden tools get a row — that is the whole point of the screen, the user
// sees the diff instead of scrolling 140 unchanged tools.
export function AgentToolOverrideRow({ tool, tier, busy, onSelect, onReset }: Props) {
  return (
    <li
      data-testid="agent-tool-override"
      data-tool-name={tool.name}
      data-tier={tier}
      className="flex flex-wrap items-center gap-2 rounded border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2.5 py-1.5"
    >
      <code className="text-xs" title={tool.description}>
        {tool.name}
      </code>
      <span className="flex items-center gap-1 text-[10px] text-[var(--color-text-dim)]">
        <AgentTierBadge tier={tool.defaultVisibility} className="opacity-60" />
        <ArrowRight size={11} />
        <AgentTierBadge tier={tier} />
      </span>
      <span className="ml-auto flex items-center gap-1.5">
        <AgentTierSelector value={tier} busy={busy} onSelect={onSelect} compact />
        <button
          data-testid="agent-tool-override-reset"
          onClick={onReset}
          disabled={busy}
          title={`Varsayılana dön (${tool.defaultVisibility})`}
          className="rounded p-1 text-[var(--color-text-dim)] transition hover:bg-[var(--color-bg)] hover:text-[var(--color-text)] disabled:opacity-40"
        >
          <RotateCcw size={13} />
        </button>
      </span>
    </li>
  )
}
