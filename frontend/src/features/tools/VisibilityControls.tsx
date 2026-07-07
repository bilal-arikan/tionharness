import type { ToolVisibility } from '@/types'
import { VISIBILITY_TIERS, visibilityMeta } from './toolMeta'

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
        color: m.color,
      }}
      title={m.hint}
    >
      {m.label}
    </span>
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
                ? 'text-white'
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
