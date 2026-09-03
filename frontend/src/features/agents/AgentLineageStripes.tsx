import type { Agent } from '@/types'
import { resolveColor } from '@/shared/lib/avatar'

interface Props {
  /** Ancestors ROOT FIRST (see lineageOf). Nothing renders for an empty list. */
  lineage: Agent[]
  /** Stripe height; defaults to stretching the row. */
  className?: string
}

// AgentLineageStripes is the roster's inheritance marker: one thin vertical
// bar per ancestor, in that ancestor's colour, ordered root → direct parent
// from the outside in. A child of a child of Titler therefore shows two bars
// (violet, then the middle agent's colour), so depth is visible at a glance
// without reading a single label. Each bar names its ancestor on hover.
export function AgentLineageStripes({ lineage, className }: Props) {
  if (lineage.length === 0) return null
  return (
    <span
      data-testid="agent-lineage-stripes"
      aria-hidden
      className={`flex shrink-0 self-stretch items-stretch gap-[3px] ${className ?? ''}`}
    >
      {lineage.map((ancestor) => (
        <span
          key={ancestor.id}
          data-testid="agent-lineage-stripe"
          data-agent-id={ancestor.id}
          title={`← ${ancestor.name}`}
          className="w-[3px] rounded-full"
          style={{ background: resolveColor(ancestor) }}
        />
      ))}
    </span>
  )
}
