import type { AgentToolTier, ToolVisibility } from '@/types'
import { AGENT_TIERS, VISIBILITY_TIERS, agentTierMeta, visibilityMeta } from './toolMeta'

// VisibilityBadge shows a tool's current context-visibility tier (Tam / Özet /
// İsim / Gizli) with a tier-accent color. Replaces the old NameOnly/Self-mgmt
// chips — every tool now carries exactly one of the four.
export function VisibilityBadge({
  visibility,
  className = '',
}: {
  visibility: ToolVisibility
  className?: string
}) {
  const m = visibilityMeta(visibility)
  return (
    <span
      className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${className}`}
      style={{
        backgroundColor: `color-mix(in srgb, ${m.color} 18%, transparent)`,
        color: m.labelColor ?? m.color,
      }}
      title={m.hint}
    >
      {m.label}
    </span>
  )
}

// AgentTierBadge is VisibilityBadge on the per-agent scale — the same four tiers
// plus 'Yasaklı'. Used to render an override's before/after pair.
export function AgentTierBadge({
  tier,
  className = '',
}: {
  tier: AgentToolTier
  className?: string
}) {
  const m = agentTierMeta(tier)
  return (
    <span
      className={`rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide ${className}`}
      style={{
        backgroundColor: `color-mix(in srgb, ${m.color} 18%, transparent)`,
        color: m.labelColor ?? m.color,
      }}
      title={m.hint}
    >
      {m.label}
    </span>
  )
}

// AgentTierSelector is the 5-way segmented control for a per-agent override:
// the four context tiers plus 'Yasaklı' (tool removed from the agent entirely).
export function AgentTierSelector({
  value,
  busy,
  onSelect,
  compact = false,
}: {
  value: AgentToolTier
  busy: boolean
  onSelect: (tier: AgentToolTier) => void
  // compact renders the tighter variant used inside list rows.
  compact?: boolean
}) {
  return (
    <div className="inline-flex overflow-hidden rounded-lg border border-[var(--color-border)]">
      {AGENT_TIERS.map((tier) => {
        const active = value === tier.value
        return (
          <button
            key={tier.value}
            data-testid="agent-tool-tier"
            data-tier={tier.value}
            onClick={() => onSelect(tier.value)}
            disabled={busy}
            title={tier.hint}
            className={`${compact ? 'px-2 py-1 text-[11px]' : 'px-3 py-2 text-xs'} font-medium transition disabled:opacity-50 ${
              active
                ? tier.value === 'full'
                  ? 'text-[var(--color-on-success)]'
                  : tier.value === 'name-only'
                    ? 'text-[var(--color-on-warning)]'
                    : 'text-[var(--color-text)]'
                : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
            }`}
            style={active ? { backgroundColor: tier.color } : undefined}
          >
            {tier.label}
          </button>
        )
      })}
    </div>
  )
}

// VisibilitySelector is the 4-way segmented control to pick a tool's context tier.
export function VisibilitySelector({
  value,
  busy,
  onSelect,
}: {
  value: ToolVisibility
  busy: boolean
  onSelect: (tier: ToolVisibility) => void
}) {
  return (
    <div className="inline-flex overflow-hidden rounded-lg border border-[var(--color-border)]">
      {VISIBILITY_TIERS.map((tier) => {
        const active = value === tier.value
        return (
          <button
            key={tier.value}
            data-testid="tool-visibility-tier"
            data-tier={tier.value}
            onClick={() => onSelect(tier.value)}
            disabled={busy}
            title={tier.hint}
            className={`px-3 py-2 text-xs font-medium transition disabled:opacity-50 ${
              active
                ? tier.value === 'full'
                  ? 'text-[var(--color-on-success)]'
                  : tier.value === 'name-only'
                    ? 'text-[var(--color-on-warning)]'
                    : 'text-[var(--color-text)]'
                : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
            }`}
            style={active ? { backgroundColor: tier.color } : undefined}
          >
            {tier.label}
          </button>
        )
      })}
    </div>
  )
}
