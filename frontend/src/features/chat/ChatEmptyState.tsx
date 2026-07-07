// ChatEmptyState is the chat column's "no active session" screen. Instead of a
// bare, disabled composer with no agent selected, it presents a proper
// "Yeni sohbete başla" (start a new chat) call-to-action:
//   - With agents: pick which agent starts the chat, then a "Yeni sohbet" button
//     that opens a fresh session with it.
//   - Without agents: guidance + a shortcut to the Agents screen to create one.
import { MessageSquarePlus, Bot } from 'lucide-react'
import type { Agent } from '@/types'
import { Button } from '@/shared/components'

interface Props {
  agents: Agent[]
  defaultAgentId: string | null
  // Start a fresh chat with the current default agent (ctl.newSession).
  onNewSession: () => void
  // Set which agent newSession will use (ctl.pickAgent).
  onSelectDefaultAgent: (id: string) => void
  // Navigate to the Agents screen (to create the first agent).
  onGoToAgents: () => void
}

// A small round agent avatar: the configured emoji/initial over its accent color.
function AgentDot({ agent }: { agent: Agent }) {
  return (
    <span
      className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[11px]"
      style={agent.color ? { backgroundColor: agent.color + '33' } : undefined}
    >
      {agent.avatar || agent.name.slice(0, 1).toUpperCase()}
    </span>
  )
}

export function ChatEmptyState({
  agents,
  defaultAgentId,
  onNewSession,
  onSelectDefaultAgent,
  onGoToAgents,
}: Props) {
  const hasAgents = agents.length > 0
  // The agent a new chat would open with (explicit default, else the first one).
  const starting = agents.find((a) => a.id === defaultAgentId) ?? agents[0]

  return (
    <div className="flex min-h-0 flex-1 items-center justify-center p-6">
      <div
        data-testid="chat-empty-state"
        className="w-full max-w-md rounded-2xl border border-[var(--color-border)] bg-[var(--color-surface)] p-7 text-center shadow-[var(--shadow-md)]"
      >
        <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
          {hasAgents ? <MessageSquarePlus size={22} /> : <Bot size={22} />}
        </div>

        {hasAgents ? (
          <>
            <h2 className="mb-1 text-lg font-semibold">Yeni sohbete başla</h2>
            <p className="mb-5 text-sm text-[var(--color-text-dim)]">
              Bir ajan seç ve sohbete başla. Mesajlar seçtiğin ajanla yürütülür.
            </p>

            {/* Agent picker — only meaningful with more than one agent. Single
                agent: just show which one will be used. */}
            {agents.length > 1 ? (
              <label className="mb-4 block text-left">
                <span className="mb-1 block text-xs text-[var(--color-text-dim)]">Ajan</span>
                <select
                  value={starting?.id ?? ''}
                  onChange={(e) => onSelectDefaultAgent(e.target.value)}
                  data-testid="chat-empty-agent-select"
                  className="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm outline-none focus:border-[var(--color-accent)]"
                >
                  {agents.map((a) => (
                    <option key={a.id} value={a.id}>
                      {(a.avatar ? a.avatar + ' ' : '') + a.name}
                    </option>
                  ))}
                </select>
              </label>
            ) : (
              starting && (
                <div className="mb-4 inline-flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-sm">
                  <AgentDot agent={starting} />
                  <span className="font-medium">{starting.name}</span>
                </div>
              )
            )}

            <Button onClick={onNewSession} size="lg" className="w-full justify-center">
              <MessageSquarePlus size={16} /> Yeni sohbet
            </Button>
          </>
        ) : (
          <>
            <h2 className="mb-1 text-lg font-semibold">Önce bir ajan oluştur</h2>
            <p className="mb-5 text-sm text-[var(--color-text-dim)]">
              Sohbet başlatmak için en az bir ajana ihtiyacın var. Ajanlar ekranından hızlıca
              bir tane oluşturabilirsin.
            </p>
            <Button onClick={onGoToAgents} size="lg" className="w-full justify-center">
              <Bot size={16} /> Ajan oluştur
            </Button>
          </>
        )}
      </div>
    </div>
  )
}
