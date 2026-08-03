import type { ReactNode } from 'react'
import type { Agent } from '@/types'
import { AgentAvatar } from './AgentAvatar'
import { useCatalog, resolveModelLabel } from '@/shared/lib/catalog'

// AgentLike is the minimal shape needed to render an agent's identity. id + name
// are required; the rest are optional so callers can pass a full Agent or a thin
// summary (e.g. a session participant with only id/name/avatar/color).
export type AgentLike = Pick<Agent, 'id' | 'name'> &
  Partial<Pick<Agent, 'avatar' | 'color' | 'provider' | 'model'>>

type Size = 'sm' | 'md' | 'lg'

const SIZES: Record<Size, { avatar: number; name: string; sub: string; gap: string }> = {
  sm: { avatar: 20, name: 'text-xs', sub: 'text-[10px]', gap: 'gap-2' },
  md: { avatar: 28, name: 'text-sm', sub: 'text-[11px]', gap: 'gap-2.5' },
  lg: { avatar: 40, name: 'text-sm font-semibold', sub: 'text-xs', gap: 'gap-3' },
}

interface Props {
  agent: AgentLike
  /** Avatar + text scale. Default 'md'. */
  size?: Size
  /**
   * Secondary line under the name:
   *  - 'model' → resolved model label from the catalog (needs agent.provider)
   *  - 'none'  → no secondary line (default)
   *  - ReactNode → custom meta (e.g. "3 tur · ~12K token")
   */
  subtitle?: 'model' | 'none' | ReactNode
  /** Draw the active ring around the avatar. */
  active?: boolean
  /** Render the name muted + italic (e.g. a disabled agent). */
  dim?: boolean
  /**
   * Show the agent id dimly on the SECOND line, left of the model (e.g. "Ada" /
   * "AGT3 · Sonnet"). Used where an agent is picked from a list or addressed by
   * id, so the id is discoverable without opening the agent — the same id the
   * send_message tool accepts. Keeping it off the name line lets the name use
   * the full width and gives every id the same place across the app.
   */
  showId?: boolean
  /** Inline content appended right after the name (e.g. an owner star). */
  nameSuffix?: ReactNode
  /** Content pinned to the right edge of the row (badges, actions). */
  trailing?: ReactNode
  /**
   * Collapse to avatar-only on narrow (phone) widths — the name + secondary line
   * are hidden below the `md` breakpoint, leaving just the avatar. Used by the
   * composer's agent trigger so it stays compact on phones.
   */
  mobileIconOnly?: boolean
  className?: string
}

// AgentIdentity is the single, generic way to show an agent across the app: its
// avatar plus name and an optional secondary line. Used by the composer picker,
// chat bubble header, session participants, board cards, agent pickers, … so
// every agent reads the same and design tweaks live in one place.
export function AgentIdentity({
  agent,
  size = 'md',
  subtitle = 'none',
  active,
  dim,
  showId,
  nameSuffix,
  trailing,
  mobileIconOnly,
  className,
}: Props) {
  const catalog = useCatalog()
  const s = SIZES[size]

  let sub: ReactNode = null
  if (subtitle === 'model') {
    // resolveModelLabel already renders the observed version ("Opus 5") rather
    // than the configured alias ("opus"), so this line names the model that
    // actually answers. The Claude Code version/plan is a property of the
    // install, not of the turn — it stays in Providers, off this line.
    if (agent.provider) sub = resolveModelLabel(catalog, agent.provider, agent.model ?? '')
  } else if (subtitle !== 'none') {
    sub = subtitle
  }

  return (
    <span className={`flex min-w-0 items-center text-left ${s.gap} ${className ?? ''}`}>
      <AgentAvatar agent={agent} size={s.avatar} active={active} />
      <span
        className={`min-w-0 flex-1 flex-col leading-tight ${mobileIconOnly ? 'hidden md:flex' : 'flex'}`}
      >
        <span className="flex min-w-0 items-baseline gap-1.5">
          <span
            className={`truncate ${s.name} ${
              dim ? 'italic text-[var(--color-text-dim)]' : 'text-[var(--color-text)]'
            }`}
          >
            {agent.name}
            {nameSuffix}
          </span>
        </span>
        {/* Second line: id first (fixed left anchor, never truncated), then the
            secondary text (model) which absorbs the remaining width. */}
        {(showId || (sub != null && sub !== '')) && (
          <span
            className={`flex min-w-0 items-baseline gap-1.5 ${s.sub} text-[var(--color-text-dim)]`}
          >
            {showId && <span className="shrink-0 font-mono opacity-60">{agent.id}</span>}
            {sub != null && sub !== '' && <span className="truncate">{sub}</span>}
          </span>
        )}
      </span>
      {trailing}
    </span>
  )
}
