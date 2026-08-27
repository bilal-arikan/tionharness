import { ChevronRight } from 'lucide-react'
import type { SessionInfo } from '@/types'
import { AgentIdentity } from '@/shared/components/agents/AgentIdentity'
import { Section } from './SessionDetailBits'
import { formatTokens } from './sessionDetailFormat'

interface Props {
  info: SessionInfo
  onSelectAgent?: (id: string) => void
}

// Agents participating in the conversation, with optional navigation.
export function SessionAgentsSection({ info, onSelectAgent }: Props) {
  return (
    <Section title={`Konuşmadaki ajanlar (${info.agents.length})`}>
      <div className="flex flex-col gap-2">
        {info.agents.map((a) => {
          const row = (
            <AgentIdentity
              agent={{ id: a.agentId, name: a.name, avatar: a.avatar, color: a.color }}
              size="sm"
              dim={a.disabled}
              nameSuffix={
                a.isOwner ? (
                  <span className="ml-1 text-[var(--color-accent)]" title="Varsayılan ajan">
                    ★
                  </span>
                ) : undefined
              }
              subtitle={`${a.turns} tur · ~${formatTokens(a.tokens)} token`}
            />
          )
          // Without a handler, render the bare row (read-only). With one,
          // wrap in a button that navigates to the Agents view focused on
          // that agent — matching the behaviour of clicking an agent in the
          // Agents roster. Disabled agents stay non-interactive.
          if (!onSelectAgent || a.disabled) return <div key={a.agentId}>{row}</div>
          return (
            <button
              key={a.agentId}
              type="button"
              onClick={() => onSelectAgent(a.agentId)}
              title={`${a.name} sayfasına git`}
              className="group flex w-full items-center justify-between rounded-md border border-transparent px-2 py-1.5 text-left transition hover:border-[var(--color-border)] hover:bg-[var(--color-surface-2)] focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--color-accent)]"
            >
              <span className="min-w-0 flex-1">{row}</span>
              <ChevronRight
                className="h-3.5 w-3.5 shrink-0 text-[var(--color-text-dim)] opacity-0 transition group-hover:opacity-100"
                aria-hidden
              />
            </button>
          )
        })}
      </div>
    </Section>
  )
}
