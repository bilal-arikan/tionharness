import { X } from 'lucide-react'
import type { AgentToolGroup, AgentToolTier } from '@/types'
import { AgentTierSelector } from '@/features/tools/VisibilityControls'

// AgentToolGroupRow is one bulk-override target: a built-in category
// ('group:files') or an MCP server ('linear__*'). Picking a tier writes the
// GROUP KEY into the agent's override map — one entry covering every tool in
// the group. An exact per-tool override still beats it, both in the UI list and
// in the backend's resolution order.
export function AgentToolGroupRow({
  group,
  tier,
  busy,
  onSelect,
  onClear,
}: {
  group: AgentToolGroup
  // The tier currently stored for this group key, or undefined when the group
  // carries no override.
  tier?: AgentToolTier
  busy: boolean
  onSelect: (tier: AgentToolTier) => void
  onClear: () => void
}) {
  return (
    <li
      data-testid="agent-tool-group-row"
      data-group-key={group.key}
      data-group-tier={tier ?? ''}
      className={`flex flex-wrap items-center gap-x-2 gap-y-1.5 rounded-lg border px-2.5 py-2 ${
        tier
          ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)]'
          : 'border-[var(--color-border)] bg-[var(--color-surface)]'
      }`}
      title={group.tools.join(', ')}
    >
      <span className="text-xs font-medium">{group.label}</span>
      <span className="text-[11px] text-[var(--color-text-dim)]">
        {group.kind === 'mcp' ? 'MCP' : 'yerleşik'} · {group.count} araç
      </span>
      <span className="ml-auto flex items-center gap-1.5">
        <AgentTierSelector
          // No override yet → no segment is active; the empty string matches no
          // tier, which is exactly the "inherit" state.
          value={(tier ?? '') as AgentToolTier}
          busy={busy}
          onSelect={onSelect}
          compact
        />
        <button
          onClick={onClear}
          disabled={busy || !tier}
          title="Grup override'ını kaldır"
          className="rounded-md p-1 text-[var(--color-text-dim)] transition hover:bg-[color-mix(in_srgb,var(--color-danger)_12%,transparent)] hover:text-[var(--color-danger)] disabled:cursor-not-allowed disabled:opacity-30"
        >
          <X size={13} />
        </button>
      </span>
    </li>
  )
}
