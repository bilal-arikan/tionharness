import { ChevronRight, Lock } from 'lucide-react'
import type { Agent } from '@/types'
import { resolveColor } from '@/shared/lib/avatar'

interface Props {
  /** Ancestors ROOT FIRST (see lineageOf). */
  lineage: Agent[]
  /** The agent whose chain this is; rendered last, non-clickable. */
  self: Pick<Agent, 'id' | 'name' | 'color' | 'avatar'>
  /** Jump to an ancestor's settings. */
  onSelectAgent?: (id: string) => void
}

// AgentLineageChips renders the inheritance chain as a breadcrumb in the
// settings header: "Titler › Türkçe Titler › (this agent)". Each ancestor chip
// carries its colour dot (the same colour the roster stripes use) and a lock
// glyph when it is a built-in, so the reader sees where the chain is anchored.
export function AgentLineageChips({ lineage, self, onSelectAgent }: Props) {
  if (lineage.length === 0) return null
  return (
    <div
      data-testid="agent-lineage-chips"
      className="flex min-w-0 flex-wrap items-center gap-1 text-[11px] text-[var(--color-text-dim)]"
    >
      <span className="mr-0.5 shrink-0">Kalıtım:</span>
      {lineage.map((ancestor) => (
        <span key={ancestor.id} className="flex items-center gap-1">
          <button
            type="button"
            data-testid="agent-lineage-chip"
            data-agent-id={ancestor.id}
            onClick={() => onSelectAgent?.(ancestor.id)}
            title={`${ancestor.name} ayarlarına git`}
            className="flex max-w-[12rem] items-center gap-1 rounded-full border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2 py-0.5 hover:border-[var(--color-accent)] hover:text-[var(--color-text)]"
          >
            <span
              className="h-2 w-2 shrink-0 rounded-full"
              style={{ background: resolveColor(ancestor) }}
            />
            <span className="truncate">{ancestor.name}</span>
            {ancestor.locked && <Lock size={10} className="shrink-0 opacity-70" />}
          </button>
          <ChevronRight size={12} className="shrink-0 opacity-60" />
        </span>
      ))}
      <span className="flex items-center gap-1 rounded-full border border-dashed border-[var(--color-border)] px-2 py-0.5 text-[var(--color-text)]">
        <span
          className="h-2 w-2 shrink-0 rounded-full"
          style={{ background: resolveColor(self) }}
        />
        <span className="truncate">{self.name}</span>
      </span>
    </div>
  )
}
